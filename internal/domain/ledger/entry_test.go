package ledger

import (
	"errors"
	"testing"
	"time"

	"jungle_gaming_teste_tecnico/internal/domain/money"
)

func ledgerTestTime() time.Time {
	return time.Date(2026, 9, 29, 22, 0, 0, 0, time.UTC)
}

func ledgerMoney(t *testing.T, value string) money.Money {
	t.Helper()

	result, err := money.Parse(value, "BRL")
	if err != nil {
		t.Fatalf("money.Parse() error = %v", err)
	}

	return result
}

func TestNewCredit(t *testing.T) {
	entry, err := New(
		"entry-1",
		"wallet-1",
		"transaction-1",
		DirectionCredit,
		ledgerMoney(t, "50.00"),
		ledgerMoney(t, "100.00"),
		ledgerMoney(t, "150.00"),
		ledgerTestTime(),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if entry.ID() != "entry-1" {
		t.Errorf("ID() = %s, want entry-1", entry.ID())
	}

	if entry.WalletID() != "wallet-1" {
		t.Errorf("WalletID() = %s, want wallet-1", entry.WalletID())
	}

	if entry.TransactionID() != "transaction-1" {
		t.Errorf(
			"TransactionID() = %s, want transaction-1",
			entry.TransactionID(),
		)
	}

	if entry.Direction() != DirectionCredit {
		t.Errorf(
			"Direction() = %s, want %s",
			entry.Direction(),
			DirectionCredit,
		)
	}

	if entry.Amount().String() != "50.00" {
		t.Errorf("Amount() = %s, want 50.00", entry.Amount())
	}

	if entry.BalanceBefore().String() != "100.00" {
		t.Errorf(
			"BalanceBefore() = %s, want 100.00",
			entry.BalanceBefore(),
		)
	}

	if entry.BalanceAfter().String() != "150.00" {
		t.Errorf(
			"BalanceAfter() = %s, want 150.00",
			entry.BalanceAfter(),
		)
	}

	if !entry.CreatedAt().Equal(ledgerTestTime()) {
		t.Errorf(
			"CreatedAt() = %v, want %v",
			entry.CreatedAt(),
			ledgerTestTime(),
		)
	}
}

func TestNewDebit(t *testing.T) {
	entry, err := New(
		"entry-1",
		"wallet-1",
		"transaction-1",
		DirectionDebit,
		ledgerMoney(t, "30.00"),
		ledgerMoney(t, "100.00"),
		ledgerMoney(t, "70.00"),
		ledgerTestTime(),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if entry.Direction() != DirectionDebit {
		t.Errorf(
			"Direction() = %s, want %s",
			entry.Direction(),
			DirectionDebit,
		)
	}

	if entry.BalanceAfter().String() != "70.00" {
		t.Errorf(
			"BalanceAfter() = %s, want 70.00",
			entry.BalanceAfter(),
		)
	}
}

func TestNewDebitAllowsZeroBalance(t *testing.T) {
	entry, err := New(
		"entry-1",
		"wallet-1",
		"transaction-1",
		DirectionDebit,
		ledgerMoney(t, "100.00"),
		ledgerMoney(t, "100.00"),
		ledgerMoney(t, "0.00"),
		ledgerTestTime(),
	)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if !entry.BalanceAfter().IsZero() {
		t.Errorf(
			"BalanceAfter() = %s, want 0.00",
			entry.BalanceAfter(),
		)
	}
}

func TestNewRejectsInvalidIdentity(t *testing.T) {
	tests := []struct {
		name          string
		id            string
		walletID      string
		transactionID string
		expected      error
	}{
		{
			name:          "entry id",
			id:            " ",
			walletID:      "wallet-1",
			transactionID: "transaction-1",
			expected:      ErrInvalidID,
		},
		{
			name:          "wallet id",
			id:            "entry-1",
			walletID:      " ",
			transactionID: "transaction-1",
			expected:      ErrInvalidWalletID,
		},
		{
			name:          "transaction id",
			id:            "entry-1",
			walletID:      "wallet-1",
			transactionID: " ",
			expected:      ErrInvalidTransactionID,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(
				tt.id,
				tt.walletID,
				tt.transactionID,
				DirectionCredit,
				ledgerMoney(t, "10.00"),
				ledgerMoney(t, "100.00"),
				ledgerMoney(t, "110.00"),
				ledgerTestTime(),
			)

			if !errors.Is(err, tt.expected) {
				t.Errorf(
					"New() error = %v, want %v",
					err,
					tt.expected,
				)
			}
		})
	}
}

func TestNewRejectsInvalidDirection(t *testing.T) {
	_, err := New(
		"entry-1",
		"wallet-1",
		"transaction-1",
		Direction("INVALID"),
		ledgerMoney(t, "10.00"),
		ledgerMoney(t, "100.00"),
		ledgerMoney(t, "110.00"),
		ledgerTestTime(),
	)

	if !errors.Is(err, ErrInvalidDirection) {
		t.Errorf(
			"New() error = %v, want %v",
			err,
			ErrInvalidDirection,
		)
	}
}

func TestNewRejectsZeroAmount(t *testing.T) {
	_, err := New(
		"entry-1",
		"wallet-1",
		"transaction-1",
		DirectionCredit,
		ledgerMoney(t, "0.00"),
		ledgerMoney(t, "100.00"),
		ledgerMoney(t, "100.00"),
		ledgerTestTime(),
	)

	if !errors.Is(err, ErrInvalidAmount) {
		t.Errorf(
			"New() error = %v, want %v",
			err,
			ErrInvalidAmount,
		)
	}
}

func TestNewRejectsNegativeAmount(t *testing.T) {
	amount, _ := money.Rehydrate(-1000, "BRL")

	_, err := New(
		"entry-1",
		"wallet-1",
		"transaction-1",
		DirectionCredit,
		amount,
		ledgerMoney(t, "100.00"),
		ledgerMoney(t, "110.00"),
		ledgerTestTime(),
	)

	if !errors.Is(err, ErrInvalidAmount) {
		t.Errorf(
			"New() error = %v, want %v",
			err,
			ErrInvalidAmount,
		)
	}
}

func TestNewRejectsNegativeBalance(t *testing.T) {
	negative, _ := money.Rehydrate(-1, "BRL")

	tests := []struct {
		name          string
		balanceBefore money.Money
		balanceAfter  money.Money
	}{
		{
			name:          "balance before",
			balanceBefore: negative,
			balanceAfter:  ledgerMoney(t, "10.00"),
		},
		{
			name:          "balance after",
			balanceBefore: ledgerMoney(t, "10.00"),
			balanceAfter:  negative,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(
				"entry-1",
				"wallet-1",
				"transaction-1",
				DirectionCredit,
				ledgerMoney(t, "10.00"),
				tt.balanceBefore,
				tt.balanceAfter,
				ledgerTestTime(),
			)

			if !errors.Is(err, ErrInvalidBalance) {
				t.Errorf(
					"New() error = %v, want %v",
					err,
					ErrInvalidBalance,
				)
			}
		})
	}
}

