package main

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/fx"

	"jungle_gaming_teste_tecnico/internal/config"
	"jungle_gaming_teste_tecnico/internal/infrastructure/postgres"
)

func main() {
	fx.New(
		config.Module,
		postgres.Module,

		fx.Invoke(
			func(*pgxpool.Pool) {},
		),
	).Run()
}
