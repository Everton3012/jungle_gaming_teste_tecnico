package sqs

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
)

var (
	ErrConsumerRequired      = errors.New("SQS consumer client is required")
	ErrReceiptHandleRequired = errors.New("SQS receipt handle is required")
)

type ConsumerConfig struct {
	QueueName         string
	MaxMessages       int32
	WaitTime          time.Duration
	VisibilityTimeout time.Duration
}

type Message struct {
	ID                     string
	Body                   string
	ReceiptHandle          string
	ReceiveCount           int
	MessageGroupID         string
	MessageDeduplicationID string
	Attributes             map[string]string
}

type Consumer struct {
	client            *awssqs.Client
	queueURL          string
	maxMessages       int32
	waitTimeSeconds   int32
	visibilitySeconds int32
}

func NewConsumer(
	ctx context.Context,
	client *Client,
	cfg ConsumerConfig,
) (*Consumer, error) {
	if client == nil || client.sqs == nil {
		return nil, ErrConsumerRequired
	}

	queueName := strings.TrimSpace(cfg.QueueName)
	if queueName == "" {
		return nil, ErrQueueNameRequired
	}

	if !strings.HasSuffix(queueName, ".fifo") {
		return nil, ErrInvalidQueue
	}

	if cfg.MaxMessages <= 0 || cfg.MaxMessages > 10 {
		return nil, errors.New(
			"SQS consumer max messages must be between 1 and 10",
		)
	}

	if cfg.WaitTime < 0 || cfg.WaitTime > 20*time.Second {
		return nil, errors.New(
			"SQS consumer wait time must be between 0 and 20 seconds",
		)
	}

	if cfg.VisibilityTimeout <= 0 {
		return nil, errors.New(
			"SQS consumer visibility timeout must be greater than zero",
		)
	}

	result, err := client.sqs.GetQueueUrl(
		ctx,
		&awssqs.GetQueueUrlInput{
			QueueName: aws.String(queueName),
		},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"get SQS consumer queue URL %q: %w",
			queueName,
			err,
		)
	}

	if result.QueueUrl == nil ||
		strings.TrimSpace(*result.QueueUrl) == "" {
		return nil, ErrQueueURLRequired
	}

	return &Consumer{
		client:            client.sqs,
		queueURL:          strings.TrimSpace(*result.QueueUrl),
		maxMessages:       cfg.MaxMessages,
		waitTimeSeconds:   int32(cfg.WaitTime / time.Second),
		visibilitySeconds: int32(cfg.VisibilityTimeout / time.Second),
	}, nil
}

func (c *Consumer) Receive(
	ctx context.Context,
) ([]Message, error) {
	result, err := c.client.ReceiveMessage(
		ctx,
		&awssqs.ReceiveMessageInput{
			QueueUrl: aws.String(c.queueURL),

			MaxNumberOfMessages: c.maxMessages,

			WaitTimeSeconds: c.waitTimeSeconds,

			VisibilityTimeout: c.visibilitySeconds,

			MessageSystemAttributeNames: []types.MessageSystemAttributeName{
				types.MessageSystemAttributeNameApproximateReceiveCount,
				types.MessageSystemAttributeNameMessageGroupId,
				types.MessageSystemAttributeNameMessageDeduplicationId,
			},

			MessageAttributeNames: []string{
				"All",
			},
		},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"receive messages from SQS: %w",
			err,
		)
	}

	messages := make(
		[]Message,
		0,
		len(result.Messages),
	)

	for _, received := range result.Messages {
		message := Message{
			Attributes: make(map[string]string),
		}

		if received.MessageId != nil {
			message.ID =
				strings.TrimSpace(*received.MessageId)
		}

		if received.Body != nil {
			message.Body = *received.Body
		}

		if received.ReceiptHandle != nil {
			message.ReceiptHandle =
				strings.TrimSpace(*received.ReceiptHandle)
		}

		if value, ok := received.Attributes[string(types.MessageSystemAttributeNameApproximateReceiveCount)]; ok {

			count, parseErr := strconv.Atoi(value)
			if parseErr == nil {
				message.ReceiveCount = count
			}
		}

		if value, ok := received.Attributes[string(types.MessageSystemAttributeNameMessageGroupId)]; ok {

			message.MessageGroupID = value
		}
		if value, ok := received.Attributes[string(types.MessageSystemAttributeNameMessageDeduplicationId)]; ok {

			message.MessageDeduplicationID = value
		}

		for key, value := range received.MessageAttributes {
			if value.StringValue != nil {
				message.Attributes[key] =
					*value.StringValue
			}
		}

		messages = append(
			messages,
			message,
		)
	}

	return messages, nil
}

func (c *Consumer) Delete(
	ctx context.Context,
	receiptHandle string,
) error {
	receiptHandle =
		strings.TrimSpace(receiptHandle)

	if receiptHandle == "" {
		return ErrReceiptHandleRequired
	}

	_, err := c.client.DeleteMessage(
		ctx,
		&awssqs.DeleteMessageInput{
			QueueUrl:      aws.String(c.queueURL),
			ReceiptHandle: aws.String(receiptHandle),
		},
	)
	if err != nil {
		return fmt.Errorf(
			"delete SQS message: %w",
			err,
		)
	}

	return nil
}

func (c *Consumer) ChangeVisibility(
	ctx context.Context,
	receiptHandle string,
	timeout time.Duration,
) error {
	receiptHandle =
		strings.TrimSpace(receiptHandle)

	if receiptHandle == "" {
		return ErrReceiptHandleRequired
	}

	if timeout < 0 ||
		timeout > 12*time.Hour {
		return errors.New(
			"SQS visibility timeout must be between 0 and 12 hours",
		)
	}

	_, err := c.client.ChangeMessageVisibility(
		ctx,
		&awssqs.ChangeMessageVisibilityInput{
			QueueUrl: aws.String(
				c.queueURL,
			),

			ReceiptHandle: aws.String(
				receiptHandle,
			),

			VisibilityTimeout: int32(
				timeout / time.Second,
			),
		},
	)
	if err != nil {
		return fmt.Errorf(
			"change SQS message visibility: %w",
			err,
		)
	}

	return nil
}

func (c *Consumer) QueueURL() string {
	if c == nil {
		return ""
	}

	return c.queueURL
}
