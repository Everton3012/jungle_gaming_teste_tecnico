package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/fx"

	appconfig "jungle_gaming_teste_tecnico/internal/config"
)

var Module = fx.Module(
	"postgres",
	fx.Provide(
		providePool,
		NewTransactionManager,
	),
)

func providePool(
	lifecycle fx.Lifecycle,
	cfg appconfig.Config,
) (*pgxpool.Pool, error) {
	pool, err := Open(
		context.Background(),
		Config{
			URL:            cfg.Database.URL,
			MaxConnections: cfg.Database.MaxConnections,
			MinConnections: cfg.Database.MinConnections,
			ConnectTimeout: cfg.Database.ConnectTimeout,
			HealthTimeout:  cfg.Database.OperationTimeout,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("open postgres: %w", err)
	}

	lifecycle.Append(
		fx.Hook{
			OnStop: func(context.Context) error {
				pool.Close()
				return nil
			},
		},
	)

	return pool, nil
}
