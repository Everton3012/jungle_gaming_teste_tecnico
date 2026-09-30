package postgres_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	postgresinfra "jungle_gaming_teste_tecnico/internal/infrastructure/postgres"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const defaultTestDatabaseURL = "postgres://jungle:jungle@localhost:5432/jungle_gaming?sslmode=disable"

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

	t.Cleanup(pool.Close)

	return pool
}

func TestTransactionManagerCommit(t *testing.T) {
	pool := newTestPool(t)

	manager, err := postgresinfra.NewTransactionManager(pool)
	if err != nil {
		t.Fatalf("create transaction manager: %v", err)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	const lockID int64 = 991001

	err = manager.WithinTransaction(
		ctx,
		func(ctx context.Context, tx pgx.Tx) error {
			_, err := tx.Exec(
				ctx,
				"SELECT pg_advisory_xact_lock($1)",
				lockID,
			)

			return err
		},
	)
	if err != nil {
		t.Fatalf("commit transaction: %v", err)
	}
}

func TestTransactionManagerRollback(t *testing.T) {
	pool := newTestPool(t)

	manager, err := postgresinfra.NewTransactionManager(pool)
	if err != nil {
		t.Fatalf("create transaction manager: %v", err)
	}

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	expected := errors.New("forced failure")

	err = manager.WithinTransaction(
		ctx,
		func(
			ctx context.Context,
			tx pgx.Tx,
		) error {
			return expected
		},
	)

	if !errors.Is(err, expected) {
		t.Fatalf(
			"expected forced failure, got %v",
			err,
		)
	}
}

func TestTransactionManagerRejectsNilFunction(t *testing.T) {
	pool := newTestPool(t)

	manager, err := postgresinfra.NewTransactionManager(pool)
	if err != nil {
		t.Fatalf("create transaction manager: %v", err)
	}

	err = manager.WithinTransaction(
		context.Background(),
		nil,
	)

	if !errors.Is(
		err,
		postgresinfra.ErrTransactionRequired,
	) {
		t.Fatalf(
			"expected ErrTransactionRequired, got %v",
			err,
		)
	}
}
