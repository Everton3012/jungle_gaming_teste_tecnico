package httptransport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	applicationidempotency "jungle_gaming_teste_tecnico/internal/application/idempotency"
	applicationquery "jungle_gaming_teste_tecnico/internal/application/query"
	applicationwager "jungle_gaming_teste_tecnico/internal/application/wager"
	appconfig "jungle_gaming_teste_tecnico/internal/config"
	domainmoney "jungle_gaming_teste_tecnico/internal/domain/money"
	domainwager "jungle_gaming_teste_tecnico/internal/domain/wager"
	domaintransaction "jungle_gaming_teste_tecnico/internal/domain/wagertransaction"
	authinfra "jungle_gaming_teste_tecnico/internal/infrastructure/auth"
	transactionpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/wagertransaction"
	walletpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/wallet"
	"jungle_gaming_teste_tecnico/internal/observability"
)

const maxRequestBodySize = 1 << 20

type WagerHandler struct {
	service          *applicationwager.Service
	query            *applicationquery.Service
	operationTimeout time.Duration
	metrics          *observability.Metrics
}

func NewWagerHandler(service *applicationwager.Service, query *applicationquery.Service, cfg appconfig.Config, metrics *observability.Metrics) (*WagerHandler, error) {
	if service == nil {
		return nil, errors.New("wager service is required")
	}
	if query == nil {
		return nil, errors.New("query service is required")
	}
	if cfg.Database.OperationTimeout <= 0 {
		return nil, errors.New("operation timeout must be greater than zero")
	}
	if metrics == nil {
		metrics = observability.Default
	}
	return &WagerHandler{service: service, query: query, operationTimeout: cfg.Database.OperationTimeout, metrics: metrics}, nil
}

func (h *WagerHandler) ProcessTransaction(writer http.ResponseWriter, request *http.Request) {
	started := time.Now()
	defer h.metrics.ObserveProcessing(started)
	principal, ok := authinfra.PrincipalFromContext(request.Context())
	if !ok || principal.ProviderID == "" {
		writeError(writer, http.StatusForbidden, "FORBIDDEN", "provider credentials are required")
		return
	}

	idempotencyKey := strings.TrimSpace(request.Header.Get("Idempotency-Key"))
	if idempotencyKey == "" {
		writeError(writer, http.StatusBadRequest, "IDEMPOTENCY_KEY_REQUIRED", "Idempotency-Key header is required")
		return
	}
	body, err := readRequestBody(writer, request)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "INVALID_REQUEST_BODY", err.Error())
		return
	}
	var input wagerTransactionRequest
	if err := decodeStrictJSON(body, &input); err != nil {
		writeError(writer, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}

	input.ProviderID = strings.TrimSpace(input.ProviderID)
	if input.ProviderID != "" && input.ProviderID != principal.ProviderID {
		writeError(writer, http.StatusForbidden, "PROVIDER_MISMATCH", "providerId does not match authenticated provider")
		return
	}
	input.ProviderID = principal.ProviderID
	operationMoney, err := domainmoney.Parse(input.Money.Amount, input.Money.Currency)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "INVALID_MONEY", err.Error())
		return
	}
	kind := domaintransaction.Kind(strings.ToUpper(strings.TrimSpace(input.Kind)))
	if !kind.IsExternal() {
		writeError(writer, http.StatusBadRequest, "INVALID_TRANSACTION_KIND", "kind must be BET, WIN, LOSS, REFUND or ROLLBACK")
		return
	}

	payloadHash, err := applicationidempotency.HashBusiness(applicationidempotency.BusinessOperation{
		ProviderID:                     input.ProviderID,
		ExternalTransactionID:          input.ExternalTransactionID,
		PlayerID:                       input.PlayerID,
		WalletID:                       input.WalletID,
		RoundID:                        input.RoundID,
		GameID:                         input.GameID,
		Kind:                           string(kind),
		Money:                          operationMoney,
		ReferenceExternalTransactionID: input.ReferenceExternalTransactionID,
	})
	if err != nil {
		writeError(writer, http.StatusBadRequest, "INVALID_PAYLOAD", "request payload cannot be canonicalized")
		return
	}

	transactionID, err := newID()
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "ID_GENERATION_FAILED", "could not create transaction identifier")
		return
	}
	ledgerEntryID, err := newID()
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "ID_GENERATION_FAILED", "could not create ledger identifier")
		return
	}
	now := time.Now().UTC()
	transaction, err := domaintransaction.NewExternal(domaintransaction.ExternalInput{
		ID:                             transactionID,
		ExternalTransactionID:          strings.TrimSpace(input.ExternalTransactionID),
		ProviderID:                     input.ProviderID,
		IdempotencyKey:                 idempotencyKey,
		PayloadHash:                    payloadHash,
		WalletID:                       strings.TrimSpace(input.WalletID),
		PlayerID:                       strings.TrimSpace(input.PlayerID),
		RoundID:                        strings.TrimSpace(input.RoundID),
		GameID:                         strings.TrimSpace(input.GameID),
		Kind:                           kind,
		Money:                          operationMoney,
		ReferenceExternalTransactionID: strings.TrimSpace(input.ReferenceExternalTransactionID),
		CreatedAt:                      now,
	})
	if err != nil {
		writeError(writer, http.StatusBadRequest, "INVALID_TRANSACTION", err.Error())
		return
	}

	correlationID := strings.TrimSpace(request.Header.Get("X-Correlation-ID"))
	if correlationID == "" {
		correlationID = transactionID
	}
	ctx, cancel := context.WithTimeout(request.Context(), h.operationTimeout)
	defer cancel()
	result, err := h.service.Process(ctx, applicationwager.ProcessInput{
		Transaction:   &transaction,
		LedgerEntryID: ledgerEntryID,
		CorrelationID: correlationID,
		OccurredAt:    now,
	})
	if err != nil {
		h.writeProcessError(writer, err)
		return
	}
	h.observeResult(result)
	if result.Transaction != nil {
		slog.Info("http_wager_processed",
			"correlationId", correlationID,
			"transactionId", result.Transaction.ID(),
			"walletId", result.Transaction.WalletID(),
			"providerId", result.Transaction.ProviderID(),
			"status", result.Transaction.Status(),
			"idempotentReplay", result.Replayed,
		)
	}
	h.writeProcessResult(writer, result)
}

