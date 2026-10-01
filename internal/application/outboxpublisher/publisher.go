package outboxpublisher

import (
	"context"
	"errors"
	"fmt"
	"time"

	domainoutbox "jungle_gaming_teste_tecnico/internal/domain/outbox"
)

var (
	ErrRepositoryRequired  = errors.New("outbox repository is required")
	ErrDestinationRequired = errors.New("outbox destination is required")
	ErrInvalidBatchSize    = errors.New("batch size must be greater than zero")
	ErrInvalidPollInterval = errors.New("poll interval must be greater than zero")
	ErrInvalidRetryBackoff = errors.New("retry backoff must be greater than zero")
	ErrInvalidMaxBackoff   = errors.New("max backoff must be greater than zero")
)

type Repository interface {
	ClaimPending(
		ctx context.Context,
		now time.Time,
		limit int,
	) ([]*domainoutbox.Event, error)

	MarkPublished(
		ctx context.Context,
		id string,
		publishedAt time.Time,
	) error

	Reschedule(
		ctx context.Context,
		id string,
		availableAt time.Time,
		lastError string,
		updatedAt time.Time,
	) error
}

type Destination interface {
	Publish(
		ctx context.Context,
		event *domainoutbox.Event,
	) error
}

type Config struct {
	BatchSize    int
	PollInterval time.Duration
	RetryBackoff time.Duration
	MaxBackoff   time.Duration
}

type Publisher struct {
	repository  Repository
	destination Destination
	config      Config
	now         func() time.Time
	sleep       func(context.Context, time.Duration) error
}

func NewPublisher(
	repository Repository,
	destination Destination,
	config Config,
) (*Publisher, error) {
	if repository == nil {
		return nil, ErrRepositoryRequired
	}

	if destination == nil {
		return nil, ErrDestinationRequired
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

	if config.MaxBackoff <= 0 {
		return nil, ErrInvalidMaxBackoff
	}

	if config.MaxBackoff < config.RetryBackoff {
		return nil, fmt.Errorf(
			"%w: max backoff cannot be lower than retry backoff",
			ErrInvalidMaxBackoff,
		)
	}

	return &Publisher{
		repository:  repository,
		destination: destination,
		config:      config,
		now: func() time.Time {
			return time.Now().UTC()
		},
		sleep: sleepContext,
	}, nil
}

func (p *Publisher) Run(
	ctx context.Context,
) error {
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}

		processed, err := p.ProcessBatch(ctx)
		if err != nil {
			if err := p.sleep(
				ctx,
				p.config.RetryBackoff,
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

		if err := p.sleep(
			ctx,
			p.config.PollInterval,
		); err != nil {
			if errors.Is(err, context.Canceled) ||
				errors.Is(err, context.DeadlineExceeded) {
				return nil
			}

			return err
		}
	}
}

func (p *Publisher) ProcessBatch(
	ctx context.Context,
) (int, error) {
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	now := p.now()

	events, err := p.repository.ClaimPending(
		ctx,
		now,
		p.config.BatchSize,
	)
	if err != nil {
		return 0, fmt.Errorf(
			"claim pending outbox events: %w",
			err,
		)
	}

	processed := 0

	for _, event := range events {
		if err := ctx.Err(); err != nil {
			return processed, err
		}

		if event == nil {
			continue
		}

		err := p.destination.Publish(
			ctx,
			event,
		)
		if err != nil {
			rescheduleAt := p.now()
			availableAt := rescheduleAt.Add(
				p.retryDelay(event.Attempts()),
			)

			rescheduleErr := p.repository.Reschedule(
				ctx,
				event.ID(),
				availableAt,
				err.Error(),
				rescheduleAt,
			)
			if rescheduleErr != nil {
				return processed, fmt.Errorf(
					"reschedule outbox event %s after publish failure: %w",
					event.ID(),
					rescheduleErr,
				)
			}

			continue
		}

		publishedAt := p.now()

		if err := p.repository.MarkPublished(
			ctx,
			event.ID(),
			publishedAt,
		); err != nil {
			return processed, fmt.Errorf(
				"mark outbox event %s published: %w",
				event.ID(),
				err,
			)
		}

		processed++
	}

	return processed, nil
}

func (p *Publisher) retryDelay(
	attempts int,
) time.Duration {
	if attempts <= 1 {
		return p.config.RetryBackoff
	}

	delay := p.config.RetryBackoff

	for attempt := 1; attempt < attempts; attempt++ {
		if delay >= p.config.MaxBackoff {
			return p.config.MaxBackoff
		}

		if delay > p.config.MaxBackoff/2 {
			return p.config.MaxBackoff
		}

		delay *= 2
	}

	if delay > p.config.MaxBackoff {
		return p.config.MaxBackoff
	}

	return delay
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
