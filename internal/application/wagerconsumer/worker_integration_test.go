package wagerconsumer

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/jackc/pgx/v5/pgxpool"

	postgresinfra "jungle_gaming_teste_tecnico/internal/infrastructure/postgres"
	inboxpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/inbox"
	sqsinfra "jungle_gaming_teste_tecnico/internal/infrastructure/sqs"
)

const (
	workerTestDatabaseURL = "postgres://jungle:jungle@localhost:5432/jungle_gaming?sslmode=disable"

	workerTestSQSEndpoint = "http://localhost:4566"

	workerTestAWSRegion = "us-east-1"

	workerTestAWSAccessKey = "test"

	workerTestAWSSecretKey = "test"

	workerTestQueueName = "wager-consumer-integration.fifo"

	workerIntegrationLockID = int64(8675309)
)

type recordingHandler struct {
	mu sync.Mutex

	calls int

	message TransactionMessage

	rawPayload []byte

	err error
}

func (h *recordingHandler) Handle(
	_ context.Context,
	message TransactionMessage,
	rawPayload []byte,
) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.calls++

	h.message = message

	h.rawPayload = append(
		[]byte(nil),
		rawPayload...,
	)

	return h.err
}

func (h *recordingHandler) Calls() int {
	h.mu.Lock()
	defer h.mu.Unlock()

	return h.calls
}

func (h *recordingHandler) Message() TransactionMessage {
	h.mu.Lock()
	defer h.mu.Unlock()

	return h.message
}

func (h *recordingHandler) RawPayload() []byte {
	h.mu.Lock()
	defer h.mu.Unlock()

	return append(
		[]byte(nil),
		h.rawPayload...,
	)
}

type workerTestEnvironment struct {
	pool *pgxpool.Pool

	inbox *inboxpostgres.Repository

	consumer *sqsinfra.Consumer

	rawSQS *awssqs.Client

	queueURL string
}

func newWorkerTestEnvironment(
	t *testing.T,
) *workerTestEnvironment {
	t.Helper()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		15*time.Second,
	)
	defer cancel()

	pool, err := postgresinfra.Open(
		ctx,
		postgresinfra.Config{
			URL: workerEnvOrDefault(
				"TEST_DATABASE_URL",
				workerTestDatabaseURL,
			),

			MaxConnections: 5,

			MinConnections: 0,

			ConnectTimeout: 5 * time.Second,

			HealthTimeout: 3 * time.Second,
		},
	)
	if err != nil {
		t.Fatalf(
			"open postgres: %v",
			err,
		)
	}

	lockConnection, err :=
		pool.Acquire(context.Background())
	if err != nil {
		pool.Close()

		t.Fatalf(
			"acquire postgres integration lock: %v",
			err,
		)
	}

	if _, err := lockConnection.Exec(
		context.Background(),
		"SELECT pg_advisory_lock($1)",
		workerIntegrationLockID,
	); err != nil {
		lockConnection.Release()
		pool.Close()

		t.Fatalf(
			"acquire postgres advisory lock: %v",
			err,
		)
	}

	t.Cleanup(func() {
		_, _ = lockConnection.Exec(
			context.Background(),
			"SELECT pg_advisory_unlock($1)",
			workerIntegrationLockID,
		)

		lockConnection.Release()

		pool.Close()
	})

	inboxRepository, err :=
		inboxpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf(
			"create inbox repository: %v",
			err,
		)
	}

	endpointURL := workerEnvOrDefault(
		"TEST_SQS_ENDPOINT_URL",
		workerTestSQSEndpoint,
	)

	region := workerEnvOrDefault(
		"TEST_AWS_REGION",
		workerTestAWSRegion,
	)

	accessKey := workerEnvOrDefault(
		"TEST_AWS_ACCESS_KEY_ID",
		workerTestAWSAccessKey,
	)

	secretKey := workerEnvOrDefault(
		"TEST_AWS_SECRET_ACCESS_KEY",
		workerTestAWSSecretKey,
	)

	queueName := workerEnvOrDefault(
		"TEST_TRANSACTION_QUEUE_NAME",
		workerTestQueueName,
	)

	client, err := sqsinfra.NewClient(
		ctx,
		sqsinfra.Config{
			Region: region,

			EndpointURL: endpointURL,

			AccessKeyID: accessKey,

			SecretAccessKey: secretKey,

			QueueName: queueName,
		},
	)
	if err != nil {
		t.Fatalf(
			"create SQS client: %v",
			err,
		)
	}

	consumer, err := sqsinfra.NewConsumer(
		ctx,
		client,
		sqsinfra.ConsumerConfig{
			QueueName: queueName,

			MaxMessages: 1,

			WaitTime: time.Second,

			VisibilityTimeout: 2 * time.Second,
		},
	)
	if err != nil {
		t.Fatalf(
			"create SQS consumer: %v",
			err,
		)
	}

	awsConfiguration, err :=
		awsconfig.LoadDefaultConfig(
			ctx,

			awsconfig.WithRegion(
				region,
			),

			awsconfig.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider(
					accessKey,
					secretKey,
					"",
				),
			),

			awsconfig.WithBaseEndpoint(
				endpointURL,
			),
		)
	if err != nil {
		t.Fatalf(
			"load AWS configuration: %v",
			err,
		)
	}

	rawSQS := awssqs.NewFromConfig(
		awsConfiguration,
	)

	environment := &workerTestEnvironment{
		pool: pool,

		inbox: inboxRepository,

		consumer: consumer,

		rawSQS: rawSQS,

		queueURL: consumer.QueueURL(),
	}

	environment.clean(
		t,
	)

	return environment
}

