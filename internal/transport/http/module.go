package httptransport

import "go.uber.org/fx"

var Module = fx.Module(
	"http",

	fx.Provide(
		NewWagerHandler,
		NewRouter,
		NewServer,
	),

	fx.Invoke(
		RunServer,
	),
)
