package wagertransaction_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	domainmoney "jungle_gaming_teste_tecnico/internal/domain/money"
	domaintransaction "jungle_gaming_teste_tecnico/internal/domain/wagertransaction"
	postgresinfra "jungle_gaming_teste_tecnico/internal/infrastructure/postgres"
	transactionpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/wagertransaction"

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
	amount int64,
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
		amount,
		now,
	)
	if err != nil {
		t.Fatalf("insert wallet: %v", err)
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

func newExternalTransaction(
	t *testing.T,
	id string,
	externalID string,
	idempotencyKey string,
	walletID string,
	playerID string,
	kind domaintransaction.Kind,
	referenceExternalID string,
	now time.Time,
) *domaintransaction.WagerTransaction {
	t.Helper()

	entity, err := domaintransaction.NewExternal(
		domaintransaction.ExternalInput{
			ID:                             id,
			ExternalTransactionID:          externalID,
			ProviderID:                     "provider-test",
			IdempotencyKey:                 idempotencyKey,
			PayloadHash:                    "payload-hash-" + id,
			WalletID:                       walletID,
			PlayerID:                       playerID,
			RoundID:                        "round-test",
			GameID:                         "game-test",
			Kind:                           kind,
			Money:                          newMoney(t, 1000, "BRL"),
			ReferenceExternalTransactionID: referenceExternalID,
			CreatedAt:                      now,
		},
	)
	if err != nil {
		t.Fatalf("create external transaction: %v", err)
	}

	return &entity
}

func assertTransaction(
	t *testing.T,
	got *domaintransaction.WagerTransaction,
	want *domaintransaction.WagerTransaction,
) {
	t.Helper()

	if got.ID() != want.ID() {
		t.Fatalf("unexpected id: got %q want %q", got.ID(), want.ID())
	}

	if got.ExternalTransactionID() != want.ExternalTransactionID() {
		t.Fatalf(
			"unexpected external transaction id: got %q want %q",
			got.ExternalTransactionID(),
			want.ExternalTransactionID(),
		)
	}

	if got.ProviderID() != want.ProviderID() {
		t.Fatalf(
			"unexpected provider id: got %q want %q",
			got.ProviderID(),
			want.ProviderID(),
		)
	}

	if got.IdempotencyKey() != want.IdempotencyKey() {
		t.Fatalf(
			"unexpected idempotency key: got %q want %q",
			got.IdempotencyKey(),
			want.IdempotencyKey(),
		)
	}

	if got.PayloadHash() != want.PayloadHash() {
		t.Fatalf(
			"unexpected payload hash: got %q want %q",
			got.PayloadHash(),
			want.PayloadHash(),
		)
	}

	if got.WalletID() != want.WalletID() {
		t.Fatalf(
			"unexpected wallet id: got %q want %q",
			got.WalletID(),
			want.WalletID(),
		)
	}

	if got.PlayerID() != want.PlayerID() {
		t.Fatalf(
			"unexpected player id: got %q want %q",
			got.PlayerID(),
			want.PlayerID(),
		)
	}

	if got.RoundID() != want.RoundID() {
		t.Fatalf(
			"unexpected round id: got %q want %q",
			got.RoundID(),
			want.RoundID(),
		)
	}

	if got.GameID() != want.GameID() {
		t.Fatalf(
			"unexpected game id: got %q want %q",
			got.GameID(),
			want.GameID(),
		)
	}

	if got.Kind() != want.Kind() {
		t.Fatalf(
			"unexpected kind: got %q want %q",
			got.Kind(),
			want.Kind(),
		)
	}

	if !got.Money().Equal(want.Money()) {
		t.Fatalf(
			"unexpected money: got %d %s want %d %s",
			got.Money().Amount(),
			got.Money().Currency(),
			want.Money().Amount(),
			want.Money().Currency(),
		)
	}

	if got.ReferenceExternalTransactionID() != want.ReferenceExternalTransactionID() {
		t.Fatalf(
			"unexpected reference external id: got %q want %q",
			got.ReferenceExternalTransactionID(),
			want.ReferenceExternalTransactionID(),
		)
	}

	if got.ReferenceTransactionID() != want.ReferenceTransactionID() {
		t.Fatalf(
			"unexpected reference transaction id: got %q want %q",
			got.ReferenceTransactionID(),
			want.ReferenceTransactionID(),
		)
	}

	if got.Status() != want.Status() {
		t.Fatalf(
			"unexpected status: got %q want %q",
			got.Status(),
			want.Status(),
		)
	}

	if got.FailureCode() != want.FailureCode() {
		t.Fatalf(
			"unexpected failure code: got %q want %q",
			got.FailureCode(),
			want.FailureCode(),
		)
	}

	gotBalance, gotBalancePresent := got.ResultBalance()
	wantBalance, wantBalancePresent := want.ResultBalance()

	if gotBalancePresent != wantBalancePresent {
		t.Fatalf(
			"unexpected result balance presence: got %v want %v",
			gotBalancePresent,
			wantBalancePresent,
		)
	}

	if gotBalancePresent && !gotBalance.Equal(wantBalance) {
		t.Fatalf(
			"unexpected result balance: got %d %s want %d %s",
			gotBalance.Amount(),
			gotBalance.Currency(),
			wantBalance.Amount(),
			wantBalance.Currency(),
		)
	}

	if !got.CreatedAt().Equal(want.CreatedAt()) {
		t.Fatalf(
			"unexpected created at: got %v want %v",
			got.CreatedAt(),
			want.CreatedAt(),
		)
	}

	if !got.UpdatedAt().Equal(want.UpdatedAt()) {
		t.Fatalf(
			"unexpected updated at: got %v want %v",
			got.UpdatedAt(),
			want.UpdatedAt(),
		)
	}
}

func TestRepositoryCreateAndFindByID(t *testing.T) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err := transactionpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Microsecond)

	insertWallet(
		t,
		pool,
		"wallet-create-find",
		"player-create-find",
		10000,
		"BRL",
		now,
	)

	original := newExternalTransaction(
		t,
		"transaction-create-find",
		"external-create-find",
		"idempotency-create-find",
		"wallet-create-find",
		"player-create-find",
		domaintransaction.KindBet,
		"",
		now,
	)

	if err := repository.Create(ctx, original); err != nil {
		t.Fatalf("create transaction: %v", err)
	}

	found, err := repository.FindByID(ctx, original.ID())
	if err != nil {
		t.Fatalf("find transaction: %v", err)
	}

	assertTransaction(t, found, original)
}

