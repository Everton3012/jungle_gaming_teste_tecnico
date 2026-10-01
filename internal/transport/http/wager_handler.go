package httptransport

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	applicationidempotency "jungle_gaming_teste_tecnico/internal/application/idempotency"
	applicationwager "jungle_gaming_teste_tecnico/internal/application/wager"
	appconfig "jungle_gaming_teste_tecnico/internal/config"
	domainmoney "jungle_gaming_teste_tecnico/internal/domain/money"
	domaintransaction "jungle_gaming_teste_tecnico/internal/domain/wagertransaction"
	walletpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/wallet"
)

const maxRequestBodySize = 1 << 20 // 1 MiB

type WagerHandler struct {
	service          *applicationwager.Service
	operationTimeout time.Duration
}

func NewWagerHandler(
	service *applicationwager.Service,
	cfg appconfig.Config,
) (*WagerHandler, error) {
	if service == nil {
		return nil, errors.New("wager service is required")
	}

	if cfg.Database.OperationTimeout <= 0 {
		return nil, errors.New(
			"operation timeout must be greater than zero",
		)
	}

	return &WagerHandler{
		service:          service,
		operationTimeout: cfg.Database.OperationTimeout,
	}, nil
}

func (h *WagerHandler) ProcessTransaction(
	writer http.ResponseWriter,
	request *http.Request,
) {
	if request.Method != http.MethodPost {
		writer.Header().Set("Allow", http.MethodPost)

		writeError(
			writer,
			http.StatusMethodNotAllowed,
			"METHOD_NOT_ALLOWED",
			"method not allowed",
		)

		return
	}

	idempotencyKey :=
		strings.TrimSpace(
			request.Header.Get("Idempotency-Key"),
		)

	if idempotencyKey == "" {
		writeError(
			writer,
			http.StatusBadRequest,
			"IDEMPOTENCY_KEY_REQUIRED",
			"Idempotency-Key header is required",
		)

		return
	}

	body, err := readRequestBody(writer, request)
	if err != nil {
		writeError(
			writer,
			http.StatusBadRequest,
			"INVALID_REQUEST_BODY",
			err.Error(),
		)

		return
	}

	var input wagerTransactionRequest

	if err := decodeStrictJSON(body, &input); err != nil {
		writeError(
			writer,
			http.StatusBadRequest,
			"INVALID_JSON",
			err.Error(),
		)

		return
	}

	payloadHash, err :=
		applicationidempotency.Hash(body)
	if err != nil {
		writeError(
			writer,
			http.StatusBadRequest,
			"INVALID_PAYLOAD",
			"request payload cannot be canonicalized",
		)

		return
	}

	operationMoney, err :=
		domainmoney.Parse(
			input.Money.Amount,
			input.Money.Currency,
		)
	if err != nil {
		writeError(
			writer,
			http.StatusBadRequest,
			"INVALID_MONEY",
			err.Error(),
		)

		return
	}

	kind :=
		domaintransaction.Kind(
			strings.ToUpper(
				strings.TrimSpace(input.Kind),
			),
		)

	if !kind.IsExternal() {
		writeError(
			writer,
			http.StatusBadRequest,
			"INVALID_TRANSACTION_KIND",
			"kind must be BET, WIN, LOSS, REFUND or ROLLBACK",
		)

		return
	}

	transactionID, err := newID()
	if err != nil {
		writeError(
			writer,
			http.StatusServiceUnavailable,
			"ID_GENERATION_FAILED",
			"could not create transaction identifier",
		)

		return
	}

	ledgerEntryID, err := newID()
	if err != nil {
		writeError(
			writer,
			http.StatusServiceUnavailable,
			"ID_GENERATION_FAILED",
			"could not create ledger identifier",
		)

		return
	}

	now := time.Now().UTC()

	transaction, err :=
		domaintransaction.NewExternal(
			domaintransaction.ExternalInput{
				ID: transactionID,

				ExternalTransactionID: input.ExternalTransactionID,

				ProviderID: input.ProviderID,

				IdempotencyKey: idempotencyKey,

				PayloadHash: payloadHash,

				WalletID: input.WalletID,

				PlayerID: input.PlayerID,

				RoundID: input.RoundID,

				GameID: input.GameID,

				Kind: kind,

				Money: operationMoney,

				ReferenceExternalTransactionID: input.ReferenceExternalTransactionID,

				CreatedAt: now,
			},
		)
	if err != nil {
		writeError(
			writer,
			http.StatusBadRequest,
			"INVALID_TRANSACTION",
			err.Error(),
		)

		return
	}

	ctx, cancel :=
		context.WithTimeout(
			request.Context(),
			h.operationTimeout,
		)
	defer cancel()

	result, err :=
		h.service.Process(
			ctx,
			applicationwager.ProcessInput{
				Transaction:   &transaction,
				LedgerEntryID: ledgerEntryID,
				OccurredAt:    now,
			},
		)
	if err != nil {
		h.writeProcessError(
			writer,
			err,
		)

		return
	}

	h.writeProcessResult(
		writer,
		result,
	)
}