func (e *workerTestEnvironment) clean(
	t *testing.T,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	_, err := e.rawSQS.PurgeQueue(
		ctx,
		&awssqs.PurgeQueueInput{
			QueueUrl: aws.String(
				e.queueURL,
			),
		},
	)

	if err != nil &&
		!strings.Contains(
			err.Error(),
			"PurgeQueueInProgress",
		) {
		t.Fatalf(
			"purge SQS queue: %v",
			err,
		)
	}

	time.Sleep(
		150 * time.Millisecond,
	)
}

func (e *workerTestEnvironment) send(
	t *testing.T,
	body string,
	groupID string,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	deduplicationID := fmt.Sprintf(
		"worker-%d",
		time.Now().UnixNano(),
	)

	_, err := e.rawSQS.SendMessage(
		ctx,
		&awssqs.SendMessageInput{
			QueueUrl: aws.String(
				e.queueURL,
			),

			MessageBody: aws.String(
				body,
			),

			MessageGroupId: aws.String(
				groupID,
			),

			MessageDeduplicationId: aws.String(
				deduplicationID,
			),
		},
	)
	if err != nil {
		t.Fatalf(
			"send SQS message: %v",
			err,
		)
	}
}

func (e *workerTestEnvironment) receive(
	t *testing.T,
) sqsinfra.Message {
	t.Helper()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	messages, err :=
		e.consumer.Receive(ctx)
	if err != nil {
		t.Fatalf(
			"receive SQS message: %v",
			err,
		)
	}

	if len(messages) != 1 {
		t.Fatalf(
			"received %d messages, want 1",
			len(messages),
		)
	}

	return messages[0]
}

func (e *workerTestEnvironment) assertQueueEmpty(
	t *testing.T,
) {
	t.Helper()

	time.Sleep(
		100 * time.Millisecond,
	)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		3*time.Second,
	)
	defer cancel()

	messages, err :=
		e.consumer.Receive(ctx)
	if err != nil {
		t.Fatalf(
			"receive SQS messages: %v",
			err,
		)
	}

	if len(messages) != 0 {
		t.Fatalf(
			"queue contains %d messages, want 0",
			len(messages),
		)
	}
}

