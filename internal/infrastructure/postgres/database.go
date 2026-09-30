package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrDatabaseURLRequired = errors.New("database URL is required")

type Config struct {
	URL            string
	MaxConnections int32
	MinConnections int32
	ConnectTimeout time.Duration
	HealthTimeout  time.Duration
}

func Open(ctx context.Context, config Config) (*pgxpool.Pool, error) {
	if strings.TrimSpace(config.URL) == "" {
		return nil, ErrDatabaseURLRequired
	}

	poolConfig, err := pgxpool.ParseConfig(config.URL)
	if err != nil {
		return nil, fmt.Errorf("parse postgres config: %w", err)
	}

	if config.MaxConnections > 0 {
		poolConfig.MaxConns = config.MaxConnections
	}

	if config.MinConnections >= 0 {
		poolConfig.MinConns = config.MinConnections
	}

	connectTimeout := config.ConnectTimeout
	if connectTimeout <= 0 {
		connectTimeout = 5 * time.Second
	}

	connectCtx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(connectCtx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create postgres pool: %w", err)
	}

	healthTimeout := config.HealthTimeout
	if healthTimeout <= 0 {
		healthTimeout = 3 * time.Second
	}

	healthCtx, healthCancel := context.WithTimeout(ctx, healthTimeout)
	defer healthCancel()

	if err := pool.Ping(healthCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping postgres: %w", err)
	}

	return pool, nil
}
