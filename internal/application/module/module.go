package module

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/fx"

	applicationoutbox "jungle_gaming_teste_tecnico/internal/application/outboxpublisher"
	applicationquery "jungle_gaming_teste_tecnico/internal/application/query"
	applicationreference "jungle_gaming_teste_tecnico/internal/application/reference"
	applicationwager "jungle_gaming_teste_tecnico/internal/application/wager"
	applicationwagerconsumer "jungle_gaming_teste_tecnico/internal/application/wagerconsumer"
	applicationwalletopening "jungle_gaming_teste_tecnico/internal/application/walletopening"
	appconfig "jungle_gaming_teste_tecnico/internal/config"
	postgresinfra "jungle_gaming_teste_tecnico/internal/infrastructure/postgres"
	inboxpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/inbox"
	ledgerpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/ledger"
	outboxpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/outbox"
	transactionpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/wagertransaction"
	walletpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/wallet"
	sqsinfra "jungle_gaming_teste_tecnico/internal/infrastructure/sqs"
)

var Module = fx.Module(
	"application",

	fx.Provide(
		provideWalletRepository,
		provideWagerTransactionRepository,
		provideLedgerRepository,
		provideOutboxRepository,
		provideInboxRepository,

		provideWagerService,
		provideQueryService,
		provideWalletOpeningService,

		provideReferenceWorker,

		provideSQSClient,
		provideSQSDestination,
		provideOutboxPublisher,

		provideSQSConsumer,
		provideWagerConsumerHandler,
		provideWagerConsumerWorker,
	),

	fx.Invoke(
		runReferenceWorker,
		runOutboxPublisher,
		runWagerConsumerWorker,
	),
)

func provideWalletRepository(
	pool *pgxpool.Pool,
) (*walletpostgres.Repository, error) {
	repository, err :=
		walletpostgres.NewRepository(pool)

	if err != nil {
		return nil, fmt.Errorf(
			"create wallet repository: %w",
			err,
		)
	}

	return repository, nil
}

func provideWagerTransactionRepository(
	pool *pgxpool.Pool,
) (*transactionpostgres.Repository, error) {
	repository, err :=
		transactionpostgres.NewRepository(pool)

	if err != nil {
		return nil, fmt.Errorf(
			"create wager transaction repository: %w",
			err,
		)
	}

	return repository, nil
}

func provideLedgerRepository(
	pool *pgxpool.Pool,
) (*ledgerpostgres.Repository, error) {
	repository, err :=
		ledgerpostgres.NewRepository(pool)

	if err != nil {
		return nil, fmt.Errorf(
			"create ledger repository: %w",
			err,
		)
	}

	return repository, nil
}

func provideOutboxRepository(
	pool *pgxpool.Pool,
) (*outboxpostgres.Repository, error) {
	repository, err :=
		outboxpostgres.NewRepository(pool)

	if err != nil {
		return nil, fmt.Errorf(
			"create outbox repository: %w",
			err,
		)
	}

	return repository, nil
}

func provideInboxRepository(
	pool *pgxpool.Pool,
) (*inboxpostgres.Repository, error) {
	repository, err :=
		inboxpostgres.NewRepository(pool)

	if err != nil {
		return nil, fmt.Errorf(
			"create inbox repository: %w",
			err,
		)
	}

	return repository, nil
}

func provideWagerService(
	transactionManager *postgresinfra.TransactionManager,
	walletRepository *walletpostgres.Repository,
	transactionRepository *transactionpostgres.Repository,
	ledgerRepository *ledgerpostgres.Repository,
	outboxRepository *outboxpostgres.Repository,
) (*applicationwager.Service, error) {
	service, err :=
		applicationwager.NewService(
			transactionManager,
			walletRepository,
			transactionRepository,
			ledgerRepository,
			outboxRepository,
		)

	if err != nil {
		return nil, fmt.Errorf(
			"create wager service: %w",
			err,
		)
	}

	return service, nil
}

func provideQueryService(
	transactionManager *postgresinfra.TransactionManager,
	walletRepository *walletpostgres.Repository,
	transactionRepository *transactionpostgres.Repository,
	ledgerRepository *ledgerpostgres.Repository,
) (*applicationquery.Service, error) {
	service, err := applicationquery.NewService(
		transactionManager,
		walletRepository,
		transactionRepository,
		ledgerRepository,
	)
	if err != nil {
		return nil, fmt.Errorf("create query service: %w", err)
	}
	return service, nil
}

func provideWalletOpeningService(
	transactionManager *postgresinfra.TransactionManager,
	walletRepository *walletpostgres.Repository,
	transactionRepository *transactionpostgres.Repository,
	ledgerRepository *ledgerpostgres.Repository,
	outboxRepository *outboxpostgres.Repository,
) (*applicationwalletopening.Service, error) {
	service, err :=
		applicationwalletopening.NewService(
			transactionManager,
			walletRepository,
			transactionRepository,
			ledgerRepository,
			outboxRepository,
		)

	if err != nil {
		return nil, fmt.Errorf(
			"create wallet opening service: %w",
			err,
		)
	}

	return service, nil
}

func provideReferenceWorker(
	transactionRepository *transactionpostgres.Repository,
	wagerService *applicationwager.Service,
	cfg appconfig.Config,
) (*applicationreference.Worker, error) {
	worker, err :=
		applicationreference.NewWorker(
			transactionRepository,
			wagerService,
			applicationreference.Config{
				BatchSize:     cfg.Reference.BatchSize,
				PollInterval:  cfg.Reference.PollInterval,
				RetryBackoff:  cfg.Reference.RetryBackoff,
				MaxBackoff:    cfg.Reference.MaxBackoff,
				LeaseDuration: cfg.Reference.LeaseDuration,
				MaxAttempts:   cfg.Reference.MaxAttempts,
			},
		)

	if err != nil {
		return nil, fmt.Errorf(
			"create reference worker: %w",
			err,
		)
	}

	return worker, nil
}