func TestWorkerProcessesValidMessage(
	t *testing.T,
) {
	environment :=
		newWorkerTestEnvironment(t)

	handler := &recordingHandler{}

	worker, err := NewWorker(
		environment.consumer,
		handler,
		environment.inbox,
	)
	if err != nil {
		t.Fatalf(
			"create worker: %v",
			err,
		)
	}

	body := `{
		"providerId":" provider-1 ",
		"externalTransactionId":" external-1 ",
		"idempotencyKey":" idempotency-1 ",
		"playerId":" player-1 ",
		"walletId":" wallet-1 ",
		"roundId":" round-1 ",
		"gameId":" game-1 ",
		"kind":" bet ",
		"money":{
			"amount":" 10.00 ",
			"currency":" brl "
		},
		"correlationId":" correlation-1 ",
		"causationId":" causation-1 ",
		"occurredAt":"2026-10-01T12:00:00Z"
	}`

	environment.send(
		t,
		body,
		"player-1",
	)

	message :=
		environment.receive(t)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	if err := worker.process(
		ctx,
		message,
	); err != nil {
		t.Fatalf(
			"process wager message: %v",
			err,
		)
	}

	if handler.Calls() != 1 {
		t.Fatalf(
			"handler calls = %d, want 1",
			handler.Calls(),
		)
	}

	handled :=
		handler.Message()

	if handled.ProviderID != "provider-1" {
		t.Fatalf(
			"provider ID = %q, want %q",
			handled.ProviderID,
			"provider-1",
		)
	}

	if handled.ExternalTransactionID !=
		"external-1" {
		t.Fatalf(
			"external transaction ID = %q, want %q",
			handled.ExternalTransactionID,
			"external-1",
		)
	}

	if handled.IdempotencyKey !=
		"idempotency-1" {
		t.Fatalf(
			"idempotency key = %q, want %q",
			handled.IdempotencyKey,
			"idempotency-1",
		)
	}

	if handled.PlayerID != "player-1" {
		t.Fatalf(
			"player ID = %q, want %q",
			handled.PlayerID,
			"player-1",
		)
	}

	if handled.WalletID != "wallet-1" {
		t.Fatalf(
			"wallet ID = %q, want %q",
			handled.WalletID,
			"wallet-1",
		)
	}

	if handled.RoundID != "round-1" {
		t.Fatalf(
			"round ID = %q, want %q",
			handled.RoundID,
			"round-1",
		)
	}

	if handled.GameID != "game-1" {
		t.Fatalf(
			"game ID = %q, want %q",
			handled.GameID,
			"game-1",
		)
	}

	if handled.Kind != "BET" {
		t.Fatalf(
			"kind = %q, want BET",
			handled.Kind,
		)
	}

	if handled.Money.Amount != "10.00" {
		t.Fatalf(
			"money amount = %q, want %q",
			handled.Money.Amount,
			"10.00",
		)
	}

	if handled.Money.Currency != "BRL" {
		t.Fatalf(
			"money currency = %q, want BRL",
			handled.Money.Currency,
		)
	}

	if string(handler.RawPayload()) != body {
		t.Fatalf(
			"raw payload changed",
		)
	}

	inboxMessage, err :=
		environment.inbox.FindByID(
			ctx,
			message.ID,
		)
	if err != nil {
		t.Fatalf(
			"find inbox message: %v",
			err,
		)
	}

	if inboxMessage.Status !=
		inboxpostgres.StatusProcessed {
		t.Fatalf(
			"inbox status = %q, want %q",
			inboxMessage.Status,
			inboxpostgres.StatusProcessed,
		)
	}

	if inboxMessage.Attempts != 1 {
		t.Fatalf(
			"inbox attempts = %d, want 1",
			inboxMessage.Attempts,
		)
	}

	if inboxMessage.ProcessedAt == nil {
		t.Fatal(
			"inbox processed_at is nil",
		)
	}

	if inboxMessage.LastError != nil {
		t.Fatalf(
			"inbox last error = %q, want nil",
			*inboxMessage.LastError,
		)
	}

	environment.assertQueueEmpty(t)
}