func (h *WagerHandler) writeProcessResult(
	writer http.ResponseWriter,
	result applicationwager.ProcessResult,
) {
	if result.Transaction == nil {
		writeError(
			writer,
			http.StatusInternalServerError,
			"INVALID_PROCESS_RESULT",
			"processing returned no transaction",
		)

		return
	}

	response := wagerTransactionResponse{
		TransactionID: result.Transaction.ID(),

		Status: string(result.Transaction.Status()),

		Balance: moneyResponse{
			Amount: formatMinorUnits(result.Balance),

			Currency: result.Currency,
		},

		IdempotentReplay: result.Replayed,
	}

	if failureCode :=
		result.Transaction.FailureCode().String(); failureCode != "" {

		response.FailureCode =
			failureCode
	}

	switch result.Transaction.Status() {
	case domaintransaction.StatusProcessed:
		writeJSON(
			writer,
			http.StatusOK,
			response,
		)

	case domaintransaction.StatusPendingReference:
		writeJSON(
			writer,
			http.StatusAccepted,
			response,
		)

	case domaintransaction.StatusRejected:
		writeJSON(
			writer,
			http.StatusUnprocessableEntity,
			response,
		)

	case domaintransaction.StatusFailed:
		writeJSON(
			writer,
			http.StatusServiceUnavailable,
			response,
		)

	default:
		writeJSON(
			writer,
			http.StatusAccepted,
			response,
		)
	}
}

func (h *WagerHandler) writeProcessError(
	writer http.ResponseWriter,
	err error,
) {
	switch {
	case errors.Is(
		err,
		applicationwager.ErrExternalIDConflict,
	):
		writeError(
			writer,
			http.StatusConflict,
			"EXTERNAL_TRANSACTION_CONFLICT",
			"external transaction id was already used by another operation",
		)

	case errors.Is(
		err,
		applicationwager.ErrReversalConflict,
	):
		writeError(
			writer,
			http.StatusConflict,
			"REVERSAL_CONFLICT",
			"reference transaction was already reversed",
		)

	case errors.Is(
		err,
		walletpostgres.ErrNotFound,
	):
		writeError(
			writer,
			http.StatusConflict,
			"EXTERNAL_TRANSACTION_CONFLICT",
			"external transaction id was already used by another operation",
		)

	case errors.Is(
		err,
		walletpostgres.ErrNotFound,
	):
		writeError(
			writer,
			http.StatusNotFound,
			"WALLET_NOT_FOUND",
			"wallet not found",
		)

	case errors.Is(
		err,
		context.DeadlineExceeded,
	):
		writeError(
			writer,
			http.StatusServiceUnavailable,
			"PROCESSING_TIMEOUT",
			"transaction processing timed out",
		)

	case errors.Is(
		err,
		context.Canceled,
	):
		writeError(
			writer,
			http.StatusServiceUnavailable,
			"PROCESSING_CANCELED",
			"transaction processing was canceled",
		)

	case errors.Is(
		err,
		applicationwager.ErrConcurrentRetryExceeded,
	):
		writeError(
			writer,
			http.StatusServiceUnavailable,
			"CONCURRENCY_RETRY_EXHAUSTED",
			"transaction could not be completed due to concurrent updates",
		)

	default:
		writeError(
			writer,
			http.StatusServiceUnavailable,
			"PROCESSING_UNAVAILABLE",
			"transaction could not be processed",
		)
	}
}

func readRequestBody(
	writer http.ResponseWriter,
	request *http.Request,
) ([]byte, error) {
	request.Body =
		http.MaxBytesReader(
			writer,
			request.Body,
			maxRequestBodySize,
		)

	body, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, err
	}

	if len(bytes.TrimSpace(body)) == 0 {
		return nil, errors.New(
			"request body is required",
		)
	}

	return body, nil
}

func decodeStrictJSON(
	body []byte,
	destination any,
) error {
	decoder :=
		json.NewDecoder(
			bytes.NewReader(body),
		)

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
		return errors.New(
			"request body must contain exactly one JSON object",
		)
	}

	return err
}

func formatMinorUnits(amount int64) string {
	sign := ""

	if amount < 0 {
		sign = "-"
		amount = -amount
	}

	return sign +
		formatInteger(amount/100) +
		"." +
		formatTwoDigits(amount%100)
}

func formatInteger(value int64) string {
	if value == 0 {
		return "0"
	}

	var buffer [20]byte
	position := len(buffer)

	for value > 0 {
		position--

		buffer[position] =
			byte('0' + value%10)

		value /= 10
	}

	return string(buffer[position:])
}

func formatTwoDigits(value int64) string {
	return string(
		[]byte{
			byte('0' + value/10),
			byte('0' + value%10),
		},
	)
}
