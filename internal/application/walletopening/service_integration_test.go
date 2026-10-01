package walletopening_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	applicationwalletopening "jungle_gaming_teste_tecnico/internal/application/walletopening"
	domainmoney "jungle_gaming_teste_tecnico/internal/domain/money"
	postgresinfra "jungle_gaming_teste_tecnico/internal/infrastructure/postgres"
	ledgerpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/ledger"
	outboxpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/outbox"
	transactionpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/wagertransaction"
	walletpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/wallet"

	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	defaultWalletOpeningTestDatabaseURL = "postgres://jungle:jungle@localhost:5432/jungle_gaming?sslmode=disable"
	walletOpeningIntegrationTestLockID  = int64(8675309)
)

type testEnvironment struct {
	pool                  *pgxpool.Pool
	service               *applicationwalletopening.Service
	walletRepository      *walletpostgres.Repository
	transactionRepository *transactionpostgres.Repository
	ledgerRepository      *ledgerpostgres.Repository
	outboxRepository      *outboxpostgres.Repository
}

func testDatabaseURL() string {
	if value := os.Getenv("TEST_DATABASE_URL"); value != "" {
		return value
	}

	return defaultWalletOpeningTestDatabaseURL
}

func newTestEnvironment(
	t *testing.T,
) *testEnvironment {
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
			MaxConnections: 20,
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
			"acquire integration lock connection: %v",
			err,
		)
	}

	if _, err := conn.Exec(
		context.Background(),
		"SELECT pg_advisory_lock($1)",
		walletOpeningIntegrationTestLockID,
	); err != nil {
		conn.Release()
		pool.Close()

		t.Fatalf(
			"acquire integration lock: %v",
			err,
		)
	}

	t.Cleanup(
		func() {
			_, _ = conn.Exec(
				context.Background(),
				"SELECT pg_advisory_unlock($1)",
				walletOpeningIntegrationTestLockID,
			)

			conn.Release()
			pool.Close()
		},
	)

	_, err = pool.Exec(
		context.Background(),
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

	walletRepository, err :=
		walletpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf(
			"create wallet repository: %v",
			err,
		)
	}

	transactionRepository, err :=
		transactionpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf(
			"create transaction repository: %v",
			err,
		)
	}

	ledgerRepository, err :=
		ledgerpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf(
			"create ledger repository: %v",
			err,
		)
	}

	outboxRepository, err :=
		outboxpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf(
			"create outbox repository: %v",
			err,
		)
	}

	transactionManager, err :=
		postgresinfra.NewTransactionManager(pool)
	if err != nil {
		t.Fatalf(
			"create transaction manager: %v",
			err,
		)
	}

	service, err :=
		applicationwalletopening.NewService(
			transactionManager,
			walletRepository,
			transactionRepository,
			ledgerRepository,
			outboxRepository,
		)
	if err != nil {
		t.Fatalf(
			"create wallet opening service: %v",
			err,
		)
	}

	return &testEnvironment{
		pool:                  pool,
		service:               service,
		walletRepository:      walletRepository,
		transactionRepository: transactionRepository,
		ledgerRepository:      ledgerRepository,
		outboxRepository:      outboxRepository,
	}
}

func mustMoney(
	t *testing.T,
	amount int64,
) domainmoney.Money {
	t.Helper()

	value, err :=
		domainmoney.New(
			amount,
			"BRL",
		)
	if err != nil {
		t.Fatalf(
			"create money: %v",
			err,
		)
	}

	return value
}