func TestWorkerMarksInvalidMessageFailed(
	t *testing.T,
) {
	environment :=
		newWorkerTestEnvironment(t)

	handler := &recordingHandler{}

	worker, err := NewWorker(
		environment.consumer,
		handler,
		environment.inbox,
	)
	if err != nil {
		t.Fatalf(
			"create worker: %v",
			err,
		)
	}

	body := `{
		"providerId":"provider-1",
		"externalTransactionId":"external-invalid",
		"idempotencyKey":"idempotency-invalid",
		"playerId":"player-1",
		"walletId":"wallet-1",
		"roundId":"round-1",
		"gameId":"game-1",
		"kind":"INVALID",
		"money":{
			"amount":"10.00",
			"currency":"BRL"
		}
	}`

	environment.send(
		t,
		body,
		"player-1",
	)

	message :=
		environment.receive(t)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	err = worker.process(
		ctx,
		message,
	)

	if err == nil {
		t.Fatal(
			"process invalid message returned nil error",
		)
	}

	if !strings.Contains(
		err.Error(),
		"unsupported transaction kind",
	) {
		t.Fatalf(
			"error = %q, want unsupported transaction kind",
			err.Error(),
		)
	}

	if handler.Calls() != 0 {
		t.Fatalf(
			"handler calls = %d, want 0",
			handler.Calls(),
		)
	}

	inboxMessage, err :=
		environment.inbox.FindByID(
			ctx,
			message.ID,
		)
	if err != nil {
		t.Fatalf(
			"find inbox message: %v",
			err,
		)
	}

	if inboxMessage.Status !=
		inboxpostgres.StatusFailed {
		t.Fatalf(
			"inbox status = %q, want %q",
			inboxMessage.Status,
			inboxpostgres.StatusFailed,
		)
	}

	if inboxMessage.LastError == nil {
		t.Fatal(
			"inbox last error is nil",
		)
	}

	if !strings.Contains(
		*inboxMessage.LastError,
		"unsupported transaction kind",
	) {
		t.Fatalf(
			"inbox last error = %q",
			*inboxMessage.LastError,
		)
	}
}

func TestWorkerMarksMalformedJSONFailed(
	t *testing.T,
) {
	environment :=
		newWorkerTestEnvironment(t)

	handler := &recordingHandler{}

	worker, err := NewWorker(
		environment.consumer,
		handler,
		environment.inbox,
	)
	if err != nil {
		t.Fatalf(
			"create worker: %v",
			err,
		)
	}

	body :=
		`{"providerId":"provider-1",`

	environment.send(
		t,
		body,
		"player-1",
	)

	message :=
		environment.receive(t)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	err = worker.process(
		ctx,
		message,
	)

	if err == nil {
		t.Fatal(
			"process malformed message returned nil error",
		)
	}

	if !strings.Contains(
		err.Error(),
		"decode wager message",
	) {
		t.Fatalf(
			"error = %q, want decode wager message",
			err.Error(),
		)
	}

	if handler.Calls() != 0 {
		t.Fatalf(
			"handler calls = %d, want 0",
			handler.Calls(),
		)
	}

	inboxMessage, err :=
		environment.inbox.FindByID(
			ctx,
			message.ID,
		)
	if err != nil {
		t.Fatalf(
			"find inbox message: %v",
			err,
		)
	}

	if inboxMessage.Status !=
		inboxpostgres.StatusFailed {
		t.Fatalf(
			"inbox status = %q, want %q",
			inboxMessage.Status,
			inboxpostgres.StatusFailed,
		)
	}

	if inboxMessage.LastError == nil {
		t.Fatal(
			"inbox last error is nil",
		)
	}
}

