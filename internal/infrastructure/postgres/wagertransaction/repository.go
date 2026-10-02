package wagertransaction

import (
	"context"
	"errors"
	"fmt"
	"time"

	domainmoney "jungle_gaming_teste_tecnico/internal/domain/money"
	domaintransaction "jungle_gaming_teste_tecnico/internal/domain/wagertransaction"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

var (
	ErrNotFound            = errors.New("wager transaction not found")
	ErrRepositoryRequired  = errors.New("database connection is required")
	ErrTransactionRequired = errors.New("wager transaction is required")
	ErrInvalidLimit        = errors.New("limit must be greater than zero")
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

	return &Repository{
		db: db,
	}, nil
}

func (r *Repository) WithDB(db DBTX) (*Repository, error) {
	return NewRepository(db)
}

func (r *Repository) Create(
	ctx context.Context,
	transaction *domaintransaction.WagerTransaction,
) error {
	if transaction == nil {
		return ErrTransactionRequired
	}

	resultBalance, hasResultBalance := transaction.ResultBalance()

	_, err := r.db.Exec(
		ctx,
		insertQuery,
		transaction.ID(),
		nullableText(transaction.ExternalTransactionID()),
		nullableText(transaction.ProviderID()),
		nullableText(transaction.IdempotencyKey()),
		nullableText(transaction.PayloadHash()),
		transaction.WalletID(),
		transaction.PlayerID(),
		nullableText(transaction.RoundID()),
		nullableText(transaction.GameID()),
		transaction.Kind(),
		transaction.Status(),
		transaction.Money().Amount(),
		transaction.Money().Currency(),
		nullableText(transaction.ReferenceExternalTransactionID()),
		nullableText(transaction.ReferenceTransactionID()),
		nullableText(transaction.FailureCode().String()),
		nullableAmount(resultBalance, hasResultBalance),
		transaction.CreatedAt(),
		transaction.UpdatedAt(),
	)
	if err != nil {
		return fmt.Errorf("create wager transaction: %w", err)
	}

	return nil
}

func (r *Repository) Update(
	ctx context.Context,
	transaction *domaintransaction.WagerTransaction,
) error {
	if transaction == nil {
		return ErrTransactionRequired
	}

	resultBalance, hasResultBalance := transaction.ResultBalance()

	result, err := r.db.Exec(
		ctx,
		updateQuery,
		transaction.ID(),
		transaction.Status(),
		nullableText(transaction.ReferenceTransactionID()),
		nullableText(transaction.FailureCode().String()),
		nullableAmount(resultBalance, hasResultBalance),
		transaction.UpdatedAt(),
	)
	if err != nil {
		return fmt.Errorf("update wager transaction: %w", err)
	}

	if result.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

func (r *Repository) FindByID(
	ctx context.Context,
	id string,
) (*domaintransaction.WagerTransaction, error) {
	return r.find(ctx, findByIDQuery, id)
}

func (r *Repository) FindByProviderExternalID(
	ctx context.Context,
	providerID string,
	externalTransactionID string,
) (*domaintransaction.WagerTransaction, error) {
	return r.find(
		ctx,
		findByProviderExternalIDQuery,
		providerID,
		externalTransactionID,
	)
}

func (r *Repository) FindByProviderIdempotencyKey(
	ctx context.Context,
	providerID string,
	idempotencyKey string,
) (*domaintransaction.WagerTransaction, error) {
	return r.find(
		ctx,
		findByProviderIdempotencyKeyQuery,
		providerID,
		idempotencyKey,
	)
}

func (r *Repository) FindPendingReferences(
	ctx context.Context,
	limit int,
) ([]domaintransaction.WagerTransaction, error) {
	if limit <= 0 {
		return nil, ErrInvalidLimit
	}

	rows, err := r.db.Query(
		ctx,
		findPendingReferencesQuery,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("find pending wager references: %w", err)
	}
	defer rows.Close()

	transactions := make(
		[]domaintransaction.WagerTransaction,
		0,
	)

	for rows.Next() {
		transaction, err := scanTransaction(rows)
		if err != nil {
			return nil, fmt.Errorf(
				"scan pending wager transaction: %w",
				err,
			)
		}

		transactions = append(transactions, *transaction)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf(
			"iterate pending wager transactions: %w",
			err,
		)
	}

	return transactions, nil
}

type PendingReferenceClaim struct {
	TransactionID string
	Attempts      int
}

func (r *Repository) ClaimPendingReferences(
	ctx context.Context,
	now time.Time,
	leaseDuration time.Duration,
	limit int,
) ([]PendingReferenceClaim, error) {
	if now.IsZero() {
		return nil, errors.New("claim time must not be zero")
	}
	if leaseDuration <= 0 {
		return nil, errors.New("reference lease duration must be greater than zero")
	}
	if limit <= 0 {
		return nil, ErrInvalidLimit
	}

	now = now.UTC()
	rows, err := r.db.Query(ctx, claimPendingReferenceIDsQuery, now, limit, now.Add(leaseDuration))
	if err != nil {
		return nil, fmt.Errorf("claim pending wager references: %w", err)
	}
	defer rows.Close()

	claims := make([]PendingReferenceClaim, 0)
	for rows.Next() {
		var claim PendingReferenceClaim
		if err := rows.Scan(&claim.TransactionID, &claim.Attempts); err != nil {
			return nil, fmt.Errorf("scan pending reference claim: %w", err)
		}
		claims = append(claims, claim)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pending reference claims: %w", err)
	}
	return claims, nil
}

func (r *Repository) SchedulePendingReference(
	ctx context.Context,
	transactionID string,
	nextAttemptAt time.Time,
) error {
	if nextAttemptAt.IsZero() {
		return errors.New("next reference attempt time must not be zero")
	}
	result, err := r.db.Exec(ctx, schedulePendingReferenceQuery, transactionID, nextAttemptAt.UTC())
	if err != nil {
		return fmt.Errorf("schedule pending reference: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) ReleasePendingReference(
	ctx context.Context,
	transactionID string,
) error {
	result, err := r.db.Exec(ctx, releasePendingReferenceQuery, transactionID)
	if err != nil {
		return fmt.Errorf("release pending reference lease: %w", err)
	}
	if result.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) find(
	ctx context.Context,
	query string,
	args ...any,
) (*domaintransaction.WagerTransaction, error) {
	transaction, err := scanTransaction(
		r.db.QueryRow(ctx, query, args...),
	)

	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}

	if err != nil {
		return nil, fmt.Errorf("find wager transaction: %w", err)
	}

	return transaction, nil
}

type scanner interface {
	Scan(dest ...any) error
}

func scanTransaction(
	row scanner,
) (*domaintransaction.WagerTransaction, error) {
	var (
		id                             string
		externalTransactionID          pgtype.Text
		providerID                     pgtype.Text
		idempotencyKey                 pgtype.Text
		payloadHash                    pgtype.Text
		walletID                       string
		playerID                       string
		roundID                        pgtype.Text
		gameID                         pgtype.Text
		kind                           string
		status                         string
		amount                         int64
		currency                       string
		referenceExternalTransactionID pgtype.Text
		referenceTransactionID         pgtype.Text
		failureCode                    pgtype.Text
		resultBalance                  pgtype.Int8
		createdAt                      time.Time
		updatedAt                      time.Time
	)

	err := row.Scan(
		&id,
		&externalTransactionID,
		&providerID,
		&idempotencyKey,
		&payloadHash,
		&walletID,
		&playerID,
		&roundID,
		&gameID,
		&kind,
		&status,
		&amount,
		&currency,
		&referenceExternalTransactionID,
		&referenceTransactionID,
		&failureCode,
		&resultBalance,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return nil, err
	}

	transactionMoney, err := domainmoney.Rehydrate(
		amount,
		currency,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"rehydrate wager transaction money: %w",
			err,
		)
	}

	var rehydratedResultBalance *domainmoney.Money

	if resultBalance.Valid {
		value, err := domainmoney.Rehydrate(
			resultBalance.Int64,
			currency,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"rehydrate wager transaction result balance: %w",
				err,
			)
		}

		rehydratedResultBalance = &value
	}

	transaction, err := domaintransaction.Rehydrate(
		domaintransaction.RehydrateInput{
			ID:                             id,
			ExternalTransactionID:          textValue(externalTransactionID),
			ProviderID:                     textValue(providerID),
			IdempotencyKey:                 textValue(idempotencyKey),
			PayloadHash:                    textValue(payloadHash),
			WalletID:                       walletID,
			PlayerID:                       playerID,
			RoundID:                        textValue(roundID),
			GameID:                         textValue(gameID),
			Kind:                           domaintransaction.Kind(kind),
			Money:                          transactionMoney,
			ReferenceExternalTransactionID: textValue(referenceExternalTransactionID),
			ReferenceTransactionID:         textValue(referenceTransactionID),
			Status:                         domaintransaction.Status(status),
			FailureCode:                    domaintransaction.FailureCode(textValue(failureCode)),
			ResultBalance:                  rehydratedResultBalance,
			CreatedAt:                      createdAt,
			UpdatedAt:                      updatedAt,
		},
	)
	if err != nil {
		return nil, fmt.Errorf(
			"rehydrate wager transaction: %w",
			err,
		)
	}

	return &transaction, nil
}

func nullableText(value string) any {
	if value == "" {
		return nil
	}

	return value
}

func nullableAmount(
	value domainmoney.Money,
	present bool,
) any {
	if !present {
		return nil
	}

	return value.Amount()
}

func textValue(value pgtype.Text) string {
	if !value.Valid {
		return ""
	}

	return value.String
}