func TestRepositoryCreateAndFindOpening(t *testing.T) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err := transactionpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Microsecond)

	insertWallet(
		t,
		pool,
		"wallet-opening",
		"player-opening",
		5000,
		"BRL",
		now,
	)

	entity, err := domaintransaction.NewOpening(
		domaintransaction.OpeningInput{
			ID:        "transaction-opening",
			WalletID:  "wallet-opening",
			PlayerID:  "player-opening",
			Money:     newMoney(t, 5000, "BRL"),
			CreatedAt: now,
		},
	)
	if err != nil {
		t.Fatalf("create opening: %v", err)
	}

	if err := repository.Create(ctx, &entity); err != nil {
		t.Fatalf("persist opening: %v", err)
	}

	found, err := repository.FindByID(ctx, entity.ID())
	if err != nil {
		t.Fatalf("find opening: %v", err)
	}

	assertTransaction(t, found, &entity)

	if found.ExternalTransactionID() != "" ||
		found.ProviderID() != "" ||
		found.IdempotencyKey() != "" ||
		found.PayloadHash() != "" ||
		found.RoundID() != "" ||
		found.GameID() != "" {
		t.Fatal("opening external metadata should be empty")
	}
}

func TestRepositoryFindByProviderExternalID(t *testing.T) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err := transactionpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Microsecond)

	insertWallet(
		t,
		pool,
		"wallet-external-find",
		"player-external-find",
		10000,
		"BRL",
		now,
	)

	original := newExternalTransaction(
		t,
		"transaction-external-find",
		"external-find",
		"idempotency-external-find",
		"wallet-external-find",
		"player-external-find",
		domaintransaction.KindWin,
		"",
		now,
	)

	if err := repository.Create(ctx, original); err != nil {
		t.Fatalf("create transaction: %v", err)
	}

	found, err := repository.FindByProviderExternalID(
		ctx,
		original.ProviderID(),
		original.ExternalTransactionID(),
	)
	if err != nil {
		t.Fatalf("find by provider/external id: %v", err)
	}

	assertTransaction(t, found, original)
}

func TestRepositoryFindByProviderIdempotencyKey(t *testing.T) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err := transactionpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Microsecond)

	insertWallet(
		t,
		pool,
		"wallet-idempotency-find",
		"player-idempotency-find",
		10000,
		"BRL",
		now,
	)

	original := newExternalTransaction(
		t,
		"transaction-idempotency-find",
		"external-idempotency-find",
		"idempotency-find",
		"wallet-idempotency-find",
		"player-idempotency-find",
		domaintransaction.KindBet,
		"",
		now,
	)

	if err := repository.Create(ctx, original); err != nil {
		t.Fatalf("create transaction: %v", err)
	}

	found, err := repository.FindByProviderIdempotencyKey(
		ctx,
		original.ProviderID(),
		original.IdempotencyKey(),
	)
	if err != nil {
		t.Fatalf("find by provider/idempotency key: %v", err)
	}

	assertTransaction(t, found, original)
}

