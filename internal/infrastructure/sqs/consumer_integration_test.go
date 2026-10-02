package sqs_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"

	sqsinfra "jungle_gaming_teste_tecnico/internal/infrastructure/sqs"
)

const defaultConsumerTestQueueName = "sqs-consumer-integration.fifo"

func TestConsumerReceivesAndDeletesMessage(t *testing.T) {
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
		"TEST_TRANSACTION_QUEUE_NAME",
		defaultConsumerTestQueueName,
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

	consumer, err := sqsinfra.NewConsumer(
		ctx,
		client,
		sqsinfra.ConsumerConfig{
			QueueName:         queueName,
			MaxMessages:       1,
			WaitTime:          time.Second,
			VisibilityTimeout: 30 * time.Second,
		},
	)
	if err != nil {
		t.Fatalf(
			"create SQS consumer: %v",
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
		consumer.QueueURL(),
	)

	body := `{"transactionId":"transaction-consumer-1","playerId":"player-consumer-1","type":"BET","amount":"10.00","currency":"BRL"}`

	deduplicationID := fmt.Sprintf(
		"consumer-test-%d",
		time.Now().UnixNano(),
	)

	_, err = rawClient.SendMessage(
		ctx,
		&awssqs.SendMessageInput{
			QueueUrl: aws.String(
				consumer.QueueURL(),
			),

			MessageBody: aws.String(
				body,
			),

			MessageGroupId: aws.String(
				"player-consumer-1",
			),

			MessageDeduplicationId: aws.String(
				deduplicationID,
			),

			MessageAttributes: map[string]types.MessageAttributeValue{
				"eventType": {
					DataType: aws.String(
						"String",
					),
					StringValue: aws.String(
						"WagerTransactionRequested",
					),
				},
			},
		},
	)
	if err != nil {
		t.Fatalf(
			"send SQS message: %v",
			err,
		)
	}

	messages, err := consumer.Receive(ctx)
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

	message := messages[0]

	if message.ID == "" {
		t.Fatal("message ID is empty")
	}

	if message.Body != body {
		t.Fatalf(
			"body = %q, want %q",
			message.Body,
			body,
		)
	}

	if message.ReceiptHandle == "" {
		t.Fatal("receipt handle is empty")
	}

	if message.ReceiveCount != 1 {
		t.Fatalf(
			"receive count = %d, want 1",
			message.ReceiveCount,
		)
	}

	if message.MessageGroupID != "player-consumer-1" {
		t.Fatalf(
			"message group ID = %q, want %q",
			message.MessageGroupID,
			"player-consumer-1",
		)
	}

	if message.MessageDeduplicationID != deduplicationID {
		t.Fatalf(
			"message deduplication ID = %q, want %q",
			message.MessageDeduplicationID,
			deduplicationID,
		)
	}

	if message.Attributes["eventType"] !=
		"WagerTransactionRequested" {
		t.Fatalf(
			"eventType = %q, want %q",
			message.Attributes["eventType"],
			"WagerTransactionRequested",
		)
	}

	if err := consumer.Delete(
		ctx,
		message.ReceiptHandle,
	); err != nil {
		t.Fatalf(
			"delete SQS message: %v",
			err,
		)
	}

	time.Sleep(
		100 * time.Millisecond,
	)

	messages, err = consumer.Receive(ctx)
	if err != nil {
		t.Fatalf(
			"receive after delete: %v",
			err,
		)
	}

	if len(messages) != 0 {
		t.Fatalf(
			"received %d messages after delete, want 0",
			len(messages),
		)
	}
}

func TestConsumerChangesMessageVisibility(
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
		"TEST_TRANSACTION_QUEUE_NAME",
		defaultConsumerTestQueueName,
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

	consumer, err := sqsinfra.NewConsumer(
		ctx,
		client,
		sqsinfra.ConsumerConfig{
			QueueName:         queueName,
			MaxMessages:       1,
			WaitTime:          time.Second,
			VisibilityTimeout: 30 * time.Second,
		},
	)
	if err != nil {
		t.Fatalf(
			"create SQS consumer: %v",
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
		consumer.QueueURL(),
	)

	deduplicationID := fmt.Sprintf(
		"visibility-test-%d",
		time.Now().UnixNano(),
	)

	_, err = rawClient.SendMessage(
		ctx,
		&awssqs.SendMessageInput{
			QueueUrl: aws.String(
				consumer.QueueURL(),
			),

			MessageBody: aws.String(
				`{"transactionId":"visibility-test"}`,
			),

			MessageGroupId: aws.String(
				"visibility-test",
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

	messages, err := consumer.Receive(ctx)
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

	if err := consumer.ChangeVisibility(
		ctx,
		messages[0].ReceiptHandle,
		0,
	); err != nil {
		t.Fatalf(
			"change message visibility: %v",
			err,
		)
	}

	messages, err = consumer.Receive(ctx)
	if err != nil {
		t.Fatalf(
			"receive visible message: %v",
			err,
		)
	}

	if len(messages) != 1 {
		t.Fatalf(
			"received %d messages after visibility reset, want 1",
			len(messages),
		)
	}

	if messages[0].ReceiveCount < 2 {
		t.Fatalf(
			"receive count = %d, want at least 2",
			messages[0].ReceiveCount,
		)
	}

	if err := consumer.Delete(
		ctx,
		messages[0].ReceiptHandle,
	); err != nil {
		t.Fatalf(
			"delete SQS message: %v",
			err,
		)
	}
}

func TestConsumerRejectsInvalidConfiguration(
	t *testing.T,
) {
	ctx := context.Background()

	client, err := sqsinfra.NewClient(
		ctx,
		sqsinfra.Config{
			Region:          defaultTestRegion,
			EndpointURL:     defaultTestEndpointURL,
			AccessKeyID:     defaultTestAccessKey,
			SecretAccessKey: defaultTestSecretKey,
			QueueName:       defaultConsumerTestQueueName,
		},
	)
	if err != nil {
		t.Fatalf(
			"create SQS client: %v",
			err,
		)
	}

	tests := []struct {
		name string
		cfg  sqsinfra.ConsumerConfig
	}{
		{
			name: "empty queue name",
			cfg: sqsinfra.ConsumerConfig{
				MaxMessages:       1,
				WaitTime:          time.Second,
				VisibilityTimeout: 30 * time.Second,
			},
		},
		{
			name: "non FIFO queue",
			cfg: sqsinfra.ConsumerConfig{
				QueueName:         "standard-queue",
				MaxMessages:       1,
				WaitTime:          time.Second,
				VisibilityTimeout: 30 * time.Second,
			},
		},
		{
			name: "zero max messages",
			cfg: sqsinfra.ConsumerConfig{
				QueueName:         defaultConsumerTestQueueName,
				MaxMessages:       0,
				WaitTime:          time.Second,
				VisibilityTimeout: 30 * time.Second,
			},
		},
		{
			name: "max messages above limit",
			cfg: sqsinfra.ConsumerConfig{
				QueueName:         defaultConsumerTestQueueName,
				MaxMessages:       11,
				WaitTime:          time.Second,
				VisibilityTimeout: 30 * time.Second,
			},
		},
		{
			name: "negative wait time",
			cfg: sqsinfra.ConsumerConfig{
				QueueName:         defaultConsumerTestQueueName,
				MaxMessages:       1,
				WaitTime:          -time.Second,
				VisibilityTimeout: 30 * time.Second,
			},
		},
		{
			name: "wait time above limit",
			cfg: sqsinfra.ConsumerConfig{
				QueueName:         defaultConsumerTestQueueName,
				MaxMessages:       1,
				WaitTime:          21 * time.Second,
				VisibilityTimeout: 30 * time.Second,
			},
		},
		{
			name: "zero visibility timeout",
			cfg: sqsinfra.ConsumerConfig{
				QueueName:   defaultConsumerTestQueueName,
				MaxMessages: 1,
				WaitTime:    time.Second,
			},
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				_, err := sqsinfra.NewConsumer(
					ctx,
					client,
					test.cfg,
				)

				if err == nil {
					t.Fatal(
						"expected error, got nil",
					)
				}
			},
		)
	}
}

func TestConsumerRejectsEmptyReceiptHandle(
	t *testing.T,
) {
	ctx := context.Background()

	client, err := sqsinfra.NewClient(
		ctx,
		sqsinfra.Config{
			Region:          defaultTestRegion,
			EndpointURL:     defaultTestEndpointURL,
			AccessKeyID:     defaultTestAccessKey,
			SecretAccessKey: defaultTestSecretKey,
			QueueName:       defaultConsumerTestQueueName,
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
			QueueName:         defaultConsumerTestQueueName,
			MaxMessages:       1,
			WaitTime:          time.Second,
			VisibilityTimeout: 30 * time.Second,
		},
	)
	if err != nil {
		t.Fatalf(
			"create consumer: %v",
			err,
		)
	}

	if err := consumer.Delete(
		ctx,
		" ",
	); err == nil {
		t.Fatal(
			"expected delete error, got nil",
		)
	}

	if err := consumer.ChangeVisibility(
		ctx,
		"",
		time.Second,
	); err == nil {
		t.Fatal(
			"expected visibility error, got nil",
		)
	}
}
