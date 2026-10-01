package outbox

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type Status string

const (
	StatusPending    Status = "PENDING"
	StatusProcessing Status = "PROCESSING"
	StatusPublished  Status = "PUBLISHED"
)

var (
	ErrIDRequired            = errors.New("outbox event id is required")
	ErrEventTypeRequired     = errors.New("outbox event type is required")
	ErrAggregateTypeRequired = errors.New("outbox aggregate type is required")
	ErrAggregateIDRequired   = errors.New("outbox aggregate id is required")
	ErrPayloadRequired       = errors.New("outbox payload is required")
	ErrInvalidStatus         = errors.New("invalid outbox status")
	ErrInvalidAttempts       = errors.New("outbox attempts cannot be negative")
	ErrInvalidTimestamps     = errors.New("invalid outbox timestamps")
	ErrInvalidPublishedState = errors.New("invalid outbox published state")
)

type Event struct {
	id            string
	eventType     string
	aggregateType string
	aggregateID   string
	payload       json.RawMessage
	status        Status
	attempts      int
	availableAt   time.Time
	publishedAt   *time.Time
	lastError     *string
	createdAt     time.Time
	updatedAt     time.Time
}

type NewEventInput struct {
	ID            string
	EventType     string
	AggregateType string
	AggregateID   string
	Payload       json.RawMessage
	CreatedAt     time.Time
}

func NewEvent(input NewEventInput) (*Event, error) {
	event := &Event{
		id:            strings.TrimSpace(input.ID),
		eventType:     strings.TrimSpace(input.EventType),
		aggregateType: strings.TrimSpace(input.AggregateType),
		aggregateID:   strings.TrimSpace(input.AggregateID),
		payload:       clonePayload(input.Payload),
		status:        StatusPending,
		attempts:      0,
		availableAt:   input.CreatedAt,
		createdAt:     input.CreatedAt,
		updatedAt:     input.CreatedAt,
	}

	if err := event.validate(); err != nil {
		return nil, err
	}

	return event, nil
}

type RehydrateInput struct {
	ID            string
	EventType     string
	AggregateType string
	AggregateID   string
	Payload       json.RawMessage
	Status        Status
	Attempts      int
	AvailableAt   time.Time
	PublishedAt   *time.Time
	LastError     *string
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

func Rehydrate(input RehydrateInput) (*Event, error) {
	event := &Event{
		id:            strings.TrimSpace(input.ID),
		eventType:     strings.TrimSpace(input.EventType),
		aggregateType: strings.TrimSpace(input.AggregateType),
		aggregateID:   strings.TrimSpace(input.AggregateID),
		payload:       clonePayload(input.Payload),
		status:        input.Status,
		attempts:      input.Attempts,
		availableAt:   input.AvailableAt,
		publishedAt:   cloneTime(input.PublishedAt),
		lastError:     cloneString(input.LastError),
		createdAt:     input.CreatedAt,
		updatedAt:     input.UpdatedAt,
	}

	if err := event.validate(); err != nil {
		return nil, err
	}

	return event, nil
}

func (e *Event) validate() error {
	if e.id == "" {
		return ErrIDRequired
	}

	if e.eventType == "" {
		return ErrEventTypeRequired
	}

	if e.aggregateType == "" {
		return ErrAggregateTypeRequired
	}

	if e.aggregateID == "" {
		return ErrAggregateIDRequired
	}

	if len(e.payload) == 0 || !json.Valid(e.payload) {
		return ErrPayloadRequired
	}

	switch e.status {
	case StatusPending, StatusProcessing, StatusPublished:
	default:
		return ErrInvalidStatus
	}

	if e.attempts < 0 {
		return ErrInvalidAttempts
	}

	if e.createdAt.IsZero() ||
		e.updatedAt.IsZero() ||
		e.availableAt.IsZero() ||
		e.updatedAt.Before(e.createdAt) {

		return ErrInvalidTimestamps
	}

	if e.status == StatusPublished && e.publishedAt == nil {
		return ErrInvalidPublishedState
	}

	if e.status != StatusPublished && e.publishedAt != nil {
		return ErrInvalidPublishedState
	}

	return nil
}

func (e *Event) ID() string {
	return e.id
}

func (e *Event) EventType() string {
	return e.eventType
}

func (e *Event) AggregateType() string {
	return e.aggregateType
}

func (e *Event) AggregateID() string {
	return e.aggregateID
}

func (e *Event) Payload() json.RawMessage {
	return clonePayload(e.payload)
}

func (e *Event) Status() Status {
	return e.status
}

func (e *Event) Attempts() int {
	return e.attempts
}

func (e *Event) AvailableAt() time.Time {
	return e.availableAt
}

func (e *Event) PublishedAt() *time.Time {
	return cloneTime(e.publishedAt)
}

func (e *Event) LastError() *string {
	return cloneString(e.lastError)
}

func (e *Event) CreatedAt() time.Time {
	return e.createdAt
}

func (e *Event) UpdatedAt() time.Time {
	return e.updatedAt
}

func clonePayload(payload json.RawMessage) json.RawMessage {
	if payload == nil {
		return nil
	}

	cloned := make(json.RawMessage, len(payload))
	copy(cloned, payload)

	return cloned
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}

	cloned := *value
	return &cloned
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}

	cloned := *value
	return &cloned
}
