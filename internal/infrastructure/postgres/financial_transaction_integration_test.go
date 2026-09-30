package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	domainledger "jungle_gaming_teste_tecnico/internal/domain/ledger"
	domainmoney "jungle_gaming_teste_tecnico/internal/domain/money"
	domaintransaction "jungle_gaming_teste_tecnico/internal/domain/wagertransaction"
	domainwallet "jungle_gaming_teste_tecnico/internal/domain/wallet"
	postgresinfra "jungle_gaming_teste_tecnico/internal/infrastructure/postgres"
	ledgerpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/ledger"
	transactionpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/wagertransaction"
	walletpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/wallet"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const financialIntegrationLockID int64 = 8675309

func lockFinancialIntegrationDatabase(
	t *testing.T,
	pool *pgxpool.Pool,
) {
	t.Helper()

	conn, err := pool.Acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire postgres connection: %v", err)
	}

	if _, err := conn.Exec(
		context.Background(),
		"SELECT pg_advisory_lock($1)",
		financialIntegrationLockID,
	); err != nil {
		conn.Release()
		t.Fatalf("acquire integration lock: %v", err)
	}

	t.Cleanup(func() {
		_, _ = conn.Exec(
			context.Background(),
			"SELECT pg_advisory_unlock($1)",
			financialIntegrationLockID,
		)

		conn.Release()
	})
}

