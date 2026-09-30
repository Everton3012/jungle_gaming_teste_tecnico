package wallet_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	domainmoney "jungle_gaming_teste_tecnico/internal/domain/money"
	domainwallet "jungle_gaming_teste_tecnico/internal/domain/wallet"
	postgresinfra "jungle_gaming_teste_tecnico/internal/infrastructure/postgres"
	walletpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/wallet"

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

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
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
		t.Fatalf("acquire postgres test lock connection: %v", err)
	}

	if _, err := conn.Exec(
		context.Background(),
		"SELECT pg_advisory_lock($1)",
		integrationTestLockID,
	); err != nil {
		conn.Release()
		pool.Close()
		t.Fatalf("acquire postgres integration test lock: %v", err)
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

func cleanDatabase(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := pool.Exec(
		ctx,
		`
		TRUNCATE TABLE
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

func newTestWallet(
	t *testing.T,
	id string,
	playerID string,
	amount int64,
	currency string,
	now time.Time,
) *domainwallet.Wallet {
	t.Helper()

	balance, err := domainmoney.New(amount, currency)
	if err != nil {
		t.Fatalf("create money: %v", err)
	}

	entity, err := domainwallet.New(
		id,
		playerID,
		balance,
		now,
	)
	if err != nil {
		t.Fatalf("create wallet: %v", err)
	}

	return &entity
}

func TestRepositoryCreateAndFindByID(t *testing.T) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err := walletpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Microsecond)

	original := newTestWallet(
		t,
		"wallet-create-find",
		"player-create-find",
		10000,
		"BRL",
		now,
	)

	if err := repository.Create(ctx, original); err != nil {
		t.Fatalf("create wallet: %v", err)
	}

	found, err := repository.FindByID(ctx, original.ID())
	if err != nil {
		t.Fatalf("find wallet by id: %v", err)
	}

	if found.ID() != original.ID() {
		t.Fatalf(
			"unexpected wallet id: got %q want %q",
			found.ID(),
			original.ID(),
		)
	}

	if found.PlayerID() != original.PlayerID() {
		t.Fatalf(
			"unexpected player id: got %q want %q",
			found.PlayerID(),
			original.PlayerID(),
		)
	}

	if found.Currency() != original.Currency() {
		t.Fatalf(
			"unexpected currency: got %q want %q",
			found.Currency(),
			original.Currency(),
		)
	}

	if found.Balance().Amount() != original.Balance().Amount() {
		t.Fatalf(
			"unexpected balance: got %d want %d",
			found.Balance().Amount(),
			original.Balance().Amount(),
		)
	}

	if found.Version() != original.Version() {
		t.Fatalf(
			"unexpected version: got %d want %d",
			found.Version(),
			original.Version(),
		)
	}

	if !found.CreatedAt().Equal(original.CreatedAt()) {
		t.Fatalf(
			"unexpected created at: got %v want %v",
			found.CreatedAt(),
			original.CreatedAt(),
		)
	}

	if !found.UpdatedAt().Equal(original.UpdatedAt()) {
		t.Fatalf(
			"unexpected updated at: got %v want %v",
			found.UpdatedAt(),
			original.UpdatedAt(),
		)
	}
}

func TestRepositoryFindByPlayerCurrency(t *testing.T) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err := walletpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Microsecond)

	original := newTestWallet(
		t,
		"wallet-player-currency",
		"player-player-currency",
		25000,
		"BRL",
		now,
	)

	if err := repository.Create(ctx, original); err != nil {
		t.Fatalf("create wallet: %v", err)
	}

	found, err := repository.FindByPlayerCurrency(
		ctx,
		original.PlayerID(),
		original.Currency(),
	)
	if err != nil {
		t.Fatalf("find wallet by player and currency: %v", err)
	}

	if found.ID() != original.ID() {
		t.Fatalf(
			"unexpected wallet id: got %q want %q",
			found.ID(),
			original.ID(),
		)
	}

	if found.PlayerID() != original.PlayerID() {
		t.Fatalf(
			"unexpected player id: got %q want %q",
			found.PlayerID(),
			original.PlayerID(),
		)
	}

	if found.Currency() != original.Currency() {
		t.Fatalf(
			"unexpected currency: got %q want %q",
			found.Currency(),
			original.Currency(),
		)
	}

	if found.Balance().Amount() != original.Balance().Amount() {
		t.Fatalf(
			"unexpected balance: got %d want %d",
			found.Balance().Amount(),
			original.Balance().Amount(),
		)
	}
}

func TestRepositoryFindByIDReturnsNotFound(t *testing.T) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err := walletpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = repository.FindByID(ctx, "wallet-does-not-exist")
	if !errors.Is(err, walletpostgres.ErrNotFound) {
		t.Fatalf(
			"expected ErrNotFound, got %v",
			err,
		)
	}
}

func TestRepositoryFindByPlayerCurrencyReturnsNotFound(t *testing.T) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err := walletpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = repository.FindByPlayerCurrency(
		ctx,
		"player-does-not-exist",
		"BRL",
	)
	if !errors.Is(err, walletpostgres.ErrNotFound) {
		t.Fatalf(
			"expected ErrNotFound, got %v",
			err,
		)
	}
}

func TestRepositoryUpdate(t *testing.T) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err := walletpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Microsecond)

	entity := newTestWallet(
		t,
		"wallet-update",
		"player-update",
		10000,
		"BRL",
		now,
	)

	if err := repository.Create(ctx, entity); err != nil {
		t.Fatalf("create wallet: %v", err)
	}

	expectedVersion := entity.Version()

	credit, err := domainmoney.New(5000, "BRL")
	if err != nil {
		t.Fatalf("create credit money: %v", err)
	}

	updatedAt := now.Add(time.Second)

	if err := entity.Credit(credit, updatedAt); err != nil {
		t.Fatalf("credit wallet: %v", err)
	}

	if err := repository.Update(
		ctx,
		entity,
		expectedVersion,
	); err != nil {
		t.Fatalf("update wallet: %v", err)
	}

	found, err := repository.FindByID(ctx, entity.ID())
	if err != nil {
		t.Fatalf("find updated wallet: %v", err)
	}

	if found.Balance().Amount() != 15000 {
		t.Fatalf(
			"unexpected balance: got %d want %d",
			found.Balance().Amount(),
			15000,
		)
	}

	if found.Version() != expectedVersion+1 {
		t.Fatalf(
			"unexpected version: got %d want %d",
			found.Version(),
			expectedVersion+1,
		)
	}

	if !found.UpdatedAt().Equal(updatedAt) {
		t.Fatalf(
			"unexpected updated at: got %v want %v",
			found.UpdatedAt(),
			updatedAt,
		)
	}
}

func TestRepositoryUpdateRejectsStaleVersion(t *testing.T) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err := walletpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Microsecond)

	entity := newTestWallet(
		t,
		"wallet-concurrent-update",
		"player-concurrent-update",
		10000,
		"BRL",
		now,
	)

	if err := repository.Create(ctx, entity); err != nil {
		t.Fatalf("create wallet: %v", err)
	}

	originalVersion := entity.Version()

	firstCredit, err := domainmoney.New(1000, "BRL")
	if err != nil {
		t.Fatalf("create first credit: %v", err)
	}

	if err := entity.Credit(
		firstCredit,
		now.Add(time.Second),
	); err != nil {
		t.Fatalf("credit wallet: %v", err)
	}

	if err := repository.Update(
		ctx,
		entity,
		originalVersion,
	); err != nil {
		t.Fatalf("first update: %v", err)
	}

	secondCredit, err := domainmoney.New(1000, "BRL")
	if err != nil {
		t.Fatalf("create second credit: %v", err)
	}

	if err := entity.Credit(
		secondCredit,
		now.Add(2*time.Second),
	); err != nil {
		t.Fatalf("second credit: %v", err)
	}

	err = repository.Update(
		ctx,
		entity,
		originalVersion,
	)
	if !errors.Is(err, walletpostgres.ErrConcurrentUpdate) {
		t.Fatalf(
			"expected ErrConcurrentUpdate, got %v",
			err,
		)
	}

	persisted, err := repository.FindByID(ctx, entity.ID())
	if err != nil {
		t.Fatalf("find persisted wallet: %v", err)
	}

	if persisted.Balance().Amount() != 11000 {
		t.Fatalf(
			"stale update changed persisted balance: got %d want %d",
			persisted.Balance().Amount(),
			11000,
		)
	}

	if persisted.Version() != originalVersion+1 {
		t.Fatalf(
			"stale update changed persisted version: got %d want %d",
			persisted.Version(),
			originalVersion+1,
		)
	}
}
