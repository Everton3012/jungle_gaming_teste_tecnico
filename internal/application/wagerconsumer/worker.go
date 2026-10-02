package wagerconsumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	applicationidempotency "jungle_gaming_teste_tecnico/internal/application/idempotency"
	postgresinfra "jungle_gaming_teste_tecnico/internal/infrastructure/postgres"
	inboxpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/inbox"
	sqsinfra "jungle_gaming_teste_tecnico/internal/infrastructure/sqs"
	"jungle_gaming_teste_tecnico/internal/observability"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	walletpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/wallet"
)

const (
	consumerName                         = "wager-transactions"
	messageTypeWagerTransactionRequested = "WagerTransactionRequested"
	maxAtomicAttempts                    = 4
)

var (
	ErrConsumerRequired          = errors.New("wager SQS consumer is required")
	ErrHandlerRequired           = errors.New("wager transaction handler is required")
	ErrInboxRepositoryRequired   = errors.New("wager inbox repository is required")
	ErrMessageIDRequired         = errors.New("SQS message ID is required")
	ErrReceiptHandleRequired     = errors.New("SQS receipt handle is required")
	ErrEnvelopeMessageIDRequired = errors.New("messageId is required")
	ErrEnvelopeTypeInvalid       = errors.New("message type must be WagerTransactionRequested")
)

type Handler interface {
	Handle(ctx context.Context, message TransactionMessage, rawPayload []byte) error
}

type TransactionalHandler interface {
	HandleInTransaction(ctx context.Context, tx pgx.Tx, message TransactionMessage) error
}

type MoneyMessage struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

type TransactionMessage struct {
	ProviderID                     string       `json:"providerId"`
	ExternalTransactionID          string       `json:"externalTransactionId"`
	IdempotencyKey                 string       `json:"idempotencyKey"`
	PlayerID                       string       `json:"playerId"`
	WalletID                       string       `json:"walletId"`
	RoundID                        string       `json:"roundId"`
	GameID                         string       `json:"gameId"`
	Kind                           string       `json:"kind"`
	Money                          MoneyMessage `json:"money"`
	ReferenceExternalTransactionID string       `json:"referenceExternalTransactionId,omitempty"`
	CorrelationID                  string       `json:"correlationId,omitempty"`
	CausationID                    string       `json:"causationId,omitempty"`
	OccurredAt                     string       `json:"occurredAt,omitempty"`
}

type MessageEnvelope struct {
	MessageID  string             `json:"messageId"`
	Type       string             `json:"type"`
	OccurredAt string             `json:"occurredAt"`
	Data       TransactionMessage `json:"data"`
}

type Worker struct {
	consumer           *sqsinfra.Consumer
	handler            Handler
	inbox              *inboxpostgres.Repository
	transactionManager *postgresinfra.TransactionManager
}

func NewWorker(consumer *sqsinfra.Consumer, handler Handler, inbox *inboxpostgres.Repository, managers ...*postgresinfra.TransactionManager) (*Worker, error) {
	if consumer == nil {
		return nil, ErrConsumerRequired
	}
	if handler == nil {
		return nil, ErrHandlerRequired
	}
	if inbox == nil {
		return nil, ErrInboxRepositoryRequired
	}
	var manager *postgresinfra.TransactionManager
	if len(managers) > 0 {
		manager = managers[0]
	}
	return &Worker{consumer: consumer, handler: handler, inbox: inbox, transactionManager: manager}, nil
}

func (w *Worker) Run(ctx context.Context) error {
	for {
		if ctx.Err() != nil {
			return nil
		}
		messages, err := w.consumer.Receive(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("receive wager messages: %w", err)
		}
		for _, message := range messages {
			if ctx.Err() != nil {
				return nil
			}
			started := time.Now()
			err := w.process(ctx, message)
			observability.Default.ObserveProcessing(started)
			if err != nil {
				slog.Error("sqs_wager_processing_failed",
					"messageId", message.ID,
					"receiveCount", message.ReceiveCount,
					"error", err.Error(),
				)
			}
			if ctx.Err() != nil {
				releaseCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				_ = w.consumer.ChangeVisibility(releaseCtx, message.ReceiptHandle, 0)
				cancel()
				return nil
			}
		}
	}
}

