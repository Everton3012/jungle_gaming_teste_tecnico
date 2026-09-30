package ledger

import (
	"context"
	"errors"
	"fmt"
	"time"

	domainledger "jungle_gaming_teste_tecnico/internal/domain/ledger"
	domainmoney "jungle_gaming_teste_tecnico/internal/domain/money"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var (
	ErrNotFound           = errors.New("ledger entry not found")
	ErrRepositoryRequired = errors.New("database connection is required")
	ErrEntryRequired      = errors.New("ledger entry is required")
)

type DBTX interface {
	Exec(
		ctx context.Context,
		sql string,
		arguments ...any,
	) (pgconn.CommandTag, error)

	Query(
		ctx context.Context,
		sql string,
		args ...any,
	) (pgx.Rows, error)

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

	return &Repository{db: db}, nil
}

func (r *Repository) WithDB(db DBTX) (*Repository, error) {
	return NewRepository(db)
}

func (r *Repository) Create(
	ctx context.Context,
	entry *domainledger.Entry,
) error {
	if entry == nil {
		return ErrEntryRequired
	}

	_, err := r.db.Exec(
		ctx,
		insertQuery,
		entry.ID(),
		entry.WalletID(),
		entry.TransactionID(),
		string(entry.Direction()),
		entry.Amount().Amount(),
		entry.Amount().Currency(),
		entry.BalanceBefore().Amount(),
		entry.BalanceAfter().Amount(),
		entry.CreatedAt(),
	)
	if err != nil {
		return fmt.Errorf("create ledger entry: %w", err)
	}

	return nil
}

func (r *Repository) FindByID(
	ctx context.Context,
	id string,
) (*domainledger.Entry, error) {
	entry, err := scanEntry(
		r.db.QueryRow(ctx, findByIDQuery, id),
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrNotFound
		}

		return nil, fmt.Errorf("find ledger entry by id: %w", err)
	}

	return entry, nil
}

func (r *Repository) FindByWallet(
	ctx context.Context,
	walletID string,
) ([]*domainledger.Entry, error) {
	rows, err := r.db.Query(
		ctx,
		findByWalletQuery,
		walletID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"find ledger entries by wallet: %w",
			err,
		)
	}
	defer rows.Close()

	return scanEntries(rows)
}

func (r *Repository) FindByTransaction(
	ctx context.Context,
	transactionID string,
) ([]*domainledger.Entry, error) {
	rows, err := r.db.Query(
		ctx,
		findByTransactionQuery,
		transactionID,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"find ledger entries by transaction: %w",
			err,
		)
	}
	defer rows.Close()

	return scanEntries(rows)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanEntry(
	row rowScanner,
) (*domainledger.Entry, error) {
	var (
		id            string
		walletID      string
		transactionID string
		direction     string
		amount        int64
		currency      string
		balanceBefore int64
		balanceAfter  int64
		createdAt     time.Time
	)

	if err := row.Scan(
		&id,
		&walletID,
		&transactionID,
		&direction,
		&amount,
		&currency,
		&balanceBefore,
		&balanceAfter,
		&createdAt,
	); err != nil {
		return nil, err
	}

	amountMoney, err := domainmoney.Rehydrate(amount, currency)
	if err != nil {
		return nil, fmt.Errorf("rehydrate ledger amount: %w", err)
	}

	beforeMoney, err := domainmoney.Rehydrate(
		balanceBefore,
		currency,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"rehydrate ledger balance before: %w",
			err,
		)
	}

	afterMoney, err := domainmoney.Rehydrate(
		balanceAfter,
		currency,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"rehydrate ledger balance after: %w",
			err,
		)
	}

	entry, err := domainledger.Rehydrate(
		id,
		walletID,
		transactionID,
		domainledger.Direction(direction),
		amountMoney,
		beforeMoney,
		afterMoney,
		createdAt,
	)
	if err != nil {
		return nil, fmt.Errorf("rehydrate ledger entry: %w", err)
	}

	return &entry, nil
}

func scanEntries(
	rows pgx.Rows,
) ([]*domainledger.Entry, error) {
	entries := make([]*domainledger.Entry, 0)

	for rows.Next() {
		entry, err := scanEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("scan ledger entry: %w", err)
		}

		entries = append(entries, entry)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate ledger entries: %w", err)
	}

	return entries, nil
}