func TestWorkerMarksHandlerFailureFailed(
	t *testing.T,
) {
	environment :=
		newWorkerTestEnvironment(t)

	handlerError :=
		errors.New(
			"simulated wager processing failure",
		)

	handler := &recordingHandler{
		err: handlerError,
	}

	worker, err := NewWorker(
		environment.consumer,
		handler,
		environment.inbox,
	)
	if err != nil {
		t.Fatalf(
			"create worker: %v",
			err,
		)
	}

	body := `{
		"providerId":"provider-1",
		"externalTransactionId":"external-handler-failure",
		"idempotencyKey":"idempotency-handler-failure",
		"playerId":"player-1",
		"walletId":"wallet-1",
		"roundId":"round-1",
		"gameId":"game-1",
		"kind":"BET",
		"money":{
			"amount":"10.00",
			"currency":"BRL"
		}
	}`

	environment.send(
		t,
		body,
		"player-1",
	)

	message :=
		environment.receive(t)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	err = worker.process(
		ctx,
		message,
	)

	if err == nil {
		t.Fatal(
			"process handler failure returned nil error",
		)
	}

	if !strings.Contains(
		err.Error(),
		handlerError.Error(),
	) {
		t.Fatalf(
			"error = %q, want %q",
			err.Error(),
			handlerError.Error(),
		)
	}

	if handler.Calls() != 1 {
		t.Fatalf(
			"handler calls = %d, want 1",
			handler.Calls(),
		)
	}

	inboxMessage, err :=
		environment.inbox.FindByID(
			ctx,
			message.ID,
		)
	if err != nil {
		t.Fatalf(
			"find inbox message: %v",
			err,
		)
	}

	if inboxMessage.Status !=
		inboxpostgres.StatusFailed {
		t.Fatalf(
			"inbox status = %q, want %q",
			inboxMessage.Status,
			inboxpostgres.StatusFailed,
		)
	}

	if inboxMessage.LastError == nil {
		t.Fatal(
			"inbox last error is nil",
		)
	}

	if !strings.Contains(
		*inboxMessage.LastError,
		handlerError.Error(),
	) {
		t.Fatalf(
			"inbox last error = %q, want handler error",
			*inboxMessage.LastError,
		)
	}
}

func TestWorkerAcknowledgesAlreadyProcessedMessage(
	t *testing.T,
) {
	environment :=
		newWorkerTestEnvironment(t)

	handler := &recordingHandler{}

	worker, err := NewWorker(
		environment.consumer,
		handler,
		environment.inbox,
	)
	if err != nil {
		t.Fatalf(
			"create worker: %v",
			err,
		)
	}

	body := `{
		"providerId":"provider-1",
		"externalTransactionId":"external-idempotent",
		"idempotencyKey":"idempotency-idempotent",
		"playerId":"player-1",
		"walletId":"wallet-1",
		"roundId":"round-1",
		"gameId":"game-1",
		"kind":"BET",
		"money":{
			"amount":"10.00",
			"currency":"BRL"
		}
	}`

	environment.send(
		t,
		body,
		"player-1",
	)

	message :=
		environment.receive(t)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	payloadHash :=
		hashPayload(
			[]byte(body),
		)

	_, err =
		environment.inbox.Begin(
			ctx,
			message.ID,
			payloadHash,
			time.Now().UTC(),
		)
	if err != nil {
		t.Fatalf(
			"begin inbox message: %v",
			err,
		)
	}

	if err := environment.inbox.MarkProcessed(
		ctx,
		message.ID,
		time.Now().UTC(),
	); err != nil {
		t.Fatalf(
			"mark inbox processed: %v",
			err,
		)
	}

	if err := worker.process(
		ctx,
		message,
	); err != nil {
		t.Fatalf(
			"process previously processed message: %v",
			err,
		)
	}

	if handler.Calls() != 0 {
		t.Fatalf(
			"handler calls = %d, want 0",
			handler.Calls(),
		)
	}

	inboxMessage, err :=
		environment.inbox.FindByID(
			ctx,
			message.ID,
		)
	if err != nil {
		t.Fatalf(
			"find inbox message: %v",
			err,
		)
	}

	if inboxMessage.Status !=
		inboxpostgres.StatusProcessed {
		t.Fatalf(
			"inbox status = %q, want %q",
			inboxMessage.Status,
			inboxpostgres.StatusProcessed,
		)
	}

	if inboxMessage.Attempts != 1 {
		t.Fatalf(
			"inbox attempts = %d, want 1",
			inboxMessage.Attempts,
		)
	}

	environment.assertQueueEmpty(t)
}

