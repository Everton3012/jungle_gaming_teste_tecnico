package outboxpublisher_test

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/jackc/pgx/v5/pgxpool"

	applicationoutbox "jungle_gaming_teste_tecnico/internal/application/outboxpublisher"
	domainoutbox "jungle_gaming_teste_tecnico/internal/domain/outbox"
	postgresinfra "jungle_gaming_teste_tecnico/internal/infrastructure/postgres"
	outboxpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/outbox"
	sqsinfra "jungle_gaming_teste_tecnico/internal/infrastructure/sqs"
)

const (
	defaultTestDatabaseURL = "postgres://jungle:jungle@localhost:5432/jungle_gaming?sslmode=disable"

	defaultTestSQSEndpoint = "http://localhost:4566"
	defaultTestAWSRegion   = "us-east-1"
	defaultTestAccessKey   = "test"
	defaultTestSecretKey   = "test"
	defaultTestQueueName   = "outbox-publisher-integration.fifo"

	integrationTestLockID = int64(8675309)
)

func TestPublisherPublishesPendingEventAndMarksItPublished(
	t *testing.T,
) {
	pool := newTestPool(t)
	cleanDatabase(t, pool)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		15*time.Second,
	)
	defer cancel()

	repository, err := outboxpostgres.NewRepository(pool)
	if err != nil {
		t.Fatalf(
			"create outbox repository: %v",
			err,
		)
	}

	endpointURL := envOrDefault(
		"TEST_SQS_ENDPOINT_URL",
		defaultTestSQSEndpoint,
	)

	region := envOrDefault(
		"TEST_AWS_REGION",
		defaultTestAWSRegion,
	)

	accessKey := envOrDefault(
		"TEST_AWS_ACCESS_KEY_ID",
		defaultTestAccessKey,
	)

	secretKey := envOrDefault(
		"TEST_AWS_SECRET_ACCESS_KEY",
		defaultTestSecretKey,
	)

	queueName := envOrDefault(
		"TEST_OUTBOX_QUEUE_NAME",
		defaultTestQueueName,
	)

	client, err := sqsinfra.NewClient(
		ctx,
		sqsinfra.Config{
			Region:          region,
			EndpointURL:     endpointURL,
			AccessKeyID:     accessKey,
			SecretAccessKey: secretKey,
			QueueName:       queueName,
		},
	)
	if err != nil {
		t.Fatalf(
			"create SQS client: %v",
			err,
		)
	}

	destination, err := sqsinfra.NewDestination(
		ctx,
		client,
		queueName,
	)
	if err != nil {
		t.Fatalf(
			"create SQS destination: %v",
			err,
		)
	}

	rawClient := newRawSQSClient(
		t,
		ctx,
		endpointURL,
		region,
		accessKey,
		secretKey,
	)

	purgeQueue(
		t,
		ctx,
		rawClient,
		destination.QueueURL(),
	)

	now := time.Now().
		UTC().
		Truncate(time.Millisecond)

	payload, err := json.Marshal(
		map[string]any{
			"eventId": "event-outbox-publisher-integration-001",

			"eventType": "WagerTransactionProcessed",

			"aggregateId": "transaction-outbox-publisher-integration-001",

			"correlationId": "correlation-outbox-publisher-integration-001",

			"occurredAt": now.Format(
				time.RFC3339Nano,
			),

			"version": 1,

			"data": map[string]any{
				"transactionId": "transaction-outbox-publisher-integration-001",
			},
		},
	)
	if err != nil {
		t.Fatalf(
			"marshal outbox payload: %v",
			err,
		)
	}

	event, err := domainoutbox.NewEvent(
		domainoutbox.NewEventInput{
			ID: "event-outbox-publisher-integration-001",

			EventType: "WagerTransactionProcessed",

			AggregateType: "WAGER_TRANSACTION",

			AggregateID: "transaction-outbox-publisher-integration-001",

			Payload: payload,

			CreatedAt: now,
		},
	)
	if err != nil {
		t.Fatalf(
			"create outbox event: %v",
			err,
		)
	}

	if err := repository.Create(
		ctx,
		event,
	); err != nil {
		t.Fatalf(
			"persist outbox event: %v",
			err,
		)
	}

	persistedBeforePublish, err := repository.FindByID(
		ctx,
		event.ID(),
	)
	if err != nil {
		t.Fatalf(
			"find pending outbox event: %v",
			err,
		)
	}

	if persistedBeforePublish.Status() !=
		domainoutbox.StatusPending {
		t.Fatalf(
			"status before publish = %q, want %q",
			persistedBeforePublish.Status(),
			domainoutbox.StatusPending,
		)
	}

	if persistedBeforePublish.Attempts() != 0 {
		t.Fatalf(
			"attempts before publish = %d, want 0",
			persistedBeforePublish.Attempts(),
		)
	}

	publisher, err := applicationoutbox.NewPublisher(
		repository,
		destination,
		applicationoutbox.Config{
			BatchSize:    10,
			PollInterval: 100 * time.Millisecond,
			RetryBackoff: 100 * time.Millisecond,
			MaxBackoff:   2 * time.Second,
		},
	)
	if err != nil {
		t.Fatalf(
			"create outbox publisher: %v",
			err,
		)
	}

	processed, err := publisher.ProcessBatch(ctx)
	if err != nil {
		t.Fatalf(
			"process outbox batch: %v",
			err,
		)
	}

	if processed != 1 {
		t.Fatalf(
			"processed events = %d, want 1",
			processed,
		)
	}

	persistedAfterPublish, err := repository.FindByID(
		ctx,
		event.ID(),
	)
	if err != nil {
		t.Fatalf(
			"find published outbox event: %v",
			err,
		)
	}

	if persistedAfterPublish.Status() !=
		domainoutbox.StatusPublished {
		t.Fatalf(
			"status after publish = %q, want %q",
			persistedAfterPublish.Status(),
			domainoutbox.StatusPublished,
		)
	}

	if persistedAfterPublish.Attempts() != 1 {
		t.Fatalf(
			"attempts after publish = %d, want 1",
			persistedAfterPublish.Attempts(),
		)
	}

	if persistedAfterPublish.PublishedAt() == nil {
		t.Fatal(
			"published_at is nil after successful publish",
		)
	}

	if persistedAfterPublish.LastError() != nil {
		t.Fatalf(
			"last_error after successful publish = %q, want nil",
			*persistedAfterPublish.LastError(),
		)
	}

	message := receiveOneMessage(
		t,
		ctx,
		rawClient,
		destination.QueueURL(),
	)

	if message.Body == nil {
		t.Fatal(
			"received SQS message has nil body",
		)
	}

	var receivedPayload any

	if err := json.Unmarshal(
		[]byte(*message.Body),
		&receivedPayload,
	); err != nil {
		t.Fatalf(
			"decode received SQS payload: %v",
			err,
		)
	}

	var expectedPayload any

	if err := json.Unmarshal(
		event.Payload(),
		&expectedPayload,
	); err != nil {
		t.Fatalf(
			"decode expected outbox payload: %v",
			err,
		)
	}

	if !reflect.DeepEqual(
		receivedPayload,
		expectedPayload,
	) {
		t.Fatalf(
			"received SQS payload = %#v, want %#v",
			receivedPayload,
			expectedPayload,
		)
	}

	assertStringMessageAttribute(
		t,
		message.MessageAttributes,
		"eventId",
		event.ID(),
	)

	assertStringMessageAttribute(
		t,
		message.MessageAttributes,
		"eventType",
		event.EventType(),
	)

	assertStringMessageAttribute(
		t,
		message.MessageAttributes,
		"aggregateType",
		event.AggregateType(),
	)

	assertStringMessageAttribute(
		t,
		message.MessageAttributes,
		"aggregateId",
		event.AggregateID(),
	)

	processedAgain, err := publisher.ProcessBatch(ctx)
	if err != nil {
		t.Fatalf(
			"process outbox batch again: %v",
			err,
		)
	}

	if processedAgain != 0 {
		t.Fatalf(
			"processed events on replay = %d, want 0",
			processedAgain,
		)
	}

	assertNoMessage(
		t,
		ctx,
		rawClient,
		destination.QueueURL(),
	)
}

