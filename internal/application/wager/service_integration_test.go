package wager_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	applicationwager "jungle_gaming_teste_tecnico/internal/application/wager"
	domainmoney "jungle_gaming_teste_tecnico/internal/domain/money"
	domaintransaction "jungle_gaming_teste_tecnico/internal/domain/wagertransaction"
	domainwallet "jungle_gaming_teste_tecnico/internal/domain/wallet"
	postgresinfra "jungle_gaming_teste_tecnico/internal/infrastructure/postgres"
	ledgerpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/ledger"
	transactionpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/wagertransaction"
	walletpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/wallet"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultTestDatabaseURL = "postgres://jungle:jungle@localhost:5432/jungle_gaming?sslmode=disable"
	integrationTestLockID  = int64(8675309)
)

type testEnvironment struct {
	pool                  *pgxpool.Pool
	service               *applicationwager.Service
	walletRepository      *walletpostgres.Repository
	transactionRepository *transactionpostgres.Repository
	ledgerRepository      *ledgerpostgres.Repository
}

func testDatabaseURL() string {
	if value := os.Getenv("TEST_DATABASE_URL"); value != "" {
		return value
	}

	return defaultTestDatabaseURL
}

func newTestEnvironment(t *testing.T) *testEnvironment {
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
			MaxConnections: 60,
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
		t.Fatalf("acquire integration lock connection: %v", err)
	}

	if _, err := conn.Exec(
		context.Background(),
		"SELECT pg_advisory_lock($1)",
		integrationTestLockID,
	); err != nil {
		conn.Release()
		pool.Close()
		t.Fatalf("acquire integration lock: %v", err)
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

	_, err = pool.Exec(
		context.Background(),
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

	walletRepository, err :=
		walletpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create wallet repository: %v", err)
	}

	transactionRepository, err :=
		transactionpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create transaction repository: %v", err)
	}

	ledgerRepository, err :=
		ledgerpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create ledger repository: %v", err)
	}

	transactionManager, err :=
		postgresinfra.NewTransactionManager(pool)
	if err != nil {
		t.Fatalf("create transaction manager: %v", err)
	}

	service, err := applicationwager.NewService(
		transactionManager,
		walletRepository,
		transactionRepository,
		ledgerRepository,
	)
	if err != nil {
		t.Fatalf("create wager service: %v", err)
	}

	return &testEnvironment{
		pool:                  pool,
		service:               service,
		walletRepository:      walletRepository,
		transactionRepository: transactionRepository,
		ledgerRepository:      ledgerRepository,
	}
}

func mustMoney(
	t *testing.T,
	amount int64,
) domainmoney.Money {
	t.Helper()

	value, err := domainmoney.New(amount, "BRL")
	if err != nil {
		t.Fatalf("create money: %v", err)
	}

	return value
}

func createWallet(
	t *testing.T,
	env *testEnvironment,
	id string,
	playerID string,
	balance int64,
) {
	t.Helper()

	now := time.Now().UTC().Truncate(time.Microsecond)

	entity, err := domainwallet.New(
		id,
		playerID,
		mustMoney(t, balance),
		now,
	)
	if err != nil {
		t.Fatalf("create wallet: %v", err)
	}

	if err := env.walletRepository.Create(
		context.Background(),
		&entity,
	); err != nil {
		t.Fatalf("persist wallet: %v", err)
	}
}

func newBet(
	t *testing.T,
	id string,
	externalID string,
	idempotencyKey string,
	payloadHash string,
	walletID string,
	playerID string,
	amount int64,
) *domaintransaction.WagerTransaction {
	t.Helper()

	entity, err := domaintransaction.NewExternal(
		domaintransaction.ExternalInput{
			ID:                    id,
			ExternalTransactionID: externalID,
			ProviderID:            "provider-test",
			IdempotencyKey:        idempotencyKey,
			PayloadHash:           payloadHash,
			WalletID:              walletID,
			PlayerID:              playerID,
			RoundID:               "round-test",
			GameID:                "game-test",
			Kind:                  domaintransaction.KindBet,
			Money:                 mustMoney(t, amount),
			CreatedAt:             time.Now().UTC(),
		},
	)
	if err != nil {
		t.Fatalf("create bet: %v", err)
	}

	return &entity
}

func countRows(
	t *testing.T,
	env *testEnvironment,
	table string,
) int {
	t.Helper()

	var count int

	query := fmt.Sprintf(
		"SELECT COUNT(*) FROM %s",
		table,
	)

	if err := env.pool.QueryRow(
		context.Background(),
		query,
	).Scan(&count); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}

	return count
}