func TestServiceCreatesWalletOpeningLedgerAndOutboxAtomically(
	t *testing.T,
) {
	env := newTestEnvironment(t)

	now :=
		time.Now().
			UTC().
			Truncate(time.Microsecond)

	result, err :=
		env.service.Create(
			context.Background(),
			applicationwalletopening.CreateInput{
				WalletID:       "wallet-opening-success",
				PlayerID:       "player-opening-success",
				TransactionID:  "transaction-opening-success",
				LedgerID:       "ledger-opening-success",
				InitialBalance: mustMoney(t, 10000),
				CreatedAt:      now,
			},
		)
	if err != nil {
		t.Fatalf(
			"create wallet: %v",
			err,
		)
	}

	if result.Wallet.ID() != "wallet-opening-success" {
		t.Fatalf(
			"wallet id = %q, want %q",
			result.Wallet.ID(),
			"wallet-opening-success",
		)
	}

	if result.Wallet.PlayerID() !=
		"player-opening-success" {
		t.Fatalf(
			"player id = %q, want %q",
			result.Wallet.PlayerID(),
			"player-opening-success",
		)
	}

	if result.Wallet.Balance().Amount() != 10000 {
		t.Fatalf(
			"balance = %d, want 10000",
			result.Wallet.Balance().Amount(),
		)
	}

	var walletCount int

	if err := env.pool.QueryRow(
		context.Background(),
		`
		SELECT COUNT(*)
		FROM wallets
		WHERE id = $1
		`,
		"wallet-opening-success",
	).Scan(&walletCount); err != nil {
		t.Fatalf(
			"count wallets: %v",
			err,
		)
	}

	if walletCount != 1 {
		t.Fatalf(
			"wallet count = %d, want 1",
			walletCount,
		)
	}

	var (
		transactionKind    string
		transactionStatus  string
		transactionAmount  int64
		transactionBalance int64
	)

	if err := env.pool.QueryRow(
		context.Background(),
		`
		SELECT
			kind,
			status,
			amount,
			result_balance
		FROM wager_transactions
		WHERE id = $1
		`,
		"transaction-opening-success",
	).Scan(
		&transactionKind,
		&transactionStatus,
		&transactionAmount,
		&transactionBalance,
	); err != nil {
		t.Fatalf(
			"find opening transaction: %v",
			err,
		)
	}

	if transactionKind != "OPENING" {
		t.Fatalf(
			"transaction kind = %q, want OPENING",
			transactionKind,
		)
	}

	if transactionStatus != "PROCESSED" {
		t.Fatalf(
			"transaction status = %q, want PROCESSED",
			transactionStatus,
		)
	}

	if transactionAmount != 10000 {
		t.Fatalf(
			"transaction amount = %d, want 10000",
			transactionAmount,
		)
	}

	if transactionBalance != 10000 {
		t.Fatalf(
			"transaction result balance = %d, want 10000",
			transactionBalance,
		)
	}

	var (
		direction     string
		ledgerAmount  int64
		balanceBefore int64
		balanceAfter  int64
	)

	if err := env.pool.QueryRow(
		context.Background(),
		`
		SELECT
			direction,
			amount,
			balance_before,
			balance_after
		FROM ledger_entries
		WHERE id = $1
		`,
		"ledger-opening-success",
	).Scan(
		&direction,
		&ledgerAmount,
		&balanceBefore,
		&balanceAfter,
	); err != nil {
		t.Fatalf(
			"find opening ledger entry: %v",
			err,
		)
	}

	if direction != "CREDIT" {
		t.Fatalf(
			"ledger direction = %q, want CREDIT",
			direction,
		)
	}

	if ledgerAmount != 10000 {
		t.Fatalf(
			"ledger amount = %d, want 10000",
			ledgerAmount,
		)
	}

	if balanceBefore != 0 {
		t.Fatalf(
			"balance before = %d, want 0",
			balanceBefore,
		)
	}

	if balanceAfter != 10000 {
		t.Fatalf(
			"balance after = %d, want 10000",
			balanceAfter,
		)
	}

	rows, err := env.pool.Query(
		context.Background(),
		`
		SELECT
			event_type,
			aggregate_type,
			aggregate_id,
			payload,
			status
		FROM outbox_events
		ORDER BY created_at, id
		`,
	)
	if err != nil {
		t.Fatalf(
			"query outbox events: %v",
			err,
		)
	}
	defer rows.Close()

	type storedEvent struct {
		eventType     string
		aggregateType string
		aggregateID   string
		payload       []byte
		status        string
	}

	events := make(
		[]storedEvent,
		0,
		2,
	)

	for rows.Next() {
		var event storedEvent

		if err := rows.Scan(
			&event.eventType,
			&event.aggregateType,
			&event.aggregateID,
			&event.payload,
			&event.status,
		); err != nil {
			t.Fatalf(
				"scan outbox event: %v",
				err,
			)
		}

		events = append(
			events,
			event,
		)
	}

	if err := rows.Err(); err != nil {
		t.Fatalf(
			"iterate outbox events: %v",
			err,
		)
	}

	if len(events) != 2 {
		t.Fatalf(
			"outbox event count = %d, want 2",
			len(events),
		)
	}

	eventTypes := make(
		map[string]storedEvent,
		len(events),
	)

	for _, event := range events {
		if event.status != "PENDING" {
			t.Fatalf(
				"outbox status = %q, want PENDING",
				event.status,
			)
		}

		eventTypes[event.eventType] = event
	}

	processedEvent, ok :=
		eventTypes["WagerTransactionProcessed"]
	if !ok {
		t.Fatal(
			"WagerTransactionProcessed event not found",
		)
	}

	if processedEvent.aggregateType !=
		"WAGER_TRANSACTION" {
		t.Fatalf(
			"processed aggregate type = %q, want WAGER_TRANSACTION",
			processedEvent.aggregateType,
		)
	}

	if processedEvent.aggregateID !=
		"transaction-opening-success" {
		t.Fatalf(
			"processed aggregate id = %q, want transaction-opening-success",
			processedEvent.aggregateID,
		)
	}

	var processedPayload struct {
		EventID       string `json:"eventId"`
		EventType     string `json:"eventType"`
		AggregateID   string `json:"aggregateId"`
		CorrelationID string `json:"correlationId"`
		Version       int    `json:"version"`
		Data          struct {
			TransactionID string `json:"transactionId"`
			WalletID      string `json:"walletId"`
			PlayerID      string `json:"playerId"`
			Kind          string `json:"kind"`
			Money         struct {
				Amount   string `json:"amount"`
				Currency string `json:"currency"`
			} `json:"money"`
			ResultBalance struct {
				Amount   string `json:"amount"`
				Currency string `json:"currency"`
			} `json:"resultBalance"`
		} `json:"data"`
	}

	if err := json.Unmarshal(
		processedEvent.payload,
		&processedPayload,
	); err != nil {
		t.Fatalf(
			"decode processed event payload: %v",
			err,
		)
	}

	if processedPayload.Data.TransactionID !=
		"transaction-opening-success" {
		t.Fatalf(
			"payload transaction id = %q",
			processedPayload.Data.TransactionID,
		)
	}

	if processedPayload.Data.Kind != "OPENING" {
		t.Fatalf(
			"payload kind = %q, want OPENING",
			processedPayload.Data.Kind,
		)
	}

	if processedPayload.Data.Money.Amount != "100.00" {
		t.Fatalf(
			"payload amount = %q, want 100.00",
			processedPayload.Data.Money.Amount,
		)
	}

	if processedPayload.Data.ResultBalance.Amount !=
		"100.00" {
		t.Fatalf(
			"payload result balance = %q, want 100.00",
			processedPayload.Data.ResultBalance.Amount,
		)
	}

	balanceEvent, ok :=
		eventTypes["WalletBalanceChanged"]
	if !ok {
		t.Fatal(
			"WalletBalanceChanged event not found",
		)
	}

	if balanceEvent.aggregateType != "WALLET" {
		t.Fatalf(
			"balance aggregate type = %q, want WALLET",
			balanceEvent.aggregateType,
		)
	}

	if balanceEvent.aggregateID !=
		"wallet-opening-success" {
		t.Fatalf(
			"balance aggregate id = %q, want wallet-opening-success",
			balanceEvent.aggregateID,
		)
	}

	var balancePayload struct {
		Data struct {
			WalletID      string `json:"walletId"`
			TransactionID string `json:"transactionId"`
			Direction     string `json:"direction"`
			Money         struct {
				Amount   string `json:"amount"`
				Currency string `json:"currency"`
			} `json:"money"`
			BalanceBefore struct {
				Amount   string `json:"amount"`
				Currency string `json:"currency"`
			} `json:"balanceBefore"`
			BalanceAfter struct {
				Amount   string `json:"amount"`
				Currency string `json:"currency"`
			} `json:"balanceAfter"`
			WalletVersion uint64 `json:"walletVersion"`
		} `json:"data"`
	}

	if err := json.Unmarshal(
		balanceEvent.payload,
		&balancePayload,
	); err != nil {
		t.Fatalf(
			"decode balance event payload: %v",
			err,
		)
	}

	if balancePayload.Data.WalletID !=
		"wallet-opening-success" {
		t.Fatalf(
			"balance payload wallet id = %q",
			balancePayload.Data.WalletID,
		)
	}

	if balancePayload.Data.Direction != "CREDIT" {
		t.Fatalf(
			"balance direction = %q, want CREDIT",
			balancePayload.Data.Direction,
		)
	}

	if balancePayload.Data.BalanceBefore.Amount !=
		"0.00" {
		t.Fatalf(
			"balance before = %q, want 0.00",
			balancePayload.Data.BalanceBefore.Amount,
		)
	}

	if balancePayload.Data.BalanceAfter.Amount !=
		"100.00" {
		t.Fatalf(
			"balance after = %q, want 100.00",
			balancePayload.Data.BalanceAfter.Amount,
		)
	}
}

