package tests

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"go.uber.org/fx"

	applicationmodule "jungle_gaming_teste_tecnico/internal/application/module"
	"jungle_gaming_teste_tecnico/internal/config"
	"jungle_gaming_teste_tecnico/internal/infrastructure/postgres"
	httptransport "jungle_gaming_teste_tecnico/internal/transport/http"
)

func TestFxApplicationStartsAndStopsWithRealDependencies(t *testing.T) {
	if !tcpAvailable("127.0.0.1:5432") || !tcpAvailable("127.0.0.1:4566") {
		t.Skip("real PostgreSQL and LocalStack are required for Fx lifecycle integration test")
	}

	t.Setenv("DATABASE_URL", "postgres://jungle:jungle@localhost:5432/jungle_gaming?sslmode=disable")
	t.Setenv("HTTP_ADDRESS", "127.0.0.1:0")
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_ENDPOINT_URL", "http://localhost:4566")
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	t.Setenv("SQS_OUTBOX_QUEUE_NAME", "fx-lifecycle-outbox.fifo")
	t.Setenv("SQS_CONSUMER_QUEUE_NAME", "fx-lifecycle-consumer.fifo")
	t.Setenv("SQS_CONSUMER_WAIT_TIME", "1s")
	t.Setenv("REFERENCE_POLL_INTERVAL", "100ms")
	t.Setenv("OUTBOX_POLL_INTERVAL", "100ms")
	t.Setenv("AUTH_ISSUER", "http://localhost:8085/realms/jungle")
	t.Setenv("AUTH_JWKS_URL", "http://localhost:8085/realms/jungle/protocol/openid-connect/certs")
	t.Setenv("AUTH_WALLET_CLIENT_ID", "wallet-service")
	t.Setenv("AUTH_PROVIDER_CLIENTS", "provider-a,provider-b")

	lockCtx, cancelLock := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelLock()
	lockConnection, err := pgx.Connect(lockCtx, "postgres://jungle:jungle@localhost:5432/jungle_gaming?sslmode=disable")
	if err != nil {
		t.Fatalf("connect PostgreSQL for integration lock: %v", err)
	}
	defer lockConnection.Close(context.Background())
	if _, err := lockConnection.Exec(context.Background(), "SELECT pg_advisory_lock($1)", int64(8675309)); err != nil {
		t.Fatalf("acquire integration lock: %v", err)
	}
	defer func() {
		_, _ = lockConnection.Exec(context.Background(), "SELECT pg_advisory_unlock($1)", int64(8675309))
	}()

	cleanSQL := `
TRUNCATE TABLE
    inbox_messages,
    outbox_events,
    ledger_entries,
    wager_transactions,
    wallets
RESTART IDENTITY CASCADE;`
	if _, err := lockConnection.Exec(context.Background(), cleanSQL); err != nil {
		t.Fatalf("clean Fx lifecycle integration state: %v", err)
	}
	defer func() {
		_, _ = lockConnection.Exec(context.Background(), cleanSQL)
	}()

	app := fx.New(
		config.Module,
		postgres.Module,
		applicationmodule.Module,
		httptransport.Module,
	)
	if err := app.Err(); err != nil {
		t.Fatalf("construct Fx application: %v", err)
	}

	startCtx, cancelStart := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelStart()
	if err := app.Start(startCtx); err != nil {
		t.Fatalf("start Fx application: %v", err)
	}

	stopCtx, cancelStop := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelStop()
	if err := app.Stop(stopCtx); err != nil {
		t.Fatalf("stop Fx application: %v", err)
	}
}

func tcpAvailable(address string) bool {
	connection, err := net.DialTimeout("tcp", address, 250*time.Millisecond)
	if err != nil {
		return false
	}
	_ = connection.Close()
	return true
}
