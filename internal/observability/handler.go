package observability

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	appconfig "jungle_gaming_teste_tecnico/internal/config"
	sqsinfra "jungle_gaming_teste_tecnico/internal/infrastructure/sqs"
)

type Handler struct {
	metrics *Metrics
	pool    *pgxpool.Pool
	sqs     *sqsinfra.Client
	dlqName string
}

func NewHandler(metrics *Metrics, pool *pgxpool.Pool, sqs *sqsinfra.Client, cfg appconfig.Config) *Handler {
	if metrics == nil {
		metrics = Default
	}
	return &Handler{metrics: metrics, pool: pool, sqs: sqs, dlqName: "wager-transactions-dlq.fifo"}
}

func (h *Handler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		writer.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	ctx, cancel := context.WithTimeout(request.Context(), 2*time.Second)
	defer cancel()
	outboxLag := h.outboxLag(ctx)
	dlqMessages := h.dlqMessages(ctx)

	count := h.metrics.ProcessingCount.Load()
	latencySeconds := 0.0
	if count > 0 {
		latencySeconds = float64(h.metrics.ProcessingNanoseconds.Load()) / float64(count) / float64(time.Second)
	}

	writer.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	values := []struct {
		name  string
		value string
	}{
		{"jungle_http_requests_total", u(h.metrics.HTTPRequests.Load())},
		{"jungle_wager_processed_total", u(h.metrics.WagerProcessed.Load())},
		{"jungle_wager_rejected_total", u(h.metrics.WagerRejected.Load())},
		{"jungle_wager_pending_total", u(h.metrics.WagerPending.Load())},
		{"jungle_idempotent_replays_total", u(h.metrics.IdempotentReplays.Load())},
		{"jungle_sqs_redeliveries_total", u(h.metrics.SQSRedeliveries.Load())},
		{"jungle_inbox_duplicates_total", u(h.metrics.InboxDuplicates.Load())},
		{"jungle_outbox_publish_retries_total", u(h.metrics.OutboxPublishRetries.Load())},
		{"jungle_concurrency_conflicts_total", u(h.metrics.ConcurrencyConflicts.Load())},
		{"jungle_reconciliation_divergences_total", u(h.metrics.ReconciliationDivergences.Load())},
		{"jungle_processing_latency_seconds_avg", strconv.FormatFloat(latencySeconds, 'f', 6, 64)},
		{"jungle_outbox_oldest_unpublished_seconds", strconv.FormatFloat(outboxLag, 'f', 3, 64)},
		{"jungle_sqs_dlq_messages", strconv.FormatInt(dlqMessages, 10)},
	}
	for _, item := range values {
		_, _ = fmt.Fprintf(writer, "%s %s\n", item.name, item.value)
	}
}

func (h *Handler) outboxLag(ctx context.Context) float64 {
	if h.pool == nil {
		return -1
	}
	var seconds float64
	err := h.pool.QueryRow(ctx, `SELECT COALESCE(EXTRACT(EPOCH FROM (NOW() - MIN(created_at))), 0)::double precision FROM outbox_events WHERE status <> 'PUBLISHED'`).Scan(&seconds)
	if err != nil || seconds < 0 {
		return 0
	}
	return seconds
}

func (h *Handler) dlqMessages(ctx context.Context) int64 {
	if h.sqs == nil {
		return -1
	}
	count, err := h.sqs.ApproximateMessages(ctx, h.dlqName)
	if err != nil {
		return -1
	}
	return count
}

func u(value uint64) string { return strconv.FormatUint(value, 10) }
