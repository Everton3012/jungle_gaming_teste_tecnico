package outbox_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	domainoutbox "jungle_gaming_teste_tecnico/internal/domain/outbox"
	postgresinfra "jungle_gaming_teste_tecnico/internal/infrastructure/postgres"
	outboxpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/outbox"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultTestDatabaseURL = "postgres://jungle:jungle@localhost:5432/jungle_gaming?sslmode=disable"
	integrationTestLockID  = int64(8675309)
)

func testDatabaseURL() string {
	if value := os.Getenv("TEST_DATABASE_URL"); value != "" {
		return value
	}

	return defaultTestDatabaseURL
}

func newTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	pool, err := postgresinfra.Open(
		ctx,
		postgresinfra.Config{
			URL:            testDatabaseURL(),
			MaxConnections: 5,
			MinConnections: 0,
			ConnectTimeout: 5 * time.Second,
			HealthTimeout:  3 * time.Second,
		},
	)
	if err != nil {
		t.Fatalf("open postgres: %v", err)
	}

	conn, err := pool.Acquire(context.Background())
	if err != nil {
		pool.Close()
		t.Fatalf(
			"acquire postgres test lock connection: %v",
			err,
		)
	}

	if _, err := conn.Exec(
		context.Background(),
		"SELECT pg_advisory_lock($1)",
		integrationTestLockID,
	); err != nil {
		conn.Release()
		pool.Close()

		t.Fatalf(
			"acquire postgres integration test lock: %v",
			err,
		)
	}

	t.Cleanup(func() {
		_, _ = conn.Exec(
			context.Background(),
			"SELECT pg_advisory_unlock($1)",
			integrationTestLockID,
		)

		conn.Release()
		pool.Close()
	})

	return pool
}

func cleanDatabase(
	t *testing.T,
	pool *pgxpool.Pool,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	_, err := pool.Exec(
		ctx,
		`
		TRUNCATE TABLE
			outbox_events,
			ledger_entries,
			wager_transactions,
			wallets
		RESTART IDENTITY CASCADE;
		`,
	)
	if err != nil {
		t.Fatalf("clean database: %v", err)
	}
}

func newEvent(
	t *testing.T,
	id string,
	availableAt time.Time,
) *domainoutbox.Event {
	t.Helper()

	event, err := domainoutbox.NewEvent(
		domainoutbox.NewEventInput{
			ID:            id,
			EventType:     "WAGER_TRANSACTION_PROCESSED",
			AggregateType: "WAGER_TRANSACTION",
			AggregateID:   "transaction-" + id,
			Payload: json.RawMessage(
				`{"transactionId":"transaction-` +
					id +
					`"}`,
			),
			CreatedAt: availableAt,
		},
	)
	if err != nil {
		t.Fatalf("create outbox event: %v", err)
	}

	return event
}

func TestRepositoryCreateAndFindByID(t *testing.T) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err := outboxpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Microsecond)

	event := newEvent(
		t,
		"outbox-create-find",
		now,
	)

	if err := repository.Create(ctx, event); err != nil {
		t.Fatalf("create outbox event: %v", err)
	}

	found, err := repository.FindByID(
		ctx,
		event.ID(),
	)
	if err != nil {
		t.Fatalf("find outbox event: %v", err)
	}

	if found.ID() != event.ID() {
		t.Fatalf(
			"unexpected id: got %q want %q",
			found.ID(),
			event.ID(),
		)
	}

	if found.EventType() != event.EventType() {
		t.Fatalf(
			"unexpected event type: got %q want %q",
			found.EventType(),
			event.EventType(),
		)
	}

	if found.AggregateType() != event.AggregateType() {
		t.Fatalf(
			"unexpected aggregate type: got %q want %q",
			found.AggregateType(),
			event.AggregateType(),
		)
	}

	if found.AggregateID() != event.AggregateID() {
		t.Fatalf(
			"unexpected aggregate id: got %q want %q",
			found.AggregateID(),
			event.AggregateID(),
		)
	}

	var gotPayload any
	if err := json.Unmarshal(found.Payload(), &gotPayload); err != nil {
		t.Fatalf("decode found payload: %v", err)
	}

	var wantPayload any
	if err := json.Unmarshal(event.Payload(), &wantPayload); err != nil {
		t.Fatalf("decode expected payload: %v", err)
	}

	if !reflect.DeepEqual(gotPayload, wantPayload) {
		t.Fatalf(
			"unexpected payload: got %s want %s",
			found.Payload(),
			event.Payload(),
		)
	}

	if found.Status() != domainoutbox.StatusPending {
		t.Fatalf(
			"unexpected status: got %q want %q",
			found.Status(),
			domainoutbox.StatusPending,
		)
	}

	if found.Attempts() != 0 {
		t.Fatalf(
			"unexpected attempts: got %d want 0",
			found.Attempts(),
		)
	}

	if !found.AvailableAt().Equal(event.AvailableAt()) {
		t.Fatalf(
			"unexpected available at: got %v want %v",
			found.AvailableAt(),
			event.AvailableAt(),
		)
	}
}

