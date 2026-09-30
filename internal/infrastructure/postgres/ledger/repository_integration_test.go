package ledger_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	domainledger "jungle_gaming_teste_tecnico/internal/domain/ledger"
	domainmoney "jungle_gaming_teste_tecnico/internal/domain/money"
	postgresinfra "jungle_gaming_teste_tecnico/internal/infrastructure/postgres"
	ledgerpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/ledger"

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

func insertWallet(
	t *testing.T,
	pool *pgxpool.Pool,
	id string,
	playerID string,
	balance int64,
	currency string,
	now time.Time,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := pool.Exec(
		ctx,
		`
		INSERT INTO wallets (
			id,
			player_id,
			currency,
			balance,
			version,
			created_at,
			updated_at
		)
		VALUES ($1, $2, $3, $4, 1, $5, $5)
		`,
		id,
		playerID,
		currency,
		balance,
		now,
	)
	if err != nil {
		t.Fatalf("insert wallet: %v", err)
	}
}

func insertTransaction(
	t *testing.T,
	pool *pgxpool.Pool,
	id string,
	walletID string,
	playerID string,
	externalTransactionID string,
	idempotencyKey string,
	now time.Time,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := pool.Exec(
		ctx,
		`
		INSERT INTO wager_transactions (
			id,
			external_transaction_id,
			provider_id,
			idempotency_key,
			payload_hash,
			wallet_id,
			player_id,
			round_id,
			game_id,
			kind,
			status,
			amount,
			currency,
			result_balance,
			created_at,
			updated_at
		)
		VALUES (
			$1,
			$2,
			'provider-test',
			$3,
			$4,
			$5,
			$6,
			'round-test',
			'game-test',
			'BET',
			'PROCESSED',
			1000,
			'BRL',
			9000,
			$7,
			$7
		)
		`,
		id,
		externalTransactionID,
		idempotencyKey,
		"payload-hash-"+id,
		walletID,
		playerID,
		now,
	)
	if err != nil {
		t.Fatalf("insert wager transaction: %v", err)
	}
}

func newMoney(
	t *testing.T,
	amount int64,
	currency string,
) domainmoney.Money {
	t.Helper()

	value, err := domainmoney.New(amount, currency)
	if err != nil {
		t.Fatalf("create money: %v", err)
	}

	return value
}

func newLedgerEntry(
	t *testing.T,
	id string,
	walletID string,
	transactionID string,
	direction domainledger.Direction,
	amount int64,
	balanceBefore int64,
	balanceAfter int64,
	now time.Time,
) *domainledger.Entry {
	t.Helper()

	entry, err := domainledger.New(
		id,
		walletID,
		transactionID,
		direction,
		newMoney(t, amount, "BRL"),
		newMoney(t, balanceBefore, "BRL"),
		newMoney(t, balanceAfter, "BRL"),
		now,
	)
	if err != nil {
		t.Fatalf("create ledger entry: %v", err)
	}

	return &entry
}

func assertLedgerEntry(
	t *testing.T,
	got *domainledger.Entry,
	want *domainledger.Entry,
) {
	t.Helper()

	if got.ID() != want.ID() {
		t.Fatalf(
			"unexpected id: got %q want %q",
			got.ID(),
			want.ID(),
		)
	}

	if got.WalletID() != want.WalletID() {
		t.Fatalf(
			"unexpected wallet id: got %q want %q",
			got.WalletID(),
			want.WalletID(),
		)
	}

	if got.TransactionID() != want.TransactionID() {
		t.Fatalf(
			"unexpected transaction id: got %q want %q",
			got.TransactionID(),
			want.TransactionID(),
		)
	}

	if got.Direction() != want.Direction() {
		t.Fatalf(
			"unexpected direction: got %q want %q",
			got.Direction(),
			want.Direction(),
		)
	}

	if got.Amount().Amount() != want.Amount().Amount() {
		t.Fatalf(
			"unexpected amount: got %d want %d",
			got.Amount().Amount(),
			want.Amount().Amount(),
		)
	}

	if got.Amount().Currency() != want.Amount().Currency() {
		t.Fatalf(
			"unexpected currency: got %q want %q",
			got.Amount().Currency(),
			want.Amount().Currency(),
		)
	}

	if got.BalanceBefore().Amount() != want.BalanceBefore().Amount() {
		t.Fatalf(
			"unexpected balance before: got %d want %d",
			got.BalanceBefore().Amount(),
			want.BalanceBefore().Amount(),
		)
	}

	if got.BalanceAfter().Amount() != want.BalanceAfter().Amount() {
		t.Fatalf(
			"unexpected balance after: got %d want %d",
			got.BalanceAfter().Amount(),
			want.BalanceAfter().Amount(),
		)
	}

	if !got.CreatedAt().Equal(want.CreatedAt()) {
		t.Fatalf(
			"unexpected created at: got %v want %v",
			got.CreatedAt(),
			want.CreatedAt(),
		)
	}
}

