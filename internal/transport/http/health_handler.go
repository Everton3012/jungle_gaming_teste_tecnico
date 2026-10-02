package httptransport

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	appconfig "jungle_gaming_teste_tecnico/internal/config"
	sqsinfra "jungle_gaming_teste_tecnico/internal/infrastructure/sqs"
)

type HealthHandler struct {
	pool *pgxpool.Pool
	sqs  *sqsinfra.Client
	cfg  appconfig.Config
}

func NewHealthHandler(pool *pgxpool.Pool, sqs *sqsinfra.Client, cfg appconfig.Config) *HealthHandler {
	return &HealthHandler{pool: pool, sqs: sqs, cfg: cfg}
}

func (h *HealthHandler) Live(writer http.ResponseWriter, _ *http.Request) {
	writeJSON(writer, http.StatusOK, healthResponse{Status: "ok"})
}

func (h *HealthHandler) Ready(writer http.ResponseWriter, request *http.Request) {
	ctx, cancel := context.WithTimeout(request.Context(), 2*time.Second)
	defer cancel()

	response := healthResponse{Status: "ready", Postgres: "ok", SQS: "ok"}
	ready := true
	if h.pool == nil || h.pool.Ping(ctx) != nil {
		response.Postgres = "unavailable"
		ready = false
	}
	if h.sqs == nil || h.sqs.Ready(ctx, h.cfg.SQS.ConsumerQueueName) != nil || h.sqs.Ready(ctx, h.cfg.SQS.OutboxQueueName) != nil {
		response.SQS = "unavailable"
		ready = false
	}
	if !ready {
		response.Status = "not_ready"
		writeJSON(writer, http.StatusServiceUnavailable, response)
		return
	}
	writeJSON(writer, http.StatusOK, response)
}
