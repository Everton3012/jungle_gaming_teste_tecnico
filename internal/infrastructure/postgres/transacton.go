package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrPoolRequired        = errors.New("postgres pool is required")
	ErrTransactionRequired = errors.New("transaction function is required")
)

type TransactionManager struct{ pool *pgxpool.Pool }

func NewTransactionManager(pool *pgxpool.Pool) (*TransactionManager, error) {
	if pool == nil {
		return nil, ErrPoolRequired
	}
	return &TransactionManager{pool: pool}, nil
}

func (m *TransactionManager) WithinTransaction(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	return m.WithinTransactionOptions(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted}, fn)
}

func (m *TransactionManager) WithinTransactionOptions(
	ctx context.Context,
	options pgx.TxOptions,
	fn func(context.Context, pgx.Tx) error,
) error {
	if fn == nil {
		return ErrTransactionRequired
	}

	tx, err := m.pool.BeginTx(ctx, options)
	if err != nil {
		return fmt.Errorf("begin postgres transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback(context.Background())
		}
	}()

	if err := fn(ctx, tx); err != nil {
		if rollbackErr := tx.Rollback(ctx); rollbackErr != nil && !errors.Is(rollbackErr, pgx.ErrTxClosed) {
			return errors.Join(err, fmt.Errorf("rollback postgres transaction: %w", rollbackErr))
		}
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit postgres transaction: %w", err)
	}
	committed = true
	return nil
}