func (w *Worker) process(ctx context.Context, message sqsinfra.Message) error {
	if message.ReceiveCount > 1 {
		observability.Default.SQSRedeliveries.Add(1)
	}
	awsMessageID := strings.TrimSpace(message.ID)
	if awsMessageID == "" {
		return ErrMessageIDRequired
	}
	receiptHandle := strings.TrimSpace(message.ReceiptHandle)
	if receiptHandle == "" {
		return ErrReceiptHandleRequired
	}
	rawPayload := []byte(message.Body)

	payloadHash, hashErr := canonicalPayloadHash(rawPayload)
	if hashErr != nil {
		return w.failLegacy(ctx, awsMessageID, rawPayload, fmt.Errorf("decode wager message %q: %w", awsMessageID, hashErr))
	}

	envelope, payload, durableID, decodeErr := decodeTransportMessage(rawPayload, awsMessageID)
	if decodeErr != nil {
		return w.failWithKnownHash(ctx, durableID, payloadHash, decodeErr)
	}
	_ = envelope

	if w.transactionManager != nil {
		if transactional, ok := w.handler.(TransactionalHandler); ok {
			return w.processAtomic(ctx, message, durableID, receiptHandle, payloadHash, payload, transactional)
		}
	}

	beginResult, err := w.inbox.BeginForConsumer(ctx, consumerName, durableID, payloadHash, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("begin inbox message %q: %w", durableID, err)
	}
	if beginResult.AlreadyProcessed {
		return w.ack(ctx, durableID, receiptHandle)
	}
	if err := w.handler.Handle(ctx, payload, rawPayload); err != nil {
		return w.failExisting(ctx, durableID, fmt.Errorf("process wager message %q: %w", durableID, err))
	}
	if err := w.inbox.MarkProcessedForConsumer(ctx, consumerName, durableID, time.Now().UTC()); err != nil {
		return fmt.Errorf("mark wager inbox message %q processed: %w", durableID, err)
	}
	return w.ack(ctx, durableID, receiptHandle)
}

