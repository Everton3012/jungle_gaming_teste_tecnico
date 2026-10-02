package httptransport

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	applicationquery "jungle_gaming_teste_tecnico/internal/application/query"
	applicationwalletopening "jungle_gaming_teste_tecnico/internal/application/walletopening"
	appconfig "jungle_gaming_teste_tecnico/internal/config"
	domainmoney "jungle_gaming_teste_tecnico/internal/domain/money"
	walletpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/wallet"
	"jungle_gaming_teste_tecnico/internal/observability"
)

type WalletHandler struct {
	service          *applicationwalletopening.Service
	query            *applicationquery.Service
	operationTimeout time.Duration
	metrics          *observability.Metrics
}

func NewWalletHandler(
	service *applicationwalletopening.Service,
	query *applicationquery.Service,
	cfg appconfig.Config,
	metrics *observability.Metrics,
) (*WalletHandler, error) {
	if service == nil {
		return nil, errors.New("wallet opening service is required")
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
	return &WalletHandler{service: service, query: query, operationTimeout: cfg.Database.OperationTimeout, metrics: metrics}, nil
}

func (h *WalletHandler) Create(writer http.ResponseWriter, request *http.Request) {
	body, err := readRequestBody(writer, request)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "INVALID_REQUEST_BODY", err.Error())
		return
	}
	var input createWalletRequest
	if err := decodeStrictJSON(body, &input); err != nil {
		writeError(writer, http.StatusBadRequest, "INVALID_JSON", err.Error())
		return
	}
	input.PlayerID = strings.TrimSpace(input.PlayerID)
	if input.PlayerID == "" {
		writeError(writer, http.StatusBadRequest, "PLAYER_ID_REQUIRED", "playerId is required")
		return
	}
	initialBalance, err := domainmoney.Parse(input.InitialBalance.Amount, input.InitialBalance.Currency)
	if err != nil {
		writeError(writer, http.StatusBadRequest, "INVALID_INITIAL_BALANCE", err.Error())
		return
	}

	walletID, err := newID()
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "ID_GENERATION_FAILED", "could not create wallet identifier")
		return
	}
	transactionID, err := newID()
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "ID_GENERATION_FAILED", "could not create opening transaction identifier")
		return
	}
	ledgerID, err := newID()
	if err != nil {
		writeError(writer, http.StatusServiceUnavailable, "ID_GENERATION_FAILED", "could not create ledger identifier")
		return
	}

	now := time.Now().UTC()
	ctx, cancel := context.WithTimeout(request.Context(), h.operationTimeout)
	defer cancel()
	result, err := h.service.Create(ctx, applicationwalletopening.CreateInput{
		WalletID: walletID, PlayerID: input.PlayerID, TransactionID: transactionID,
		LedgerID: ledgerID, InitialBalance: initialBalance, CreatedAt: now,
	})
	if err != nil {
		h.writeCreateError(writer, err)
		return
	}
	writeJSON(writer, http.StatusCreated, presentWallet(&result.Wallet))
}

func (h *WalletHandler) Get(writer http.ResponseWriter, request *http.Request) {
	walletID := strings.TrimSpace(request.PathValue("walletId"))
	if walletID == "" {
		writeError(writer, http.StatusBadRequest, "WALLET_ID_REQUIRED", "walletId is required")
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), h.operationTimeout)
	defer cancel()
	wallet, err := h.query.GetWallet(ctx, walletID)
	if err != nil {
		if errors.Is(err, walletpostgres.ErrNotFound) {
			writeError(writer, http.StatusNotFound, "WALLET_NOT_FOUND", "wallet not found")
			return
		}
		writeError(writer, http.StatusServiceUnavailable, "QUERY_UNAVAILABLE", "wallet could not be loaded")
		return
	}
	writeJSON(writer, http.StatusOK, presentWallet(wallet))
}

