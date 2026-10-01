package walletopening

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	domainmoney "jungle_gaming_teste_tecnico/internal/domain/money"
	domainopening "jungle_gaming_teste_tecnico/internal/domain/opening"
	domainwallet "jungle_gaming_teste_tecnico/internal/domain/wallet"
	postgresinfra "jungle_gaming_teste_tecnico/internal/infrastructure/postgres"
	ledgerpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/ledger"
	outboxpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/outbox"
	transactionpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/wagertransaction"
	walletpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/wallet"
)

var (
	ErrTransactionManagerRequired    = errors.New("transaction manager is required")
	ErrWalletRepositoryRequired      = errors.New("wallet repository is required")
	ErrTransactionRepositoryRequired = errors.New("transaction repository is required")
	ErrLedgerRepositoryRequired      = errors.New("ledger repository is required")
	ErrOutboxRepositoryRequired      = errors.New("outbox repository is required")
	ErrWalletAlreadyExists           = errors.New("wallet already exists")
)

type Service struct {
	transactionManager    *postgresinfra.TransactionManager
	walletRepository      *walletpostgres.Repository
	transactionRepository *transactionpostgres.Repository
	ledgerRepository      *ledgerpostgres.Repository
	outboxRepository      *outboxpostgres.Repository
}

type CreateInput struct {
	WalletID       string
	PlayerID       string
	TransactionID  string
	LedgerID       string
	InitialBalance domainmoney.Money
	CreatedAt      time.Time
}

type CreateResult struct {
	Wallet domainwallet.Wallet
}

func NewService(
	transactionManager *postgresinfra.TransactionManager,
	walletRepository *walletpostgres.Repository,
	transactionRepository *transactionpostgres.Repository,
	ledgerRepository *ledgerpostgres.Repository,
	outboxRepository *outboxpostgres.Repository,
) (*Service, error) {
	if transactionManager == nil {
		return nil, ErrTransactionManagerRequired
	}

	if walletRepository == nil {
		return nil, ErrWalletRepositoryRequired
	}

	if transactionRepository == nil {
		return nil, ErrTransactionRepositoryRequired
	}

	if ledgerRepository == nil {
		return nil, ErrLedgerRepositoryRequired
	}

	if outboxRepository == nil {
		return nil, ErrOutboxRepositoryRequired
	}

	return &Service{
		transactionManager:    transactionManager,
		walletRepository:      walletRepository,
		transactionRepository: transactionRepository,
		ledgerRepository:      ledgerRepository,
		outboxRepository:      outboxRepository,
	}, nil
}

func (s *Service) Create(
	ctx context.Context,
	input CreateInput,
) (CreateResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}

	var result CreateResult

	err := s.transactionManager.WithinTransaction(
		ctx,
		func(
			ctx context.Context,
			tx pgx.Tx,
		) error {
			txWalletRepository, err :=
				s.walletRepository.WithDB(tx)
			if err != nil {
				return fmt.Errorf(
					"create transactional wallet repository: %w",
					err,
				)
			}

			txTransactionRepository, err :=
				s.transactionRepository.WithDB(tx)
			if err != nil {
				return fmt.Errorf(
					"create transactional wager transaction repository: %w",
					err,
				)
			}

			txLedgerRepository, err :=
				s.ledgerRepository.WithDB(tx)
			if err != nil {
				return fmt.Errorf(
					"create transactional ledger repository: %w",
					err,
				)
			}

			txOutboxRepository, err :=
				s.outboxRepository.WithDB(tx)
			if err != nil {
				return fmt.Errorf(
					"create transactional outbox repository: %w",
					err,
				)
			}

			existing, err :=
				txWalletRepository.FindByPlayerCurrency(
					ctx,
					strings.TrimSpace(input.PlayerID),
					input.InitialBalance.Currency(),
				)

			switch {
			case err == nil && existing != nil:
				return ErrWalletAlreadyExists

			case err != nil &&
				!errors.Is(
					err,
					walletpostgres.ErrNotFound,
				):
				return fmt.Errorf(
					"check existing wallet: %w",
					err,
				)
			}

			openingResult, err :=
				domainopening.Process(
					domainopening.Input{
						WalletID:       input.WalletID,
						PlayerID:       input.PlayerID,
						TransactionID:  input.TransactionID,
						LedgerID:       input.LedgerID,
						InitialBalance: input.InitialBalance,
						CreatedAt:      input.CreatedAt,
					},
				)
			if err != nil {
				return fmt.Errorf(
					"process wallet opening: %w",
					err,
				)
			}

			if err :=
				txWalletRepository.Create(
					ctx,
					&openingResult.Wallet,
				); err != nil {
				return fmt.Errorf(
					"persist wallet: %w",
					err,
				)
			}

			if openingResult.Transaction != nil {
				if err :=
					txTransactionRepository.Create(
						ctx,
						openingResult.Transaction,
					); err != nil {
					return fmt.Errorf(
						"persist opening transaction: %w",
						err,
					)
				}
			}

			if openingResult.LedgerEntry != nil {
				if err :=
					txLedgerRepository.Create(
						ctx,
						openingResult.LedgerEntry,
					); err != nil {
					return fmt.Errorf(
						"persist opening ledger entry: %w",
						err,
					)
				}
			}

			if openingResult.Transaction != nil &&
				openingResult.LedgerEntry != nil {

				if err := persistOpeningEvents(
					ctx,
					txOutboxRepository,
					&openingResult.Wallet,
					openingResult.Transaction,
					openingResult.LedgerEntry,
					input.CreatedAt,
				); err != nil {
					return fmt.Errorf(
						"persist opening events: %w",
						err,
					)
				}
			}

			result = CreateResult{
				Wallet: openingResult.Wallet,
			}

			return nil
		},
	)
	if err != nil {
		return CreateResult{}, err
	}

	return result, nil
}
