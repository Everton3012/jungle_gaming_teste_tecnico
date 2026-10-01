package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultHTTPAddress                = ":8080"
	defaultDatabaseMaxConnections     = 20
	defaultDatabaseMinConnections     = 2
	defaultDatabaseConnectTimeout     = 5 * time.Second
	defaultDatabaseOperationTimeout   = 5 * time.Second
	defaultReferencePollInterval      = 2 * time.Second
	defaultReferenceRetryBackoff      = 5 * time.Second
	defaultReferenceBatchSize         = 100
	defaultOutboxPollInterval         = 2 * time.Second
	defaultOutboxRetryBackoff         = 5 * time.Second
	defaultOutboxMaxBackoff           = 5 * time.Minute
	defaultOutboxBatchSize            = 100
	defaultApplicationShutdownTimeout = 10 * time.Second

	defaultAWSRegion          = "us-east-1"
	defaultAWSEndpointURL     = "http://localstack:4566"
	defaultAWSAccessKeyID     = "test"
	defaultAWSSecretAccessKey = "test"
	defaultSQSOutboxQueueName = "wager-events.fifo"
)

type Config struct {
	HTTP        HTTPConfig
	Database    DatabaseConfig
	Reference   ReferenceConfig
	Outbox      OutboxConfig
	SQS         SQSConfig
	Application ApplicationConfig
}

type HTTPConfig struct {
	Address string
}

type DatabaseConfig struct {
	URL              string
	MaxConnections   int32
	MinConnections   int32
	ConnectTimeout   time.Duration
	OperationTimeout time.Duration
}

type ReferenceConfig struct {
	PollInterval time.Duration
	RetryBackoff time.Duration
	BatchSize    int
}

type OutboxConfig struct {
	PollInterval time.Duration
	RetryBackoff time.Duration
	MaxBackoff   time.Duration
	BatchSize    int
}

type SQSConfig struct {
	Region          string
	EndpointURL     string
	AccessKeyID     string
	SecretAccessKey string
	OutboxQueueName string
}

type ApplicationConfig struct {
	ShutdownTimeout time.Duration
}

