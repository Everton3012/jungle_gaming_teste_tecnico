package wagerconsumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	sqsinfra "jungle_gaming_teste_tecnico/internal/infrastructure/sqs"
)

var (
	ErrConsumerRequired = errors.New(
		"wager SQS consumer is required",
	)

	ErrHandlerRequired = errors.New(
		"wager transaction handler is required",
	)
)

type Handler interface {
	Handle(
		ctx context.Context,
		message TransactionMessage,
		rawPayload []byte,
	) error
}

type MoneyMessage struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

type TransactionMessage struct {
	ProviderID            string `json:"providerId"`
	ExternalTransactionID string `json:"externalTransactionId"`
	IdempotencyKey        string `json:"idempotencyKey"`

	PlayerID string `json:"playerId"`
	WalletID string `json:"walletId"`

	RoundID string `json:"roundId"`
	GameID  string `json:"gameId"`

	Kind string `json:"kind"`

	Money MoneyMessage `json:"money"`

	ReferenceExternalTransactionID string `json:"referenceExternalTransactionId,omitempty"`

	CorrelationID string `json:"correlationId,omitempty"`
	CausationID   string `json:"causationId,omitempty"`
	OccurredAt    string `json:"occurredAt,omitempty"`
}

type Worker struct {
	consumer *sqsinfra.Consumer
	handler  Handler
}

func NewWorker(
	consumer *sqsinfra.Consumer,
	handler Handler,
) (*Worker, error) {
	if consumer == nil {
		return nil, ErrConsumerRequired
	}

	if handler == nil {
		return nil, ErrHandlerRequired
	}

	return &Worker{
		consumer: consumer,
		handler:  handler,
	}, nil
}

func (w *Worker) Run(
	ctx context.Context,
) error {
	for {
		if ctx.Err() != nil {
			return nil
		}

		messages, err := w.consumer.Receive(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}

			return fmt.Errorf(
				"receive wager messages: %w",
				err,
			)
		}

		for _, message := range messages {
			_ = w.process(
				ctx,
				message,
			)
		}
	}
}

func (w *Worker) process(
	ctx context.Context,
	message sqsinfra.Message,
) error {
	rawPayload := []byte(message.Body)

	var payload TransactionMessage

	if err := json.Unmarshal(
		rawPayload,
		&payload,
	); err != nil {
		return fmt.Errorf(
			"decode wager message %q: %w",
			message.ID,
			err,
		)
	}

	normalize(&payload)

	if err := validate(payload); err != nil {
		return fmt.Errorf(
			"validate wager message %q: %w",
			message.ID,
			err,
		)
	}

	if err := w.handler.Handle(
		ctx,
		payload,
		rawPayload,
	); err != nil {
		return fmt.Errorf(
			"process wager message %q: %w",
			message.ID,
			err,
		)
	}

	if err := w.consumer.Delete(
		ctx,
		message.ReceiptHandle,
	); err != nil {
		return fmt.Errorf(
			"acknowledge wager message %q: %w",
			message.ID,
			err,
		)
	}

	return nil
}

func normalize(
	message *TransactionMessage,
) {
	message.ProviderID =
		strings.TrimSpace(message.ProviderID)

	message.ExternalTransactionID =
		strings.TrimSpace(
			message.ExternalTransactionID,
		)

	message.IdempotencyKey =
		strings.TrimSpace(
			message.IdempotencyKey,
		)

	message.PlayerID =
		strings.TrimSpace(message.PlayerID)

	message.WalletID =
		strings.TrimSpace(message.WalletID)

	message.RoundID =
		strings.TrimSpace(message.RoundID)

	message.GameID =
		strings.TrimSpace(message.GameID)

	message.Kind =
		strings.ToUpper(
			strings.TrimSpace(message.Kind),
		)

	message.Money.Amount =
		strings.TrimSpace(message.Money.Amount)

	message.Money.Currency =
		strings.ToUpper(
			strings.TrimSpace(
				message.Money.Currency,
			),
		)

	message.ReferenceExternalTransactionID =
		strings.TrimSpace(
			message.ReferenceExternalTransactionID,
		)

	message.CorrelationID =
		strings.TrimSpace(
			message.CorrelationID,
		)

	message.CausationID =
		strings.TrimSpace(
			message.CausationID,
		)

	message.OccurredAt =
		strings.TrimSpace(
			message.OccurredAt,
		)
}

func validate(
	message TransactionMessage,
) error {
	if message.ProviderID == "" {
		return errors.New(
			"providerId is required",
		)
	}

	if message.ExternalTransactionID == "" {
		return errors.New(
			"externalTransactionId is required",
		)
	}

	if message.IdempotencyKey == "" {
		return errors.New(
			"idempotencyKey is required",
		)
	}

	if message.PlayerID == "" {
		return errors.New(
			"playerId is required",
		)
	}

	if message.WalletID == "" {
		return errors.New(
			"walletId is required",
		)
	}

	if message.RoundID == "" {
		return errors.New(
			"roundId is required",
		)
	}

	if message.GameID == "" {
		return errors.New(
			"gameId is required",
		)
	}

	switch message.Kind {
	case "BET",
		"WIN",
		"LOSS",
		"REFUND",
		"ROLLBACK":

	default:
		return fmt.Errorf(
			"unsupported transaction kind %q",
			message.Kind,
		)
	}

	if message.Money.Amount == "" {
		return errors.New(
			"money.amount is required",
		)
	}

	if message.Money.Currency == "" {
		return errors.New(
			"money.currency is required",
		)
	}

	if message.Kind == "REFUND" ||
		message.Kind == "ROLLBACK" {

		if message.ReferenceExternalTransactionID == "" {
			return errors.New(
				"referenceExternalTransactionId is required for reversal transactions",
			)
		}
	}

	if message.OccurredAt != "" {
		if _, err := time.Parse(
			time.RFC3339Nano,
			message.OccurredAt,
		); err != nil {
			return fmt.Errorf(
				"invalid occurredAt: %w",
				err,
			)
		}
	}

	return nil
}