func TestNewRejectsCurrencyMismatch(t *testing.T) {
	amount, _ := money.Parse("10.00", "BRL")
	before, _ := money.Parse("100.00", "BRL")
	after, _ := money.Parse("110.00", "USD")

	_, err := New(
		"entry-1",
		"wallet-1",
		"transaction-1",
		DirectionCredit,
		amount,
		before,
		after,
		ledgerTestTime(),
	)

	if !errors.Is(err, money.ErrCurrencyMismatch) {
		t.Errorf(
			"New() error = %v, want %v",
			err,
			money.ErrCurrencyMismatch,
		)
	}
}

func TestNewRejectsInvalidCreditEquation(t *testing.T) {
	_, err := New(
		"entry-1",
		"wallet-1",
		"transaction-1",
		DirectionCredit,
		ledgerMoney(t, "50.00"),
		ledgerMoney(t, "100.00"),
		ledgerMoney(t, "140.00"),
		ledgerTestTime(),
	)

	if !errors.Is(err, ErrInvalidBalance) {
		t.Errorf(
			"New() error = %v, want %v",
			err,
			ErrInvalidBalance,
		)
	}
}

func TestNewRejectsInvalidDebitEquation(t *testing.T) {
	_, err := New(
		"entry-1",
		"wallet-1",
		"transaction-1",
		DirectionDebit,
		ledgerMoney(t, "30.00"),
		ledgerMoney(t, "100.00"),
		ledgerMoney(t, "80.00"),
		ledgerTestTime(),
	)

	if !errors.Is(err, ErrInvalidBalance) {
		t.Errorf(
			"New() error = %v, want %v",
			err,
			ErrInvalidBalance,
		)
	}
}

func TestNewRejectsDebitGreaterThanBalance(t *testing.T) {
	_, err := New(
		"entry-1",
		"wallet-1",
		"transaction-1",
		DirectionDebit,
		ledgerMoney(t, "110.00"),
		ledgerMoney(t, "100.00"),
		ledgerMoney(t, "0.00"),
		ledgerTestTime(),
	)

	if !errors.Is(err, ErrInvalidBalance) {
		t.Errorf(
			"New() error = %v, want %v",
			err,
			ErrInvalidBalance,
		)
	}
}

func TestNewRejectsZeroTimestamp(t *testing.T) {
	_, err := New(
		"entry-1",
		"wallet-1",
		"transaction-1",
		DirectionCredit,
		ledgerMoney(t, "10.00"),
		ledgerMoney(t, "100.00"),
		ledgerMoney(t, "110.00"),
		time.Time{},
	)

	if !errors.Is(err, ErrInvalidTimestamp) {
		t.Errorf(
			"New() error = %v, want %v",
			err,
			ErrInvalidTimestamp,
		)
	}
}

func TestRehydrate(t *testing.T) {
	entry, err := Rehydrate(
		"entry-1",
		"wallet-1",
		"transaction-1",
		DirectionDebit,
		ledgerMoney(t, "25.00"),
		ledgerMoney(t, "100.00"),
		ledgerMoney(t, "75.00"),
		ledgerTestTime(),
	)
	if err != nil {
		t.Fatalf("Rehydrate() error = %v", err)
	}

	if entry.ID() != "entry-1" {
		t.Errorf("ID() = %s, want entry-1", entry.ID())
	}

	if entry.Direction() != DirectionDebit {
		t.Errorf(
			"Direction() = %s, want %s",
			entry.Direction(),
			DirectionDebit,
		)
	}

	if entry.BalanceAfter().String() != "75.00" {
		t.Errorf(
			"BalanceAfter() = %s, want 75.00",
			entry.BalanceAfter(),
		)
	}
}

func TestDirectionProperties(t *testing.T) {
	if !DirectionDebit.IsValid() {
		t.Error("DEBIT should be valid")
	}

	if !DirectionCredit.IsValid() {
		t.Error("CREDIT should be valid")
	}

	if Direction("INVALID").IsValid() {
		t.Error("INVALID should not be valid")
	}
}
