package wager

import (
	"context"
	"errors"
	"fmt"
	"time"

	domainwager "jungle_gaming_teste_tecnico/internal/domain/wager"
	domaintransaction "jungle_gaming_teste_tecnico/internal/domain/wagertransaction"
	postgresinfra "jungle_gaming_teste_tecnico/internal/infrastructure/postgres"
	ledgerpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/ledger"
	transactionpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/wagertransaction"
	walletpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/wallet"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrServiceRequired         = errors.New("wager service dependencies are required")
	ErrTransactionRequired     = errors.New("wager transaction is required")
	ErrReferenceRequired       = errors.New("reference transaction is required")
	ErrNotPendingReference     = errors.New("transaction is not pending reference")
	ErrIdempotencyConflict     = errors.New("idempotency key already used with different payload")
	ErrExternalIDConflict      = errors.New("external transaction id already used with different operation")
	ErrConcurrentRetryExceeded = errors.New("concurrent wallet update retry limit exceeded")
)

const defaultMaxRetries = 10

type Service struct {
	transactionManager    *postgresinfra.TransactionManager
	walletRepository      *walletpostgres.Repository
	transactionRepository *transactionpostgres.Repository
	ledgerRepository      *ledgerpostgres.Repository
	processor             domainwager.Processor
	maxRetries            int
}

type ProcessInput struct {
	Transaction   *domaintransaction.WagerTransaction
	Reference     *domaintransaction.WagerTransaction
	LedgerEntryID string
	OccurredAt    time.Time
}

type ProcessResult struct {
	Transaction *domaintransaction.WagerTransaction
	Balance     int64
	Currency    string
	Replayed    bool
}

func NewService(
	transactionManager *postgresinfra.TransactionManager,
	walletRepository *walletpostgres.Repository,
	transactionRepository *transactionpostgres.Repository,
	ledgerRepository *ledgerpostgres.Repository,
) (*Service, error) {
	if transactionManager == nil ||
		walletRepository == nil ||
		transactionRepository == nil ||
		ledgerRepository == nil {
		return nil, ErrServiceRequired
	}

	return &Service{
		transactionManager:    transactionManager,
		walletRepository:      walletRepository,
		transactionRepository: transactionRepository,
		ledgerRepository:      ledgerRepository,
		processor:             domainwager.NewProcessor(),
		maxRetries:            defaultMaxRetries,
	}, nil
}

func (s *Service) Process(
	ctx context.Context,
	input ProcessInput,
) (ProcessResult, error) {
	if input.Transaction == nil {
		return ProcessResult{}, ErrTransactionRequired
	}

	for attempt := 0; attempt < s.maxRetries; attempt++ {
		result, retry, err := s.processAttempt(ctx, input)
		if err == nil {
			return result, nil
		}

		if !retry {
			return ProcessResult{}, err
		}

		if err := waitForRetry(ctx, attempt); err != nil {
			return ProcessResult{}, err
		}
	}

	return ProcessResult{}, ErrConcurrentRetryExceeded
}

func (s *Service) ResumePendingReference(
	ctx context.Context,
	transactionID string,
	ledgerEntryID string,
	occurredAt time.Time,
) (ProcessResult, error) {
	for attempt := 0; attempt < s.maxRetries; attempt++ {
		result, retry, err := s.resumePendingReferenceAttempt(
			ctx,
			transactionID,
			ledgerEntryID,
			occurredAt,
		)
		if err == nil {
			return result, nil
		}

		if !retry {
			return ProcessResult{}, err
		}

		if err := waitForRetry(ctx, attempt); err != nil {
			return ProcessResult{}, err
		}
	}

	return ProcessResult{}, ErrConcurrentRetryExceeded
}

func (s *Service) processAttempt(
	ctx context.Context,
	input ProcessInput,
) (ProcessResult, bool, error) {
	var result ProcessResult

	err := s.transactionManager.WithinTransaction(
		ctx,
		func(ctx context.Context, tx pgx.Tx) error {
			txWalletRepository, err :=
				s.walletRepository.WithDB(tx)
			if err != nil {
				return err
			}

			txTransactionRepository, err :=
				s.transactionRepository.WithDB(tx)
			if err != nil {
				return err
			}

			txLedgerRepository, err :=
				s.ledgerRepository.WithDB(tx)
			if err != nil {
				return err
			}

			existing, err :=
				txTransactionRepository.FindByProviderIdempotencyKey(
					ctx,
					input.Transaction.ProviderID(),
					input.Transaction.IdempotencyKey(),
				)

			switch {
			case err == nil:
				replay, err := replayResult(
					existing,
					input.Transaction,
					ErrIdempotencyConflict,
				)
				if err != nil {
					return err
				}

				result = replay
				return nil

			case !errors.Is(err, transactionpostgres.ErrNotFound):
				return err
			}

			existing, err =
				txTransactionRepository.FindByProviderExternalID(
					ctx,
					input.Transaction.ProviderID(),
					input.Transaction.ExternalTransactionID(),
				)

			switch {
			case err == nil:
				replay, err := replayResult(
					existing,
					input.Transaction,
					ErrExternalIDConflict,
				)
				if err != nil {
					return err
				}

				result = replay
				return nil

			case !errors.Is(err, transactionpostgres.ErrNotFound):
				return err
			}

			walletEntity, err :=
				txWalletRepository.FindByID(
					ctx,
					input.Transaction.WalletID(),
				)
			if err != nil {
				return err
			}

			expectedVersion := walletEntity.Version()

			transactionCopy := *input.Transaction

			var referenceCopy *domaintransaction.WagerTransaction
			if input.Reference != nil {
				value := *input.Reference
				referenceCopy = &value
			}

			domainResult, err := s.processor.Process(
				walletEntity,
				&transactionCopy,
				referenceCopy,
				input.LedgerEntryID,
				input.OccurredAt,
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
				&transactionCopy,
			); err != nil {
				return err
			}

			entry, hasLedger := domainResult.LedgerEntry()
			if hasLedger {
				if err := txLedgerRepository.Create(
					ctx,
					&entry,
				); err != nil {
					return err
				}
			}

			result = ProcessResult{
				Transaction: &transactionCopy,
				Balance:     domainResult.Balance().Amount(),
				Currency:    domainResult.Balance().Currency(),
				Replayed:    false,
			}

			return nil
		},
	)

	if err == nil {
		return result, false, nil
	}

	if errors.Is(err, walletpostgres.ErrConcurrentUpdate) {
		return ProcessResult{}, true, err
	}

	if isUniqueViolation(err) {
		return ProcessResult{}, true, err
	}

	return ProcessResult{}, false, err
}

