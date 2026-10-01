package httptransport

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	applicationwalletopening "jungle_gaming_teste_tecnico/internal/application/walletopening"
	appconfig "jungle_gaming_teste_tecnico/internal/config"
	domainmoney "jungle_gaming_teste_tecnico/internal/domain/money"
)

type WalletHandler struct {
	service          *applicationwalletopening.Service
	operationTimeout time.Duration
}

func NewWalletHandler(
	service *applicationwalletopening.Service,
	cfg appconfig.Config,
) (*WalletHandler, error) {
	if service == nil {
		return nil, errors.New("wallet opening service is required")
	}

	if cfg.Database.OperationTimeout <= 0 {
		return nil, errors.New(
			"operation timeout must be greater than zero",
		)
	}

	return &WalletHandler{
		service:          service,
		operationTimeout: cfg.Database.OperationTimeout,
	}, nil
}

func (h *WalletHandler) Create(
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

	var input createWalletRequest

	if err := decodeStrictJSON(body, &input); err != nil {
		writeError(
			writer,
			http.StatusBadRequest,
			"INVALID_JSON",
			err.Error(),
		)

		return
	}

	input.PlayerID = strings.TrimSpace(input.PlayerID)

	if input.PlayerID == "" {
		writeError(
			writer,
			http.StatusBadRequest,
			"PLAYER_ID_REQUIRED",
			"playerId is required",
		)

		return
	}

	initialBalance, err :=
		domainmoney.Parse(
			input.InitialBalance.Amount,
			input.InitialBalance.Currency,
		)
	if err != nil {
		writeError(
			writer,
			http.StatusBadRequest,
			"INVALID_INITIAL_BALANCE",
			err.Error(),
		)

		return
	}

	walletID, err := newID()
	if err != nil {
		writeError(
			writer,
			http.StatusServiceUnavailable,
			"ID_GENERATION_FAILED",
			"could not create wallet identifier",
		)

		return
	}

	transactionID, err := newID()
	if err != nil {
		writeError(
			writer,
			http.StatusServiceUnavailable,
			"ID_GENERATION_FAILED",
			"could not create opening transaction identifier",
		)

		return
	}

	ledgerID, err := newID()
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

	ctx, cancel :=
		context.WithTimeout(
			request.Context(),
			h.operationTimeout,
		)
	defer cancel()

	result, err :=
		h.service.Create(
			ctx,
			applicationwalletopening.CreateInput{
				WalletID:       walletID,
				PlayerID:       input.PlayerID,
				TransactionID:  transactionID,
				LedgerID:       ledgerID,
				InitialBalance: initialBalance,
				CreatedAt:      now,
			},
		)
	if err != nil {
		h.writeCreateError(
			writer,
			err,
		)

		return
	}

	wallet := result.Wallet

	writeJSON(
		writer,
		http.StatusCreated,
		createWalletResponse{
			WalletID: wallet.ID(),
			PlayerID: wallet.PlayerID(),
			Balance: moneyResponse{
				Amount: formatMinorUnits(
					wallet.Balance().Amount(),
				),
				Currency: wallet.Currency(),
			},
		},
	)
}

func (h *WalletHandler) writeCreateError(
	writer http.ResponseWriter,
	err error,
) {
	switch {
	case errors.Is(
		err,
		applicationwalletopening.ErrWalletAlreadyExists,
	):
		writeError(
			writer,
			http.StatusConflict,
			"WALLET_ALREADY_EXISTS",
			"a wallet already exists for this player and currency",
		)

	case errors.Is(
		err,
		context.DeadlineExceeded,
	):
		writeError(
			writer,
			http.StatusServiceUnavailable,
			"PROCESSING_TIMEOUT",
			"wallet creation timed out",
		)

	case errors.Is(
		err,
		context.Canceled,
	):
		writeError(
			writer,
			http.StatusServiceUnavailable,
			"PROCESSING_CANCELED",
			"wallet creation was canceled",
		)

	default:
		writeError(
			writer,
			http.StatusServiceUnavailable,
			"WALLET_CREATION_UNAVAILABLE",
			"wallet could not be created",
		)
	}
}