func TestWorkerRetriesFailedMessage(
	t *testing.T,
) {
	environment :=
		newWorkerTestEnvironment(t)

	handler := &recordingHandler{
		err: errors.New(
			"temporary failure",
		),
	}

	worker, err := NewWorker(
		environment.consumer,
		handler,
		environment.inbox,
	)
	if err != nil {
		t.Fatalf(
			"create worker: %v",
			err,
		)
	}

	body := `{
		"providerId":"provider-1",
		"externalTransactionId":"external-retry",
		"idempotencyKey":"idempotency-retry",
		"playerId":"player-1",
		"walletId":"wallet-1",
		"roundId":"round-1",
		"gameId":"game-1",
		"kind":"BET",
		"money":{
			"amount":"10.00",
			"currency":"BRL"
		}
	}`

	environment.send(
		t,
		body,
		"player-1",
	)

	firstMessage :=
		environment.receive(t)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	err = worker.process(
		ctx,
		firstMessage,
	)

	if err == nil {
		t.Fatal(
			"first processing attempt returned nil error",
		)
	}

	firstInboxMessage, err :=
		environment.inbox.FindByID(
			ctx,
			firstMessage.ID,
		)
	if err != nil {
		t.Fatalf(
			"find failed inbox message: %v",
			err,
		)
	}

	if firstInboxMessage.Status !=
		inboxpostgres.StatusFailed {
		t.Fatalf(
			"first inbox status = %q, want %q",
			firstInboxMessage.Status,
			inboxpostgres.StatusFailed,
		)
	}

	if firstInboxMessage.Attempts != 1 {
		t.Fatalf(
			"first attempts = %d, want 1",
			firstInboxMessage.Attempts,
		)
	}

	handler.mu.Lock()
	handler.err = nil
	handler.mu.Unlock()

	if err := environment.consumer.ChangeVisibility(
		ctx,
		firstMessage.ReceiptHandle,
		0,
	); err != nil {
		t.Fatalf(
			"reset SQS visibility: %v",
			err,
		)
	}

	secondMessage :=
		environment.receive(t)

	if secondMessage.ID != firstMessage.ID {
		t.Fatalf(
			"redelivered message ID = %q, want %q",
			secondMessage.ID,
			firstMessage.ID,
		)
	}

	if err := worker.process(
		ctx,
		secondMessage,
	); err != nil {
		t.Fatalf(
			"process retry: %v",
			err,
		)
	}

	if handler.Calls() != 2 {
		t.Fatalf(
			"handler calls = %d, want 2",
			handler.Calls(),
		)
	}

	finalInboxMessage, err :=
		environment.inbox.FindByID(
			ctx,
			secondMessage.ID,
		)
	if err != nil {
		t.Fatalf(
			"find final inbox message: %v",
			err,
		)
	}

	if finalInboxMessage.Status !=
		inboxpostgres.StatusProcessed {
		t.Fatalf(
			"final inbox status = %q, want %q",
			finalInboxMessage.Status,
			inboxpostgres.StatusProcessed,
		)
	}

	if finalInboxMessage.Attempts != 2 {
		t.Fatalf(
			"final attempts = %d, want 2",
			finalInboxMessage.Attempts,
		)
	}

	if finalInboxMessage.LastError != nil {
		t.Fatalf(
			"final inbox last error = %q, want nil",
			*finalInboxMessage.LastError,
		)
	}

	environment.assertQueueEmpty(t)
}