func provideSQSClient(
	cfg appconfig.Config,
) (*sqsinfra.Client, error) {
	client, err := sqsinfra.NewClient(
		context.Background(),
		sqsinfra.Config{
			Region: cfg.SQS.Region,

			EndpointURL: cfg.SQS.EndpointURL,

			AccessKeyID: cfg.SQS.AccessKeyID,

			SecretAccessKey: cfg.SQS.SecretAccessKey,

			QueueName: cfg.SQS.OutboxQueueName,
		},
	)

	if err != nil {
		return nil, fmt.Errorf(
			"create SQS client: %w",
			err,
		)
	}

	return client, nil
}

func provideSQSDestination(
	client *sqsinfra.Client,
	cfg appconfig.Config,
) (*sqsinfra.Destination, error) {
	destination, err :=
		sqsinfra.NewDestination(
			context.Background(),
			client,
			cfg.SQS.OutboxQueueName,
		)

	if err != nil {
		return nil, fmt.Errorf(
			"create SQS outbox destination: %w",
			err,
		)
	}

	return destination, nil
}

func provideOutboxPublisher(
	repository *outboxpostgres.Repository,
	destination *sqsinfra.Destination,
	cfg appconfig.Config,
) (*applicationoutbox.Publisher, error) {
	publisher, err :=
		applicationoutbox.NewPublisher(
			repository,
			destination,
			applicationoutbox.Config{
				BatchSize: cfg.Outbox.BatchSize,

				PollInterval: cfg.Outbox.PollInterval,

				RetryBackoff: cfg.Outbox.RetryBackoff,

				MaxBackoff: cfg.Outbox.MaxBackoff,
			},
		)

	if err != nil {
		return nil, fmt.Errorf(
			"create outbox publisher: %w",
			err,
		)
	}

	return publisher, nil
}

func provideSQSConsumer(
	client *sqsinfra.Client,
	cfg appconfig.Config,
) (*sqsinfra.Consumer, error) {
	consumer, err :=
		sqsinfra.NewConsumer(
			context.Background(),
			client,
			sqsinfra.ConsumerConfig{
				QueueName: cfg.SQS.ConsumerQueueName,

				MaxMessages: cfg.SQS.ConsumerMaxMessages,

				WaitTime: cfg.SQS.ConsumerWaitTime,

				VisibilityTimeout: cfg.SQS.ConsumerVisibilityTimeout,
			},
		)

	if err != nil {
		return nil, fmt.Errorf(
			"create SQS wager consumer: %w",
			err,
		)
	}

	return consumer, nil
}

func provideWagerConsumerHandler(
	service *applicationwager.Service,
) (*applicationwagerconsumer.ServiceHandler, error) {
	handler, err :=
		applicationwagerconsumer.NewServiceHandler(
			service,
		)

	if err != nil {
		return nil, fmt.Errorf(
			"create wager consumer handler: %w",
			err,
		)
	}

	return handler, nil
}

func provideWagerConsumerWorker(
	consumer *sqsinfra.Consumer,
	handler *applicationwagerconsumer.ServiceHandler,
	inboxRepository *inboxpostgres.Repository,
	transactionManager *postgresinfra.TransactionManager,
) (*applicationwagerconsumer.Worker, error) {
	worker, err :=
		applicationwagerconsumer.NewWorker(
			consumer,
			handler,
			inboxRepository,
			transactionManager,
		)

	if err != nil {
		return nil, fmt.Errorf(
			"create wager consumer worker: %w",
			err,
		)
	}

	return worker, nil
}

func runReferenceWorker(
	lifecycle fx.Lifecycle,
	shutdowner fx.Shutdowner,
	worker *applicationreference.Worker,
) {
	runWorker(
		lifecycle,
		shutdowner,
		"reference-worker",
		worker.Run,
	)
}

func runOutboxPublisher(
	lifecycle fx.Lifecycle,
	shutdowner fx.Shutdowner,
	publisher *applicationoutbox.Publisher,
) {
	runWorker(
		lifecycle,
		shutdowner,
		"outbox-publisher",
		publisher.Run,
	)
}

func runWagerConsumerWorker(
	lifecycle fx.Lifecycle,
	shutdowner fx.Shutdowner,
	worker *applicationwagerconsumer.Worker,
) {
	runWorker(
		lifecycle,
		shutdowner,
		"wager-consumer",
		worker.Run,
	)
}

func runWorker(
	lifecycle fx.Lifecycle,
	shutdowner fx.Shutdowner,
	name string,
	run func(context.Context) error,
) {
	var (
		cancel context.CancelFunc

		done chan error
	)

	lifecycle.Append(
		fx.Hook{
			OnStart: func(
				context.Context,
			) error {
				workerContext, workerCancel :=
					context.WithCancel(
						context.Background(),
					)

				cancel = workerCancel

				done = make(
					chan error,
					1,
				)

				go func() {
					err := run(workerContext)
					done <- err
					if err != nil && workerContext.Err() == nil {
						slog.Error("background_worker_stopped", "worker", name, "error", err)
						if shutdownErr := shutdowner.Shutdown(); shutdownErr != nil {
							slog.Error("application_shutdown_request_failed", "worker", name, "error", shutdownErr)
						}
					}
				}()

				return nil
			},

			OnStop: func(
				ctx context.Context,
			) error {
				if cancel == nil {
					return nil
				}

				cancel()

				if done == nil {
					return nil
				}

				select {
				case err := <-done:
					return err

				case <-ctx.Done():
					return ctx.Err()
				}
			},
		},
	)
}
