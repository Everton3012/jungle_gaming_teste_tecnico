package query

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	domainledger "jungle_gaming_teste_tecnico/internal/domain/ledger"
	domainmoney "jungle_gaming_teste_tecnico/internal/domain/money"
	domaintransaction "jungle_gaming_teste_tecnico/internal/domain/wagertransaction"
	domainwallet "jungle_gaming_teste_tecnico/internal/domain/wallet"
	postgresinfra "jungle_gaming_teste_tecnico/internal/infrastructure/postgres"
	ledgerpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/ledger"
	transactionpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/wagertransaction"
	walletpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/wallet"
)

var (
	ErrDependenciesRequired = errors.New("query service dependencies are required")
	ErrInvalidCursor        = errors.New("invalid ledger cursor")
	ErrInvalidLimit         = errors.New("ledger limit must be between 1 and 200")
)

type Service struct {
	transactionManager *postgresinfra.TransactionManager
	wallets            *walletpostgres.Repository
	transactions       *transactionpostgres.Repository
	ledger             *ledgerpostgres.Repository
}

type LedgerPage struct {
	Entries    []*domainledger.Entry
	NextCursor string
}

type ReconciliationResult struct {
	WalletID          string
	StoredBalance     domainmoney.Money
	CalculatedBalance domainmoney.Money
	Difference        domainmoney.Money
	Consistent        bool
	CheckedEntries    int
}

type ledgerCursor struct {
	CreatedAt string `json:"createdAt"`
	ID        string `json:"id"`
}

func NewService(
	transactionManager *postgresinfra.TransactionManager,
	wallets *walletpostgres.Repository,
	transactions *transactionpostgres.Repository,
	ledger *ledgerpostgres.Repository,
) (*Service, error) {
	if transactionManager == nil || wallets == nil || transactions == nil || ledger == nil {
		return nil, ErrDependenciesRequired
	}
	return &Service{transactionManager: transactionManager, wallets: wallets, transactions: transactions, ledger: ledger}, nil
}

func (s *Service) GetWallet(ctx context.Context, walletID string) (*domainwallet.Wallet, error) {
	return s.wallets.FindByID(ctx, strings.TrimSpace(walletID))
}

func (s *Service) GetTransaction(ctx context.Context, transactionID string) (*domaintransaction.WagerTransaction, error) {
	return s.transactions.FindByID(ctx, strings.TrimSpace(transactionID))
}

func (s *Service) GetProviderTransaction(ctx context.Context, providerID, externalID string) (*domaintransaction.WagerTransaction, error) {
	return s.transactions.FindByProviderExternalID(ctx, strings.TrimSpace(providerID), strings.TrimSpace(externalID))
}

func (s *Service) ListLedger(ctx context.Context, walletID, cursor string, limit int) (LedgerPage, error) {
	if limit <= 0 || limit > 200 {
		return LedgerPage{}, ErrInvalidLimit
	}

	var afterCreatedAt *time.Time
	var afterID string
	if strings.TrimSpace(cursor) != "" {
		decoded, err := decodeCursor(cursor)
		if err != nil {
			return LedgerPage{}, err
		}
		parsed, err := time.Parse(time.RFC3339Nano, decoded.CreatedAt)
		if err != nil || strings.TrimSpace(decoded.ID) == "" {
			return LedgerPage{}, ErrInvalidCursor
		}
		parsed = parsed.UTC()
		afterCreatedAt = &parsed
		afterID = decoded.ID
	}

	entries, err := s.ledger.FindByWalletPage(ctx, strings.TrimSpace(walletID), afterCreatedAt, afterID, limit+1)
	if err != nil {
		return LedgerPage{}, err
	}

	page := LedgerPage{Entries: entries}
	if len(entries) > limit {
		last := entries[limit-1]
		page.Entries = entries[:limit]
		page.NextCursor, err = encodeCursor(last.CreatedAt(), last.ID())
		if err != nil {
			return LedgerPage{}, err
		}
	}
	return page, nil
}

func (s *Service) Reconcile(ctx context.Context, walletID string) (ReconciliationResult, error) {
	var result ReconciliationResult
	err := s.transactionManager.WithinTransactionOptions(
		ctx,
		pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly},
		func(ctx context.Context, tx pgx.Tx) error {
			wallets, err := s.wallets.WithDB(tx)
			if err != nil {
				return err
			}
			ledger, err := s.ledger.WithDB(tx)
			if err != nil {
				return err
			}

			walletEntity, err := wallets.FindByID(ctx, strings.TrimSpace(walletID))
			if err != nil {
				return err
			}
			entries, err := ledger.FindByWallet(ctx, walletEntity.ID())
			if err != nil {
				return err
			}

			calculated, err := domainmoney.Zero(walletEntity.Currency())
			if err != nil {
				return err
			}
			for _, entry := range entries {
				if entry.Direction() == domainledger.DirectionCredit {
					calculated, err = calculated.Add(entry.Amount())
				} else {
					calculated, err = calculated.Sub(entry.Amount())
				}
				if err != nil {
					return fmt.Errorf("reconstruct ledger balance: %w", err)
				}
			}

			difference, err := walletEntity.Balance().Sub(calculated)
			if err != nil {
				return err
			}
			result = ReconciliationResult{
				WalletID:          walletEntity.ID(),
				StoredBalance:     walletEntity.Balance(),
				CalculatedBalance: calculated,
				Difference:        difference,
				Consistent:        difference.IsZero(),
				CheckedEntries:    len(entries),
			}
			return nil
		},
	)
	if err != nil {
		return ReconciliationResult{}, err
	}
	return result, nil
}

func encodeCursor(createdAt time.Time, id string) (string, error) {
	payload, err := json.Marshal(ledgerCursor{CreatedAt: createdAt.UTC().Format(time.RFC3339Nano), ID: id})
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

func decodeCursor(value string) (ledgerCursor, error) {
	payload, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(value))
	if err != nil {
		return ledgerCursor{}, ErrInvalidCursor
	}
	var cursor ledgerCursor
	if err := json.Unmarshal(payload, &cursor); err != nil {
		return ledgerCursor{}, ErrInvalidCursor
	}
	return cursor, nil
}
