package outbox

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	domainoutbox "jungle_gaming_teste_tecnico/internal/domain/outbox"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrNotFound           = errors.New("outbox event not found")
	ErrRepositoryRequired = errors.New("database connection is required")
	ErrEventRequired      = errors.New("outbox event is required")
	ErrInvalidLimit       = errors.New("limit must be greater than zero")
	ErrInvalidTime        = errors.New("time must not be zero")
	ErrLastErrorRequired  = errors.New("last error is required")
	ErrInvalidState       = errors.New("outbox event is not in the expected state")
)

type DBTX interface {
	Exec(
		ctx context.Context,
		sql string,
		arguments ...any,
	) (pgconn.CommandTag, error)

	Query(
		ctx context.Context,
		sql string,
		args ...any,
	) (pgx.Rows, error)

	QueryRow(
		ctx context.Context,
		sql string,
		args ...any,
	) pgx.Row
}

type Repository struct {
	db DBTX
}

func NewRepository(db DBTX) (*Repository, error) {
	if db == nil {
		return nil, ErrRepositoryRequired
	}

	return &Repository{
		db: db,
	}, nil
}

func (r *Repository) WithDB(
	db DBTX,
) (*Repository, error) {
	return NewRepository(db)
}

func (r *Repository) Create(
	ctx context.Context,
	event *domainoutbox.Event,
) error {
	if event == nil {
		return ErrEventRequired
	}

	_, err := r.db.Exec(
		ctx,
		insertQuery,
		event.ID(),
		event.EventType(),
		event.AggregateType(),
		event.AggregateID(),
		event.Payload(),
		string(event.Status()),
		event.Attempts(),
		event.AvailableAt(),
		nullableTime(event.PublishedAt()),
		nullableString(event.LastError()),
		event.CreatedAt(),
		event.UpdatedAt(),
	)
	if err != nil {
		return fmt.Errorf(
			"create outbox event: %w",
			err,
		)
	}

	return nil
}

func (r *Repository) FindByID(
	ctx context.Context,
	id string,
) (*domainoutbox.Event, error) {
	event, err := scanEvent(
		r.db.QueryRow(
			ctx,
			findByIDQuery,
			id,
		),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}

	if err != nil {
		return nil, fmt.Errorf(
			"find outbox event by id: %w",
			err,
		)
	}

	return event, nil
}

func (r *Repository) ClaimPending(
	ctx context.Context,
	now time.Time,
	limit int,
) ([]*domainoutbox.Event, error) {
	if now.IsZero() {
		return nil, ErrInvalidTime
	}

	if limit <= 0 {
		return nil, ErrInvalidLimit
	}

	rows, err := r.db.Query(
		ctx,
		claimPendingQuery,
		now,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"claim pending outbox events: %w",
			err,
		)
	}
	defer rows.Close()

	events := make(
		[]*domainoutbox.Event,
		0,
	)

	for rows.Next() {
		event, err := scanEvent(rows)
		if err != nil {
			return nil, fmt.Errorf(
				"scan claimed outbox event: %w",
				err,
			)
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate claimed outbox events: %w",
			err,
		)
	}

	return events, nil
}

func (r *Repository) MarkPublished(
	ctx context.Context,
	id string,
	publishedAt time.Time,
) error {
	if publishedAt.IsZero() {
		return ErrInvalidTime
	}

	result, err := r.db.Exec(
		ctx,
		markPublishedQuery,
		id,
		publishedAt,
	)
	if err != nil {
		return fmt.Errorf(
			"mark outbox event published: %w",
			err,
		)
	}

	if result.RowsAffected() == 0 {
		return ErrInvalidState
	}

	return nil
}

func (r *Repository) Reschedule(
	ctx context.Context,
	id string,
	availableAt time.Time,
	lastError string,
	updatedAt time.Time,
) error {
	if availableAt.IsZero() || updatedAt.IsZero() {
		return ErrInvalidTime
	}

	if lastError == "" {
		return ErrLastErrorRequired
	}

	result, err := r.db.Exec(
		ctx,
		rescheduleQuery,
		id,
		availableAt,
		lastError,
		updatedAt,
	)
	if err != nil {
		return fmt.Errorf(
			"reschedule outbox event: %w",
			err,
		)
	}

	if result.RowsAffected() == 0 {
		return ErrInvalidState
	}

	return nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanEvent(
	row scanner,
) (*domainoutbox.Event, error) {
	var (
		id            string
		eventType     string
		aggregateType string
		aggregateID   string
		payload       []byte
		status        string
		attempts      int
		availableAt   time.Time
		publishedAt   pgtype.Timestamptz
		lastError     pgtype.Text
		createdAt     time.Time
		updatedAt     time.Time
	)

	err := row.Scan(
		&id,
		&eventType,
		&aggregateType,
		&aggregateID,
		&payload,
		&status,
		&attempts,
		&availableAt,
		&publishedAt,
		&lastError,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return nil, err
	}

	event, err := domainoutbox.Rehydrate(
		domainoutbox.RehydrateInput{
			ID:            id,
			EventType:     eventType,
			AggregateType: aggregateType,
			AggregateID:   aggregateID,
			Payload:       json.RawMessage(payload),
			Status:        domainoutbox.Status(status),
			Attempts:      attempts,
			AvailableAt:   availableAt,
			PublishedAt:   timestampValue(publishedAt),
			LastError:     textPointer(lastError),
			CreatedAt:     createdAt,
			UpdatedAt:     updatedAt,
		},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"rehydrate outbox event: %w",
			err,
		)
	}

	return event, nil
}

func nullableTime(
	value *time.Time,
) any {
	if value == nil {
		return nil
	}

	return *value
}

func nullableString(
	value *string,
) any {
	if value == nil {
		return nil
	}

	return *value
}

func timestampValue(
	value pgtype.Timestamptz,
) *time.Time {
	if !value.Valid {
		return nil
	}

	result := value.Time

	return &result
}

func textPointer(
	value pgtype.Text,
) *string {
	if !value.Valid {
		return nil
	}

	result := value.String

	return &result
}