func (h *WagerHandler) GetTransaction(writer http.ResponseWriter, request *http.Request) {
	principal, ok := authinfra.PrincipalFromContext(request.Context())
	if !ok || principal.ProviderID == "" {
		writeError(writer, http.StatusForbidden, "FORBIDDEN", "provider credentials are required")
		return
	}
	transactionID := strings.TrimSpace(request.PathValue("transactionId"))
	if transactionID == "" {
		writeError(writer, http.StatusBadRequest, "TRANSACTION_ID_REQUIRED", "transactionId is required")
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), h.operationTimeout)
	defer cancel()
	transaction, err := h.query.GetTransaction(ctx, transactionID)
	if err != nil {
		if errors.Is(err, transactionpostgres.ErrNotFound) {
			writeError(writer, http.StatusNotFound, "TRANSACTION_NOT_FOUND", "transaction not found")
			return
		}
		writeError(writer, http.StatusServiceUnavailable, "QUERY_UNAVAILABLE", "transaction could not be loaded")
		return
	}
	if transaction.ProviderID() != principal.ProviderID {
		writeError(writer, http.StatusNotFound, "TRANSACTION_NOT_FOUND", "transaction not found")
		return
	}
	writeJSON(writer, http.StatusOK, presentTransaction(transaction))
}

func (h *WagerHandler) GetProviderTransaction(writer http.ResponseWriter, request *http.Request) {
	principal, ok := authinfra.PrincipalFromContext(request.Context())
	if !ok || principal.ProviderID == "" {
		writeError(writer, http.StatusForbidden, "FORBIDDEN", "provider credentials are required")
		return
	}
	providerID := strings.TrimSpace(request.PathValue("providerId"))
	if providerID != principal.ProviderID {
		writeError(writer, http.StatusForbidden, "PROVIDER_MISMATCH", "provider cannot access another provider's transactions")
		return
	}
	externalID := strings.TrimSpace(request.PathValue("externalTransactionId"))
	if externalID == "" {
		writeError(writer, http.StatusBadRequest, "EXTERNAL_TRANSACTION_ID_REQUIRED", "externalTransactionId is required")
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), h.operationTimeout)
	defer cancel()
	transaction, err := h.query.GetProviderTransaction(ctx, providerID, externalID)
	if err != nil {
		if errors.Is(err, transactionpostgres.ErrNotFound) {
			writeError(writer, http.StatusNotFound, "TRANSACTION_NOT_FOUND", "transaction not found")
			return
		}
		writeError(writer, http.StatusServiceUnavailable, "QUERY_UNAVAILABLE", "transaction could not be loaded")
		return
	}
	writeJSON(writer, http.StatusOK, presentTransaction(transaction))
}

