package inbox_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	postgresinfra "jungle_gaming_teste_tecnico/internal/infrastructure/postgres"
	inboxpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/inbox"

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
		TRUNCATE TABLE inbox_messages
		RESTART IDENTITY CASCADE;
		`,
	)
	if err != nil {
		t.Fatalf(
			"clean inbox database: %v",
			err,
		)
	}
}

func TestRepositoryBeginCreatesMessage(
	t *testing.T,
) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err :=
		inboxpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf(
			"create inbox repository: %v",
			err,
		)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	now :=
		time.Now().
			UTC().
			Truncate(time.Microsecond)

	result, err := repository.Begin(
		ctx,
		"message-begin-create",
		"hash-begin-create",
		now,
	)
	if err != nil {
		t.Fatalf(
			"begin inbox message: %v",
			err,
		)
	}

	if result == nil {
		t.Fatal("begin result is nil")
	}

	if result.Message == nil {
		t.Fatal("begin message is nil")
	}

	if result.AlreadyProcessed {
		t.Fatal(
			"new inbox message unexpectedly already processed",
		)
	}

	message := result.Message

	if message.MessageID != "message-begin-create" {
		t.Fatalf(
			"message ID = %q, want %q",
			message.MessageID,
			"message-begin-create",
		)
	}

	if message.PayloadHash != "hash-begin-create" {
		t.Fatalf(
			"payload hash = %q, want %q",
			message.PayloadHash,
			"hash-begin-create",
		)
	}

	if message.Status != inboxpostgres.StatusProcessing {
		t.Fatalf(
			"status = %q, want %q",
			message.Status,
			inboxpostgres.StatusProcessing,
		)
	}

	if message.Attempts != 1 {
		t.Fatalf(
			"attempts = %d, want 1",
			message.Attempts,
		)
	}

	if message.LastError != nil {
		t.Fatalf(
			"last error = %q, want nil",
			*message.LastError,
		)
	}

	if message.ProcessedAt != nil {
		t.Fatalf(
			"processed at = %v, want nil",
			message.ProcessedAt,
		)
	}

	found, err := repository.FindByID(
		ctx,
		"message-begin-create",
	)
	if err != nil {
		t.Fatalf(
			"find created inbox message: %v",
			err,
		)
	}

	if found.Status != inboxpostgres.StatusProcessing {
		t.Fatalf(
			"persisted status = %q, want %q",
			found.Status,
			inboxpostgres.StatusProcessing,
		)
	}

	if found.Attempts != 1 {
		t.Fatalf(
			"persisted attempts = %d, want 1",
			found.Attempts,
		)
	}
}

func TestRepositoryMarkProcessed(
	t *testing.T,
) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err :=
		inboxpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf(
			"create inbox repository: %v",
			err,
		)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	now :=
		time.Now().
			UTC().
			Truncate(time.Microsecond)

	_, err = repository.Begin(
		ctx,
		"message-processed",
		"hash-processed",
		now,
	)
	if err != nil {
		t.Fatalf(
			"begin inbox message: %v",
			err,
		)
	}

	processedAt :=
		now.Add(time.Second)

	if err := repository.MarkProcessed(
		ctx,
		"message-processed",
		processedAt,
	); err != nil {
		t.Fatalf(
			"mark inbox message processed: %v",
			err,
		)
	}

	found, err := repository.FindByID(
		ctx,
		"message-processed",
	)
	if err != nil {
		t.Fatalf(
			"find processed inbox message: %v",
			err,
		)
	}

	if found.Status != inboxpostgres.StatusProcessed {
		t.Fatalf(
			"status = %q, want %q",
			found.Status,
			inboxpostgres.StatusProcessed,
		)
	}

	if found.ProcessedAt == nil {
		t.Fatal(
			"processed at is nil",
		)
	}

	if found.LastError != nil {
		t.Fatalf(
			"last error = %q, want nil",
			*found.LastError,
		)
	}

	if found.Attempts != 1 {
		t.Fatalf(
			"attempts = %d, want 1",
			found.Attempts,
		)
	}
}

func TestRepositoryProcessedMessageIsIdempotent(
	t *testing.T,
) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err :=
		inboxpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf(
			"create inbox repository: %v",
			err,
		)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	now :=
		time.Now().
			UTC().
			Truncate(time.Microsecond)

	const (
		messageID   = "message-idempotent"
		payloadHash = "hash-idempotent"
	)

	first, err := repository.Begin(
		ctx,
		messageID,
		payloadHash,
		now,
	)
	if err != nil {
		t.Fatalf(
			"begin inbox message: %v",
			err,
		)
	}

	if first.AlreadyProcessed {
		t.Fatal(
			"first processing unexpectedly already processed",
		)
	}

	if err := repository.MarkProcessed(
		ctx,
		messageID,
		now.Add(time.Second),
	); err != nil {
		t.Fatalf(
			"mark inbox message processed: %v",
			err,
		)
	}

	second, err := repository.Begin(
		ctx,
		messageID,
		payloadHash,
		now.Add(2*time.Second),
	)
	if err != nil {
		t.Fatalf(
			"begin processed inbox message again: %v",
			err,
		)
	}

	if !second.AlreadyProcessed {
		t.Fatal(
			"processed inbox message should be already processed",
		)
	}

	if second.Message.Status !=
		inboxpostgres.StatusProcessed {
		t.Fatalf(
			"status = %q, want %q",
			second.Message.Status,
			inboxpostgres.StatusProcessed,
		)
	}

	if second.Message.Attempts != 1 {
		t.Fatalf(
			"attempts = %d, want 1",
			second.Message.Attempts,
		)
	}
}

func TestRepositoryFailedMessageCanBeRetried(
	t *testing.T,
) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err :=
		inboxpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf(
			"create inbox repository: %v",
			err,
		)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	now :=
		time.Now().
			UTC().
			Truncate(time.Microsecond)

	const (
		messageID   = "message-retry"
		payloadHash = "hash-retry"
	)

	_, err = repository.Begin(
		ctx,
		messageID,
		payloadHash,
		now,
	)
	if err != nil {
		t.Fatalf(
			"begin inbox message: %v",
			err,
		)
	}

	if err := repository.MarkFailed(
		ctx,
		messageID,
		"temporary processing failure",
		now.Add(time.Second),
	); err != nil {
		t.Fatalf(
			"mark inbox message failed: %v",
			err,
		)
	}

	failed, err := repository.FindByID(
		ctx,
		messageID,
	)
	if err != nil {
		t.Fatalf(
			"find failed inbox message: %v",
			err,
		)
	}

	if failed.Status != inboxpostgres.StatusFailed {
		t.Fatalf(
			"failed status = %q, want %q",
			failed.Status,
			inboxpostgres.StatusFailed,
		)
	}

	if failed.LastError == nil {
		t.Fatal(
			"failed message last error is nil",
		)
	}

	if *failed.LastError !=
		"temporary processing failure" {
		t.Fatalf(
			"last error = %q, want %q",
			*failed.LastError,
			"temporary processing failure",
		)
	}

	retry, err := repository.Begin(
		ctx,
		messageID,
		payloadHash,
		now.Add(2*time.Second),
	)
	if err != nil {
		t.Fatalf(
			"retry inbox message: %v",
			err,
		)
	}

	if retry.AlreadyProcessed {
		t.Fatal(
			"failed message unexpectedly already processed",
		)
	}

	if retry.Message.Status !=
		inboxpostgres.StatusProcessing {
		t.Fatalf(
			"retry status = %q, want %q",
			retry.Message.Status,
			inboxpostgres.StatusProcessing,
		)
	}

	if retry.Message.Attempts != 2 {
		t.Fatalf(
			"retry attempts = %d, want 2",
			retry.Message.Attempts,
		)
	}

	if retry.Message.LastError != nil {
		t.Fatalf(
			"retry last error = %q, want nil",
			*retry.Message.LastError,
		)
	}

	if retry.Message.ProcessedAt != nil {
		t.Fatalf(
			"retry processed at = %v, want nil",
			retry.Message.ProcessedAt,
		)
	}
}

func TestRepositoryDetectsPayloadConflict(
	t *testing.T,
) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err :=
		inboxpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf(
			"create inbox repository: %v",
			err,
		)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	now :=
		time.Now().
			UTC().
			Truncate(time.Microsecond)

	const messageID = "message-conflict"

	_, err = repository.Begin(
		ctx,
		messageID,
		"original-payload-hash",
		now,
	)
	if err != nil {
		t.Fatalf(
			"begin inbox message: %v",
			err,
		)
	}

	_, err = repository.Begin(
		ctx,
		messageID,
		"different-payload-hash",
		now.Add(time.Second),
	)

	if !errors.Is(
		err,
		inboxpostgres.ErrPayloadConflict,
	) {
		t.Fatalf(
			"expected ErrPayloadConflict, got %v",
			err,
		)
	}
}

func TestRepositoryMarkFailedDoesNotOverrideProcessed(
	t *testing.T,
) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err :=
		inboxpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf(
			"create inbox repository: %v",
			err,
		)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	now :=
		time.Now().
			UTC().
			Truncate(time.Microsecond)

	const messageID = "message-processed-cannot-fail"

	_, err = repository.Begin(
		ctx,
		messageID,
		"hash-processed-cannot-fail",
		now,
	)
	if err != nil {
		t.Fatalf(
			"begin inbox message: %v",
			err,
		)
	}

	if err := repository.MarkProcessed(
		ctx,
		messageID,
		now.Add(time.Second),
	); err != nil {
		t.Fatalf(
			"mark inbox message processed: %v",
			err,
		)
	}

	if err := repository.MarkFailed(
		ctx,
		messageID,
		"late failure",
		now.Add(2*time.Second),
	); err != nil {
		t.Fatalf(
			"mark processed message failed: %v",
			err,
		)
	}

	found, err := repository.FindByID(
		ctx,
		messageID,
	)
	if err != nil {
		t.Fatalf(
			"find inbox message: %v",
			err,
		)
	}

	if found.Status !=
		inboxpostgres.StatusProcessed {
		t.Fatalf(
			"status = %q, want %q",
			found.Status,
			inboxpostgres.StatusProcessed,
		)
	}

	if found.LastError != nil {
		t.Fatalf(
			"last error = %q, want nil",
			*found.LastError,
		)
	}
}

func TestRepositoryFindByIDReturnsNotFound(
	t *testing.T,
) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err :=
		inboxpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf(
			"create inbox repository: %v",
			err,
		)
	}

	_, err = repository.FindByID(
		context.Background(),
		"message-does-not-exist",
	)

	if !errors.Is(
		err,
		inboxpostgres.ErrNotFound,
	) {
		t.Fatalf(
			"expected ErrNotFound, got %v",
			err,
		)
	}
}

func TestRepositoryValidatesInput(
	t *testing.T,
) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err :=
		inboxpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf(
			"create inbox repository: %v",
			err,
		)
	}

	ctx := context.Background()
	now := time.Now().UTC()

	tests := []struct {
		name string
		run  func() error
		want error
	}{
		{
			name: "begin empty message ID",
			run: func() error {
				_, err := repository.Begin(
					ctx,
					" ",
					"hash",
					now,
				)

				return err
			},
			want: inboxpostgres.ErrMessageIDRequired,
		},
		{
			name: "begin empty payload hash",
			run: func() error {
				_, err := repository.Begin(
					ctx,
					"message",
					" ",
					now,
				)

				return err
			},
			want: inboxpostgres.ErrPayloadHashRequired,
		},
		{
			name: "begin zero time",
			run: func() error {
				_, err := repository.Begin(
					ctx,
					"message",
					"hash",
					time.Time{},
				)

				return err
			},
			want: inboxpostgres.ErrInvalidTime,
		},
		{
			name: "find empty message ID",
			run: func() error {
				_, err := repository.FindByID(
					ctx,
					" ",
				)

				return err
			},
			want: inboxpostgres.ErrMessageIDRequired,
		},
		{
			name: "mark processed empty message ID",
			run: func() error {
				return repository.MarkProcessed(
					ctx,
					"",
					now,
				)
			},
			want: inboxpostgres.ErrMessageIDRequired,
		},
		{
			name: "mark processed zero time",
			run: func() error {
				return repository.MarkProcessed(
					ctx,
					"message",
					time.Time{},
				)
			},
			want: inboxpostgres.ErrInvalidTime,
		},
		{
			name: "mark failed empty message ID",
			run: func() error {
				return repository.MarkFailed(
					ctx,
					"",
					"failure",
					now,
				)
			},
			want: inboxpostgres.ErrMessageIDRequired,
		},
		{
			name: "mark failed empty error",
			run: func() error {
				return repository.MarkFailed(
					ctx,
					"message",
					" ",
					now,
				)
			},
			want: inboxpostgres.ErrLastErrorRequired,
		},
		{
			name: "mark failed zero time",
			run: func() error {
				return repository.MarkFailed(
					ctx,
					"message",
					"failure",
					time.Time{},
				)
			},
			want: inboxpostgres.ErrInvalidTime,
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				err := test.run()

				if !errors.Is(
					err,
					test.want,
				) {
					t.Fatalf(
						"expected %v, got %v",
						test.want,
						err,
					)
				}
			},
		)
	}
}

func TestNewRepositoryRejectsNilDatabase(
	t *testing.T,
) {
	_, err := inboxpostgres.NewRepository(nil)

	if !errors.Is(
		err,
		inboxpostgres.ErrRepositoryRequired,
	) {
		t.Fatalf(
			"expected ErrRepositoryRequired, got %v",
			err,
		)
	}
}
