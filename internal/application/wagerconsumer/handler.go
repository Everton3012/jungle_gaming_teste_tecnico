package wagerconsumer

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	applicationidempotency "jungle_gaming_teste_tecnico/internal/application/idempotency"
	applicationwager "jungle_gaming_teste_tecnico/internal/application/wager"
	domainmoney "jungle_gaming_teste_tecnico/internal/domain/money"
	domaintransaction "jungle_gaming_teste_tecnico/internal/domain/wagertransaction"
)

var ErrServiceRequired = errors.New(
	"wager service is required",
)

type ServiceHandler struct {
	service *applicationwager.Service
}

func NewServiceHandler(
	service *applicationwager.Service,
) (*ServiceHandler, error) {
	if service == nil {
		return nil, ErrServiceRequired
	}

	return &ServiceHandler{
		service: service,
	}, nil
}

func (h *ServiceHandler) Handle(
	ctx context.Context,
	message TransactionMessage,
	rawPayload []byte,
) error {
	payloadHash, err :=
		applicationidempotency.Hash(
			rawPayload,
		)
	if err != nil {
		return fmt.Errorf(
			"hash wager payload: %w",
			err,
		)
	}

	operationMoney, err :=
		domainmoney.Parse(
			message.Money.Amount,
			message.Money.Currency,
		)
	if err != nil {
		return fmt.Errorf(
			"parse wager money: %w",
			err,
		)
	}

	kind :=
		domaintransaction.Kind(
			strings.ToUpper(
				strings.TrimSpace(
					message.Kind,
				),
			),
		)

	if !kind.IsExternal() {
		return fmt.Errorf(
			"invalid wager transaction kind %q",
			message.Kind,
		)
	}

	transactionID, err := newID()
	if err != nil {
		return fmt.Errorf(
			"generate wager transaction ID: %w",
			err,
		)
	}

	ledgerEntryID, err := newID()
	if err != nil {
		return fmt.Errorf(
			"generate ledger entry ID: %w",
			err,
		)
	}

	occurredAt := time.Now().UTC()

	if message.OccurredAt != "" {
		occurredAt, err = time.Parse(
			time.RFC3339Nano,
			message.OccurredAt,
		)
		if err != nil {
			return fmt.Errorf(
				"parse occurredAt: %w",
				err,
			)
		}

		occurredAt = occurredAt.UTC()
	}

	transaction, err :=
		domaintransaction.NewExternal(
			domaintransaction.ExternalInput{
				ID: transactionID,

				ExternalTransactionID: message.ExternalTransactionID,

				ProviderID: message.ProviderID,

				IdempotencyKey: message.IdempotencyKey,

				PayloadHash: payloadHash,

				WalletID: message.WalletID,

				PlayerID: message.PlayerID,

				RoundID: message.RoundID,

				GameID: message.GameID,

				Kind: kind,

				Money: operationMoney,

				ReferenceExternalTransactionID: message.ReferenceExternalTransactionID,

				CreatedAt: occurredAt,
			},
		)
	if err != nil {
		return fmt.Errorf(
			"create wager transaction: %w",
			err,
		)
	}

	_, err = h.service.Process(
		ctx,
		applicationwager.ProcessInput{
			Transaction: &transaction,

			LedgerEntryID: ledgerEntryID,

			CorrelationID: message.CorrelationID,

			CausationID: message.CausationID,

			OccurredAt: occurredAt,
		},
	)
	if err != nil {
		return fmt.Errorf(
			"process wager transaction: %w",
			err,
		)
	}

	return nil
}

func newID() (string, error) {
	var value [16]byte

	if _, err := rand.Read(value[:]); err != nil {
		return "", fmt.Errorf(
			"generate id: %w",
			err,
		)
	}

	value[6] =
		(value[6] & 0x0f) | 0x40

	value[8] =
		(value[8] & 0x3f) | 0x80

	return fmt.Sprintf(
		"%08x-%04x-%04x-%04x-%012x",
		value[0:4],
		value[4:6],
		value[6:8],
		value[8:10],
		value[10:16],
	), nil
}