func TestRepositoryFindByIDReturnsNotFound(t *testing.T) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err := outboxpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	_, err = repository.FindByID(
		ctx,
		"does-not-exist",
	)

	if !errors.Is(
		err,
		outboxpostgres.ErrNotFound,
	) {
		t.Fatalf(
			"expected ErrNotFound, got %v",
			err,
		)
	}
}

func TestRepositoryClaimPending(t *testing.T) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err := outboxpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Microsecond)

	event := newEvent(
		t,
		"outbox-claim",
		now,
	)

	if err := repository.Create(ctx, event); err != nil {
		t.Fatalf("create outbox event: %v", err)
	}

	claimed, err := repository.ClaimPending(
		ctx,
		now.Add(time.Second),
		10,
	)
	if err != nil {
		t.Fatalf("claim pending events: %v", err)
	}

	if len(claimed) != 1 {
		t.Fatalf(
			"unexpected claimed count: got %d want 1",
			len(claimed),
		)
	}

	if claimed[0].ID() != event.ID() {
		t.Fatalf(
			"unexpected claimed id: got %q want %q",
			claimed[0].ID(),
			event.ID(),
		)
	}

	if claimed[0].Status() != domainoutbox.StatusProcessing {
		t.Fatalf(
			"unexpected status: got %q want %q",
			claimed[0].Status(),
			domainoutbox.StatusProcessing,
		)
	}

	if claimed[0].Attempts() != 1 {
		t.Fatalf(
			"unexpected attempts: got %d want 1",
			claimed[0].Attempts(),
		)
	}
}

func TestRepositoryClaimPendingIgnoresFutureEvent(
	t *testing.T,
) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err := outboxpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Microsecond)

	event := newEvent(
		t,
		"outbox-future",
		now.Add(time.Hour),
	)

	if err := repository.Create(ctx, event); err != nil {
		t.Fatalf("create future event: %v", err)
	}

	claimed, err := repository.ClaimPending(
		ctx,
		now,
		10,
	)
	if err != nil {
		t.Fatalf("claim pending events: %v", err)
	}

	if len(claimed) != 0 {
		t.Fatalf(
			"unexpected claimed count: got %d want 0",
			len(claimed),
		)
	}
}

func TestRepositoryClaimPendingRespectsLimit(t *testing.T) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err := outboxpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Microsecond)

	for i, id := range []string{
		"outbox-limit-1",
		"outbox-limit-2",
		"outbox-limit-3",
	} {
		event := newEvent(
			t,
			id,
			now.Add(time.Duration(i)*time.Microsecond),
		)

		if err := repository.Create(ctx, event); err != nil {
			t.Fatalf(
				"create event %s: %v",
				id,
				err,
			)
		}
	}

	claimed, err := repository.ClaimPending(
		ctx,
		now.Add(time.Second),
		2,
	)
	if err != nil {
		t.Fatalf("claim pending events: %v", err)
	}

	if len(claimed) != 2 {
		t.Fatalf(
			"unexpected claimed count: got %d want 2",
			len(claimed),
		)
	}
}

func TestRepositoryMarkPublished(t *testing.T) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err := outboxpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Microsecond)

	event := newEvent(
		t,
		"outbox-published",
		now,
	)

	if err := repository.Create(ctx, event); err != nil {
		t.Fatalf("create event: %v", err)
	}

	claimed, err := repository.ClaimPending(
		ctx,
		now.Add(time.Second),
		1,
	)
	if err != nil {
		t.Fatalf("claim event: %v", err)
	}

	if len(claimed) != 1 {
		t.Fatalf(
			"unexpected claimed count: %d",
			len(claimed),
		)
	}

	publishedAt := now.Add(2 * time.Second)

	if err := repository.MarkPublished(
		ctx,
		event.ID(),
		publishedAt,
	); err != nil {
		t.Fatalf("mark published: %v", err)
	}

	found, err := repository.FindByID(
		ctx,
		event.ID(),
	)
	if err != nil {
		t.Fatalf("find published event: %v", err)
	}

	if found.Status() != domainoutbox.StatusPublished {
		t.Fatalf(
			"unexpected status: got %q want %q",
			found.Status(),
			domainoutbox.StatusPublished,
		)
	}

	if found.PublishedAt() == nil {
		t.Fatal("published at must not be nil")
	}

	if !found.PublishedAt().Equal(publishedAt) {
		t.Fatalf(
			"unexpected published at: got %v want %v",
			*found.PublishedAt(),
			publishedAt,
		)
	}
}

