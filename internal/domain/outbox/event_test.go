package outbox

import (
	"encoding/json"
	"errors"
	"testing"
	"time"
)

func TestNewEventCreatesPendingEvent(t *testing.T) {
	now := time.Now().UTC()

	event, err := NewEvent(
		NewEventInput{
			ID:            "event-1",
			EventType:     "WAGER_TRANSACTION_PROCESSED",
			AggregateType: "WAGER_TRANSACTION",
			AggregateID:   "transaction-1",
			Payload: json.RawMessage(
				`{"transactionId":"transaction-1"}`,
			),
			CreatedAt: now,
		},
	)
	if err != nil {
		t.Fatalf("NewEvent() error = %v", err)
	}

	if event.ID() != "event-1" {
		t.Fatalf("unexpected id: %s", event.ID())
	}

	if event.Status() != StatusPending {
		t.Fatalf(
			"unexpected status: %s",
			event.Status(),
		)
	}

	if event.Attempts() != 0 {
		t.Fatalf(
			"unexpected attempts: %d",
			event.Attempts(),
		)
	}

	if !event.AvailableAt().Equal(now) {
		t.Fatalf(
			"unexpected availableAt: %s",
			event.AvailableAt(),
		)
	}

	if event.PublishedAt() != nil {
		t.Fatal("publishedAt must be nil")
	}
}

func TestNewEventRejectsInvalidValues(t *testing.T) {
	now := time.Now().UTC()

	tests := []struct {
		name  string
		input NewEventInput
		want  error
	}{
		{
			name: "missing id",
			input: NewEventInput{
				EventType:     "TYPE",
				AggregateType: "AGGREGATE",
				AggregateID:   "aggregate-1",
				Payload:       json.RawMessage(`{}`),
				CreatedAt:     now,
			},
			want: ErrIDRequired,
		},
		{
			name: "missing event type",
			input: NewEventInput{
				ID:            "event-1",
				AggregateType: "AGGREGATE",
				AggregateID:   "aggregate-1",
				Payload:       json.RawMessage(`{}`),
				CreatedAt:     now,
			},
			want: ErrEventTypeRequired,
		},
		{
			name: "missing aggregate type",
			input: NewEventInput{
				ID:          "event-1",
				EventType:   "TYPE",
				AggregateID: "aggregate-1",
				Payload:     json.RawMessage(`{}`),
				CreatedAt:   now,
			},
			want: ErrAggregateTypeRequired,
		},
		{
			name: "missing aggregate id",
			input: NewEventInput{
				ID:            "event-1",
				EventType:     "TYPE",
				AggregateType: "AGGREGATE",
				Payload:       json.RawMessage(`{}`),
				CreatedAt:     now,
			},
			want: ErrAggregateIDRequired,
		},
		{
			name: "invalid payload",
			input: NewEventInput{
				ID:            "event-1",
				EventType:     "TYPE",
				AggregateType: "AGGREGATE",
				AggregateID:   "aggregate-1",
				Payload:       json.RawMessage(`{`),
				CreatedAt:     now,
			},
			want: ErrPayloadRequired,
		},
		{
			name: "zero created at",
			input: NewEventInput{
				ID:            "event-1",
				EventType:     "TYPE",
				AggregateType: "AGGREGATE",
				AggregateID:   "aggregate-1",
				Payload:       json.RawMessage(`{}`),
			},
			want: ErrInvalidTimestamps,
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				_, err := NewEvent(test.input)

				if !errors.Is(err, test.want) {
					t.Fatalf(
						"NewEvent() error = %v, want %v",
						err,
						test.want,
					)
				}
			},
		)
	}
}

func TestRehydratePublishedEvent(t *testing.T) {
	now := time.Now().UTC()
	publishedAt := now.Add(time.Second)

	event, err := Rehydrate(
		RehydrateInput{
			ID:            "event-1",
			EventType:     "WAGER_TRANSACTION_PROCESSED",
			AggregateType: "WAGER_TRANSACTION",
			AggregateID:   "transaction-1",
			Payload:       json.RawMessage(`{"value":1}`),
			Status:        StatusPublished,
			Attempts:      1,
			AvailableAt:   now,
			PublishedAt:   &publishedAt,
			CreatedAt:     now,
			UpdatedAt:     publishedAt,
		},
	)
	if err != nil {
		t.Fatalf("Rehydrate() error = %v", err)
	}

	if event.Status() != StatusPublished {
		t.Fatalf(
			"unexpected status: %s",
			event.Status(),
		)
	}

	if event.PublishedAt() == nil {
		t.Fatal("publishedAt must not be nil")
	}
}

func TestRehydrateRejectsInvalidPublishedState(t *testing.T) {
	now := time.Now().UTC()

	_, err := Rehydrate(
		RehydrateInput{
			ID:            "event-1",
			EventType:     "TYPE",
			AggregateType: "AGGREGATE",
			AggregateID:   "aggregate-1",
			Payload:       json.RawMessage(`{}`),
			Status:        StatusPublished,
			AvailableAt:   now,
			CreatedAt:     now,
			UpdatedAt:     now,
		},
	)

	if !errors.Is(err, ErrInvalidPublishedState) {
		t.Fatalf(
			"Rehydrate() error = %v, want %v",
			err,
			ErrInvalidPublishedState,
		)
	}
}

func TestPayloadReturnsCopy(t *testing.T) {
	now := time.Now().UTC()

	event, err := NewEvent(
		NewEventInput{
			ID:            "event-1",
			EventType:     "TYPE",
			AggregateType: "AGGREGATE",
			AggregateID:   "aggregate-1",
			Payload:       json.RawMessage(`{"value":1}`),
			CreatedAt:     now,
		},
	)
	if err != nil {
		t.Fatalf("NewEvent() error = %v", err)
	}

	payload := event.Payload()
	payload[0] = '['

	if string(event.Payload()) != `{"value":1}` {
		t.Fatal("Payload() exposed internal state")
	}
}
