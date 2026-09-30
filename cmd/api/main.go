package main

import (
	"go.uber.org/fx"

	applicationmodule "jungle_gaming_teste_tecnico/internal/application/module"
	"jungle_gaming_teste_tecnico/internal/config"
	"jungle_gaming_teste_tecnico/internal/infrastructure/postgres"
)

func main() {
	fx.New(
		config.Module,
		postgres.Module,
		applicationmodule.Module,
	).Run()
}
