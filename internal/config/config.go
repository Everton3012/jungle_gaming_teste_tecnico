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
	defaultReferenceMaxBackoff        = 5 * time.Minute
	defaultReferenceLeaseDuration     = 30 * time.Second
	defaultReferenceBatchSize         = 100
	defaultReferenceMaxAttempts       = 10
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

	defaultSQSConsumerQueueName         = "wager-transactions.fifo"
	defaultSQSConsumerMaxMessages       = 10
	defaultSQSConsumerWaitTime          = 20 * time.Second
	defaultSQSConsumerVisibilityTimeout = 30 * time.Second

	defaultAuthIssuer          = "http://localhost:8085/realms/jungle"
	defaultAuthJWKSURL         = "http://keycloak:8080/realms/jungle/protocol/openid-connect/certs"
	defaultAuthWalletClientID  = "wallet-service"
	defaultAuthProviderClients = "provider-a,provider-b"
)

type Config struct {
	HTTP        HTTPConfig
	Database    DatabaseConfig
	Reference   ReferenceConfig
	Outbox      OutboxConfig
	SQS         SQSConfig
	Auth        AuthConfig
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
	PollInterval  time.Duration
	RetryBackoff  time.Duration
	MaxBackoff    time.Duration
	LeaseDuration time.Duration
	BatchSize     int
	MaxAttempts   int
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

	ConsumerQueueName         string
	ConsumerMaxMessages       int32
	ConsumerWaitTime          time.Duration
	ConsumerVisibilityTimeout time.Duration
}

