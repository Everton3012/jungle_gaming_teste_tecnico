package opening

import (
	"errors"
	"testing"
	"time"

	"jungle_gaming_teste_tecnico/internal/domain/ledger"
	"jungle_gaming_teste_tecnico/internal/domain/money"
	"jungle_gaming_teste_tecnico/internal/domain/wagertransaction"
	"jungle_gaming_teste_tecnico/internal/domain/wallet"
)

func TestProcessWithZeroBalance(t *testing.T) {
	initialBalance := mustMoney(t, 0, "BRL")
	createdAt := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

	result, err := Process(Input{
		WalletID:       "wallet-1",
		PlayerID:       "player-1",
		InitialBalance: initialBalance,
		CreatedAt:      createdAt,
	})
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	if result.Wallet.ID() != "wallet-1" {
		t.Fatalf("Wallet.ID() = %q", result.Wallet.ID())
	}

	if result.Wallet.PlayerID() != "player-1" {
		t.Fatalf("Wallet.PlayerID() = %q", result.Wallet.PlayerID())
	}

	if result.Wallet.Balance().Amount() != 0 {
		t.Fatalf("Wallet.Balance() = %d", result.Wallet.Balance().Amount())
	}

	if result.Wallet.Version() != 1 {
		t.Fatalf("Wallet.Version() = %d", result.Wallet.Version())
	}

	if result.Transaction != nil {
		t.Fatal("Transaction must be nil for zero opening balance")
	}

	if result.LedgerEntry != nil {
		t.Fatal("LedgerEntry must be nil for zero opening balance")
	}
}

func TestProcessWithPositiveBalance(t *testing.T) {
	initialBalance := mustMoney(t, 10000, "BRL")
	createdAt := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

	result, err := Process(Input{
		WalletID:       "wallet-1",
		PlayerID:       "player-1",
		TransactionID:  "opening-1",
		LedgerID:       "ledger-1",
		InitialBalance: initialBalance,
		CreatedAt:      createdAt,
	})
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	if result.Wallet.Balance().Amount() != 10000 {
		t.Fatalf("Wallet.Balance() = %d", result.Wallet.Balance().Amount())
	}

	if result.Wallet.Version() != 1 {
		t.Fatalf("Wallet.Version() = %d", result.Wallet.Version())
	}

	if result.Transaction == nil {
		t.Fatal("Transaction must not be nil")
	}

	if result.Transaction.Kind() != wagertransaction.KindOpening {
		t.Fatalf("Transaction.Kind() = %s", result.Transaction.Kind())
	}

	if result.Transaction.Status() != wagertransaction.StatusProcessed {
		t.Fatalf("Transaction.Status() = %s", result.Transaction.Status())
	}

	if result.Transaction.ExternalTransactionID() != "" {
		t.Fatalf(
			"Transaction.ExternalTransactionID() = %q",
			result.Transaction.ExternalTransactionID(),
		)
	}

	if result.Transaction.ProviderID() != "" {
		t.Fatalf(
			"Transaction.ProviderID() = %q",
			result.Transaction.ProviderID(),
		)
	}

	resultBalance, ok := result.Transaction.ResultBalance()
	if !ok {
		t.Fatal("Transaction.ResultBalance() must exist")
	}

	if !resultBalance.Equal(initialBalance) {
		t.Fatalf(
			"Transaction.ResultBalance() = %s",
			resultBalance.String(),
		)
	}

	if result.LedgerEntry == nil {
		t.Fatal("LedgerEntry must not be nil")
	}

	if result.LedgerEntry.Direction() != ledger.DirectionCredit {
		t.Fatalf(
			"LedgerEntry.Direction() = %s",
			result.LedgerEntry.Direction(),
		)
	}

	if result.LedgerEntry.TransactionID() != "opening-1" {
		t.Fatalf(
			"LedgerEntry.TransactionID() = %q",
			result.LedgerEntry.TransactionID(),
		)
	}

	if result.LedgerEntry.BalanceBefore().Amount() != 0 {
		t.Fatalf(
			"LedgerEntry.BalanceBefore() = %d",
			result.LedgerEntry.BalanceBefore().Amount(),
		)
	}

	if !result.LedgerEntry.BalanceAfter().Equal(initialBalance) {
		t.Fatalf(
			"LedgerEntry.BalanceAfter() = %s",
			result.LedgerEntry.BalanceAfter().String(),
		)
	}
}