func (h *WalletHandler) Ledger(writer http.ResponseWriter, request *http.Request) {
	walletID := strings.TrimSpace(request.PathValue("walletId"))
	if walletID == "" {
		writeError(writer, http.StatusBadRequest, "WALLET_ID_REQUIRED", "walletId is required")
		return
	}
	limit := 50
	if raw := strings.TrimSpace(request.URL.Query().Get("limit")); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil {
			writeError(writer, http.StatusBadRequest, "INVALID_LIMIT", "limit must be an integer")
			return
		}
		limit = value
	}
	ctx, cancel := context.WithTimeout(request.Context(), h.operationTimeout)
	defer cancel()
	if _, err := h.query.GetWallet(ctx, walletID); err != nil {
		if errors.Is(err, walletpostgres.ErrNotFound) {
			writeError(writer, http.StatusNotFound, "WALLET_NOT_FOUND", "wallet not found")
			return
		}
		writeError(writer, http.StatusServiceUnavailable, "QUERY_UNAVAILABLE", "wallet could not be loaded")
		return
	}
	page, err := h.query.ListLedger(ctx, walletID, request.URL.Query().Get("cursor"), limit)
	if err != nil {
		if errors.Is(err, applicationquery.ErrInvalidLimit) || errors.Is(err, applicationquery.ErrInvalidCursor) {
			writeError(writer, http.StatusBadRequest, "INVALID_PAGINATION", err.Error())
			return
		}
		writeError(writer, http.StatusServiceUnavailable, "QUERY_UNAVAILABLE", "ledger could not be loaded")
		return
	}
	items := make([]ledgerEntryResponse, 0, len(page.Entries))
	for _, entry := range page.Entries {
		items = append(items, presentLedgerEntry(entry))
	}
	writeJSON(writer, http.StatusOK, ledgerPageResponse{Items: items, NextCursor: page.NextCursor})
}

func (h *WalletHandler) Reconcile(writer http.ResponseWriter, request *http.Request) {
	walletID := strings.TrimSpace(request.PathValue("walletId"))
	if walletID == "" {
		writeError(writer, http.StatusBadRequest, "WALLET_ID_REQUIRED", "walletId is required")
		return
	}
	ctx, cancel := context.WithTimeout(request.Context(), h.operationTimeout)
	defer cancel()
	result, err := h.query.Reconcile(ctx, walletID)
	if err != nil {
		if errors.Is(err, walletpostgres.ErrNotFound) {
			writeError(writer, http.StatusNotFound, "WALLET_NOT_FOUND", "wallet not found")
			return
		}
		writeError(writer, http.StatusServiceUnavailable, "RECONCILIATION_UNAVAILABLE", "wallet reconciliation failed")
		return
	}
	if !result.Consistent {
		h.metrics.ReconciliationDivergences.Add(1)
		slog.Warn("wallet_reconciliation_divergence",
			"walletId", result.WalletID,
			"checkedEntries", result.CheckedEntries,
		)
	}
	writeJSON(writer, http.StatusOK, reconciliationResponse{
		WalletID:          result.WalletID,
		StoredBalance:     presentMoney(result.StoredBalance),
		CalculatedBalance: presentMoney(result.CalculatedBalance),
		Difference:        presentMoney(result.Difference),
		Consistent:        result.Consistent,
		CheckedEntries:    result.CheckedEntries,
	})
}

func (h *WalletHandler) writeCreateError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, applicationwalletopening.ErrWalletAlreadyExists):
		writeError(writer, http.StatusConflict, "WALLET_ALREADY_EXISTS", "a wallet already exists for this player and currency")
	case errors.Is(err, context.DeadlineExceeded):
		writeError(writer, http.StatusServiceUnavailable, "PROCESSING_TIMEOUT", "wallet creation timed out")
	case errors.Is(err, context.Canceled):
		writeError(writer, http.StatusServiceUnavailable, "PROCESSING_CANCELED", "wallet creation was canceled")
	default:
		writeError(writer, http.StatusServiceUnavailable, "WALLET_CREATION_UNAVAILABLE", "wallet could not be created")
	}
}