func TestRepositoryFindReturnsNotFound(t *testing.T) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err := transactionpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err = repository.FindByID(ctx, "does-not-exist")
	if !errors.Is(err, transactionpostgres.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	_, err = repository.FindByProviderExternalID(
		ctx,
		"provider-test",
		"does-not-exist",
	)
	if !errors.Is(err, transactionpostgres.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	_, err = repository.FindByProviderIdempotencyKey(
		ctx,
		"provider-test",
		"does-not-exist",
	)
	if !errors.Is(err, transactionpostgres.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestRepositoryUpdateProcessed(t *testing.T) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err := transactionpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Microsecond)

	insertWallet(
		t,
		pool,
		"wallet-update-processed",
		"player-update-processed",
		10000,
		"BRL",
		now,
	)

	transaction := newExternalTransaction(
		t,
		"transaction-update-processed",
		"external-update-processed",
		"idempotency-update-processed",
		"wallet-update-processed",
		"player-update-processed",
		domaintransaction.KindBet,
		"",
		now,
	)

	if err := repository.Create(ctx, transaction); err != nil {
		t.Fatalf("create transaction: %v", err)
	}

	resultBalance := newMoney(t, 9000, "BRL")
	updatedAt := now.Add(time.Second)

	if err := transaction.MarkProcessed(resultBalance, updatedAt); err != nil {
		t.Fatalf("mark processed: %v", err)
	}

	if err := repository.Update(ctx, transaction); err != nil {
		t.Fatalf("update transaction: %v", err)
	}

	found, err := repository.FindByID(ctx, transaction.ID())
	if err != nil {
		t.Fatalf("find transaction: %v", err)
	}

	assertTransaction(t, found, transaction)
}

func TestRepositoryPendingReferenceAndResolution(t *testing.T) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err := transactionpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Microsecond)

	insertWallet(
		t,
		pool,
		"wallet-reference",
		"player-reference",
		10000,
		"BRL",
		now,
	)

	originalBet := newExternalTransaction(
		t,
		"transaction-reference-bet",
		"external-reference-bet",
		"idempotency-reference-bet",
		"wallet-reference",
		"player-reference",
		domaintransaction.KindBet,
		"",
		now,
	)

	if err := repository.Create(ctx, originalBet); err != nil {
		t.Fatalf("create referenced bet: %v", err)
	}

	refund := newExternalTransaction(
		t,
		"transaction-reference-refund",
		"external-reference-refund",
		"idempotency-reference-refund",
		"wallet-reference",
		"player-reference",
		domaintransaction.KindRefund,
		originalBet.ExternalTransactionID(),
		now.Add(time.Second),
	)

	if err := refund.MarkPendingReference(now.Add(2 * time.Second)); err != nil {
		t.Fatalf("mark pending reference: %v", err)
	}

	if err := repository.Create(ctx, refund); err != nil {
		t.Fatalf("create pending reference: %v", err)
	}

	pending, err := repository.FindPendingReferences(ctx, 10)
	if err != nil {
		t.Fatalf("find pending references: %v", err)
	}

	if len(pending) != 1 {
		t.Fatalf(
			"unexpected pending reference count: got %d want 1",
			len(pending),
		)
	}

	if pending[0].ID() != refund.ID() {
		t.Fatalf(
			"unexpected pending transaction: got %q want %q",
			pending[0].ID(),
			refund.ID(),
		)
	}

	if err := refund.ResolveReference(
		originalBet.ID(),
		now.Add(3*time.Second),
	); err != nil {
		t.Fatalf("resolve reference: %v", err)
	}

	if err := repository.Update(ctx, refund); err != nil {
		t.Fatalf("persist resolved reference: %v", err)
	}

	found, err := repository.FindByID(ctx, refund.ID())
	if err != nil {
		t.Fatalf("find resolved transaction: %v", err)
	}

	assertTransaction(t, found, refund)

	if found.Status() != domaintransaction.StatusPending {
		t.Fatalf(
			"unexpected status: got %q want %q",
			found.Status(),
			domaintransaction.StatusPending,
		)
	}

	if found.ReferenceTransactionID() != originalBet.ID() {
		t.Fatalf(
			"unexpected reference transaction: got %q want %q",
			found.ReferenceTransactionID(),
			originalBet.ID(),
		)
	}
}