func TestProcessPositiveBalanceKeepsInitialWalletVersion(t *testing.T) {
	initialBalance := mustMoney(t, 5000, "BRL")

	result, err := Process(Input{
		WalletID:       "wallet-1",
		PlayerID:       "player-1",
		TransactionID:  "opening-1",
		LedgerID:       "ledger-1",
		InitialBalance: initialBalance,
		CreatedAt:      time.Now(),
	})
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	if result.Wallet.Version() != 1 {
		t.Fatalf("Wallet.Version() = %d, want 1", result.Wallet.Version())
	}
}

func TestProcessZeroBalanceDoesNotRequireTransactionOrLedgerIDs(t *testing.T) {
	result, err := Process(Input{
		WalletID:       "wallet-1",
		PlayerID:       "player-1",
		InitialBalance: mustMoney(t, 0, "USD"),
		CreatedAt:      time.Now(),
	})
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	if result.Transaction != nil {
		t.Fatal("Transaction must be nil")
	}

	if result.LedgerEntry != nil {
		t.Fatal("LedgerEntry must be nil")
	}
}

func TestProcessPositiveBalanceRequiresTransactionID(t *testing.T) {
	_, err := Process(Input{
		WalletID:       "wallet-1",
		PlayerID:       "player-1",
		LedgerID:       "ledger-1",
		InitialBalance: mustMoney(t, 100, "BRL"),
		CreatedAt:      time.Now(),
	})

	if !errors.Is(err, ErrInvalidTransactionID) {
		t.Fatalf("Process() error = %v", err)
	}
}

func TestProcessPositiveBalanceRequiresLedgerID(t *testing.T) {
	_, err := Process(Input{
		WalletID:       "wallet-1",
		PlayerID:       "player-1",
		TransactionID:  "opening-1",
		InitialBalance: mustMoney(t, 100, "BRL"),
		CreatedAt:      time.Now(),
	})

	if !errors.Is(err, ErrInvalidLedgerID) {
		t.Fatalf("Process() error = %v", err)
	}
}

func TestProcessRejectsNegativeInitialBalance(t *testing.T) {
	initialBalance, err := money.Rehydrate(-100, "BRL")
	if err != nil {
		t.Fatalf("money.Rehydrate() error = %v", err)
	}

	_, err = Process(Input{
		WalletID:       "wallet-1",
		PlayerID:       "player-1",
		InitialBalance: initialBalance,
		CreatedAt:      time.Now(),
	})

	if !errors.Is(err, wallet.ErrInsufficientBalance) {
		t.Fatalf("Process() error = %v", err)
	}
}

func TestProcessRejectsInvalidIdentity(t *testing.T) {
	initialBalance := mustMoney(t, 0, "BRL")
	createdAt := time.Now()

	tests := []struct {
		name  string
		input Input
		want  error
	}{
		{
			name: "wallet id",
			input: Input{
				PlayerID:       "player-1",
				InitialBalance: initialBalance,
				CreatedAt:      createdAt,
			},
			want: ErrInvalidWalletID,
		},
		{
			name: "player id",
			input: Input{
				WalletID:       "wallet-1",
				InitialBalance: initialBalance,
				CreatedAt:      createdAt,
			},
			want: ErrInvalidPlayerID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Process(tt.input)

			if !errors.Is(err, tt.want) {
				t.Fatalf("Process() error = %v", err)
			}
		})
	}
}

func TestProcessRejectsZeroTimestamp(t *testing.T) {
	_, err := Process(Input{
		WalletID:       "wallet-1",
		PlayerID:       "player-1",
		InitialBalance: mustMoney(t, 0, "BRL"),
	})

	if !errors.Is(err, ErrInvalidTimestamp) {
		t.Fatalf("Process() error = %v", err)
	}
}

func mustMoney(t *testing.T, amount int64, currency string) money.Money {
	t.Helper()

	value, err := money.New(amount, currency)
	if err != nil {
		t.Fatalf("money.New() error = %v", err)
	}

	return value
}