type AuthConfig struct {
	Issuer          string
	JWKSURL         string
	WalletClientID  string
	ProviderClients []string
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

	referenceMaxAttempts, err := envInt(
		"REFERENCE_MAX_ATTEMPTS",
		defaultReferenceMaxAttempts,
	)
	if err != nil {
		return Config{}, err
	}

	referenceMaxBackoff, err := envDuration(
		"REFERENCE_MAX_BACKOFF",
		defaultReferenceMaxBackoff,
	)
	if err != nil {
		return Config{}, err
	}

	referenceLeaseDuration, err := envDuration(
		"REFERENCE_LEASE_DURATION",
		defaultReferenceLeaseDuration,
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

	consumerMaxMessages, err := envInt(
		"SQS_CONSUMER_MAX_MESSAGES",
		defaultSQSConsumerMaxMessages,
	)
	if err != nil {
		return Config{}, err
	}

	consumerWaitTime, err := envDuration(
		"SQS_CONSUMER_WAIT_TIME",
		defaultSQSConsumerWaitTime,
	)
	if err != nil {
		return Config{}, err
	}

	consumerVisibilityTimeout, err := envDuration(
		"SQS_CONSUMER_VISIBILITY_TIMEOUT",
		defaultSQSConsumerVisibilityTimeout,
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
			URL: strings.TrimSpace(
				os.Getenv("DATABASE_URL"),
			),
			MaxConnections:   int32(maxConnections),
			MinConnections:   int32(minConnections),
			ConnectTimeout:   connectTimeout,
			OperationTimeout: operationTimeout,
		},

		Reference: ReferenceConfig{
			PollInterval:  referencePollInterval,
			RetryBackoff:  referenceRetryBackoff,
			MaxBackoff:    referenceMaxBackoff,
			LeaseDuration: referenceLeaseDuration,
			BatchSize:     referenceBatchSize,
			MaxAttempts:   referenceMaxAttempts,
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

			ConsumerQueueName: envOrDefault(
				"SQS_CONSUMER_QUEUE_NAME",
				defaultSQSConsumerQueueName,
			),

			ConsumerMaxMessages: int32(consumerMaxMessages),

			ConsumerWaitTime: consumerWaitTime,

			ConsumerVisibilityTimeout: consumerVisibilityTimeout,
		},

		Auth: AuthConfig{
			Issuer:          envOrDefault("AUTH_ISSUER", defaultAuthIssuer),
			JWKSURL:         envOrDefault("AUTH_JWKS_URL", defaultAuthJWKSURL),
			WalletClientID:  envOrDefault("AUTH_WALLET_CLIENT_ID", defaultAuthWalletClientID),
			ProviderClients: splitCSV(envOrDefault("AUTH_PROVIDER_CLIENTS", defaultAuthProviderClients)),
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
		return errors.New(
			"DATABASE_URL is required",
		)
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

	if c.Database.MinConnections >
		c.Database.MaxConnections {
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

	if c.Reference.MaxAttempts <= 0 {
		return errors.New("REFERENCE_MAX_ATTEMPTS must be greater than zero")
	}

	if c.Reference.MaxBackoff < c.Reference.RetryBackoff {
		return errors.New("REFERENCE_MAX_BACKOFF cannot be lower than REFERENCE_RETRY_BACKOFF")
	}

	if c.Reference.LeaseDuration <= 0 {
		return errors.New("REFERENCE_LEASE_DURATION must be greater than zero")
	}

	if c.Outbox.PollInterval != 0 &&
		c.Outbox.PollInterval <= 0 {
		return errors.New(
			"OUTBOX_POLL_INTERVAL must be greater than zero",
		)
	}

	if c.Outbox.RetryBackoff != 0 &&
		c.Outbox.RetryBackoff <= 0 {
		return errors.New(
			"OUTBOX_RETRY_BACKOFF must be greater than zero",
		)
	}

	if c.Outbox.MaxBackoff != 0 &&
		c.Outbox.MaxBackoff <= 0 {
		return errors.New(
			"OUTBOX_MAX_BACKOFF must be greater than zero",
		)
	}

	if c.Outbox.RetryBackoff != 0 &&
		c.Outbox.MaxBackoff != 0 &&
		c.Outbox.MaxBackoff <
			c.Outbox.RetryBackoff {
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
		if strings.TrimSpace(
			c.SQS.AccessKeyID,
		) == "" {
			return errors.New(
				"AWS_ACCESS_KEY_ID is required when SQS is configured",
			)
		}

		if strings.TrimSpace(
			c.SQS.SecretAccessKey,
		) == "" {
			return errors.New(
				"AWS_SECRET_ACCESS_KEY is required when SQS is configured",
			)
		}

		if strings.TrimSpace(
			c.SQS.OutboxQueueName,
		) == "" {
			return errors.New(
				"SQS_OUTBOX_QUEUE_NAME is required when SQS is configured",
			)
		}

		if !strings.HasSuffix(
			strings.TrimSpace(
				c.SQS.OutboxQueueName,
			),
			".fifo",
		) {
			return errors.New(
				"SQS_OUTBOX_QUEUE_NAME must reference a FIFO queue",
			)
		}

		if strings.TrimSpace(
			c.SQS.ConsumerQueueName,
		) == "" {
			return errors.New(
				"SQS_CONSUMER_QUEUE_NAME is required when SQS is configured",
			)
		}

		if !strings.HasSuffix(
			strings.TrimSpace(
				c.SQS.ConsumerQueueName,
			),
			".fifo",
		) {
			return errors.New(
				"SQS_CONSUMER_QUEUE_NAME must reference a FIFO queue",
			)
		}

		if c.SQS.ConsumerMaxMessages <= 0 ||
			c.SQS.ConsumerMaxMessages > 10 {
			return errors.New(
				"SQS_CONSUMER_MAX_MESSAGES must be between 1 and 10",
			)
		}

		if c.SQS.ConsumerWaitTime < 0 ||
			c.SQS.ConsumerWaitTime >
				20*time.Second {
			return errors.New(
				"SQS_CONSUMER_WAIT_TIME must be between 0 and 20 seconds",
			)
		}

		if c.SQS.ConsumerVisibilityTimeout <= 0 ||
			c.SQS.ConsumerVisibilityTimeout >
				12*time.Hour {
			return errors.New(
				"SQS_CONSUMER_VISIBILITY_TIMEOUT must be between 1 second and 12 hours",
			)
		}
	}

	if strings.TrimSpace(c.Auth.Issuer) == "" {
		return errors.New("AUTH_ISSUER is required")
	}

	if strings.TrimSpace(c.Auth.JWKSURL) == "" {
		return errors.New("AUTH_JWKS_URL is required")
	}

	if strings.TrimSpace(c.Auth.WalletClientID) == "" {
		return errors.New("AUTH_WALLET_CLIENT_ID is required")
	}

	if len(c.Auth.ProviderClients) == 0 {
		return errors.New("AUTH_PROVIDER_CLIENTS must contain at least one provider client")
	}

	if c.Application.ShutdownTimeout <= 0 {
		return errors.New(
			"APPLICATION_SHUTDOWN_TIMEOUT must be greater than zero",
		)
	}

	if strings.TrimSpace(c.HTTP.Address) == "" {
		return errors.New(
			"HTTP_ADDRESS is required",
		)
	}

	return nil
}

func envOrDefault(
	key string,
	fallback string,
) string {
	value := strings.TrimSpace(
		os.Getenv(key),
	)

	if value == "" {
		return fallback
	}

	return value
}

func envInt(
	key string,
	fallback int,
) (int, error) {
	value := strings.TrimSpace(
		os.Getenv(key),
	)

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

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if _, exists := seen[part]; exists {
			continue
		}
		seen[part] = struct{}{}
		result = append(result, part)
	}
	return result
}

func envDuration(
	key string,
	fallback time.Duration,
) (time.Duration, error) {
	value := strings.TrimSpace(
		os.Getenv(key),
	)

	if value == "" {
		return fallback, nil
	}

	parsed, err :=
		time.ParseDuration(value)

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
		"http=%s db_max=%d db_min=%d reference_batch=%d outbox_batch=%d sqs_region=%s sqs_outbox_queue=%s sqs_consumer_queue=%s",
		c.HTTP.Address,
		c.Database.MaxConnections,
		c.Database.MinConnections,
		c.Reference.BatchSize,
		c.Outbox.BatchSize,
		c.SQS.Region,
		c.SQS.OutboxQueueName,
		c.SQS.ConsumerQueueName,
	)
}
