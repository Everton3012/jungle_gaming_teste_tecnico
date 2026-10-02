package httptransport

import (
	"go.uber.org/fx"

	authinfra "jungle_gaming_teste_tecnico/internal/infrastructure/auth"
	"jungle_gaming_teste_tecnico/internal/observability"
)

var Module = fx.Module(
	"http",
	fx.Provide(
		provideMetrics,
		authinfra.NewVerifier,
		NewHealthHandler,
		observability.NewHandler,
		NewWagerHandler,
		NewWalletHandler,
		NewRouter,
		NewServer,
	),
	fx.Invoke(RunServer),
)

func provideMetrics() *observability.Metrics { return observability.Default }