func TestRepositoryUpdateRejected(t *testing.T) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err := transactionpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Microsecond)

	insertWallet(
		t,
		pool,
		"wallet-rejected",
		"player-rejected",
		10000,
		"BRL",
		now,
	)

	transaction := newExternalTransaction(
		t,
		"transaction-rejected",
		"external-rejected",
		"idempotency-rejected",
		"wallet-rejected",
		"player-rejected",
		domaintransaction.KindBet,
		"",
		now,
	)

	if err := repository.Create(ctx, transaction); err != nil {
		t.Fatalf("create transaction: %v", err)
	}

	failureCode, err := domaintransaction.NewFailureCode(
		"INSUFFICIENT_BALANCE",
	)
	if err != nil {
		t.Fatalf("create failure code: %v", err)
	}

	resultBalance := newMoney(t, 10000, "BRL")

	if err := transaction.MarkRejected(
		failureCode,
		resultBalance,
		now.Add(time.Second),
	); err != nil {
		t.Fatalf("mark rejected: %v", err)
	}

	if err := repository.Update(ctx, transaction); err != nil {
		t.Fatalf("update rejected transaction: %v", err)
	}

	found, err := repository.FindByID(ctx, transaction.ID())
	if err != nil {
		t.Fatalf("find rejected transaction: %v", err)
	}

	assertTransaction(t, found, transaction)
}

func TestRepositoryRejectsDuplicateProviderExternalID(t *testing.T) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err := transactionpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Microsecond)

	insertWallet(
		t,
		pool,
		"wallet-duplicate-external",
		"player-duplicate-external",
		10000,
		"BRL",
		now,
	)

	first := newExternalTransaction(
		t,
		"transaction-duplicate-external-1",
		"same-external-id",
		"idempotency-duplicate-external-1",
		"wallet-duplicate-external",
		"player-duplicate-external",
		domaintransaction.KindBet,
		"",
		now,
	)

	second := newExternalTransaction(
		t,
		"transaction-duplicate-external-2",
		"same-external-id",
		"idempotency-duplicate-external-2",
		"wallet-duplicate-external",
		"player-duplicate-external",
		domaintransaction.KindBet,
		"",
		now.Add(time.Second),
	)

	if err := repository.Create(ctx, first); err != nil {
		t.Fatalf("create first transaction: %v", err)
	}

	if err := repository.Create(ctx, second); err == nil {
		t.Fatal("expected duplicate external transaction id to fail")
	}
}

func TestRepositoryRejectsDuplicateProviderIdempotencyKey(t *testing.T) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err := transactionpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Microsecond)

	insertWallet(
		t,
		pool,
		"wallet-duplicate-idempotency",
		"player-duplicate-idempotency",
		10000,
		"BRL",
		now,
	)

	first := newExternalTransaction(
		t,
		"transaction-duplicate-idempotency-1",
		"external-duplicate-idempotency-1",
		"same-idempotency-key",
		"wallet-duplicate-idempotency",
		"player-duplicate-idempotency",
		domaintransaction.KindBet,
		"",
		now,
	)

	second := newExternalTransaction(
		t,
		"transaction-duplicate-idempotency-2",
		"external-duplicate-idempotency-2",
		"same-idempotency-key",
		"wallet-duplicate-idempotency",
		"player-duplicate-idempotency",
		domaintransaction.KindBet,
		"",
		now.Add(time.Second),
	)

	if err := repository.Create(ctx, first); err != nil {
		t.Fatalf("create first transaction: %v", err)
	}

	if err := repository.Create(ctx, second); err == nil {
		t.Fatal("expected duplicate idempotency key to fail")
	}
}

func TestRepositoryUpdateReturnsNotFound(t *testing.T) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	repository, err := transactionpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create repository: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	now := time.Now().UTC().Truncate(time.Microsecond)

	transaction := newExternalTransaction(
		t,
		"transaction-update-not-found",
		"external-update-not-found",
		"idempotency-update-not-found",
		"wallet-does-not-exist",
		"player-does-not-exist",
		domaintransaction.KindBet,
		"",
		now,
	)

	resultBalance := newMoney(t, 9000, "BRL")

	if err := transaction.MarkProcessed(
		resultBalance,
		now.Add(time.Second),
	); err != nil {
		t.Fatalf("mark processed: %v", err)
	}

	err = repository.Update(ctx, transaction)

	if !errors.Is(err, transactionpostgres.ErrNotFound) {
		t.Fatalf(
			"expected ErrNotFound, got %v",
			err,
		)
	}
}