func TestServiceProcessesBet(t *testing.T) {
	env := newTestEnvironment(t)

	createWallet(
		t,
		env,
		"wallet-bet",
		"player-bet",
		10000,
	)

	transaction := newBet(
		t,
		"transaction-bet",
		"external-bet",
		"idempotency-bet",
		"hash-bet",
		"wallet-bet",
		"player-bet",
		2000,
	)

	result, err := env.service.Process(
		context.Background(),
		applicationwager.ProcessInput{
			Transaction:   transaction,
			LedgerEntryID: "ledger-bet",
			OccurredAt:    time.Now().UTC(),
		},
	)
	if err != nil {
		t.Fatalf("process bet: %v", err)
	}

	if result.Replayed {
		t.Fatal("first execution must not be replay")
	}

	if result.Balance != 8000 {
		t.Fatalf(
			"balance = %d, want 8000",
			result.Balance,
		)
	}

	walletEntity, err :=
		env.walletRepository.FindByID(
			context.Background(),
			"wallet-bet",
		)
	if err != nil {
		t.Fatalf("find wallet: %v", err)
	}

	if walletEntity.Balance().Amount() != 8000 {
		t.Fatalf(
			"persisted balance = %d, want 8000",
			walletEntity.Balance().Amount(),
		)
	}

	if countRows(t, env, "wager_transactions") != 1 {
		t.Fatal("expected exactly one wager transaction")
	}

	if countRows(t, env, "ledger_entries") != 1 {
		t.Fatal("expected exactly one ledger entry")
	}
}

func TestServiceReplaysSameBet(t *testing.T) {
	env := newTestEnvironment(t)

	createWallet(
		t,
		env,
		"wallet-replay",
		"player-replay",
		10000,
	)

	first := newBet(
		t,
		"transaction-replay-1",
		"external-replay",
		"idempotency-replay",
		"hash-replay",
		"wallet-replay",
		"player-replay",
		2000,
	)

	firstResult, err := env.service.Process(
		context.Background(),
		applicationwager.ProcessInput{
			Transaction:   first,
			LedgerEntryID: "ledger-replay-1",
			OccurredAt:    time.Now().UTC(),
		},
	)
	if err != nil {
		t.Fatalf("first process: %v", err)
	}

	second := newBet(
		t,
		"transaction-replay-2",
		"external-replay",
		"idempotency-replay",
		"hash-replay",
		"wallet-replay",
		"player-replay",
		2000,
	)

	secondResult, err := env.service.Process(
		context.Background(),
		applicationwager.ProcessInput{
			Transaction:   second,
			LedgerEntryID: "ledger-replay-2",
			OccurredAt:    time.Now().UTC(),
		},
	)
	if err != nil {
		t.Fatalf("replay process: %v", err)
	}

	if !secondResult.Replayed {
		t.Fatal("second execution must be replay")
	}

	if secondResult.Balance != firstResult.Balance {
		t.Fatalf(
			"replay balance = %d, want %d",
			secondResult.Balance,
			firstResult.Balance,
		)
	}

	walletEntity, err :=
		env.walletRepository.FindByID(
			context.Background(),
			"wallet-replay",
		)
	if err != nil {
		t.Fatalf("find wallet: %v", err)
	}

	if walletEntity.Balance().Amount() != 8000 {
		t.Fatalf(
			"wallet debited more than once: balance=%d",
			walletEntity.Balance().Amount(),
		)
	}

	if countRows(t, env, "wager_transactions") != 1 {
		t.Fatal("replay created duplicate transaction")
	}

	if countRows(t, env, "ledger_entries") != 1 {
		t.Fatal("replay created duplicate ledger entry")
	}
}

