package config

import (
	"strings"
	"testing"
	"time"
)

const testDatabaseURL = "postgres://jungle:jungle@localhost:5432/jungle_gaming?sslmode=disable"

func TestLoadRequiresDatabaseURL(t *testing.T) {
	t.Setenv("DATABASE_URL", "")

	_, err := Load()
	if err == nil {
		t.Fatal("Load() error = nil, want error")
	}

	if !strings.Contains(err.Error(), "DATABASE_URL") {
		t.Fatalf(
			"Load() error = %q, want DATABASE_URL error",
			err,
		)
	}
}

func TestLoadUsesDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", testDatabaseURL)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.HTTP.Address != defaultHTTPAddress {
		t.Fatalf(
			"HTTP address = %q, want %q",
			cfg.HTTP.Address,
			defaultHTTPAddress,
		)
	}

	if cfg.Database.MaxConnections != defaultDatabaseMaxConnections {
		t.Fatalf(
			"max connections = %d, want %d",
			cfg.Database.MaxConnections,
			defaultDatabaseMaxConnections,
		)
	}

	if cfg.Database.MinConnections != defaultDatabaseMinConnections {
		t.Fatalf(
			"min connections = %d, want %d",
			cfg.Database.MinConnections,
			defaultDatabaseMinConnections,
		)
	}

	if cfg.Database.ConnectTimeout != defaultDatabaseConnectTimeout {
		t.Fatalf(
			"connect timeout = %s, want %s",
			cfg.Database.ConnectTimeout,
			defaultDatabaseConnectTimeout,
		)
	}

	if cfg.Database.OperationTimeout != defaultDatabaseOperationTimeout {
		t.Fatalf(
			"operation timeout = %s, want %s",
			cfg.Database.OperationTimeout,
			defaultDatabaseOperationTimeout,
		)
	}

	if cfg.Reference.PollInterval != defaultReferencePollInterval {
		t.Fatalf(
			"poll interval = %s, want %s",
			cfg.Reference.PollInterval,
			defaultReferencePollInterval,
		)
	}

	if cfg.Reference.RetryBackoff != defaultReferenceRetryBackoff {
		t.Fatalf(
			"retry backoff = %s, want %s",
			cfg.Reference.RetryBackoff,
			defaultReferenceRetryBackoff,
		)
	}

	if cfg.Reference.BatchSize != defaultReferenceBatchSize {
		t.Fatalf(
			"batch size = %d, want %d",
			cfg.Reference.BatchSize,
			defaultReferenceBatchSize,
		)
	}

	if cfg.Application.ShutdownTimeout != defaultApplicationShutdownTimeout {
		t.Fatalf(
			"shutdown timeout = %s, want %s",
			cfg.Application.ShutdownTimeout,
			defaultApplicationShutdownTimeout,
		)
	}
}

func TestLoadReadsEnvironment(t *testing.T) {
	t.Setenv("DATABASE_URL", testDatabaseURL)
	t.Setenv("HTTP_ADDRESS", ":9090")
	t.Setenv("DATABASE_MAX_CONNECTIONS", "30")
	t.Setenv("DATABASE_MIN_CONNECTIONS", "5")
	t.Setenv("DATABASE_CONNECT_TIMEOUT", "7s")
	t.Setenv("DATABASE_OPERATION_TIMEOUT", "3s")
	t.Setenv("REFERENCE_POLL_INTERVAL", "4s")
	t.Setenv("REFERENCE_RETRY_BACKOFF", "9s")
	t.Setenv("REFERENCE_BATCH_SIZE", "50")
	t.Setenv("APPLICATION_SHUTDOWN_TIMEOUT", "15s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.HTTP.Address != ":9090" {
		t.Fatalf("HTTP address = %q, want :9090", cfg.HTTP.Address)
	}

	if cfg.Database.MaxConnections != 30 {
		t.Fatalf(
			"max connections = %d, want 30",
			cfg.Database.MaxConnections,
		)
	}

	if cfg.Database.MinConnections != 5 {
		t.Fatalf(
			"min connections = %d, want 5",
			cfg.Database.MinConnections,
		)
	}

	if cfg.Database.ConnectTimeout != 7*time.Second {
		t.Fatalf(
			"connect timeout = %s, want 7s",
			cfg.Database.ConnectTimeout,
		)
	}

	if cfg.Database.OperationTimeout != 3*time.Second {
		t.Fatalf(
			"operation timeout = %s, want 3s",
			cfg.Database.OperationTimeout,
		)
	}

	if cfg.Reference.PollInterval != 4*time.Second {
		t.Fatalf(
			"poll interval = %s, want 4s",
			cfg.Reference.PollInterval,
		)
	}

	if cfg.Reference.RetryBackoff != 9*time.Second {
		t.Fatalf(
			"retry backoff = %s, want 9s",
			cfg.Reference.RetryBackoff,
		)
	}

	if cfg.Reference.BatchSize != 50 {
		t.Fatalf(
			"batch size = %d, want 50",
			cfg.Reference.BatchSize,
		)
	}

	if cfg.Application.ShutdownTimeout != 15*time.Second {
		t.Fatalf(
			"shutdown timeout = %s, want 15s",
			cfg.Application.ShutdownTimeout,
		)
	}
}

