package module

import (
	"context"
	"fmt"

	"go.uber.org/fx"

	applicationreference "jungle_gaming_teste_tecnico/internal/application/reference"
	applicationwager "jungle_gaming_teste_tecnico/internal/application/wager"
	appconfig "jungle_gaming_teste_tecnico/internal/config"
	postgresinfra "jungle_gaming_teste_tecnico/internal/infrastructure/postgres"
	ledgerpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/ledger"
	transactionpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/wagertransaction"
	walletpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/wallet"

	"github.com/jackc/pgx/v5/pgxpool"
)

var Module = fx.Module(
	"application",

	fx.Provide(
		provideWalletRepository,
		provideWagerTransactionRepository,
		provideLedgerRepository,
		provideWagerService,
		provideReferenceWorker,
	),

	fx.Invoke(runReferenceWorker),
)

func provideWalletRepository(
	pool *pgxpool.Pool,
) (*walletpostgres.Repository, error) {
	repository, err := walletpostgres.NewRepository(pool)
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
	repository, err := transactionpostgres.NewRepository(pool)
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
	repository, err := ledgerpostgres.NewRepository(pool)
	if err != nil {
		return nil, fmt.Errorf(
			"create ledger repository: %w",
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
) (*applicationwager.Service, error) {
	service, err := applicationwager.NewService(
		transactionManager,
		walletRepository,
		transactionRepository,
		ledgerRepository,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create wager service: %w",
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
	worker, err := applicationreference.NewWorker(
		transactionRepository,
		wagerService,
		applicationreference.Config{
			BatchSize:    cfg.Reference.BatchSize,
			PollInterval: cfg.Reference.PollInterval,
			RetryBackoff: cfg.Reference.RetryBackoff,
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

func runReferenceWorker(
	lifecycle fx.Lifecycle,
	worker *applicationreference.Worker,
) {
	var (
		cancel context.CancelFunc
		done   chan error
	)

	lifecycle.Append(
		fx.Hook{
			OnStart: func(context.Context) error {
				workerContext, workerCancel :=
					context.WithCancel(context.Background())

				cancel = workerCancel
				done = make(chan error, 1)

				go func() {
					done <- worker.Run(workerContext)
				}()

				return nil
			},

			OnStop: func(ctx context.Context) error {
				if cancel == nil {
					return nil
				}

				cancel()

				select {
				case err := <-done:
					if err != nil {
						return fmt.Errorf(
							"stop reference worker: %w",
							err,
						)
					}

					return nil

				case <-ctx.Done():
					return fmt.Errorf(
						"wait reference worker shutdown: %w",
						ctx.Err(),
					)
				}
			},
		},
	)
}
