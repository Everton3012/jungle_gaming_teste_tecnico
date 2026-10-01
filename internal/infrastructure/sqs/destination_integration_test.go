package sqs_test

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"

	domainoutbox "jungle_gaming_teste_tecnico/internal/domain/outbox"
	sqsinfra "jungle_gaming_teste_tecnico/internal/infrastructure/sqs"
)

const (
	defaultTestEndpointURL = "http://localhost:4566"
	defaultTestRegion      = "us-east-1"
	defaultTestAccessKey   = "test"
	defaultTestSecretKey   = "test"
	defaultTestQueueName   = "wager-events.fifo"
)

func TestDestinationPublishesOutboxEventToRealSQS(
	t *testing.T,
) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	endpointURL := envOrDefault(
		"TEST_SQS_ENDPOINT_URL",
		defaultTestEndpointURL,
	)

	region := envOrDefault(
		"TEST_AWS_REGION",
		defaultTestRegion,
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

	rawClient := newRawClient(
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
			"eventId":       "event-sqs-integration-001",
			"eventType":     "WagerTransactionProcessed",
			"aggregateId":   "transaction-sqs-integration-001",
			"correlationId": "correlation-sqs-integration-001",
			"occurredAt":    now.Format(time.RFC3339Nano),
			"version":       1,
			"data": map[string]any{
				"transactionId": "transaction-sqs-integration-001",
			},
		},
	)
	if err != nil {
		t.Fatalf(
			"marshal payload: %v",
			err,
		)
	}

	event, err := domainoutbox.NewEvent(
		domainoutbox.NewEventInput{
			ID: "event-sqs-integration-001",

			EventType: "WagerTransactionProcessed",

			AggregateType: "WAGER_TRANSACTION",

			AggregateID: "transaction-sqs-integration-001",

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

	if err := destination.Publish(
		ctx,
		event,
	); err != nil {
		t.Fatalf(
			"publish event: %v",
			err,
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

	var received map[string]any

	if err := json.Unmarshal(
		[]byte(*message.Body),
		&received,
	); err != nil {
		t.Fatalf(
			"decode received payload: %v",
			err,
		)
	}

	if received["eventId"] !=
		"event-sqs-integration-001" {
		t.Fatalf(
			"eventId = %#v, want %q",
			received["eventId"],
			"event-sqs-integration-001",
		)
	}

	if received["eventType"] !=
		"WagerTransactionProcessed" {
		t.Fatalf(
			"eventType = %#v, want %q",
			received["eventType"],
			"WagerTransactionProcessed",
		)
	}

	if received["aggregateId"] !=
		"transaction-sqs-integration-001" {
		t.Fatalf(
			"aggregateId = %#v, want %q",
			received["aggregateId"],
			"transaction-sqs-integration-001",
		)
	}

	assertStringMessageAttribute(
		t,
		message.MessageAttributes,
		"eventId",
		"event-sqs-integration-001",
	)

	assertStringMessageAttribute(
		t,
		message.MessageAttributes,
		"eventType",
		"WagerTransactionProcessed",
	)

	assertStringMessageAttribute(
		t,
		message.MessageAttributes,
		"aggregateType",
		"WAGER_TRANSACTION",
	)

	assertStringMessageAttribute(
		t,
		message.MessageAttributes,
		"aggregateId",
		"transaction-sqs-integration-001",
	)
}

func TestDestinationRejectsNonFIFOQueue(
	t *testing.T,
) {
	ctx, cancel := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancel()

	client, err := sqsinfra.NewClient(
		ctx,
		sqsinfra.Config{
			Region: envOrDefault(
				"TEST_AWS_REGION",
				defaultTestRegion,
			),

			EndpointURL: envOrDefault(
				"TEST_SQS_ENDPOINT_URL",
				defaultTestEndpointURL,
			),

			AccessKeyID: envOrDefault(
				"TEST_AWS_ACCESS_KEY_ID",
				defaultTestAccessKey,
			),

			SecretAccessKey: envOrDefault(
				"TEST_AWS_SECRET_ACCESS_KEY",
				defaultTestSecretKey,
			),
		},
	)
	if err != nil {
		t.Fatalf(
			"create SQS client: %v",
			err,
		)
	}

	_, err = sqsinfra.NewDestination(
		ctx,
		client,
		"not-fifo",
	)

	if err == nil {
		t.Fatal(
			"expected error for non-FIFO queue",
		)
	}
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

func receiveOneMessage(
	t *testing.T,
	ctx context.Context,
	client *awssqs.Client,
	queueURL string,
) awssqsMessage {
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

	return awssqsMessage{
		Body: message.Body,

		Attributes: message.Attributes,

		MessageAttributes: message.MessageAttributes,
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

type awssqsMessage struct {
	Body *string

	Attributes map[string]string

	MessageAttributes map[string]types.MessageAttributeValue
}

func newRawClient(
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
			"load AWS config: %v",
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
