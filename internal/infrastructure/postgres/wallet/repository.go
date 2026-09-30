package wallet

import (
	"context"
	"errors"
	"fmt"
	"time"

	domainmoney "jungle_gaming_teste_tecnico/internal/domain/money"
	domainwallet "jungle_gaming_teste_tecnico/internal/domain/wallet"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrNotFound           = errors.New("wallet not found")
	ErrConcurrentUpdate   = errors.New("wallet was concurrently updated")
	ErrRepositoryRequired = errors.New("database connection is required")
	ErrWalletRequired     = errors.New("wallet is required")
)

type DBTX interface {
	Exec(
		ctx context.Context,
		sql string,
		arguments ...any,
	) (pgconn.CommandTag, error)

	QueryRow(
		ctx context.Context,
		sql string,
		args ...any,
	) pgx.Row
}

type Repository struct {
	db DBTX
}

func NewRepository(db DBTX) (*Repository, error) {
	if db == nil {
		return nil, ErrRepositoryRequired
	}

	return &Repository{
		db: db,
	}, nil
}

func (r *Repository) WithDB(db DBTX) (*Repository, error) {
	return NewRepository(db)
}

func (r *Repository) Create(
	ctx context.Context,
	wallet *domainwallet.Wallet,
) error {
	if wallet == nil {
		return ErrWalletRequired
	}

	_, err := r.db.Exec(
		ctx,
		insertQuery,
		wallet.ID(),
		wallet.PlayerID(),
		wallet.Currency(),
		wallet.Balance().Amount(),
		wallet.Version(),
		wallet.CreatedAt(),
		wallet.UpdatedAt(),
	)
	if err != nil {
		return fmt.Errorf("create wallet: %w", err)
	}

	return nil
}

func (r *Repository) FindByID(
	ctx context.Context,
	id string,
) (*domainwallet.Wallet, error) {
	return r.find(ctx, findByIDQuery, id)
}

func (r *Repository) FindByPlayerCurrency(
	ctx context.Context,
	playerID string,
	currency string,
) (*domainwallet.Wallet, error) {
	return r.find(
		ctx,
		findByPlayerCurrencyQuery,
		playerID,
		currency,
	)
}

func (r *Repository) Update(
	ctx context.Context,
	wallet *domainwallet.Wallet,
	expectedVersion uint64,
) error {
	if wallet == nil {
		return ErrWalletRequired
	}

	result, err := r.db.Exec(
		ctx,
		updateQuery,
		wallet.ID(),
		wallet.Balance().Amount(),
		wallet.Version(),
		wallet.UpdatedAt(),
		expectedVersion,
	)
	if err != nil {
		return fmt.Errorf("update wallet: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrConcurrentUpdate
	}

	return nil
}

func (r *Repository) find(
	ctx context.Context,
	query string,
	args ...any,
) (*domainwallet.Wallet, error) {
	var (
		id        string
		playerID  string
		currency  string
		amount    int64
		version   uint64
		createdAt time.Time
		updatedAt time.Time
	)

	err := r.db.QueryRow(ctx, query, args...).Scan(
		&id,
		&playerID,
		&currency,
		&amount,
		&version,
		&createdAt,
		&updatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("find wallet: %w", err)
	}

	balance, err := domainmoney.Rehydrate(amount, currency)
	if err != nil {
		return nil, fmt.Errorf("rehydrate wallet balance: %w", err)
	}

	entity, err := domainwallet.Rehydrate(
		id,
		playerID,
		balance,
		version,
		createdAt,
		updatedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("rehydrate wallet: %w", err)
	}

	return &entity, nil
}