func newTestPool(
	t *testing.T,
) *pgxpool.Pool {
	t.Helper()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		20*time.Second,
	)
	defer cancel()

	pool, err := postgresinfra.Open(
		ctx,
		postgresinfra.Config{
			URL: envOrDefault(
				"TEST_DATABASE_URL",
				defaultTestDatabaseURL,
			),

			MaxConnections: 5,
			MinConnections: 0,
			ConnectTimeout: 10 * time.Second,
			HealthTimeout:  10 * time.Second,
		},
	)
	if err != nil {
		t.Fatalf(
			"open postgres: %v",
			err,
		)
	}

	conn, err := pool.Acquire(
		context.Background(),
	)
	if err != nil {
		pool.Close()

		t.Fatalf(
			"acquire postgres integration test lock connection: %v",
			err,
		)
	}

	if _, err := conn.Exec(
		context.Background(),
		"SELECT pg_advisory_lock($1)",
		integrationTestLockID,
	); err != nil {
		conn.Release()
		pool.Close()

		t.Fatalf(
			"acquire postgres integration test lock: %v",
			err,
		)
	}

	t.Cleanup(
		func() {
			_, _ = conn.Exec(
				context.Background(),
				"SELECT pg_advisory_unlock($1)",
				integrationTestLockID,
			)

			conn.Release()
			pool.Close()
		},
	)

	return pool
}

