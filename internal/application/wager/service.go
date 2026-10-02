package wager

import (
	"context"
	"errors"
	"fmt"
	"time"

	domainledger "jungle_gaming_teste_tecnico/internal/domain/ledger"
	domainwager "jungle_gaming_teste_tecnico/internal/domain/wager"
	domaintransaction "jungle_gaming_teste_tecnico/internal/domain/wagertransaction"
	domainwallet "jungle_gaming_teste_tecnico/internal/domain/wallet"
	postgresinfra "jungle_gaming_teste_tecnico/internal/infrastructure/postgres"
	ledgerpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/ledger"
	outboxpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/outbox"
	transactionpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/wagertransaction"
	walletpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/wallet"
	"jungle_gaming_teste_tecnico/internal/observability"

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
	ErrReversalConflict        = errors.New("reference transaction was already reversed")
	ErrConcurrentRetryExceeded = errors.New("concurrent wallet update retry limit exceeded")
)

const defaultMaxRetries = 10

type Service struct {
	transactionManager    *postgresinfra.TransactionManager
	walletRepository      *walletpostgres.Repository
	transactionRepository *transactionpostgres.Repository
	ledgerRepository      *ledgerpostgres.Repository
	outboxRepository      *outboxpostgres.Repository
	processor             domainwager.Processor
	maxRetries            int
}

