package httptransport

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"go.uber.org/fx"

	appconfig "jungle_gaming_teste_tecnico/internal/config"
)

func NewServer(
	handler http.Handler,
	cfg appconfig.Config,
) *http.Server {
	return &http.Server{
		Addr:              cfg.HTTP.Address,
		Handler:           handler,
		ReadHeaderTimeout: cfg.Database.OperationTimeout,
		ReadTimeout:       cfg.Database.OperationTimeout,
		WriteTimeout:      cfg.Database.OperationTimeout,
		IdleTimeout:       cfg.Database.OperationTimeout * 2,
	}
}

func RunServer(
	lifecycle fx.Lifecycle,
	shutdowner fx.Shutdowner,
	server *http.Server,
	cfg appconfig.Config,
) {
	var listener net.Listener

	lifecycle.Append(
		fx.Hook{
			OnStart: func(ctx context.Context) error {
				var err error
				listener, err = net.Listen("tcp", server.Addr)
				if err != nil {
					return fmt.Errorf("listen HTTP on %s: %w", server.Addr, err)
				}

				go func() {
					err := server.Serve(listener)
					if err == nil || errors.Is(err, http.ErrServerClosed) {
						return
					}
					slog.Error("http_server_stopped", "address", server.Addr, "error", err)
					if shutdownErr := shutdowner.Shutdown(); shutdownErr != nil {
						slog.Error("application_shutdown_request_failed", "component", "http-server", "error", shutdownErr)
					}
				}()
				return nil
			},

			OnStop: func(ctx context.Context) error {
				shutdownContext, cancel := context.WithTimeout(ctx, cfg.Application.ShutdownTimeout)
				defer cancel()
				if err := server.Shutdown(shutdownContext); err != nil {
					return fmt.Errorf("shutdown HTTP server: %w", err)
				}
				return nil
			},
		},
	)
}