func prepareLedgerDependencies(
	t *testing.T,
	pool *pgxpool.Pool,
	walletID string,
	playerID string,
	transactionID string,
	now time.Time,
) {
	t.Helper()

	insertWallet(
		t,
		pool,
		walletID,
		playerID,
		10000,
		"BRL",
		now,
	)

	insertTransaction(
		t,
		pool,
		transactionID,
		walletID,
		playerID,
		"external-"+transactionID,
		"idempotency-"+transactionID,
		now,
	)
}

func TestRepositoryCreateAndFindByID(t *testing.T) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err := ledgerpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Microsecond)

	prepareLedgerDependencies(
		t,
		pool,
		"wallet-ledger-create",
		"player-ledger-create",
		"transaction-ledger-create",
		now,
	)

	entry := newLedgerEntry(
		t,
		"ledger-create",
		"wallet-ledger-create",
		"transaction-ledger-create",
		domainledger.DirectionDebit,
		1000,
		10000,
		9000,
		now,
	)

	if err := repository.Create(ctx, entry); err != nil {
		t.Fatalf("create ledger entry: %v", err)
	}

	found, err := repository.FindByID(ctx, entry.ID())
	if err != nil {
		t.Fatalf("find ledger entry: %v", err)
	}

	assertLedgerEntry(t, found, entry)
}

func TestRepositoryFindByIDReturnsNotFound(t *testing.T) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err := ledgerpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = repository.FindByID(
		ctx,
		"ledger-does-not-exist",
	)
	if !errors.Is(err, ledgerpostgres.ErrNotFound) {
		t.Fatalf(
			"expected ErrNotFound, got %v",
			err,
		)
	}
}

func TestRepositoryFindByWallet(t *testing.T) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err := ledgerpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Microsecond)

	walletID := "wallet-ledger-wallet"
	playerID := "player-ledger-wallet"

	insertWallet(
		t,
		pool,
		walletID,
		playerID,
		10000,
		"BRL",
		now,
	)

	insertTransaction(
		t,
		pool,
		"transaction-ledger-wallet-1",
		walletID,
		playerID,
		"external-ledger-wallet-1",
		"idempotency-ledger-wallet-1",
		now,
	)

	insertTransaction(
		t,
		pool,
		"transaction-ledger-wallet-2",
		walletID,
		playerID,
		"external-ledger-wallet-2",
		"idempotency-ledger-wallet-2",
		now.Add(time.Second),
	)

	first := newLedgerEntry(
		t,
		"ledger-wallet-1",
		walletID,
		"transaction-ledger-wallet-1",
		domainledger.DirectionDebit,
		1000,
		10000,
		9000,
		now,
	)

	second := newLedgerEntry(
		t,
		"ledger-wallet-2",
		walletID,
		"transaction-ledger-wallet-2",
		domainledger.DirectionCredit,
		500,
		9000,
		9500,
		now.Add(time.Second),
	)

	if err := repository.Create(ctx, first); err != nil {
		t.Fatalf("create first ledger entry: %v", err)
	}

	if err := repository.Create(ctx, second); err != nil {
		t.Fatalf("create second ledger entry: %v", err)
	}

	entries, err := repository.FindByWallet(ctx, walletID)
	if err != nil {
		t.Fatalf("find ledger entries by wallet: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf(
			"unexpected number of entries: got %d want 2",
			len(entries),
		)
	}

	assertLedgerEntry(t, entries[0], first)
	assertLedgerEntry(t, entries[1], second)
}

func TestRepositoryFindByWalletReturnsEmptySlice(t *testing.T) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err := ledgerpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	entries, err := repository.FindByWallet(
		ctx,
		"wallet-does-not-exist",
	)
	if err != nil {
		t.Fatalf("find ledger entries by wallet: %v", err)
	}

	if len(entries) != 0 {
		t.Fatalf(
			"expected empty result, got %d entries",
			len(entries),
		)
	}
}