func TestRepositoryReschedule(t *testing.T) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err := outboxpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Microsecond)

	event := newEvent(
		t,
		"outbox-reschedule",
		now,
	)

	if err := repository.Create(ctx, event); err != nil {
		t.Fatalf("create event: %v", err)
	}

	claimed, err := repository.ClaimPending(
		ctx,
		now.Add(time.Second),
		1,
	)
	if err != nil {
		t.Fatalf("claim event: %v", err)
	}

	if len(claimed) != 1 {
		t.Fatalf(
			"unexpected claimed count: %d",
			len(claimed),
		)
	}

	availableAt := now.Add(time.Minute)
	updatedAt := now.Add(2 * time.Second)

	if err := repository.Reschedule(
		ctx,
		event.ID(),
		availableAt,
		"publisher unavailable",
		updatedAt,
	); err != nil {
		t.Fatalf("reschedule event: %v", err)
	}

	found, err := repository.FindByID(
		ctx,
		event.ID(),
	)
	if err != nil {
		t.Fatalf("find rescheduled event: %v", err)
	}

	if found.Status() != domainoutbox.StatusPending {
		t.Fatalf(
			"unexpected status: got %q want %q",
			found.Status(),
			domainoutbox.StatusPending,
		)
	}

	if found.Attempts() != 1 {
		t.Fatalf(
			"unexpected attempts: got %d want 1",
			found.Attempts(),
		)
	}

	if !found.AvailableAt().Equal(availableAt) {
		t.Fatalf(
			"unexpected available at: got %v want %v",
			found.AvailableAt(),
			availableAt,
		)
	}

	if found.LastError() == nil {
		t.Fatal("last error must not be nil")
	}

	if *found.LastError() != "publisher unavailable" {
		t.Fatalf(
			"unexpected last error: %q",
			*found.LastError(),
		)
	}
}

func TestRepositoryConcurrentClaimsDoNotDuplicateEvents(
	t *testing.T,
) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err := outboxpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Microsecond)

	const eventCount = 20

	for i := 0; i < eventCount; i++ {
		id := "outbox-concurrent-" +
			time.Unix(0, int64(i+1)).
				UTC().
				Format("150405.000000000")

		event := newEvent(
			t,
			id,
			now.Add(time.Duration(i)*time.Microsecond),
		)

		if err := repository.Create(ctx, event); err != nil {
			t.Fatalf(
				"create event %d: %v",
				i,
				err,
			)
		}
	}

	start := make(chan struct{})

	type claimResult struct {
		events []*domainoutbox.Event
		err    error
	}

	results := make(
		chan claimResult,
		2,
	)

	var waitGroup sync.WaitGroup
	waitGroup.Add(2)

	claim := func() {
		defer waitGroup.Done()

		<-start

		events, err := repository.ClaimPending(
			ctx,
			now.Add(time.Second),
			eventCount/2,
		)

		results <- claimResult{
			events: events,
			err:    err,
		}
	}

	go claim()
	go claim()

	close(start)

	waitGroup.Wait()
	close(results)

	claimedIDs := make(map[string]struct{})

	total := 0

	for result := range results {
		if result.err != nil {
			t.Fatalf(
				"concurrent claim failed: %v",
				result.err,
			)
		}

		total += len(result.events)

		for _, event := range result.events {
			if _, exists := claimedIDs[event.ID()]; exists {
				t.Fatalf(
					"event %q was claimed more than once",
					event.ID(),
				)
			}

			claimedIDs[event.ID()] = struct{}{}
		}
	}

	if total != eventCount {
		t.Fatalf(
			"unexpected total claimed events: got %d want %d",
			total,
			eventCount,
		)
	}

	if len(claimedIDs) != eventCount {
		t.Fatalf(
			"unexpected unique claimed events: got %d want %d",
			len(claimedIDs),
			eventCount,
		)
	}
}

func TestRepositoryRejectsInvalidOperations(t *testing.T) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err := outboxpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Microsecond)

	if err := repository.Create(ctx, nil); !errors.Is(
		err,
		outboxpostgres.ErrEventRequired,
	) {
		t.Fatalf(
			"expected ErrEventRequired, got %v",
			err,
		)
	}

	if _, err := repository.ClaimPending(
		ctx,
		now,
		0,
	); !errors.Is(
		err,
		outboxpostgres.ErrInvalidLimit,
	) {
		t.Fatalf(
			"expected ErrInvalidLimit, got %v",
			err,
		)
	}

	if _, err := repository.ClaimPending(
		ctx,
		time.Time{},
		1,
	); !errors.Is(
		err,
		outboxpostgres.ErrInvalidTime,
	) {
		t.Fatalf(
			"expected ErrInvalidTime, got %v",
			err,
		)
	}

	if err := repository.MarkPublished(
		ctx,
		"does-not-exist",
		now,
	); !errors.Is(
		err,
		outboxpostgres.ErrInvalidState,
	) {
		t.Fatalf(
			"expected ErrInvalidState, got %v",
			err,
		)
	}
}