func TestServiceRejectsSameIdempotencyKeyWithDifferentPayload(
	t *testing.T,
) {
	env := newTestEnvironment(t)

	createWallet(
		t,
		env,
		"wallet-conflict",
		"player-conflict",
		10000,
	)

	first := newBet(
		t,
		"transaction-conflict-1",
		"external-conflict-1",
		"idempotency-conflict",
		"hash-conflict-1",
		"wallet-conflict",
		"player-conflict",
		2000,
	)

	_, err := env.service.Process(
		context.Background(),
		applicationwager.ProcessInput{
			Transaction:   first,
			LedgerEntryID: "ledger-conflict-1",
			OccurredAt:    time.Now().UTC(),
		},
	)
	if err != nil {
		t.Fatalf("first process: %v", err)
	}

	second := newBet(
		t,
		"transaction-conflict-2",
		"external-conflict-2",
		"idempotency-conflict",
		"hash-conflict-2",
		"wallet-conflict",
		"player-conflict",
		2000,
	)

	_, err = env.service.Process(
		context.Background(),
		applicationwager.ProcessInput{
			Transaction:   second,
			LedgerEntryID: "ledger-conflict-2",
			OccurredAt:    time.Now().UTC(),
		},
	)

	if !errors.Is(
		err,
		applicationwager.ErrIdempotencyConflict,
	) {
		t.Fatalf(
			"expected ErrIdempotencyConflict, got %v",
			err,
		)
	}
}

func TestServiceSameBet50TimesConcurrently(t *testing.T) {
	env := newTestEnvironment(t)

	createWallet(
		t,
		env,
		"wallet-50",
		"player-50",
		10000,
	)

	const workers = 50

	start := make(chan struct{})
	results := make(
		chan applicationwager.ProcessResult,
		workers,
	)
	errs := make(chan error, workers)

	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)

		go func(index int) {
			defer wg.Done()

			<-start

			transaction := newBet(
				t,
				fmt.Sprintf("transaction-50-%d", index),
				"external-50",
				"idempotency-50",
				"hash-50",
				"wallet-50",
				"player-50",
				2000,
			)

			result, err := env.service.Process(
				context.Background(),
				applicationwager.ProcessInput{
					Transaction: transaction,
					LedgerEntryID: fmt.Sprintf(
						"ledger-50-%d",
						index,
					),
					OccurredAt: time.Now().UTC(),
				},
			)

			if err != nil {
				errs <- err
				return
			}

			results <- result
		}(i)
	}

	close(start)

	wg.Wait()
	close(results)
	close(errs)

	var errorsFound []error

	for err := range errs {
		errorsFound = append(errorsFound, err)
	}

	if len(errorsFound) != 0 {
		t.Fatalf(
			"%d concurrent requests failed; first error: %v",
			len(errorsFound),
			errorsFound[0],
		)
	}

	successfulExecutions := 0
	replays := 0

	for result := range results {
		if result.Replayed {
			replays++
		} else {
			successfulExecutions++
		}

		if result.Balance != 8000 {
			t.Fatalf(
				"unexpected result balance: %d",
				result.Balance,
			)
		}
	}

	if successfulExecutions != 1 {
		t.Fatalf(
			"real executions = %d, want 1",
			successfulExecutions,
		)
	}

	if replays != workers-1 {
		t.Fatalf(
			"replays = %d, want %d",
			replays,
			workers-1,
		)
	}

	walletEntity, err :=
		env.walletRepository.FindByID(
			context.Background(),
			"wallet-50",
		)
	if err != nil {
		t.Fatalf("find wallet: %v", err)
	}

	if walletEntity.Balance().Amount() != 8000 {
		t.Fatalf(
			"final balance = %d, want 8000",
			walletEntity.Balance().Amount(),
		)
	}

	if countRows(t, env, "wager_transactions") != 1 {
		t.Fatal("expected exactly one wager transaction")
	}

	if countRows(t, env, "ledger_entries") != 1 {
		t.Fatal("expected exactly one ledger entry")
	}
}

