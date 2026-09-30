package opening

import (
	"strings"
	"time"

	"jungle_gaming_teste_tecnico/internal/domain/ledger"
	"jungle_gaming_teste_tecnico/internal/domain/money"
	"jungle_gaming_teste_tecnico/internal/domain/wagertransaction"
	"jungle_gaming_teste_tecnico/internal/domain/wallet"
)

type Input struct {
	WalletID       string
	PlayerID       string
	TransactionID  string
	LedgerID       string
	InitialBalance money.Money
	CreatedAt      time.Time
}

type Result struct {
	Wallet      wallet.Wallet
	Transaction *wagertransaction.WagerTransaction
	LedgerEntry *ledger.Entry
}

func Process(input Input) (Result, error) {
	if err := validateInput(input); err != nil {
		return Result{}, err
	}

	createdWallet, err := wallet.New(
		input.WalletID,
		input.PlayerID,
		input.InitialBalance,
		input.CreatedAt,
	)
	if err != nil {
		return Result{}, err
	}

	if input.InitialBalance.IsZero() {
		return Result{
			Wallet: createdWallet,
		}, nil
	}

	transaction, err := wagertransaction.NewOpening(
		wagertransaction.OpeningInput{
			ID:        input.TransactionID,
			WalletID:  input.WalletID,
			PlayerID:  input.PlayerID,
			Money:     input.InitialBalance,
			CreatedAt: input.CreatedAt,
		},
	)
	if err != nil {
		return Result{}, err
	}

	zeroBalance, err := money.Zero(input.InitialBalance.Currency())
	if err != nil {
		return Result{}, err
	}

	entry, err := ledger.New(
		input.LedgerID,
		input.WalletID,
		input.TransactionID,
		ledger.DirectionCredit,
		input.InitialBalance,
		zeroBalance,
		input.InitialBalance,
		input.CreatedAt,
	)
	if err != nil {
		return Result{}, err
	}

	return Result{
		Wallet:      createdWallet,
		Transaction: &transaction,
		LedgerEntry: &entry,
	}, nil
}

func validateInput(input Input) error {
	if strings.TrimSpace(input.WalletID) == "" {
		return ErrInvalidWalletID
	}

	if strings.TrimSpace(input.PlayerID) == "" {
		return ErrInvalidPlayerID
	}

	if input.InitialBalance.Currency() == "" {
		return money.ErrInvalidCurrency
	}

	if input.InitialBalance.Amount() < 0 {
		return wallet.ErrInsufficientBalance
	}

	if input.CreatedAt.IsZero() {
		return ErrInvalidTimestamp
	}

	if input.InitialBalance.IsZero() {
		return nil
	}

	if strings.TrimSpace(input.TransactionID) == "" {
		return ErrInvalidTransactionID
	}

	if strings.TrimSpace(input.LedgerID) == "" {
		return ErrInvalidLedgerID
	}

	return nil
}