func cleanFinancialDatabase(
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

func financialMoney(
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

func createFinancialWallet(
	t *testing.T,
	repository *walletpostgres.Repository,
	ctx context.Context,
	id string,
	playerID string,
	balance int64,
	now time.Time,
) {
	t.Helper()

	entity, err := domainwallet.New(
		id,
		playerID,
		financialMoney(t, balance),
		now,
	)
	if err != nil {
		t.Fatalf("create wallet domain entity: %v", err)
	}

	if err := repository.Create(ctx, &entity); err != nil {
		t.Fatalf("persist wallet: %v", err)
	}
}

func TestFinancialTransactionCommit(t *testing.T) {
	pool := newTestPool(t)

	lockFinancialIntegrationDatabase(t, pool)
	cleanFinancialDatabase(t, pool)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	walletRepository, err := walletpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create wallet repository: %v", err)
	}

	transactionRepository, err :=
		transactionpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create transaction repository: %v", err)
	}

	ledgerRepository, err := ledgerpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create ledger repository: %v", err)
	}

	manager, err := postgresinfra.NewTransactionManager(pool)
	if err != nil {
		t.Fatalf("create transaction manager: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)

	const (
		walletID      = "wallet-financial-commit"
		playerID      = "player-financial-commit"
		transactionID = "transaction-financial-commit"
		ledgerID      = "ledger-financial-commit"
	)

	createFinancialWallet(
		t,
		walletRepository,
		ctx,
		walletID,
		playerID,
		10000,
		now,
	)

	err = manager.WithinTransaction(
		ctx,
		func(ctx context.Context, tx pgx.Tx) error {
			txWalletRepository, err :=
				walletRepository.WithDB(tx)
			if err != nil {
				return err
			}

			txTransactionRepository, err :=
				transactionRepository.WithDB(tx)
			if err != nil {
				return err
			}

			txLedgerRepository, err :=
				ledgerRepository.WithDB(tx)
			if err != nil {
				return err
			}

			walletEntity, err :=
				txWalletRepository.FindByID(ctx, walletID)
			if err != nil {
				return err
			}

			expectedVersion := walletEntity.Version()
			balanceBefore := walletEntity.Balance()
			betAmount := financialMoney(t, 2000)

			if err := walletEntity.Debit(
				betAmount,
				now.Add(time.Second),
			); err != nil {
				return err
			}

			transactionEntity, err :=
				domaintransaction.NewExternal(
					domaintransaction.ExternalInput{
						ID:                    transactionID,
						ExternalTransactionID: "external-financial-commit",
						ProviderID:            "provider-financial",
						IdempotencyKey:        "idempotency-financial-commit",
						PayloadHash:           "payload-financial-commit",
						WalletID:              walletID,
						PlayerID:              playerID,
						RoundID:               "round-financial-commit",
						GameID:                "game-financial-commit",
						Kind:                  domaintransaction.KindBet,
						Money:                 betAmount,
						CreatedAt:             now.Add(time.Second),
					},
				)
			if err != nil {
				return err
			}

			if err := transactionEntity.MarkProcessed(
				walletEntity.Balance(),
				now.Add(time.Second),
			); err != nil {
				return err
			}

			ledgerEntry, err := domainledger.New(
				ledgerID,
				walletID,
				transactionID,
				domainledger.DirectionDebit,
				betAmount,
				balanceBefore,
				walletEntity.Balance(),
				now.Add(time.Second),
			)
			if err != nil {
				return err
			}

			if err := txWalletRepository.Update(
				ctx,
				walletEntity,
				expectedVersion,
			); err != nil {
				return err
			}

			if err := txTransactionRepository.Create(
				ctx,
				&transactionEntity,
			); err != nil {
				return err
			}

			if err := txLedgerRepository.Create(
				ctx,
				&ledgerEntry,
			); err != nil {
				return err
			}

			return nil
		},
	)
	if err != nil {
		t.Fatalf("execute financial transaction: %v", err)
	}

	persistedWallet, err :=
		walletRepository.FindByID(ctx, walletID)
	if err != nil {
		t.Fatalf("find persisted wallet: %v", err)
	}

	if persistedWallet.Balance().Amount() != 8000 {
		t.Fatalf(
			"wallet balance = %d, want 8000",
			persistedWallet.Balance().Amount(),
		)
	}

	if persistedWallet.Version() != 2 {
		t.Fatalf(
			"wallet version = %d, want 2",
			persistedWallet.Version(),
		)
	}

	persistedTransaction, err :=
		transactionRepository.FindByID(
			ctx,
			transactionID,
		)
	if err != nil {
		t.Fatalf("find persisted transaction: %v", err)
	}

	if persistedTransaction.Status() !=
		domaintransaction.StatusProcessed {
		t.Fatalf(
			"transaction status = %s, want %s",
			persistedTransaction.Status(),
			domaintransaction.StatusProcessed,
		)
	}

	resultBalance, ok :=
		persistedTransaction.ResultBalance()
	if !ok {
		t.Fatal("transaction result balance not persisted")
	}

	if resultBalance.Amount() != 8000 {
		t.Fatalf(
			"result balance = %d, want 8000",
			resultBalance.Amount(),
		)
	}

	persistedLedger, err :=
		ledgerRepository.FindByID(ctx, ledgerID)
	if err != nil {
		t.Fatalf("find persisted ledger entry: %v", err)
	}

	if persistedLedger.BalanceBefore().Amount() != 10000 {
		t.Fatalf(
			"ledger balance before = %d, want 10000",
			persistedLedger.BalanceBefore().Amount(),
		)
	}

	if persistedLedger.BalanceAfter().Amount() != 8000 {
		t.Fatalf(
			"ledger balance after = %d, want 8000",
			persistedLedger.BalanceAfter().Amount(),
		)
	}

	if persistedLedger.Amount().Amount() != 2000 {
		t.Fatalf(
			"ledger amount = %d, want 2000",
			persistedLedger.Amount().Amount(),
		)
	}
}

func TestFinancialTransactionRollback(t *testing.T) {
	pool := newTestPool(t)

	lockFinancialIntegrationDatabase(t, pool)
	cleanFinancialDatabase(t, pool)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	walletRepository, err := walletpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create wallet repository: %v", err)
	}

	transactionRepository, err :=
		transactionpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create transaction repository: %v", err)
	}

	ledgerRepository, err := ledgerpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf("create ledger repository: %v", err)
	}

	manager, err := postgresinfra.NewTransactionManager(pool)
	if err != nil {
		t.Fatalf("create transaction manager: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)

	const (
		walletID      = "wallet-financial-rollback"
		playerID      = "player-financial-rollback"
		transactionID = "transaction-financial-rollback"
		ledgerID      = "ledger-financial-rollback"
	)

	createFinancialWallet(
		t,
		walletRepository,
		ctx,
		walletID,
		playerID,
		10000,
		now,
	)

	err = manager.WithinTransaction(
		ctx,
		func(ctx context.Context, tx pgx.Tx) error {
			txWalletRepository, err :=
				walletRepository.WithDB(tx)
			if err != nil {
				return err
			}

			txTransactionRepository, err :=
				transactionRepository.WithDB(tx)
			if err != nil {
				return err
			}

			txLedgerRepository, err :=
				ledgerRepository.WithDB(tx)
			if err != nil {
				return err
			}

			walletEntity, err :=
				txWalletRepository.FindByID(ctx, walletID)
			if err != nil {
				return err
			}

			expectedVersion := walletEntity.Version()
			balanceBefore := walletEntity.Balance()
			betAmount := financialMoney(t, 2000)

			if err := walletEntity.Debit(
				betAmount,
				now.Add(time.Second),
			); err != nil {
				return err
			}

			transactionEntity, err :=
				domaintransaction.NewExternal(
					domaintransaction.ExternalInput{
						ID:                    transactionID,
						ExternalTransactionID: "external-financial-rollback",
						ProviderID:            "provider-financial",
						IdempotencyKey:        "idempotency-financial-rollback",
						PayloadHash:           "payload-financial-rollback",
						WalletID:              walletID,
						PlayerID:              playerID,
						RoundID:               "round-financial-rollback",
						GameID:                "game-financial-rollback",
						Kind:                  domaintransaction.KindBet,
						Money:                 betAmount,
						CreatedAt:             now.Add(time.Second),
					},
				)
			if err != nil {
				return err
			}

			if err := transactionEntity.MarkProcessed(
				walletEntity.Balance(),
				now.Add(time.Second),
			); err != nil {
				return err
			}

			ledgerEntry, err := domainledger.New(
				ledgerID,
				walletID,
				transactionID,
				domainledger.DirectionDebit,
				betAmount,
				balanceBefore,
				walletEntity.Balance(),
				now.Add(time.Second),
			)
			if err != nil {
				return err
			}

			if err := txWalletRepository.Update(
				ctx,
				walletEntity,
				expectedVersion,
			); err != nil {
				return err
			}

			if err := txTransactionRepository.Create(
				ctx,
				&transactionEntity,
			); err != nil {
				return err
			}

			if err := txLedgerRepository.Create(
				ctx,
				&ledgerEntry,
			); err != nil {
				return err
			}

			// Força falha após os três writes.
			// A segunda inserção viola a PK/UNIQUE do Ledger.
			if err := txLedgerRepository.Create(
				ctx,
				&ledgerEntry,
			); err != nil {
				return err
			}

			return nil
		},
	)

	if err == nil {
		t.Fatal("expected financial transaction to rollback")
	}

	persistedWallet, err :=
		walletRepository.FindByID(ctx, walletID)
	if err != nil {
		t.Fatalf("find wallet after rollback: %v", err)
	}

	if persistedWallet.Balance().Amount() != 10000 {
		t.Fatalf(
			"wallet balance after rollback = %d, want 10000",
			persistedWallet.Balance().Amount(),
		)
	}

	if persistedWallet.Version() != 1 {
		t.Fatalf(
			"wallet version after rollback = %d, want 1",
			persistedWallet.Version(),
		)
	}

	_, err = transactionRepository.FindByID(
		ctx,
		transactionID,
	)
	if !errors.Is(err, transactionpostgres.ErrNotFound) {
		t.Fatalf(
			"transaction should not exist after rollback: %v",
			err,
		)
	}

	_, err = ledgerRepository.FindByID(
		ctx,
		ledgerID,
	)
	if !errors.Is(err, ledgerpostgres.ErrNotFound) {
		t.Fatalf(
			"ledger should not exist after rollback: %v",
			err,
		)
	}
}
