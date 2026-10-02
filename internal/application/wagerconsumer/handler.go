package wagerconsumer

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	applicationidempotency "jungle_gaming_teste_tecnico/internal/application/idempotency"
	applicationwager "jungle_gaming_teste_tecnico/internal/application/wager"
	domainmoney "jungle_gaming_teste_tecnico/internal/domain/money"
	domaintransaction "jungle_gaming_teste_tecnico/internal/domain/wagertransaction"
	"jungle_gaming_teste_tecnico/internal/observability"
)

var ErrServiceRequired = errors.New("wager service is required")

type ServiceHandler struct{ service *applicationwager.Service }

func NewServiceHandler(service *applicationwager.Service) (*ServiceHandler, error) {
	if service == nil {
		return nil, ErrServiceRequired
	}
	return &ServiceHandler{service: service}, nil
}

func (h *ServiceHandler) Handle(ctx context.Context, message TransactionMessage, _ []byte) error {
	input, err := buildProcessInput(message)
	if err != nil {
		return err
	}
	result, err := h.service.Process(ctx, input)
	if err != nil {
		return fmt.Errorf("process wager transaction: %w", err)
	}
	observeConsumerResult(message, result)
	return nil
}

func (h *ServiceHandler) HandleInTransaction(ctx context.Context, tx pgx.Tx, message TransactionMessage) error {
	input, err := buildProcessInput(message)
	if err != nil {
		return err
	}
	result, err := h.service.ProcessInTransaction(ctx, tx, input)
	if err != nil {
		return fmt.Errorf("process wager transaction in inbox transaction: %w", err)
	}
	observeConsumerResult(message, result)
	return nil
}

func buildProcessInput(message TransactionMessage) (applicationwager.ProcessInput, error) {
	operationMoney, err := domainmoney.Parse(message.Money.Amount, message.Money.Currency)
	if err != nil {
		return applicationwager.ProcessInput{}, fmt.Errorf("parse wager money: %w", err)
	}

	kind := domaintransaction.Kind(strings.ToUpper(strings.TrimSpace(message.Kind)))
	if !kind.IsExternal() {
		return applicationwager.ProcessInput{}, fmt.Errorf("invalid wager transaction kind %q", message.Kind)
	}

	payloadHash, err := applicationidempotency.HashBusiness(applicationidempotency.BusinessOperation{
		ProviderID:                     message.ProviderID,
		ExternalTransactionID:          message.ExternalTransactionID,
		PlayerID:                       message.PlayerID,
		WalletID:                       message.WalletID,
		RoundID:                        message.RoundID,
		GameID:                         message.GameID,
		Kind:                           string(kind),
		Money:                          operationMoney,
		ReferenceExternalTransactionID: message.ReferenceExternalTransactionID,
	})
	if err != nil {
		return applicationwager.ProcessInput{}, fmt.Errorf("hash wager business payload: %w", err)
	}

	transactionID, err := newID()
	if err != nil {
		return applicationwager.ProcessInput{}, fmt.Errorf("generate wager transaction ID: %w", err)
	}
	ledgerEntryID, err := newID()
	if err != nil {
		return applicationwager.ProcessInput{}, fmt.Errorf("generate ledger entry ID: %w", err)
	}

	occurredAt := time.Now().UTC()
	if message.OccurredAt != "" {
		occurredAt, err = time.Parse(time.RFC3339Nano, message.OccurredAt)
		if err != nil {
			return applicationwager.ProcessInput{}, fmt.Errorf("parse occurredAt: %w", err)
		}
		occurredAt = occurredAt.UTC()
	}

	transaction, err := domaintransaction.NewExternal(domaintransaction.ExternalInput{
		ID:                             transactionID,
		ExternalTransactionID:          message.ExternalTransactionID,
		ProviderID:                     message.ProviderID,
		IdempotencyKey:                 message.IdempotencyKey,
		PayloadHash:                    payloadHash,
		WalletID:                       message.WalletID,
		PlayerID:                       message.PlayerID,
		RoundID:                        message.RoundID,
		GameID:                         message.GameID,
		Kind:                           kind,
		Money:                          operationMoney,
		ReferenceExternalTransactionID: message.ReferenceExternalTransactionID,
		CreatedAt:                      occurredAt,
	})
	if err != nil {
		return applicationwager.ProcessInput{}, fmt.Errorf("create wager transaction: %w", err)
	}

	return applicationwager.ProcessInput{
		Transaction:   &transaction,
		LedgerEntryID: ledgerEntryID,
		CorrelationID: message.CorrelationID,
		CausationID:   message.CausationID,
		OccurredAt:    occurredAt,
	}, nil
}

func observeConsumerResult(message TransactionMessage, result applicationwager.ProcessResult) {
	if result.Replayed {
		observability.Default.IdempotentReplays.Add(1)
	}
	if result.Transaction != nil {
		switch result.Transaction.Status() {
		case domaintransaction.StatusProcessed:
			observability.Default.WagerProcessed.Add(1)
		case domaintransaction.StatusRejected:
			observability.Default.WagerRejected.Add(1)
		case domaintransaction.StatusPendingReference:
			observability.Default.WagerPending.Add(1)
		}
		slog.Info("sqs_wager_processed",
			"messageId", message.CorrelationID,
			"transactionId", result.Transaction.ID(),
			"walletId", result.Transaction.WalletID(),
			"providerId", result.Transaction.ProviderID(),
			"status", result.Transaction.Status(),
			"idempotentReplay", result.Replayed,
		)
	}
}

func newID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf("generate id: %w", err)
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}