type ProcessInput struct {
	Transaction   *domaintransaction.WagerTransaction
	Reference     *domaintransaction.WagerTransaction
	LedgerEntryID string
	CorrelationID string
	CausationID   string
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
	outboxRepository *outboxpostgres.Repository,
) (*Service, error) {
	if transactionManager == nil ||
		walletRepository == nil ||
		transactionRepository == nil ||
		ledgerRepository == nil ||
		outboxRepository == nil {
		return nil, ErrServiceRequired
	}

	return &Service{
		transactionManager:    transactionManager,
		walletRepository:      walletRepository,
		transactionRepository: transactionRepository,
		ledgerRepository:      ledgerRepository,
		outboxRepository:      outboxRepository,
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

			txOutboxRepository, err :=
				s.outboxRepository.WithDB(tx)
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

				if existing.Status() ==
					domaintransaction.StatusPendingReference {
					walletEntity, err :=
						txWalletRepository.FindByID(
							ctx,
							existing.WalletID(),
						)
					if err != nil {
						return err
					}

					replay.Balance =
						walletEntity.Balance().Amount()
					replay.Currency =
						walletEntity.Balance().Currency()
				}

				result = replay
				return nil

			case !errors.Is(
				err,
				transactionpostgres.ErrNotFound,
			):
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

				if existing.Status() ==
					domaintransaction.StatusPendingReference {
					walletEntity, err :=
						txWalletRepository.FindByID(
							ctx,
							existing.WalletID(),
						)
					if err != nil {
						return err
					}

					replay.Balance =
						walletEntity.Balance().Amount()
					replay.Currency =
						walletEntity.Balance().Currency()
				}

				result = replay
				return nil

			case !errors.Is(
				err,
				transactionpostgres.ErrNotFound,
			):
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

			referenceCopy, err := resolveProcessReference(
				ctx,
				txTransactionRepository,
				&transactionCopy,
				input.Reference,
			)
			if err != nil {
				return err
			}

			domainResult, err := s.processor.Process(
				walletEntity,
				&transactionCopy,
				referenceCopy,
				input.LedgerEntryID,
				input.OccurredAt,
			)

			if err != nil {
				handled, handlingErr := handleProcessorRejection(
					ctx, err, txTransactionRepository, txOutboxRepository,
					walletEntity, &transactionCopy, input.OccurredAt,
					eventMetadata{CorrelationID: input.CorrelationID, CausationID: input.CausationID},
					&result,
				)
				if handled {
					return handlingErr
				}
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

			var ledgerEntry *domainledger.Entry
			if hasLedger {
				ledgerEntry = &entry
			}

			if err := persistProcessedEvents(
				ctx,
				txOutboxRepository,
				&transactionCopy,
				ledgerEntry,
				walletEntity.Version(),
				input.OccurredAt,
				eventMetadata{
					CorrelationID: input.CorrelationID,
					CausationID:   input.CausationID,
				},
			); err != nil {
				return err
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

	if errors.Is(
		err,
		walletpostgres.ErrConcurrentUpdate,
	) {
		observability.Default.ConcurrencyConflicts.Add(1)
		return ProcessResult{}, true, err
	}

	if isReversalUniqueViolation(err) {
		return ProcessResult{}, false, ErrReversalConflict
	}

	if isUniqueViolation(err) {
		return ProcessResult{}, true, err
	}

	return ProcessResult{}, false, err
}

func (s *Service) ProcessInTransaction(
	ctx context.Context,
	tx pgx.Tx,
	input ProcessInput,
) (ProcessResult, error) {
	if input.Transaction == nil {
		return ProcessResult{}, ErrTransactionRequired
	}
	if tx == nil {
		return ProcessResult{}, postgresinfra.ErrTransactionRequired
	}

	var result ProcessResult
	err := func() error {
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

		txOutboxRepository, err :=
			s.outboxRepository.WithDB(tx)
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

			if existing.Status() ==
				domaintransaction.StatusPendingReference {
				walletEntity, err :=
					txWalletRepository.FindByID(
						ctx,
						existing.WalletID(),
					)
				if err != nil {
					return err
				}

				replay.Balance =
					walletEntity.Balance().Amount()
				replay.Currency =
					walletEntity.Balance().Currency()
			}

			result = replay
			return nil

		case !errors.Is(
			err,
			transactionpostgres.ErrNotFound,
		):
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

			if existing.Status() ==
				domaintransaction.StatusPendingReference {
				walletEntity, err :=
					txWalletRepository.FindByID(
						ctx,
						existing.WalletID(),
					)
				if err != nil {
					return err
				}

				replay.Balance =
					walletEntity.Balance().Amount()
				replay.Currency =
					walletEntity.Balance().Currency()
			}

			result = replay
			return nil

		case !errors.Is(
			err,
			transactionpostgres.ErrNotFound,
		):
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

		referenceCopy, err := resolveProcessReference(
			ctx,
			txTransactionRepository,
			&transactionCopy,
			input.Reference,
		)
		if err != nil {
			return err
		}

		domainResult, err := s.processor.Process(
			walletEntity,
			&transactionCopy,
			referenceCopy,
			input.LedgerEntryID,
			input.OccurredAt,
		)

		if err != nil {
			handled, handlingErr := handleProcessorRejection(
				ctx, err, txTransactionRepository, txOutboxRepository,
				walletEntity, &transactionCopy, input.OccurredAt,
				eventMetadata{CorrelationID: input.CorrelationID, CausationID: input.CausationID},
				&result,
			)
			if handled {
				return handlingErr
			}
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

		var ledgerEntry *domainledger.Entry
		if hasLedger {
			ledgerEntry = &entry
		}

		if err := persistProcessedEvents(
			ctx,
			txOutboxRepository,
			&transactionCopy,
			ledgerEntry,
			walletEntity.Version(),
			input.OccurredAt,
			eventMetadata{
				CorrelationID: input.CorrelationID,
				CausationID:   input.CausationID,
			},
		); err != nil {
			return err
		}

		result = ProcessResult{
			Transaction: &transactionCopy,
			Balance:     domainResult.Balance().Amount(),
			Currency:    domainResult.Balance().Currency(),
			Replayed:    false,
		}

		return nil
	}()
	if err != nil {
		if isReversalUniqueViolation(err) {
			return ProcessResult{}, ErrReversalConflict
		}
		return ProcessResult{}, err
	}

	return result, nil
}

func resolveProcessReference(
	ctx context.Context,
	transactionRepository *transactionpostgres.Repository,
	transaction *domaintransaction.WagerTransaction,
	providedReference *domaintransaction.WagerTransaction,
) (*domaintransaction.WagerTransaction, error) {
	if transaction == nil {
		return nil, ErrTransactionRequired
	}

	if !transaction.Kind().RequiresReference() &&
		transaction.ReferenceExternalTransactionID() == "" {
		return nil, nil
	}

	if providedReference != nil {
		value := *providedReference
		return &value, nil
	}

	referenceExternalID :=
		transaction.ReferenceExternalTransactionID()

	if referenceExternalID == "" {
		return nil, nil
	}

	reference, err :=
		transactionRepository.FindByProviderExternalID(
			ctx,
			transaction.ProviderID(),
			referenceExternalID,
		)

	if errors.Is(
		err,
		transactionpostgres.ErrNotFound,
	) {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	value := *reference

	return &value, nil
}

func persistPendingReference(
	ctx context.Context,
	transactionRepository *transactionpostgres.Repository,
	outboxRepository *outboxpostgres.Repository,
	walletEntity *domainwallet.Wallet,
	transaction *domaintransaction.WagerTransaction,
	occurredAt time.Time,
	metadata eventMetadata,
	result *ProcessResult,
) error {
	if err := transaction.MarkPendingReference(
		occurredAt,
	); err != nil {
		return err
	}

	if err := transactionRepository.Create(
		ctx,
		transaction,
	); err != nil {
		return err
	}

	if err := persistPendingReferenceEvent(
		ctx,
		outboxRepository,
		transaction,
		occurredAt,
		metadata,
	); err != nil {
		return err
	}

	*result = ProcessResult{
		Transaction: transaction,
		Balance:     walletEntity.Balance().Amount(),
		Currency:    walletEntity.Balance().Currency(),
		Replayed:    false,
	}

	return nil
}

func handleProcessorRejection(
	ctx context.Context,
	processorErr error,
	transactionRepository *transactionpostgres.Repository,
	outboxRepository *outboxpostgres.Repository,
	walletEntity *domainwallet.Wallet,
	transaction *domaintransaction.WagerTransaction,
	occurredAt time.Time,
	metadata eventMetadata,
	result *ProcessResult,
) (bool, error) {
	if (errors.Is(processorErr, domainwager.ErrReferenceRequired) ||
		errors.Is(processorErr, domainwager.ErrReferenceNotProcessed)) &&
		(transaction.Kind().RequiresReference() || transaction.ReferenceExternalTransactionID() != "") {
		return true, persistPendingReference(
			ctx, transactionRepository, outboxRepository, walletEntity, transaction,
			occurredAt, metadata, result,
		)
	}

	if errors.Is(processorErr, domainwallet.ErrInsufficientBalance) {
		return true, persistInsufficientFundsRejection(
			ctx, transactionRepository, outboxRepository, walletEntity, transaction,
			occurredAt, metadata, result,
		)
	}

	if failureCode, ok := businessRejectionCode(processorErr); ok {
		return true, persistBusinessRejection(
			ctx, transactionRepository, outboxRepository, walletEntity, transaction,
			failureCode, occurredAt, metadata, result,
		)
	}

	return false, nil
}

func businessRejectionCode(err error) (domaintransaction.FailureCode, bool) {
	switch {
	case errors.Is(err, domainwager.ErrReferenceRejected):
		return domaintransaction.FailureCodeReferenceRejected, true
	case errors.Is(err, domainwager.ErrReferenceFailed):
		return domaintransaction.FailureCodeReferenceFailed, true
	case errors.Is(err, domainwager.ErrReferenceMismatch),
		errors.Is(err, domainwager.ErrInvalidReferenceKind),
		errors.Is(err, domainwager.ErrInvalidReferenceAmount):
		return domaintransaction.FailureCodeInvalidReference, true
	case errors.Is(err, domainwager.ErrInvalidAmount):
		return domaintransaction.FailureCodeInvalidAmount, true
	case errors.Is(err, domainwager.ErrWalletMismatch):
		return domaintransaction.FailureCodeWalletMismatch, true
	case errors.Is(err, domainwager.ErrPlayerMismatch):
		return domaintransaction.FailureCodePlayerMismatch, true
	default:
		return "", false
	}
}

func persistBusinessRejection(
	ctx context.Context,
	transactionRepository *transactionpostgres.Repository,
	outboxRepository *outboxpostgres.Repository,
	walletEntity *domainwallet.Wallet,
	transaction *domaintransaction.WagerTransaction,
	failureCode domaintransaction.FailureCode,
	occurredAt time.Time,
	metadata eventMetadata,
	result *ProcessResult,
) error {
	if err := transaction.MarkRejected(failureCode, walletEntity.Balance(), occurredAt); err != nil {
		return err
	}

	if err := transactionRepository.Create(ctx, transaction); err != nil {
		return err
	}

	if err := persistRejectedEvent(ctx, outboxRepository, transaction, occurredAt, metadata); err != nil {
		return err
	}

	*result = ProcessResult{
		Transaction: transaction,
		Balance:     walletEntity.Balance().Amount(),
		Currency:    walletEntity.Balance().Currency(),
		Replayed:    false,
	}
	return nil
}

func persistInsufficientFundsRejection(
	ctx context.Context,
	transactionRepository *transactionpostgres.Repository,
	outboxRepository *outboxpostgres.Repository,
	walletEntity *domainwallet.Wallet,
	transaction *domaintransaction.WagerTransaction,
	occurredAt time.Time,
	metadata eventMetadata,
	result *ProcessResult,
) error {
	failureCode := domaintransaction.FailureCodeInsufficientFunds
	if transaction.Kind() == domaintransaction.KindRollback {
		failureCode = domaintransaction.FailureCodeReversalInsufficientFunds
	}

	if err := transaction.MarkRejected(
		failureCode,
		walletEntity.Balance(),
		occurredAt,
	); err != nil {
		return err
	}

	if err := transactionRepository.Create(
		ctx,
		transaction,
	); err != nil {
		return err
	}

	if err := persistRejectedEvent(
		ctx,
		outboxRepository,
		transaction,
		occurredAt,
		metadata,
	); err != nil {
		return err
	}

	*result = ProcessResult{
		Transaction: transaction,
		Balance:     walletEntity.Balance().Amount(),
		Currency:    walletEntity.Balance().Currency(),
		Replayed:    false,
	}

	return nil
}

func (s *Service) RejectPendingReference(
	ctx context.Context,
	transactionID string,
	failureCode domaintransaction.FailureCode,
	occurredAt time.Time,
) (ProcessResult, error) {
	var result ProcessResult

	err := s.transactionManager.WithinTransaction(
		ctx,
		func(ctx context.Context, tx pgx.Tx) error {
			txWalletRepository, err := s.walletRepository.WithDB(tx)
			if err != nil {
				return err
			}

			txTransactionRepository, err := s.transactionRepository.WithDB(tx)
			if err != nil {
				return err
			}

			txOutboxRepository, err := s.outboxRepository.WithDB(tx)
			if err != nil {
				return err
			}

			pending, err := txTransactionRepository.FindByID(ctx, transactionID)
			if err != nil {
				return err
			}

			if pending.Status().IsTerminal() {
				balance, ok := pending.ResultBalance()
				if !ok {
					return ErrNotPendingReference
				}
				result = ProcessResult{
					Transaction: pending,
					Balance:     balance.Amount(),
					Currency:    balance.Currency(),
					Replayed:    true,
				}
				return nil
			}

			if pending.Status() != domaintransaction.StatusPendingReference {
				return ErrNotPendingReference
			}

			walletEntity, err := txWalletRepository.FindByID(ctx, pending.WalletID())
			if err != nil {
				return err
			}

			pendingCopy := *pending
			if err := pendingCopy.MarkRejected(failureCode, walletEntity.Balance(), occurredAt); err != nil {
				return err
			}

			if err := txTransactionRepository.Update(ctx, &pendingCopy); err != nil {
				return err
			}

			if err := persistRejectedEvent(
				ctx,
				txOutboxRepository,
				&pendingCopy,
				occurredAt,
				eventMetadata{CorrelationID: pendingCopy.ID()},
			); err != nil {
				return err
			}

			result = ProcessResult{
				Transaction: &pendingCopy,
				Balance:     walletEntity.Balance().Amount(),
				Currency:    walletEntity.Balance().Currency(),
				Replayed:    false,
			}
			return nil
		},
	)
	if err != nil {
		return ProcessResult{}, err
	}
	return result, nil
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

			txOutboxRepository, err :=
				s.outboxRepository.WithDB(tx)
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

			if pending.Status() ==
				domaintransaction.StatusProcessed {
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

			if errors.Is(
				err,
				transactionpostgres.ErrNotFound,
			) {
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

			var ledgerEntry *domainledger.Entry
			if hasLedger {
				ledgerEntry = &entry
			}

			if err := persistProcessedEvents(
				ctx,
				txOutboxRepository,
				&pendingCopy,
				ledgerEntry,
				walletEntity.Version(),
				occurredAt,
				eventMetadata{
					CorrelationID: pendingCopy.ID(),
				},
			); err != nil {
				return err
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

	if errors.Is(
		err,
		walletpostgres.ErrConcurrentUpdate,
	) {
		observability.Default.ConcurrencyConflicts.Add(1)
		return ProcessResult{}, true, err
	}

	if isLedgerUniqueViolation(err) {
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

	balance, hasBalance := existing.ResultBalance()

	if !hasBalance {
		if existing.Status() ==
			domaintransaction.StatusPendingReference {
			return ProcessResult{
				Transaction: existing,
				Replayed:    true,
			}, nil
		}

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

func isReversalUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError

	if !errors.As(err, &pgErr) {
		return false
	}

	return pgErr.Code == "23505" &&
		pgErr.ConstraintName ==
			"wager_transactions_processed_reversal_unique"
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
	delay :=
		time.Duration(attempt+1) *
			5 *
			time.Millisecond

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()

	case <-timer.C:
		return nil
	}
}