func (h *WagerHandler) observeResult(result applicationwager.ProcessResult) {
	if result.Replayed {
		h.metrics.IdempotentReplays.Add(1)
	}
	if result.Transaction == nil {
		return
	}
	switch result.Transaction.Status() {
	case domaintransaction.StatusProcessed:
		h.metrics.WagerProcessed.Add(1)
	case domaintransaction.StatusRejected:
		h.metrics.WagerRejected.Add(1)
	case domaintransaction.StatusPendingReference:
		h.metrics.WagerPending.Add(1)
	}
}

func (h *WagerHandler) writeProcessResult(writer http.ResponseWriter, result applicationwager.ProcessResult) {
	if result.Transaction == nil {
		writeError(writer, http.StatusInternalServerError, "INVALID_PROCESS_RESULT", "processing returned no transaction")
		return
	}
	response := wagerTransactionResponse{
		TransactionID:    result.Transaction.ID(),
		Status:           string(result.Transaction.Status()),
		Balance:          moneyResponse{Amount: formatMinorUnits(result.Balance), Currency: result.Currency},
		IdempotentReplay: result.Replayed,
	}
	if failureCode := result.Transaction.FailureCode().String(); failureCode != "" {
		response.FailureCode = failureCode
	}
	switch result.Transaction.Status() {
	case domaintransaction.StatusProcessed:
		writeJSON(writer, http.StatusOK, response)
	case domaintransaction.StatusPendingReference:
		writeJSON(writer, http.StatusAccepted, response)
	case domaintransaction.StatusRejected:
		writeJSON(writer, http.StatusUnprocessableEntity, response)
	case domaintransaction.StatusFailed:
		writeJSON(writer, http.StatusServiceUnavailable, response)
	default:
		writeJSON(writer, http.StatusAccepted, response)
	}
}

func (h *WagerHandler) writeProcessError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, applicationwager.ErrIdempotencyConflict):
		writeError(writer, http.StatusConflict, "IDEMPOTENCY_CONFLICT", "idempotency key was reused with different business content")
	case errors.Is(err, applicationwager.ErrExternalIDConflict):
		writeError(writer, http.StatusConflict, "EXTERNAL_TRANSACTION_CONFLICT", "external transaction id was already used by another operation")
	case errors.Is(err, applicationwager.ErrReversalConflict):
		writeError(writer, http.StatusConflict, "REVERSAL_CONFLICT", "reference transaction was already reversed")
	case errors.Is(err, walletpostgres.ErrNotFound):
		writeError(writer, http.StatusNotFound, "WALLET_NOT_FOUND", "wallet not found")
	case errors.Is(err, domainwager.ErrCurrencyMismatch):
		writeError(writer, http.StatusUnprocessableEntity, "CURRENCY_MISMATCH", "transaction currency does not match wallet currency")
	case errors.Is(err, context.DeadlineExceeded):
		writeError(writer, http.StatusServiceUnavailable, "PROCESSING_TIMEOUT", "transaction processing timed out")
	case errors.Is(err, context.Canceled):
		writeError(writer, http.StatusServiceUnavailable, "PROCESSING_CANCELED", "transaction processing was canceled")
	case errors.Is(err, applicationwager.ErrConcurrentRetryExceeded):
		h.metrics.ConcurrencyConflicts.Add(1)
		writeError(writer, http.StatusServiceUnavailable, "CONCURRENCY_RETRY_EXHAUSTED", "transaction could not be completed due to concurrent updates")
	default:
		writeError(writer, http.StatusServiceUnavailable, "PROCESSING_UNAVAILABLE", "transaction could not be processed")
	}
}

func readRequestBody(writer http.ResponseWriter, request *http.Request) ([]byte, error) {
	request.Body = http.MaxBytesReader(writer, request.Body, maxRequestBodySize)
	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, errors.New("request body is required")
	}
	return body, nil
}

func decodeStrictJSON(body []byte, destination any) error {
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	err := decoder.Decode(&trailing)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("request body must contain exactly one JSON object")
	}
	return err
}

func formatMinorUnits(amount int64) string {
	if amount >= 0 {
		return domainMoneyString(amount)
	}
	return "-" + domainMoneyString(-amount)
}

func domainMoneyString(amount int64) string {
	return formatInteger(amount/100) + "." + formatTwoDigits(amount%100)
}

func formatInteger(value int64) string {
	if value == 0 {
		return "0"
	}
	var buffer [20]byte
	position := len(buffer)
	for value > 0 {
		position--
		buffer[position] = byte('0' + value%10)
		value /= 10
	}
	return string(buffer[position:])
}

func formatTwoDigits(value int64) string {
	return string([]byte{byte('0' + value/10), byte('0' + value%10)})
}