func TestRepositoryFindByTransaction(t *testing.T) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err := ledgerpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Microsecond)

	prepareLedgerDependencies(
		t,
		pool,
		"wallet-ledger-transaction",
		"player-ledger-transaction",
		"transaction-ledger-transaction",
		now,
	)

	entry := newLedgerEntry(
		t,
		"ledger-transaction",
		"wallet-ledger-transaction",
		"transaction-ledger-transaction",
		domainledger.DirectionDebit,
		1000,
		10000,
		9000,
		now,
	)

	if err := repository.Create(ctx, entry); err != nil {
		t.Fatalf("create ledger entry: %v", err)
	}

	entries, err := repository.FindByTransaction(
		ctx,
		entry.TransactionID(),
	)
	if err != nil {
		t.Fatalf(
			"find ledger entries by transaction: %v",
			err,
		)
	}

	if len(entries) != 1 {
		t.Fatalf(
			"unexpected number of entries: got %d want 1",
			len(entries),
		)
	}

	assertLedgerEntry(t, entries[0], entry)
}

func TestRepositoryFindByTransactionReturnsEmptySlice(t *testing.T) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err := ledgerpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	entries, err := repository.FindByTransaction(
		ctx,
		"transaction-does-not-exist",
	)
	if err != nil {
		t.Fatalf(
			"find ledger entries by transaction: %v",
			err,
		)
	}

	if len(entries) != 0 {
		t.Fatalf(
			"expected empty result, got %d entries",
			len(entries),
		)
	}
}

func TestRepositoryRejectsDuplicateWalletTransaction(t *testing.T) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err := ledgerpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Microsecond)

	prepareLedgerDependencies(
		t,
		pool,
		"wallet-ledger-duplicate",
		"player-ledger-duplicate",
		"transaction-ledger-duplicate",
		now,
	)

	first := newLedgerEntry(
		t,
		"ledger-duplicate-1",
		"wallet-ledger-duplicate",
		"transaction-ledger-duplicate",
		domainledger.DirectionDebit,
		1000,
		10000,
		9000,
		now,
	)

	second := newLedgerEntry(
		t,
		"ledger-duplicate-2",
		"wallet-ledger-duplicate",
		"transaction-ledger-duplicate",
		domainledger.DirectionDebit,
		1000,
		10000,
		9000,
		now.Add(time.Second),
	)

	if err := repository.Create(ctx, first); err != nil {
		t.Fatalf("create first ledger entry: %v", err)
	}

	if err := repository.Create(ctx, second); err == nil {
		t.Fatal(
			"expected duplicate wallet/transaction to be rejected",
		)
	}
}

func TestLedgerEntryCannotBeUpdated(t *testing.T) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err := ledgerpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Microsecond)

	prepareLedgerDependencies(
		t,
		pool,
		"wallet-ledger-update",
		"player-ledger-update",
		"transaction-ledger-update",
		now,
	)

	entry := newLedgerEntry(
		t,
		"ledger-update",
		"wallet-ledger-update",
		"transaction-ledger-update",
		domainledger.DirectionDebit,
		1000,
		10000,
		9000,
		now,
	)

	if err := repository.Create(ctx, entry); err != nil {
		t.Fatalf("create ledger entry: %v", err)
	}

	_, err = pool.Exec(
		ctx,
		`
		UPDATE ledger_entries
		SET balance_after = 8000
		WHERE id = $1
		`,
		entry.ID(),
	)
	if err == nil {
		t.Fatal("expected ledger UPDATE to be rejected")
	}
}

func TestLedgerEntryCannotBeDeleted(t *testing.T) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err := ledgerpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Microsecond)

	prepareLedgerDependencies(
		t,
		pool,
		"wallet-ledger-delete",
		"player-ledger-delete",
		"transaction-ledger-delete",
		now,
	)

	entry := newLedgerEntry(
		t,
		"ledger-delete",
		"wallet-ledger-delete",
		"transaction-ledger-delete",
		domainledger.DirectionDebit,
		1000,
		10000,
		9000,
		now,
	)

	if err := repository.Create(ctx, entry); err != nil {
		t.Fatalf("create ledger entry: %v", err)
	}

	_, err = pool.Exec(
		ctx,
		`
		DELETE FROM ledger_entries
		WHERE id = $1
		`,
		entry.ID(),
	)
	if err == nil {
		t.Fatal("expected ledger DELETE to be rejected")
	}
}