func (s *Service) resumePendingReferenceAttempt(
	ctx context.Context,
	transactionID string,
	ledgerEntryID string,
	occurredAt time.Time,
) (ProcessResult, bool, error) {
	var result ProcessResult

	err := s.transactionManager.WithinTransaction(
		ctx,
		func(ctx context.Context, tx pgx.Tx) error {
			txWalletRepository, err :=
				s.walletRepository.WithDB(tx)
			if err != nil {
				return err
			}

			txTransactionRepository, err :=
				s.transactionRepository.WithDB(tx)
			if err != nil {
				return err
			}

			txLedgerRepository, err :=
				s.ledgerRepository.WithDB(tx)
			if err != nil {
				return err
			}

			pending, err :=
				txTransactionRepository.FindByID(
					ctx,
					transactionID,
				)
			if err != nil {
				return err
			}

			// Outra instância pode ter concluído enquanto este worker
			// aguardava para executar.
			if pending.Status() == domaintransaction.StatusProcessed {
				balance, ok := pending.ResultBalance()
				if !ok {
					return fmt.Errorf(
						"processed transaction has no result balance",
					)
				}

				result = ProcessResult{
					Transaction: pending,
					Balance:     balance.Amount(),
					Currency:    balance.Currency(),
					Replayed:    true,
				}

				return nil
			}

			if pending.Status() !=
				domaintransaction.StatusPendingReference {
				return ErrNotPendingReference
			}

			if pending.ReferenceExternalTransactionID() == "" {
				return ErrReferenceRequired
			}

			reference, err :=
				txTransactionRepository.FindByProviderExternalID(
					ctx,
					pending.ProviderID(),
					pending.ReferenceExternalTransactionID(),
				)
			if errors.Is(err, transactionpostgres.ErrNotFound) {
				return ErrReferenceRequired
			}
			if err != nil {
				return err
			}

			walletEntity, err :=
				txWalletRepository.FindByID(
					ctx,
					pending.WalletID(),
				)
			if err != nil {
				return err
			}

			expectedVersion := walletEntity.Version()

			pendingCopy := *pending
			referenceCopy := *reference

			domainResult, err := s.processor.Process(
				walletEntity,
				&pendingCopy,
				&referenceCopy,
				ledgerEntryID,
				occurredAt,
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

			// Aqui a operação já existe no banco.
			// Portanto é UPDATE, não CREATE.
			if err := txTransactionRepository.Update(
				ctx,
				&pendingCopy,
			); err != nil {
				return err
			}

			entry, hasLedger := domainResult.LedgerEntry()
			if hasLedger {
				if err := txLedgerRepository.Create(
					ctx,
					&entry,
				); err != nil {
					return err
				}
			}

			result = ProcessResult{
				Transaction: &pendingCopy,
				Balance:     domainResult.Balance().Amount(),
				Currency:    domainResult.Balance().Currency(),
				Replayed:    false,
			}

			return nil
		},
	)

	if err == nil {
		return result, false, nil
	}

	if errors.Is(err, walletpostgres.ErrConcurrentUpdate) {
		return ProcessResult{}, true, err
	}

	if isLedgerUniqueViolation(err) {
		// Outra instância pode ter concluído a mesma pendência.
		return ProcessResult{}, true, err
	}

	return ProcessResult{}, false, err
}

func replayResult(
	existing *domaintransaction.WagerTransaction,
	incoming *domaintransaction.WagerTransaction,
	conflictError error,
) (ProcessResult, error) {
	if existing.PayloadHash() != incoming.PayloadHash() {
		return ProcessResult{}, conflictError
	}

	balance, ok := existing.ResultBalance()
	if !ok {
		return ProcessResult{}, fmt.Errorf(
			"idempotent transaction has no result balance",
		)
	}

	return ProcessResult{
		Transaction: existing,
		Balance:     balance.Amount(),
		Currency:    balance.Currency(),
		Replayed:    true,
	}, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError

	if !errors.As(err, &pgErr) {
		return false
	}

	if pgErr.Code != "23505" {
		return false
	}

	switch pgErr.ConstraintName {
	case "wager_transactions_provider_external_unique",
		"wager_transactions_provider_idempotency_unique":
		return true
	default:
		return false
	}
}

func isLedgerUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError

	if !errors.As(err, &pgErr) {
		return false
	}

	if pgErr.Code != "23505" {
		return false
	}

	switch pgErr.ConstraintName {
	case "ledger_entries_pkey",
		"ledger_entries_wallet_transaction_unique":
		return true
	default:
		return false
	}
}

func waitForRetry(
	ctx context.Context,
	attempt int,
) error {
	delay := time.Duration(attempt+1) * 5 * time.Millisecond

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()

	case <-timer.C:
		return nil
	}
}