func cleanDatabase(
	t *testing.T,
	pool *pgxpool.Pool,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	_, err := pool.Exec(
		ctx,
		`
		TRUNCATE TABLE
			outbox_events,
			ledger_entries,
			wager_transactions,
			wallets
		RESTART IDENTITY CASCADE;
		`,
	)
	if err != nil {
		t.Fatalf(
			"clean database: %v",
			err,
		)
	}
}

func newRawSQSClient(
	t *testing.T,
	ctx context.Context,
	endpointURL string,
	region string,
	accessKey string,
	secretKey string,
) *awssqs.Client {
	t.Helper()

	cfg, err := awsconfig.LoadDefaultConfig(
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
	)
	if err != nil {
		t.Fatalf(
			"load AWS configuration: %v",
			err,
		)
	}

	return awssqs.NewFromConfig(
		cfg,
		func(options *awssqs.Options) {
			options.BaseEndpoint =
				aws.String(endpointURL)
		},
	)
}

func purgeQueue(
	t *testing.T,
	ctx context.Context,
	client *awssqs.Client,
	queueURL string,
) {
	t.Helper()

	_, err := client.PurgeQueue(
		ctx,
		&awssqs.PurgeQueueInput{
			QueueUrl: aws.String(
				queueURL,
			),
		},
	)
	if err != nil {
		t.Fatalf(
			"purge SQS queue: %v",
			err,
		)
	}
}

type receivedMessage struct {
	Body *string

	MessageAttributes map[string]types.MessageAttributeValue
}

func receiveOneMessage(
	t *testing.T,
	ctx context.Context,
	client *awssqs.Client,
	queueURL string,
) receivedMessage {
	t.Helper()

	result, err := client.ReceiveMessage(
		ctx,
		&awssqs.ReceiveMessageInput{
			QueueUrl: aws.String(
				queueURL,
			),

			MaxNumberOfMessages: 1,

			WaitTimeSeconds: 2,

			MessageAttributeNames: []string{
				"All",
			},
		},
	)
	if err != nil {
		t.Fatalf(
			"receive SQS message: %v",
			err,
		)
	}

	if len(result.Messages) != 1 {
		t.Fatalf(
			"received messages = %d, want 1",
			len(result.Messages),
		)
	}

	message := result.Messages[0]

	return receivedMessage{
		Body: message.Body,

		MessageAttributes: message.MessageAttributes,
	}
}

func assertNoMessage(
	t *testing.T,
	ctx context.Context,
	client *awssqs.Client,
	queueURL string,
) {
	t.Helper()

	result, err := client.ReceiveMessage(
		ctx,
		&awssqs.ReceiveMessageInput{
			QueueUrl: aws.String(
				queueURL,
			),

			MaxNumberOfMessages: 1,

			WaitTimeSeconds: 1,

			MessageAttributeNames: []string{
				"All",
			},
		},
	)
	if err != nil {
		t.Fatalf(
			"receive SQS messages: %v",
			err,
		)
	}

	if len(result.Messages) != 0 {
		t.Fatalf(
			"received %d unexpected SQS messages",
			len(result.Messages),
		)
	}
}

func assertStringMessageAttribute(
	t *testing.T,
	attributes map[string]types.MessageAttributeValue,
	name string,
	want string,
) {
	t.Helper()

	attribute, ok := attributes[name]
	if !ok {
		t.Fatalf(
			"message attribute %q not found",
			name,
		)
	}

	if attribute.DataType == nil {
		t.Fatalf(
			"message attribute %q has nil data type",
			name,
		)
	}

	if *attribute.DataType != "String" {
		t.Fatalf(
			"message attribute %q data type = %q, want String",
			name,
			*attribute.DataType,
		)
	}

	if attribute.StringValue == nil {
		t.Fatalf(
			"message attribute %q has nil string value",
			name,
		)
	}

	if *attribute.StringValue != want {
		t.Fatalf(
			"message attribute %q = %q, want %q",
			name,
			*attribute.StringValue,
			want,
		)
	}
}

func envOrDefault(
	key string,
	fallback string,
) string {
	value := os.Getenv(key)

	if value == "" {
		return fallback
	}

	return value
}