func TestWorkerRejectsPayloadConflict(
	t *testing.T,
) {
	environment :=
		newWorkerTestEnvironment(t)

	handler := &recordingHandler{}

	worker, err := NewWorker(
		environment.consumer,
		handler,
		environment.inbox,
	)
	if err != nil {
		t.Fatalf(
			"create worker: %v",
			err,
		)
	}

	body := `{
		"providerId":"provider-1",
		"externalTransactionId":"external-conflict",
		"idempotencyKey":"idempotency-conflict",
		"playerId":"player-1",
		"walletId":"wallet-1",
		"roundId":"round-1",
		"gameId":"game-1",
		"kind":"BET",
		"money":{
			"amount":"10.00",
			"currency":"BRL"
		}
	}`

	environment.send(
		t,
		body,
		"player-1",
	)

	message :=
		environment.receive(t)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	_, err =
		environment.inbox.Begin(
			ctx,
			message.ID,
			"different-payload-hash",
			time.Now().UTC(),
		)
	if err != nil {
		t.Fatalf(
			"seed inbox message: %v",
			err,
		)
	}

	err = worker.process(
		ctx,
		message,
	)

	if err == nil {
		t.Fatal(
			"payload conflict returned nil error",
		)
	}

	if !errors.Is(
		err,
		inboxpostgres.ErrPayloadConflict,
	) {
		t.Fatalf(
			"error = %v, want ErrPayloadConflict",
			err,
		)
	}

	if handler.Calls() != 0 {
		t.Fatalf(
			"handler calls = %d, want 0",
			handler.Calls(),
		)
	}
}

func TestWorkerConstructorValidation(
	t *testing.T,
) {
	handler :=
		&recordingHandler{}

	if _, err := NewWorker(
		nil,
		handler,
		nil,
	); !errors.Is(
		err,
		ErrConsumerRequired,
	) {
		t.Fatalf(
			"nil consumer error = %v, want %v",
			err,
			ErrConsumerRequired,
		)
	}
}

func TestWorkerProcessRejectsMissingMessageID(
	t *testing.T,
) {
	environment :=
		newWorkerTestEnvironment(t)

	handler :=
		&recordingHandler{}

	worker, err := NewWorker(
		environment.consumer,
		handler,
		environment.inbox,
	)
	if err != nil {
		t.Fatalf(
			"create worker: %v",
			err,
		)
	}

	err = worker.process(
		context.Background(),
		sqsinfra.Message{
			ReceiptHandle: "receipt",
			Body:          "{}",
		},
	)

	if !errors.Is(
		err,
		ErrMessageIDRequired,
	) {
		t.Fatalf(
			"error = %v, want %v",
			err,
			ErrMessageIDRequired,
		)
	}
}

func TestWorkerProcessRejectsMissingReceiptHandle(
	t *testing.T,
) {
	environment :=
		newWorkerTestEnvironment(t)

	handler :=
		&recordingHandler{}

	worker, err := NewWorker(
		environment.consumer,
		handler,
		environment.inbox,
	)
	if err != nil {
		t.Fatalf(
			"create worker: %v",
			err,
		)
	}

	err = worker.process(
		context.Background(),
		sqsinfra.Message{
			ID:   "message-without-receipt",
			Body: "{}",
		},
	)

	if !errors.Is(
		err,
		ErrReceiptHandleRequired,
	) {
		t.Fatalf(
			"error = %v, want %v",
			err,
			ErrReceiptHandleRequired,
		)
	}
}

func workerEnvOrDefault(
	key string,
	fallback string,
) string {
	value :=
		strings.TrimSpace(
			os.Getenv(key),
		)

	if value == "" {
		return fallback
	}

	return value
}
