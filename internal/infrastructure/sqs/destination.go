package sqs

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	awssqs "github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"

	domainoutbox "jungle_gaming_teste_tecnico/internal/domain/outbox"
)

var (
	ErrRegionRequired     = errors.New("AWS region is required")
	ErrAccessKeyRequired  = errors.New("AWS access key id is required")
	ErrSecretKeyRequired  = errors.New("AWS secret access key is required")
	ErrQueueNameRequired  = errors.New("SQS queue name is required")
	ErrClientRequired     = errors.New("SQS client is required")
	ErrEventRequired      = errors.New("outbox event is required")
	ErrQueueURLRequired   = errors.New("SQS queue URL is required")
	ErrInvalidQueue       = errors.New("SQS queue must be FIFO")
	ErrInvalidAggregateID = errors.New("outbox aggregate id is required")
	ErrInvalidEventID     = errors.New("outbox event id is required")
)

type Config struct {
	Region          string
	EndpointURL     string
	AccessKeyID     string
	SecretAccessKey string
	QueueName       string
}

type Client struct {
	sqs *awssqs.Client
}

type Destination struct {
	client   *awssqs.Client
	queueURL string
}

func NewClient(
	ctx context.Context,
	cfg Config,
) (*Client, error) {
	cfg.Region = strings.TrimSpace(cfg.Region)
	cfg.EndpointURL = strings.TrimSpace(cfg.EndpointURL)
	cfg.AccessKeyID = strings.TrimSpace(cfg.AccessKeyID)
	cfg.SecretAccessKey = strings.TrimSpace(cfg.SecretAccessKey)

	if cfg.Region == "" {
		return nil, ErrRegionRequired
	}

	if cfg.AccessKeyID == "" {
		return nil, ErrAccessKeyRequired
	}

	if cfg.SecretAccessKey == "" {
		return nil, ErrSecretKeyRequired
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(
		ctx,
		awsconfig.WithRegion(cfg.Region),
		awsconfig.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				cfg.AccessKeyID,
				cfg.SecretAccessKey,
				"",
			),
		),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"load AWS configuration: %w",
			err,
		)
	}

	client := awssqs.NewFromConfig(
		awsCfg,
		func(options *awssqs.Options) {
			if cfg.EndpointURL != "" {
				options.BaseEndpoint =
					aws.String(cfg.EndpointURL)
			}
		},
	)

	return &Client{
		sqs: client,
	}, nil
}

func NewDestination(
	ctx context.Context,
	client *Client,
	queueName string,
) (*Destination, error) {
	if client == nil || client.sqs == nil {
		return nil, ErrClientRequired
	}

	queueName = strings.TrimSpace(queueName)

	if queueName == "" {
		return nil, ErrQueueNameRequired
	}

	if !strings.HasSuffix(
		queueName,
		".fifo",
	) {
		return nil, ErrInvalidQueue
	}

	result, err := client.sqs.GetQueueUrl(
		ctx,
		&awssqs.GetQueueUrlInput{
			QueueName: aws.String(queueName),
		},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"get SQS queue URL %q: %w",
			queueName,
			err,
		)
	}

	if result.QueueUrl == nil ||
		strings.TrimSpace(*result.QueueUrl) == "" {
		return nil, ErrQueueURLRequired
	}

	return &Destination{
		client:   client.sqs,
		queueURL: *result.QueueUrl,
	}, nil
}

func (d *Destination) Publish(
	ctx context.Context,
	event *domainoutbox.Event,
) error {
	if event == nil {
		return ErrEventRequired
	}

	eventID := strings.TrimSpace(
		event.ID(),
	)
	if eventID == "" {
		return ErrInvalidEventID
	}

	aggregateID := strings.TrimSpace(
		event.AggregateID(),
	)
	if aggregateID == "" {
		return ErrInvalidAggregateID
	}

	messageAttributes := map[string]types.MessageAttributeValue{
		"eventId": {
			DataType:    aws.String("String"),
			StringValue: aws.String(eventID),
		},
		"eventType": {
			DataType: aws.String("String"),
			StringValue: aws.String(
				event.EventType(),
			),
		},
		"aggregateType": {
			DataType: aws.String("String"),
			StringValue: aws.String(
				event.AggregateType(),
			),
		},
		"aggregateId": {
			DataType: aws.String("String"),
			StringValue: aws.String(
				aggregateID,
			),
		},
	}

	_, err := d.client.SendMessage(
		ctx,
		&awssqs.SendMessageInput{
			QueueUrl:               aws.String(d.queueURL),
			MessageBody:            aws.String(string(event.Payload())),
			MessageGroupId:         aws.String(aggregateID),
			MessageDeduplicationId: aws.String(eventID),
			MessageAttributes:      messageAttributes,
		},
	)
	if err != nil {
		return fmt.Errorf(
			"publish outbox event %s to SQS: %w",
			eventID,
			err,
		)
	}

	return nil
}

func (d *Destination) QueueURL() string {
	if d == nil {
		return ""
	}

	return d.queueURL
}