func TestServiceCreatesZeroBalanceWalletWithoutFinancialEvents(
	t *testing.T,
) {
	env := newTestEnvironment(t)

	now :=
		time.Now().
			UTC().
			Truncate(time.Microsecond)

	result, err :=
		env.service.Create(
			context.Background(),
			applicationwalletopening.CreateInput{
				WalletID:       "wallet-opening-zero",
				PlayerID:       "player-opening-zero",
				TransactionID:  "transaction-opening-zero",
				LedgerID:       "ledger-opening-zero",
				InitialBalance: mustMoney(t, 0),
				CreatedAt:      now,
			},
		)
	if err != nil {
		t.Fatalf(
			"create zero balance wallet: %v",
			err,
		)
	}

	if result.Wallet.Balance().Amount() != 0 {
		t.Fatalf(
			"balance = %d, want 0",
			result.Wallet.Balance().Amount(),
		)
	}

	var (
		walletCount      int
		transactionCount int
		ledgerCount      int
		outboxCount      int
	)

	if err := env.pool.QueryRow(
		context.Background(),
		"SELECT COUNT(*) FROM wallets",
	).Scan(&walletCount); err != nil {
		t.Fatalf("count wallets: %v", err)
	}

	if err := env.pool.QueryRow(
		context.Background(),
		"SELECT COUNT(*) FROM wager_transactions",
	).Scan(&transactionCount); err != nil {
		t.Fatalf(
			"count transactions: %v",
			err,
		)
	}

	if err := env.pool.QueryRow(
		context.Background(),
		"SELECT COUNT(*) FROM ledger_entries",
	).Scan(&ledgerCount); err != nil {
		t.Fatalf(
			"count ledger entries: %v",
			err,
		)
	}

	if err := env.pool.QueryRow(
		context.Background(),
		"SELECT COUNT(*) FROM outbox_events",
	).Scan(&outboxCount); err != nil {
		t.Fatalf(
			"count outbox events: %v",
			err,
		)
	}

	if walletCount != 1 {
		t.Fatalf(
			"wallet count = %d, want 1",
			walletCount,
		)
	}

	if transactionCount != 0 {
		t.Fatalf(
			"transaction count = %d, want 0",
			transactionCount,
		)
	}

	if ledgerCount != 0 {
		t.Fatalf(
			"ledger count = %d, want 0",
			ledgerCount,
		)
	}

	if outboxCount != 0 {
		t.Fatalf(
			"outbox count = %d, want 0",
			outboxCount,
		)
	}
}

