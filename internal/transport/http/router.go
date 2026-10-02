package httptransport

import (
	"log/slog"
	"net/http"
	"time"

	authinfra "jungle_gaming_teste_tecnico/internal/infrastructure/auth"
	"jungle_gaming_teste_tecnico/internal/observability"
)

func NewRouter(
	verifier *authinfra.Verifier,
	wagerHandler *WagerHandler,
	walletHandler *WalletHandler,
	healthHandler *HealthHandler,
	metricsHandler *observability.Handler,
	metrics *observability.Metrics,
) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /health/live", http.HandlerFunc(healthHandler.Live))
	mux.Handle("GET /health/ready", http.HandlerFunc(healthHandler.Ready))
	mux.Handle("GET /metrics", metricsHandler)

	mux.Handle("POST /wallets", verifier.Middleware(authinfra.RequireInternal, http.HandlerFunc(walletHandler.Create)))
	mux.Handle("GET /wallets/{walletId}", verifier.Middleware(authinfra.RequireInternal, http.HandlerFunc(walletHandler.Get)))
	mux.Handle("GET /wallets/{walletId}/ledger", verifier.Middleware(authinfra.RequireInternal, http.HandlerFunc(walletHandler.Ledger)))
	mux.Handle("POST /wallets/{walletId}/reconciliation", verifier.Middleware(authinfra.RequireInternal, http.HandlerFunc(walletHandler.Reconcile)))

	mux.Handle("POST /wagering/transactions", verifier.Middleware(authinfra.RequireProvider, http.HandlerFunc(wagerHandler.ProcessTransaction)))
	mux.Handle("GET /wagering/transactions/{transactionId}", verifier.Middleware(authinfra.RequireProvider, http.HandlerFunc(wagerHandler.GetTransaction)))
	mux.Handle("GET /providers/{providerId}/wagering/transactions/{externalTransactionId}", verifier.Middleware(authinfra.RequireProvider, http.HandlerFunc(wagerHandler.GetProviderTransaction)))

	return requestObservability(metrics, mux)
}

type responseRecorder struct {
	http.ResponseWriter
	status int
}

func (r *responseRecorder) WriteHeader(status int) {
	if r.status == 0 {
		r.status = status
	}
	r.ResponseWriter.WriteHeader(status)
}

func requestObservability(metrics *observability.Metrics, next http.Handler) http.Handler {
	if metrics == nil {
		metrics = observability.Default
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		started := time.Now()
		metrics.HTTPRequests.Add(1)
		recorder := &responseRecorder{ResponseWriter: writer}
		next.ServeHTTP(recorder, request)
		status := recorder.status
		if status == 0 {
			status = http.StatusOK
		}
		slog.Info("http_request",
			"method", request.Method,
			"path", request.URL.Path,
			"status", status,
			"duration_ms", time.Since(started).Milliseconds(),
		)
	})
}
