package inbox

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

const DefaultConsumerName = "wager-transactions"

var (
	ErrRepositoryRequired   = errors.New("database connection is required")
	ErrConsumerNameRequired = errors.New("inbox consumer name is required")
	ErrMessageIDRequired    = errors.New("inbox message ID is required")
	ErrPayloadHashRequired  = errors.New("inbox payload hash is required")
	ErrInvalidTime          = errors.New("time must not be zero")
	ErrLastErrorRequired    = errors.New("last error is required")
	ErrNotFound             = errors.New("inbox message not found")
	ErrPayloadConflict      = errors.New("inbox message payload conflict")
)

const (
	StatusProcessing = "PROCESSING"
	StatusProcessed  = "PROCESSED"
	StatusFailed     = "FAILED"
)

type DBTX interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

type Message struct {
	ConsumerName string
	MessageID    string
	PayloadHash  string
	Status       string
	Attempts     int
	LastError    *string
	ReceivedAt   time.Time
	ProcessedAt  *time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type BeginResult struct {
	Message          *Message
	AlreadyProcessed bool
}

type Repository struct{ db DBTX }

func NewRepository(db DBTX) (*Repository, error) {
	if db == nil {
		return nil, ErrRepositoryRequired
	}
	return &Repository{db: db}, nil
}

func (r *Repository) WithDB(db DBTX) (*Repository, error) { return NewRepository(db) }

func (r *Repository) Begin(ctx context.Context, messageID, payloadHash string, now time.Time) (*BeginResult, error) {
	return r.BeginForConsumer(ctx, DefaultConsumerName, messageID, payloadHash, now)
}

func (r *Repository) BeginForConsumer(ctx context.Context, consumerName, messageID, payloadHash string, now time.Time) (*BeginResult, error) {
	consumerName = strings.TrimSpace(consumerName)
	messageID = strings.TrimSpace(messageID)
	payloadHash = strings.TrimSpace(payloadHash)
	if consumerName == "" {
		return nil, ErrConsumerNameRequired
	}
	if messageID == "" {
		return nil, ErrMessageIDRequired
	}
	if payloadHash == "" {
		return nil, ErrPayloadHashRequired
	}
	if now.IsZero() {
		return nil, ErrInvalidTime
	}
	now = now.UTC()

	const insertQuery = `
INSERT INTO inbox_messages (
    consumer_name, message_id, payload_hash, status, attempts, last_error,
    received_at, processed_at, created_at, updated_at
)
VALUES ($1, $2, $3, 'PROCESSING', 1, NULL, $4, NULL, $4, $4)
ON CONFLICT (consumer_name, message_id) DO NOTHING
RETURNING
    consumer_name, message_id, payload_hash, status, attempts, last_error,
    received_at, processed_at, created_at, updated_at;`

	message, err := scanMessage(r.db.QueryRow(ctx, insertQuery, consumerName, messageID, payloadHash, now))
	if err == nil {
		return &BeginResult{Message: message, AlreadyProcessed: false}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, fmt.Errorf("create inbox message: %w", err)
	}

	existing, err := r.FindByConsumerMessageID(ctx, consumerName, messageID)
	if err != nil {
		return nil, err
	}
	if existing.PayloadHash != payloadHash {
		return nil, fmt.Errorf("%w: consumer %q message %q", ErrPayloadConflict, consumerName, messageID)
	}
	if existing.Status == StatusProcessed {
		return &BeginResult{Message: existing, AlreadyProcessed: true}, nil
	}

	const retryQuery = `
UPDATE inbox_messages
SET status = 'PROCESSING', attempts = attempts + 1, last_error = NULL,
    received_at = $3, processed_at = NULL, updated_at = $3
WHERE consumer_name = $1 AND message_id = $2
RETURNING
    consumer_name, message_id, payload_hash, status, attempts, last_error,
    received_at, processed_at, created_at, updated_at;`

	retried, err := scanMessage(r.db.QueryRow(ctx, retryQuery, consumerName, messageID, now))
	if err != nil {
		return nil, fmt.Errorf("retry inbox message: %w", err)
	}
	return &BeginResult{Message: retried, AlreadyProcessed: false}, nil
}

func (r *Repository) FindByID(ctx context.Context, messageID string) (*Message, error) {
	return r.FindByConsumerMessageID(ctx, DefaultConsumerName, messageID)
}

func (r *Repository) FindByConsumerMessageID(ctx context.Context, consumerName, messageID string) (*Message, error) {
	consumerName = strings.TrimSpace(consumerName)
	messageID = strings.TrimSpace(messageID)
	if consumerName == "" {
		return nil, ErrConsumerNameRequired
	}
	if messageID == "" {
		return nil, ErrMessageIDRequired
	}

	const query = `
SELECT consumer_name, message_id, payload_hash, status, attempts, last_error,
       received_at, processed_at, created_at, updated_at
FROM inbox_messages
WHERE consumer_name = $1 AND message_id = $2;`

	message, err := scanMessage(r.db.QueryRow(ctx, query, consumerName, messageID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("find inbox message by ID: %w", err)
	}
	return message, nil
}

func (r *Repository) MarkProcessed(ctx context.Context, messageID string, now time.Time) error {
	return r.MarkProcessedForConsumer(ctx, DefaultConsumerName, messageID, now)
}

func (r *Repository) MarkProcessedForConsumer(ctx context.Context, consumerName, messageID string, now time.Time) error {
	consumerName = strings.TrimSpace(consumerName)
	messageID = strings.TrimSpace(messageID)
	if consumerName == "" {
		return ErrConsumerNameRequired
	}
	if messageID == "" {
		return ErrMessageIDRequired
	}
	if now.IsZero() {
		return ErrInvalidTime
	}
	now = now.UTC()

	const query = `
UPDATE inbox_messages
SET status = 'PROCESSED', last_error = NULL, processed_at = $3, updated_at = $3
WHERE consumer_name = $1 AND message_id = $2 AND status = 'PROCESSING';`
	result, err := r.db.Exec(ctx, query, consumerName, messageID, now)
	if err != nil {
		return fmt.Errorf("mark inbox message processed: %w", err)
	}
	if result.RowsAffected() == 0 {
		message, findErr := r.FindByConsumerMessageID(ctx, consumerName, messageID)
		if findErr != nil {
			return findErr
		}
		if message.Status == StatusProcessed {
			return nil
		}
		return fmt.Errorf("cannot mark inbox message %q processed from status %q", messageID, message.Status)
	}
	return nil
}

func (r *Repository) MarkFailed(ctx context.Context, messageID, lastError string, now time.Time) error {
	return r.MarkFailedForConsumer(ctx, DefaultConsumerName, messageID, lastError, now)
}

func (r *Repository) MarkFailedForConsumer(ctx context.Context, consumerName, messageID, lastError string, now time.Time) error {
	consumerName = strings.TrimSpace(consumerName)
	messageID = strings.TrimSpace(messageID)
	lastError = strings.TrimSpace(lastError)
	if consumerName == "" {
		return ErrConsumerNameRequired
	}
	if messageID == "" {
		return ErrMessageIDRequired
	}
	if lastError == "" {
		return ErrLastErrorRequired
	}
	if now.IsZero() {
		return ErrInvalidTime
	}
	now = now.UTC()

	const query = `
UPDATE inbox_messages
SET status = 'FAILED', last_error = $3, processed_at = NULL, updated_at = $4
WHERE consumer_name = $1 AND message_id = $2 AND status <> 'PROCESSED';`
	result, err := r.db.Exec(ctx, query, consumerName, messageID, lastError, now)
	if err != nil {
		return fmt.Errorf("mark inbox message failed: %w", err)
	}
	if result.RowsAffected() == 0 {
		message, findErr := r.FindByConsumerMessageID(ctx, consumerName, messageID)
		if findErr != nil {
			return findErr
		}
		if message.Status == StatusProcessed {
			return nil
		}
		return fmt.Errorf("cannot mark inbox message %q failed from status %q", messageID, message.Status)
	}
	return nil
}

type scanner interface{ Scan(dest ...any) error }

func scanMessage(row scanner) (*Message, error) {
	var consumerName, messageID, payloadHash, status string
	var attempts int
	var lastError pgtype.Text
	var receivedAt, createdAt, updatedAt time.Time
	var processedAt pgtype.Timestamptz
	if err := row.Scan(&consumerName, &messageID, &payloadHash, &status, &attempts, &lastError, &receivedAt, &processedAt, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	return &Message{
		ConsumerName: consumerName,
		MessageID:    messageID,
		PayloadHash:  payloadHash,
		Status:       status,
		Attempts:     attempts,
		LastError:    textPointer(lastError),
		ReceivedAt:   receivedAt,
		ProcessedAt:  timestampPointer(processedAt),
		CreatedAt:    createdAt,
		UpdatedAt:    updatedAt,
	}, nil
}

func textPointer(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}

func timestampPointer(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	result := value.Time
	return &result
}