func TestServiceTwoBets80AgainstBalance100(t *testing.T) {
	env := newTestEnvironment(t)

	ctx := context.Background()

	createWallet(
		t,
		env,
		"wallet-two-bets",
		"player-two-bets",
		10000,
	)

	firstBet := newBet(
		t,
		"transaction-bet-80-a",
		"external-bet-80-a",
		"idempotency-bet-80-a",
		"hash-bet-80-a",
		"wallet-two-bets",
		"player-two-bets",
		8000,
	)

	secondBet := newBet(
		t,
		"transaction-bet-80-b",
		"external-bet-80-b",
		"idempotency-bet-80-b",
		"hash-bet-80-b",
		"wallet-two-bets",
		"player-two-bets",
		8000,
	)

	type operationResult struct {
		result applicationwager.ProcessResult
		err    error
	}

	start := make(chan struct{})
	results := make(chan operationResult, 2)

	process := func(
		transaction *domaintransaction.WagerTransaction,
		ledgerEntryID string,
	) {
		<-start

		result, err := env.service.Process(
			ctx,
			applicationwager.ProcessInput{
				Transaction:   transaction,
				LedgerEntryID: ledgerEntryID,
				OccurredAt:    time.Now().UTC(),
			},
		)

		results <- operationResult{
			result: result,
			err:    err,
		}
	}

	go process(
		firstBet,
		"ledger-bet-80-a",
	)

	go process(
		secondBet,
		"ledger-bet-80-b",
	)

	close(start)

	var processedResult *applicationwager.ProcessResult
	var rejectedResult *applicationwager.ProcessResult

	for i := 0; i < 2; i++ {
		operation := <-results

		if operation.err != nil {
			t.Fatalf(
				"process concurrent bet: %v",
				operation.err,
			)
		}

		if operation.result.Transaction == nil {
			t.Fatal("result transaction must not be nil")
		}

		switch operation.result.Transaction.Status() {
		case domaintransaction.StatusProcessed:
			if processedResult != nil {
				t.Fatal("more than one bet was processed")
			}

			value := operation.result
			processedResult = &value

		case domaintransaction.StatusRejected:
			if rejectedResult != nil {
				t.Fatal("more than one bet was rejected")
			}

			value := operation.result
			rejectedResult = &value

		default:
			t.Fatalf(
				"unexpected transaction status: %s",
				operation.result.Transaction.Status(),
			)
		}
	}

	if processedResult == nil {
		t.Fatal("expected exactly one processed bet")
	}

	if rejectedResult == nil {
		t.Fatal("expected exactly one rejected bet")
	}

	if processedResult.Balance != 2000 {
		t.Fatalf(
			"processed result balance = %d, want 2000",
			processedResult.Balance,
		)
	}

	if rejectedResult.Balance != 2000 {
		t.Fatalf(
			"rejected result balance = %d, want 2000",
			rejectedResult.Balance,
		)
	}

	if rejectedResult.Transaction.FailureCode() !=
		domaintransaction.FailureCodeInsufficientFunds {
		t.Fatalf(
			"rejected failure code = %s, want %s",
			rejectedResult.Transaction.FailureCode(),
			domaintransaction.FailureCodeInsufficientFunds,
		)
	}

	walletEntity, err := env.walletRepository.FindByID(
		ctx,
		"wallet-two-bets",
	)
	if err != nil {
		t.Fatalf("find wallet: %v", err)
	}

	if walletEntity.Balance().Amount() != 2000 {
		t.Fatalf(
			"wallet balance = %d, want 2000",
			walletEntity.Balance().Amount(),
		)
	}

	if got := countRows(
		t,
		env,
		"wager_transactions",
	); got != 2 {
		t.Fatalf(
			"wager transaction count = %d, want 2",
			got,
		)
	}

	if got := countRows(
		t,
		env,
		"ledger_entries",
	); got != 1 {
		t.Fatalf(
			"ledger entry count = %d, want 1",
			got,
		)
	}

	persistedFirst, err :=
		env.transactionRepository.FindByID(
			ctx,
			firstBet.ID(),
		)
	if err != nil {
		t.Fatalf("find first bet: %v", err)
	}

	persistedSecond, err :=
		env.transactionRepository.FindByID(
			ctx,
			secondBet.ID(),
		)
	if err != nil {
		t.Fatalf("find second bet: %v", err)
	}

	var persistedRejected *domaintransaction.WagerTransaction

	switch {
	case persistedFirst.Status() ==
		domaintransaction.StatusProcessed &&
		persistedSecond.Status() ==
			domaintransaction.StatusRejected:

		persistedRejected = persistedSecond

	case persistedFirst.Status() ==
		domaintransaction.StatusRejected &&
		persistedSecond.Status() ==
			domaintransaction.StatusProcessed:

		persistedRejected = persistedFirst

	default:
		t.Fatalf(
			"persisted statuses = %s and %s, want one PROCESSED and one REJECTED",
			persistedFirst.Status(),
			persistedSecond.Status(),
		)
	}

	if persistedRejected.FailureCode() !=
		domaintransaction.FailureCodeInsufficientFunds {
		t.Fatalf(
			"persisted rejected failure code = %s, want %s",
			persistedRejected.FailureCode(),
			domaintransaction.FailureCodeInsufficientFunds,
		)
	}

	var replayInput *domaintransaction.WagerTransaction
	var replayLedgerID string

	if persistedRejected.ID() == firstBet.ID() {
		replayInput = firstBet
		replayLedgerID = "ledger-replay-bet-80-a"
	} else {
		replayInput = secondBet
		replayLedgerID = "ledger-replay-bet-80-b"
	}

	replay, err := env.service.Process(
		ctx,
		applicationwager.ProcessInput{
			Transaction:   replayInput,
			LedgerEntryID: replayLedgerID,
			OccurredAt:    time.Now().UTC(),
		},
	)
	if err != nil {
		t.Fatalf(
			"replay rejected bet: %v",
			err,
		)
	}

	if !replay.Replayed {
		t.Fatal(
			"rejected transaction replay must be marked as replay",
		)
	}

	if replay.Transaction == nil {
		t.Fatal(
			"replayed transaction must not be nil",
		)
	}

	if replay.Transaction.Status() !=
		domaintransaction.StatusRejected {
		t.Fatalf(
			"replay status = %s, want %s",
			replay.Transaction.Status(),
			domaintransaction.StatusRejected,
		)
	}

	if replay.Transaction.FailureCode() !=
		domaintransaction.FailureCodeInsufficientFunds {
		t.Fatalf(
			"replay failure code = %s, want %s",
			replay.Transaction.FailureCode(),
			domaintransaction.FailureCodeInsufficientFunds,
		)
	}

	if replay.Balance != 2000 {
		t.Fatalf(
			"replay balance = %d, want 2000",
			replay.Balance,
		)
	}

	walletAfterReplay, err :=
		env.walletRepository.FindByID(
			ctx,
			"wallet-two-bets",
		)
	if err != nil {
		t.Fatalf(
			"find wallet after replay: %v",
			err,
		)
	}

	if walletAfterReplay.Balance().Amount() != 2000 {
		t.Fatalf(
			"wallet balance after replay = %d, want 2000",
			walletAfterReplay.Balance().Amount(),
		)
	}

	if got := countRows(
		t,
		env,
		"wager_transactions",
	); got != 2 {
		t.Fatalf(
			"wager transaction count after replay = %d, want 2",
			got,
		)
	}

	if got := countRows(
		t,
		env,
		"ledger_entries",
	); got != 1 {
		t.Fatalf(
			"ledger entry count after replay = %d, want 1",
			got,
		)
	}
}