func TestServiceRejectsDuplicatePlayerCurrencyWithoutAdditionalWrites(
	t *testing.T,
) {
	env := newTestEnvironment(t)

	now :=
		time.Now().
			UTC().
			Truncate(time.Microsecond)

	firstInput :=
		applicationwalletopening.CreateInput{
			WalletID:       "wallet-opening-first",
			PlayerID:       "player-opening-duplicate",
			TransactionID:  "transaction-opening-first",
			LedgerID:       "ledger-opening-first",
			InitialBalance: mustMoney(t, 10000),
			CreatedAt:      now,
		}

	if _, err :=
		env.service.Create(
			context.Background(),
			firstInput,
		); err != nil {
		t.Fatalf(
			"create first wallet: %v",
			err,
		)
	}

	secondInput :=
		applicationwalletopening.CreateInput{
			WalletID:       "wallet-opening-second",
			PlayerID:       "player-opening-duplicate",
			TransactionID:  "transaction-opening-second",
			LedgerID:       "ledger-opening-second",
			InitialBalance: mustMoney(t, 5000),
			CreatedAt:      now.Add(time.Second),
		}

	_, err :=
		env.service.Create(
			context.Background(),
			secondInput,
		)

	if !errors.Is(
		err,
		applicationwalletopening.ErrWalletAlreadyExists,
	) {
		t.Fatalf(
			"error = %v, want ErrWalletAlreadyExists",
			err,
		)
	}

	var (
		walletCount      int
		transactionCount int
		ledgerCount      int
		outboxCount      int
	)

	if err := env.pool.QueryRow(
		context.Background(),
		"SELECT COUNT(*) FROM wallets",
	).Scan(&walletCount); err != nil {
		t.Fatalf("count wallets: %v", err)
	}

	if err := env.pool.QueryRow(
		context.Background(),
		"SELECT COUNT(*) FROM wager_transactions",
	).Scan(&transactionCount); err != nil {
		t.Fatalf(
			"count transactions: %v",
			err,
		)
	}

	if err := env.pool.QueryRow(
		context.Background(),
		"SELECT COUNT(*) FROM ledger_entries",
	).Scan(&ledgerCount); err != nil {
		t.Fatalf(
			"count ledger entries: %v",
			err,
		)
	}

	if err := env.pool.QueryRow(
		context.Background(),
		"SELECT COUNT(*) FROM outbox_events",
	).Scan(&outboxCount); err != nil {
		t.Fatalf(
			"count outbox events: %v",
			err,
		)
	}

	if walletCount != 1 {
		t.Fatalf(
			"wallet count = %d, want 1",
			walletCount,
		)
	}

	if transactionCount != 1 {
		t.Fatalf(
			"transaction count = %d, want 1",
			transactionCount,
		)
	}

	if ledgerCount != 1 {
		t.Fatalf(
			"ledger count = %d, want 1",
			ledgerCount,
		)
	}

	if outboxCount != 2 {
		t.Fatalf(
			"outbox count = %d, want 2",
			outboxCount,
		)
	}
}