func TestLoadRejectsInvalidEnvironmentValues(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{
			name:  "invalid max connections",
			key:   "DATABASE_MAX_CONNECTIONS",
			value: "abc",
		},
		{
			name:  "invalid min connections",
			key:   "DATABASE_MIN_CONNECTIONS",
			value: "abc",
		},
		{
			name:  "invalid connect timeout",
			key:   "DATABASE_CONNECT_TIMEOUT",
			value: "banana",
		},
		{
			name:  "invalid operation timeout",
			key:   "DATABASE_OPERATION_TIMEOUT",
			value: "banana",
		},
		{
			name:  "invalid poll interval",
			key:   "REFERENCE_POLL_INTERVAL",
			value: "banana",
		},
		{
			name:  "invalid retry backoff",
			key:   "REFERENCE_RETRY_BACKOFF",
			value: "banana",
		},
		{
			name:  "invalid batch size",
			key:   "REFERENCE_BATCH_SIZE",
			value: "abc",
		},
		{
			name:  "invalid shutdown timeout",
			key:   "APPLICATION_SHUTDOWN_TIMEOUT",
			value: "banana",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", testDatabaseURL)
			t.Setenv(tt.key, tt.value)

			_, err := Load()
			if err == nil {
				t.Fatal("Load() error = nil, want error")
			}

			if !strings.Contains(err.Error(), tt.key) {
				t.Fatalf(
					"Load() error = %q, want error containing %q",
					err,
					tt.key,
				)
			}
		})
	}
}

func TestLoadRejectsSemanticallyInvalidEnvironmentValues(t *testing.T) {
	tests := []struct {
		name  string
		key   string
		value string
	}{
		{
			name:  "zero max connections",
			key:   "DATABASE_MAX_CONNECTIONS",
			value: "0",
		},
		{
			name:  "negative min connections",
			key:   "DATABASE_MIN_CONNECTIONS",
			value: "-1",
		},
		{
			name:  "zero connect timeout",
			key:   "DATABASE_CONNECT_TIMEOUT",
			value: "0s",
		},
		{
			name:  "zero operation timeout",
			key:   "DATABASE_OPERATION_TIMEOUT",
			value: "0s",
		},
		{
			name:  "zero poll interval",
			key:   "REFERENCE_POLL_INTERVAL",
			value: "0s",
		},
		{
			name:  "zero retry backoff",
			key:   "REFERENCE_RETRY_BACKOFF",
			value: "0s",
		},
		{
			name:  "zero batch size",
			key:   "REFERENCE_BATCH_SIZE",
			value: "0",
		},
		{
			name:  "zero shutdown timeout",
			key:   "APPLICATION_SHUTDOWN_TIMEOUT",
			value: "0s",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", testDatabaseURL)
			t.Setenv(tt.key, tt.value)

			_, err := Load()
			if err == nil {
				t.Fatal("Load() error = nil, want error")
			}
		})
	}
}

func TestConfigValidateRejectsInvalidValues(t *testing.T) {
	valid := Config{
		HTTP: HTTPConfig{
			Address: ":8080",
		},
		Database: DatabaseConfig{
			URL:              testDatabaseURL,
			MaxConnections:   10,
			MinConnections:   1,
			ConnectTimeout:   time.Second,
			OperationTimeout: time.Second,
		},
		Reference: ReferenceConfig{
			PollInterval: time.Second,
			RetryBackoff: time.Second,
			BatchSize:    10,
		},
		Application: ApplicationConfig{
			ShutdownTimeout: time.Second,
		},
	}

	tests := []struct {
		name   string
		mutate func(*Config)
	}{
		{
			name: "missing database URL",
			mutate: func(c *Config) {
				c.Database.URL = ""
			},
		},
		{
			name: "zero max connections",
			mutate: func(c *Config) {
				c.Database.MaxConnections = 0
			},
		},
		{
			name: "negative min connections",
			mutate: func(c *Config) {
				c.Database.MinConnections = -1
			},
		},
		{
			name: "min greater than max",
			mutate: func(c *Config) {
				c.Database.MinConnections = 11
			},
		},
		{
			name: "zero connect timeout",
			mutate: func(c *Config) {
				c.Database.ConnectTimeout = 0
			},
		},
		{
			name: "zero operation timeout",
			mutate: func(c *Config) {
				c.Database.OperationTimeout = 0
			},
		},
		{
			name: "zero poll interval",
			mutate: func(c *Config) {
				c.Reference.PollInterval = 0
			},
		},
		{
			name: "zero retry backoff",
			mutate: func(c *Config) {
				c.Reference.RetryBackoff = 0
			},
		},
		{
			name: "zero batch size",
			mutate: func(c *Config) {
				c.Reference.BatchSize = 0
			},
		},
		{
			name: "zero shutdown timeout",
			mutate: func(c *Config) {
				c.Application.ShutdownTimeout = 0
			},
		},
		{
			name: "empty HTTP address",
			mutate: func(c *Config) {
				c.HTTP.Address = ""
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := valid
			tt.mutate(&cfg)

			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate() error = nil, want error")
			}
		})
	}
}