func TestServiceDifferentWalletsConcurrently(t *testing.T) {
	env := newTestEnvironment(t)

	createWallet(
		t,
		env,
		"wallet-a",
		"player-a",
		10000,
	)

	createWallet(
		t,
		env,
		"wallet-b",
		"player-b",
		10000,
	)

	type operation struct {
		walletID string
		playerID string
		suffix   string
	}

	operations := []operation{
		{
			walletID: "wallet-a",
			playerID: "player-a",
			suffix:   "a",
		},
		{
			walletID: "wallet-b",
			playerID: "player-b",
			suffix:   "b",
		},
	}

	start := make(chan struct{})
	errs := make(chan error, len(operations))

	var wg sync.WaitGroup

	for _, operation := range operations {
		operation := operation

		wg.Add(1)

		go func() {
			defer wg.Done()

			<-start

			transaction := newBet(
				t,
				"transaction-"+operation.suffix,
				"external-"+operation.suffix,
				"idempotency-"+operation.suffix,
				"hash-"+operation.suffix,
				operation.walletID,
				operation.playerID,
				2000,
			)

			_, err := env.service.Process(
				context.Background(),
				applicationwager.ProcessInput{
					Transaction: transaction,
					LedgerEntryID: "ledger-" +
						operation.suffix,
					OccurredAt: time.Now().UTC(),
				},
			)

			errs <- err
		}()
	}

	close(start)

	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf(
				"concurrent different-wallet operation failed: %v",
				err,
			)
		}
	}

	walletA, err :=
		env.walletRepository.FindByID(
			context.Background(),
			"wallet-a",
		)
	if err != nil {
		t.Fatalf("find wallet A: %v", err)
	}

	walletB, err :=
		env.walletRepository.FindByID(
			context.Background(),
			"wallet-b",
		)
	if err != nil {
		t.Fatalf("find wallet B: %v", err)
	}

	if walletA.Balance().Amount() != 8000 {
		t.Fatalf(
			"wallet A balance = %d, want 8000",
			walletA.Balance().Amount(),
		)
	}

	if walletB.Balance().Amount() != 8000 {
		t.Fatalf(
			"wallet B balance = %d, want 8000",
			walletB.Balance().Amount(),
		)
	}

	if countRows(t, env, "wager_transactions") != 2 {
		t.Fatal("expected two wager transactions")
	}

	if countRows(t, env, "ledger_entries") != 2 {
		t.Fatal("expected two ledger entries")
	}
}
