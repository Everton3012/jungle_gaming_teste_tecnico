package reference

import (
	"context"
	"errors"
	"fmt"
	"time"

	applicationwager "jungle_gaming_teste_tecnico/internal/application/wager"
	domaintransaction "jungle_gaming_teste_tecnico/internal/domain/wagertransaction"
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

type WagerService interface {
	ResumePendingReference(
		ctx context.Context,
		transactionID string,
		ledgerEntryID string,
		occurredAt time.Time,
	) (applicationwager.ProcessResult, error)
}

type Config struct {
	BatchSize    int
	PollInterval time.Duration
	RetryBackoff time.Duration
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
			if err := w.sleep(
				ctx,
				w.config.RetryBackoff,
			); err != nil {
				if errors.Is(err, context.Canceled) ||
					errors.Is(err, context.DeadlineExceeded) {
					return nil
				}

				return err
			}

			continue
		}

		if processed > 0 {
			continue
		}

		if err := w.sleep(
			ctx,
			w.config.PollInterval,
		); err != nil {
			if errors.Is(err, context.Canceled) ||
				errors.Is(err, context.DeadlineExceeded) {
				return nil
			}

			return err
		}
	}
}

func (w *Worker) ProcessBatch(
	ctx context.Context,
) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	pending, err := w.repository.FindPendingReferences(
		ctx,
		w.config.BatchSize,
	)
	if err != nil {
		return 0, fmt.Errorf(
			"find pending references: %w",
			err,
		)
	}

	processed := 0

	for i := range pending {
		if err := ctx.Err(); err != nil {
			return processed, err
		}

		transaction := pending[i]

		ledgerEntryID := ledgerID(transaction.ID())

		_, err := w.service.ResumePendingReference(
			ctx,
			transaction.ID(),
			ledgerEntryID,
			w.now(),
		)

		switch {
		case err == nil:
			processed++

		case errors.Is(
			err,
			applicationwager.ErrReferenceRequired,
		):
			// A referência ainda não existe.
			// Mantemos PENDING_REFERENCE para uma tentativa futura.
			continue

		case errors.Is(
			err,
			applicationwager.ErrNotPendingReference,
		):
			// Outra instância pode ter resolvido a transação
			// entre o SELECT do batch e o processamento.
			continue

		default:
			return processed, fmt.Errorf(
				"resume pending reference %s: %w",
				transaction.ID(),
				err,
			)
		}
	}

	return processed, nil
}

func ledgerID(transactionID string) string {
	return "reference-" + transactionID
}

func sleepContext(
	ctx context.Context,
	duration time.Duration,
) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()

	case <-timer.C:
		return nil
	}
}
