package reference

import (
	"context"
	"errors"
	"fmt"
	"time"

	applicationwager "jungle_gaming_teste_tecnico/internal/application/wager"
	domainwager "jungle_gaming_teste_tecnico/internal/domain/wager"
	domaintransaction "jungle_gaming_teste_tecnico/internal/domain/wagertransaction"
	transactionpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/wagertransaction"
)

var (
	ErrPendingRepositoryRequired = errors.New("pending repository is required")
	ErrWagerServiceRequired      = errors.New("wager service is required")
	ErrInvalidBatchSize          = errors.New("batch size must be greater than zero")
	ErrInvalidPollInterval       = errors.New("poll interval must be greater than zero")
	ErrInvalidRetryBackoff       = errors.New("retry backoff must be greater than zero")
)

type PendingRepository interface {
	FindPendingReferences(
		ctx context.Context,
		limit int,
	) ([]domaintransaction.WagerTransaction, error)
}

type durablePendingRepository interface {
	ClaimPendingReferences(
		ctx context.Context,
		now time.Time,
		leaseDuration time.Duration,
		limit int,
	) ([]transactionpostgres.PendingReferenceClaim, error)

	SchedulePendingReference(
		ctx context.Context,
		transactionID string,
		nextAttemptAt time.Time,
	) error
}

type WagerService interface {
	ResumePendingReference(
		ctx context.Context,
		transactionID string,
		ledgerEntryID string,
		occurredAt time.Time,
	) (applicationwager.ProcessResult, error)
}

type rejectingWagerService interface {
	RejectPendingReference(
		ctx context.Context,
		transactionID string,
		failureCode domaintransaction.FailureCode,
		occurredAt time.Time,
	) (applicationwager.ProcessResult, error)
}

type Config struct {
	BatchSize     int
	PollInterval  time.Duration
	RetryBackoff  time.Duration
	MaxBackoff    time.Duration
	LeaseDuration time.Duration
	MaxAttempts   int
}

type Worker struct {
	repository PendingRepository
	service    WagerService
	config     Config
	now        func() time.Time
	sleep      func(context.Context, time.Duration) error
}

func NewWorker(
	repository PendingRepository,
	service WagerService,
	config Config,
) (*Worker, error) {
	if repository == nil {
		return nil, ErrPendingRepositoryRequired
	}
	if service == nil {
		return nil, ErrWagerServiceRequired
	}
	if config.BatchSize <= 0 {
		return nil, ErrInvalidBatchSize
	}
	if config.PollInterval <= 0 {
		return nil, ErrInvalidPollInterval
	}
	if config.RetryBackoff <= 0 {
		return nil, ErrInvalidRetryBackoff
	}
	if config.MaxBackoff == 0 {
		config.MaxBackoff = 5 * time.Minute
	}
	if config.MaxBackoff < config.RetryBackoff {
		return nil, errors.New("max reference backoff cannot be lower than retry backoff")
	}
	if config.LeaseDuration == 0 {
		config.LeaseDuration = 30 * time.Second
	}
	if config.LeaseDuration < 0 {
		return nil, errors.New("reference lease duration must be greater than zero")
	}
	if config.MaxAttempts == 0 {
		config.MaxAttempts = 10
	}
	if config.MaxAttempts < 1 {
		return nil, errors.New("reference max attempts must be greater than zero")
	}

	return &Worker{
		repository: repository,
		service:    service,
		config:     config,
		now: func() time.Time {
			return time.Now().UTC()
		},
		sleep: sleepContext,
	}, nil
}

func (w *Worker) Run(ctx context.Context) error {
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}

		processed, err := w.ProcessBatch(ctx)
		if err != nil {
			if err := w.sleep(ctx, w.config.RetryBackoff); err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return nil
				}
				return err
			}
			continue
		}

		if processed > 0 {
			continue
		}

		if err := w.sleep(ctx, w.config.PollInterval); err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			return err
		}
	}
}

func (w *Worker) ProcessBatch(ctx context.Context) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	if repository, ok := w.repository.(durablePendingRepository); ok {
		if service, ok := w.service.(rejectingWagerService); ok {
			return w.processDurableBatch(ctx, repository, service)
		}
	}

	return w.processLegacyBatch(ctx)
}