func (w *Worker) processAtomic(ctx context.Context, brokerMessage sqsinfra.Message, durableID, receiptHandle, payloadHash string, payload TransactionMessage, handler TransactionalHandler) error {
	var alreadyProcessed bool
	var lastErr error

	for attempt := 0; attempt < maxAtomicAttempts; attempt++ {
		alreadyProcessed = false
		lastErr = w.transactionManager.WithinTransaction(ctx, func(ctx context.Context, tx pgx.Tx) error {
			txInbox, err := w.inbox.WithDB(tx)
			if err != nil {
				return err
			}
			beginResult, err := txInbox.BeginForConsumer(ctx, consumerName, durableID, payloadHash, time.Now().UTC())
			if err != nil {
				return err
			}
			if beginResult.AlreadyProcessed {
				observability.Default.InboxDuplicates.Add(1)
				alreadyProcessed = true
				return nil
			}
			if err := handler.HandleInTransaction(ctx, tx, payload); err != nil {
				return err
			}
			return txInbox.MarkProcessedForConsumer(ctx, consumerName, durableID, time.Now().UTC())
		})
		if lastErr == nil {
			break
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if !isRetryableAtomicError(lastErr) {
			return fmt.Errorf("process wager message %q atomically: %w", durableID, lastErr)
		}
		timer := time.NewTimer(time.Duration(attempt+1) * 10 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
	if lastErr != nil {
		return fmt.Errorf("process wager message %q atomically after retries: %w", durableID, lastErr)
	}
	_ = alreadyProcessed // both paths acknowledge only after commit.
	return w.ack(ctx, durableID, receiptHandle)
}

func isRetryableAtomicError(err error) bool {
	if errors.Is(err, walletpostgres.ErrConcurrentUpdate) {
		return true
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "40001", // serialization_failure
			"40P01", // deadlock_detected
			"23505": // unique_violation caused by a racing idempotent insert
			return true
		}
	}

	return false
}

func decodeTransportMessage(rawPayload []byte, awsMessageID string) (MessageEnvelope, TransactionMessage, string, error) {
	var envelope MessageEnvelope
	if err := json.Unmarshal(rawPayload, &envelope); err != nil {
		return MessageEnvelope{}, TransactionMessage{}, awsMessageID, fmt.Errorf("decode wager message %q: %w", awsMessageID, err)
	}

	// Official transport envelope.
	if strings.TrimSpace(envelope.MessageID) != "" || strings.TrimSpace(envelope.Type) != "" {
		envelope.MessageID = strings.TrimSpace(envelope.MessageID)
		envelope.Type = strings.TrimSpace(envelope.Type)
		envelope.OccurredAt = strings.TrimSpace(envelope.OccurredAt)
		if envelope.MessageID == "" {
			return envelope, TransactionMessage{}, awsMessageID, ErrEnvelopeMessageIDRequired
		}
		if envelope.Type != messageTypeWagerTransactionRequested {
			return envelope, TransactionMessage{}, envelope.MessageID, ErrEnvelopeTypeInvalid
		}
		payload := envelope.Data
		if payload.OccurredAt == "" {
			payload.OccurredAt = envelope.OccurredAt
		}
		if payload.CorrelationID == "" {
			payload.CorrelationID = envelope.MessageID
		}
		normalize(&payload)
		if err := validate(payload); err != nil {
			return envelope, payload, envelope.MessageID, fmt.Errorf("validate wager message %q: %w", envelope.MessageID, err)
		}
		if envelope.OccurredAt == "" {
			return envelope, payload, envelope.MessageID, errors.New("occurredAt is required")
		}
		if _, err := time.Parse(time.RFC3339Nano, envelope.OccurredAt); err != nil {
			return envelope, payload, envelope.MessageID, fmt.Errorf("occurredAt must be RFC3339: %w", err)
		}
		return envelope, payload, envelope.MessageID, nil
	}

	var payload TransactionMessage
	if err := json.Unmarshal(rawPayload, &payload); err != nil {
		return MessageEnvelope{}, TransactionMessage{}, awsMessageID, err
	}
	normalize(&payload)
	if err := validate(payload); err != nil {
		return MessageEnvelope{}, payload, awsMessageID, fmt.Errorf("validate wager message %q: %w", awsMessageID, err)
	}
	return MessageEnvelope{}, payload, awsMessageID, nil
}

func (w *Worker) ack(ctx context.Context, messageID, receiptHandle string) error {
	if err := w.consumer.Delete(ctx, receiptHandle); err != nil {
		return fmt.Errorf("acknowledge wager message %q: %w", messageID, err)
	}
	return nil
}

func (w *Worker) failLegacy(ctx context.Context, messageID string, rawPayload []byte, processingErr error) error {
	payloadHash, err := canonicalPayloadHash(rawPayload)
	if err != nil {
		payloadHash = "invalid-json"
	}
	return w.failWithKnownHash(ctx, messageID, payloadHash, processingErr)
}

func (w *Worker) failWithKnownHash(ctx context.Context, messageID, payloadHash string, processingErr error) error {
	if processingErr == nil {
		return nil
	}
	messageID = strings.TrimSpace(messageID)
	if messageID == "" {
		return processingErr
	}
	begin, beginErr := w.inbox.BeginForConsumer(ctx, consumerName, messageID, payloadHash, time.Now().UTC())
	if beginErr != nil {
		return errors.Join(processingErr, fmt.Errorf("begin failed inbox message %q: %w", messageID, beginErr))
	}
	if begin.AlreadyProcessed {
		return processingErr
	}
	return w.failExisting(ctx, messageID, processingErr)
}

func (w *Worker) failExisting(ctx context.Context, messageID string, processingErr error) error {
	if processingErr == nil {
		return nil
	}
	markErr := w.inbox.MarkFailedForConsumer(ctx, consumerName, messageID, processingErr.Error(), time.Now().UTC())
	if markErr != nil {
		return errors.Join(processingErr, fmt.Errorf("mark wager inbox message %q failed: %w", messageID, markErr))
	}
	return processingErr
}

func canonicalPayloadHash(payload []byte) (string, error) {
	return applicationidempotency.Hash(payload)
}

func hashPayload(payload []byte) string {
	value, _ := canonicalPayloadHash(payload)
	return value
}

func normalize(message *TransactionMessage) {
	message.ProviderID = strings.TrimSpace(message.ProviderID)
	message.ExternalTransactionID = strings.TrimSpace(message.ExternalTransactionID)
	message.IdempotencyKey = strings.TrimSpace(message.IdempotencyKey)
	message.PlayerID = strings.TrimSpace(message.PlayerID)
	message.WalletID = strings.TrimSpace(message.WalletID)
	message.RoundID = strings.TrimSpace(message.RoundID)
	message.GameID = strings.TrimSpace(message.GameID)
	message.Kind = strings.ToUpper(strings.TrimSpace(message.Kind))
	message.Money.Amount = strings.TrimSpace(message.Money.Amount)
	message.Money.Currency = strings.ToUpper(strings.TrimSpace(message.Money.Currency))
	message.ReferenceExternalTransactionID = strings.TrimSpace(message.ReferenceExternalTransactionID)
	message.CorrelationID = strings.TrimSpace(message.CorrelationID)
	message.CausationID = strings.TrimSpace(message.CausationID)
	message.OccurredAt = strings.TrimSpace(message.OccurredAt)
}

func validate(message TransactionMessage) error {
	if message.ProviderID == "" {
		return errors.New("providerId is required")
	}
	if message.ExternalTransactionID == "" {
		return errors.New("externalTransactionId is required")
	}
	if message.IdempotencyKey == "" {
		return errors.New("idempotencyKey is required")
	}
	if message.PlayerID == "" {
		return errors.New("playerId is required")
	}
	if message.WalletID == "" {
		return errors.New("walletId is required")
	}
	if message.RoundID == "" {
		return errors.New("roundId is required")
	}
	if message.GameID == "" {
		return errors.New("gameId is required")
	}
	switch message.Kind {
	case "BET", "WIN", "LOSS", "REFUND", "ROLLBACK":
	default:
		return fmt.Errorf("unsupported transaction kind %q", message.Kind)
	}
	if message.Money.Amount == "" {
		return errors.New("money.amount is required")
	}
	if message.Money.Currency == "" {
		return errors.New("money.currency is required")
	}
	if (message.Kind == "REFUND" || message.Kind == "ROLLBACK") && message.ReferenceExternalTransactionID == "" {
		return errors.New("referenceExternalTransactionId is required for reversal")
	}
	if message.OccurredAt != "" {
		if _, err := time.Parse(time.RFC3339Nano, message.OccurredAt); err != nil {
			return fmt.Errorf("occurredAt must be RFC3339: %w", err)
		}
	}
	return nil
}