func Load() (Config, error) {
	maxConnections, err := envInt(
		"DATABASE_MAX_CONNECTIONS",
		defaultDatabaseMaxConnections,
	)
	if err != nil {
		return Config{}, err
	}

	minConnections, err := envInt(
		"DATABASE_MIN_CONNECTIONS",
		defaultDatabaseMinConnections,
	)
	if err != nil {
		return Config{}, err
	}

	connectTimeout, err := envDuration(
		"DATABASE_CONNECT_TIMEOUT",
		defaultDatabaseConnectTimeout,
	)
	if err != nil {
		return Config{}, err
	}

	operationTimeout, err := envDuration(
		"DATABASE_OPERATION_TIMEOUT",
		defaultDatabaseOperationTimeout,
	)
	if err != nil {
		return Config{}, err
	}

	referencePollInterval, err := envDuration(
		"REFERENCE_POLL_INTERVAL",
		defaultReferencePollInterval,
	)
	if err != nil {
		return Config{}, err
	}

	referenceRetryBackoff, err := envDuration(
		"REFERENCE_RETRY_BACKOFF",
		defaultReferenceRetryBackoff,
	)
	if err != nil {
		return Config{}, err
	}

	referenceBatchSize, err := envInt(
		"REFERENCE_BATCH_SIZE",
		defaultReferenceBatchSize,
	)
	if err != nil {
		return Config{}, err
	}

	outboxPollInterval, err := envDuration(
		"OUTBOX_POLL_INTERVAL",
		defaultOutboxPollInterval,
	)
	if err != nil {
		return Config{}, err
	}

	outboxRetryBackoff, err := envDuration(
		"OUTBOX_RETRY_BACKOFF",
		defaultOutboxRetryBackoff,
	)
	if err != nil {
		return Config{}, err
	}

	outboxMaxBackoff, err := envDuration(
		"OUTBOX_MAX_BACKOFF",
		defaultOutboxMaxBackoff,
	)
	if err != nil {
		return Config{}, err
	}

	outboxBatchSize, err := envInt(
		"OUTBOX_BATCH_SIZE",
		defaultOutboxBatchSize,
	)
	if err != nil {
		return Config{}, err
	}

	shutdownTimeout, err := envDuration(
		"APPLICATION_SHUTDOWN_TIMEOUT",
		defaultApplicationShutdownTimeout,
	)
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		HTTP: HTTPConfig{
			Address: envOrDefault(
				"HTTP_ADDRESS",
				defaultHTTPAddress,
			),
		},
		Database: DatabaseConfig{
			URL:              strings.TrimSpace(os.Getenv("DATABASE_URL")),
			MaxConnections:   int32(maxConnections),
			MinConnections:   int32(minConnections),
			ConnectTimeout:   connectTimeout,
			OperationTimeout: operationTimeout,
		},
		Reference: ReferenceConfig{
			PollInterval: referencePollInterval,
			RetryBackoff: referenceRetryBackoff,
			BatchSize:    referenceBatchSize,
		},
		Outbox: OutboxConfig{
			PollInterval: outboxPollInterval,
			RetryBackoff: outboxRetryBackoff,
			MaxBackoff:   outboxMaxBackoff,
			BatchSize:    outboxBatchSize,
		},
		SQS: SQSConfig{
			Region: envOrDefault(
				"AWS_REGION",
				defaultAWSRegion,
			),
			EndpointURL: envOrDefault(
				"AWS_ENDPOINT_URL",
				defaultAWSEndpointURL,
			),
			AccessKeyID: envOrDefault(
				"AWS_ACCESS_KEY_ID",
				defaultAWSAccessKeyID,
			),
			SecretAccessKey: envOrDefault(
				"AWS_SECRET_ACCESS_KEY",
				defaultAWSSecretAccessKey,
			),
			OutboxQueueName: envOrDefault(
				"SQS_OUTBOX_QUEUE_NAME",
				defaultSQSOutboxQueueName,
			),
		},
		Application: ApplicationConfig{
			ShutdownTimeout: shutdownTimeout,
		},
	}

	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (c Config) Validate() error {
	if strings.TrimSpace(c.Database.URL) == "" {
		return errors.New("DATABASE_URL is required")
	}

	if c.Database.MaxConnections <= 0 {
		return errors.New(
			"DATABASE_MAX_CONNECTIONS must be greater than zero",
		)
	}

	if c.Database.MinConnections < 0 {
		return errors.New(
			"DATABASE_MIN_CONNECTIONS cannot be negative",
		)
	}

	if c.Database.MinConnections > c.Database.MaxConnections {
		return errors.New(
			"DATABASE_MIN_CONNECTIONS cannot exceed DATABASE_MAX_CONNECTIONS",
		)
	}

	if c.Database.ConnectTimeout <= 0 {
		return errors.New(
			"DATABASE_CONNECT_TIMEOUT must be greater than zero",
		)
	}

	if c.Database.OperationTimeout <= 0 {
		return errors.New(
			"DATABASE_OPERATION_TIMEOUT must be greater than zero",
		)
	}

	if c.Reference.PollInterval <= 0 {
		return errors.New(
			"REFERENCE_POLL_INTERVAL must be greater than zero",
		)
	}

	if c.Reference.RetryBackoff <= 0 {
		return errors.New(
			"REFERENCE_RETRY_BACKOFF must be greater than zero",
		)
	}

	if c.Reference.BatchSize <= 0 {
		return errors.New(
			"REFERENCE_BATCH_SIZE must be greater than zero",
		)
	}

	if c.Outbox.PollInterval != 0 && c.Outbox.PollInterval <= 0 {
		return errors.New(
			"OUTBOX_POLL_INTERVAL must be greater than zero",
		)
	}

	if c.Outbox.RetryBackoff != 0 && c.Outbox.RetryBackoff <= 0 {
		return errors.New(
			"OUTBOX_RETRY_BACKOFF must be greater than zero",
		)
	}

	if c.Outbox.MaxBackoff != 0 && c.Outbox.MaxBackoff <= 0 {
		return errors.New(
			"OUTBOX_MAX_BACKOFF must be greater than zero",
		)
	}

	if c.Outbox.RetryBackoff != 0 &&
		c.Outbox.MaxBackoff != 0 &&
		c.Outbox.MaxBackoff < c.Outbox.RetryBackoff {
		return errors.New(
			"OUTBOX_MAX_BACKOFF cannot be lower than OUTBOX_RETRY_BACKOFF",
		)
	}

	if c.Outbox.BatchSize < 0 {
		return errors.New(
			"OUTBOX_BATCH_SIZE cannot be negative",
		)
	}

	if strings.TrimSpace(c.SQS.Region) != "" {
		if strings.TrimSpace(c.SQS.AccessKeyID) == "" {
			return errors.New(
				"AWS_ACCESS_KEY_ID is required when SQS is configured",
			)
		}

		if strings.TrimSpace(c.SQS.SecretAccessKey) == "" {
			return errors.New(
				"AWS_SECRET_ACCESS_KEY is required when SQS is configured",
			)
		}

		if strings.TrimSpace(c.SQS.OutboxQueueName) == "" {
			return errors.New(
				"SQS_OUTBOX_QUEUE_NAME is required when SQS is configured",
			)
		}

		if !strings.HasSuffix(
			strings.TrimSpace(c.SQS.OutboxQueueName),
			".fifo",
		) {
			return errors.New(
				"SQS_OUTBOX_QUEUE_NAME must reference a FIFO queue",
			)
		}
	}

	if c.Application.ShutdownTimeout <= 0 {
		return errors.New(
			"APPLICATION_SHUTDOWN_TIMEOUT must be greater than zero",
		)
	}

	if strings.TrimSpace(c.HTTP.Address) == "" {
		return errors.New("HTTP_ADDRESS is required")
	}

	return nil
}

func envOrDefault(
	key string,
	fallback string,
) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	return value
}

func envInt(
	key string,
	fallback int,
) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf(
			"%s must be a valid integer: %w",
			key,
			err,
		)
	}

	return parsed, nil
}

func envDuration(
	key string,
	fallback time.Duration,
) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf(
			"%s must be a valid duration: %w",
			key,
			err,
		)
	}

	return parsed, nil
}

func (c Config) String() string {
	return fmt.Sprintf(
		"http=%s db_max=%d db_min=%d reference_batch=%d outbox_batch=%d sqs_region=%s sqs_queue=%s",
		c.HTTP.Address,
		c.Database.MaxConnections,
		c.Database.MinConnections,
		c.Reference.BatchSize,
		c.Outbox.BatchSize,
		c.SQS.Region,
		c.SQS.OutboxQueueName,
	)
}