func (w *Worker) processDurableBatch(
	ctx context.Context,
	repository durablePendingRepository,
	service rejectingWagerService,
) (int, error) {
	now := w.now()
	claims, err := repository.ClaimPendingReferences(ctx, now, w.config.LeaseDuration, w.config.BatchSize)
	if err != nil {
		return 0, fmt.Errorf("claim pending references: %w", err)
	}

	processed := 0
	for _, claim := range claims {
		if err := ctx.Err(); err != nil {
			return processed, err
		}

		_, err := w.service.ResumePendingReference(
			ctx,
			claim.TransactionID,
			ledgerID(claim.TransactionID),
			w.now(),
		)

		switch {
		case err == nil:
			processed++

		case errors.Is(err, applicationwager.ErrNotPendingReference):
			continue

		case errors.Is(err, applicationwager.ErrReferenceRequired),
			errors.Is(err, domainwager.ErrReferenceNotProcessed):
			if claim.Attempts >= w.config.MaxAttempts {
				if _, rejectErr := service.RejectPendingReference(
					ctx,
					claim.TransactionID,
					domaintransaction.FailureCodeReferenceNotFound,
					w.now(),
				); rejectErr != nil && !errors.Is(rejectErr, applicationwager.ErrNotPendingReference) {
					return processed, fmt.Errorf("expire pending reference %s: %w", claim.TransactionID, rejectErr)
				}
				processed++
				continue
			}

			next := w.now().Add(w.retryDelay(claim.Attempts))
			if scheduleErr := repository.SchedulePendingReference(ctx, claim.TransactionID, next); scheduleErr != nil && !errors.Is(scheduleErr, transactionpostgres.ErrNotFound) {
				return processed, fmt.Errorf("schedule pending reference %s: %w", claim.TransactionID, scheduleErr)
			}

		case errors.Is(err, domainwager.ErrReferenceRejected):
			if _, rejectErr := service.RejectPendingReference(ctx, claim.TransactionID, domaintransaction.FailureCodeReferenceRejected, w.now()); rejectErr != nil && !errors.Is(rejectErr, applicationwager.ErrNotPendingReference) {
				return processed, fmt.Errorf("reject pending reference %s: %w", claim.TransactionID, rejectErr)
			}
			processed++

		case errors.Is(err, domainwager.ErrReferenceFailed):
			if _, rejectErr := service.RejectPendingReference(ctx, claim.TransactionID, domaintransaction.FailureCodeReferenceFailed, w.now()); rejectErr != nil && !errors.Is(rejectErr, applicationwager.ErrNotPendingReference) {
				return processed, fmt.Errorf("reject failed reference %s: %w", claim.TransactionID, rejectErr)
			}
			processed++

		case isPermanentReferenceError(err):
			if _, rejectErr := service.RejectPendingReference(ctx, claim.TransactionID, domaintransaction.FailureCodeInvalidReference, w.now()); rejectErr != nil && !errors.Is(rejectErr, applicationwager.ErrNotPendingReference) {
				return processed, fmt.Errorf("reject invalid reference %s: %w", claim.TransactionID, rejectErr)
			}
			processed++

		default:
			// Infrastructure and concurrency failures keep the durable lease. Another
			// instance will reclaim the record after the lease expires.
			return processed, fmt.Errorf("resume pending reference %s: %w", claim.TransactionID, err)
		}
	}

	return processed, nil
}

func (w *Worker) processLegacyBatch(ctx context.Context) (int, error) {
	pending, err := w.repository.FindPendingReferences(ctx, w.config.BatchSize)
	if err != nil {
		return 0, fmt.Errorf("find pending references: %w", err)
	}

	processed := 0
	for i := range pending {
		if err := ctx.Err(); err != nil {
			return processed, err
		}

		transaction := pending[i]
		_, err := w.service.ResumePendingReference(ctx, transaction.ID(), ledgerID(transaction.ID()), w.now())
		switch {
		case err == nil:
			processed++
		case errors.Is(err, applicationwager.ErrReferenceRequired), errors.Is(err, applicationwager.ErrNotPendingReference):
			continue
		default:
			return processed, fmt.Errorf("resume pending reference %s: %w", transaction.ID(), err)
		}
	}
	return processed, nil
}

func (w *Worker) retryDelay(attempts int) time.Duration {
	if attempts <= 1 {
		return w.config.RetryBackoff
	}
	delay := w.config.RetryBackoff
	for i := 1; i < attempts; i++ {
		if delay >= w.config.MaxBackoff/2 {
			return w.config.MaxBackoff
		}
		delay *= 2
	}
	if delay > w.config.MaxBackoff {
		return w.config.MaxBackoff
	}
	return delay
}

func isPermanentReferenceError(err error) bool {
	return errors.Is(err, domainwager.ErrReferenceMismatch) ||
		errors.Is(err, domainwager.ErrInvalidReferenceKind) ||
		errors.Is(err, domainwager.ErrInvalidReferenceAmount) ||
		errors.Is(err, domainwager.ErrCurrencyMismatch) ||
		errors.Is(err, domainwager.ErrPlayerMismatch) ||
		errors.Is(err, domainwager.ErrWalletMismatch)
}

func ledgerID(transactionID string) string {
	return "reference-" + transactionID
}

func sleepContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
