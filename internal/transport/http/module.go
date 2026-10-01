package httptransport

import "go.uber.org/fx"

var Module = fx.Module(
	"http",

	fx.Provide(
		NewWagerHandler,
		NewWalletHandler,
		NewRouter,
		NewServer,
	),

	fx.Invoke(
		RunServer,
	),
)
