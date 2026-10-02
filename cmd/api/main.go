package main

import (
	"log/slog"
	"os"

	"go.uber.org/fx"

	applicationmodule "jungle_gaming_teste_tecnico/internal/application/module"
	"jungle_gaming_teste_tecnico/internal/config"
	"jungle_gaming_teste_tecnico/internal/infrastructure/postgres"
	httptransport "jungle_gaming_teste_tecnico/internal/transport/http"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	fx.New(
		config.Module,
		postgres.Module,
		applicationmodule.Module,
		httptransport.Module,
	).Run()
}
