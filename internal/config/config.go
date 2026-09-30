package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
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
	defaultApplicationShutdownTimeout = 10 * time.Second
)

type Config struct {
	HTTP        HTTPConfig
	Database    DatabaseConfig
	Reference   ReferenceConfig
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
			URL:              os.Getenv("DATABASE_URL"),
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
	if c.Database.URL == "" {
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

	if c.Application.ShutdownTimeout <= 0 {
		return errors.New(
			"APPLICATION_SHUTDOWN_TIMEOUT must be greater than zero",
		)
	}

	if c.HTTP.Address == "" {
		return errors.New("HTTP_ADDRESS is required")
	}

	return nil
}

func envOrDefault(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	return value
}

func envInt(key string, fallback int) (int, error) {
	value := os.Getenv(key)
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
	value := os.Getenv(key)
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
		"http=%s db_max=%d db_min=%d reference_batch=%d",
		c.HTTP.Address,
		c.Database.MaxConnections,
		c.Database.MinConnections,
		c.Reference.BatchSize,
	)
}
