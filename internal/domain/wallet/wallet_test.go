package wallet

import (
	"errors"
	"math"
	"testing"
	"time"

	"jungle_gaming_teste_tecnico/internal/domain/money"
)

func testTime() time.Time {
	return time.Date(2026, 9, 29, 20, 0, 0, 0, time.UTC)
}

func TestNew(t *testing.T) {
	now := testTime()
	balance, _ := money.Parse("100.00", "BRL")

	w, err := New("wallet-1", "player-1", balance, now)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if w.ID() != "wallet-1" {
		t.Errorf("ID() = %s, want wallet-1", w.ID())
	}

	if w.PlayerID() != "player-1" {
		t.Errorf("PlayerID() = %s, want player-1", w.PlayerID())
	}

	if w.Balance().String() != "100.00" {
		t.Errorf("Balance() = %s, want 100.00", w.Balance())
	}

	if w.Currency() != "BRL" {
		t.Errorf("Currency() = %s, want BRL", w.Currency())
	}

	if w.Version() != 1 {
		t.Errorf("Version() = %d, want 1", w.Version())
	}

	if !w.CreatedAt().Equal(now) {
		t.Errorf("CreatedAt() = %v, want %v", w.CreatedAt(), now)
	}

	if !w.UpdatedAt().Equal(now) {
		t.Errorf("UpdatedAt() = %v, want %v", w.UpdatedAt(), now)
	}
}

func TestNewAllowsZeroBalance(t *testing.T) {
	balance, _ := money.Zero("BRL")

	w, err := New("wallet-1", "player-1", balance, testTime())
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if !w.Balance().IsZero() {
		t.Errorf("Balance() = %s, want 0.00", w.Balance())
	}

	if w.Version() != 1 {
		t.Errorf("Version() = %d, want 1", w.Version())
	}
}

func TestNewRejectsInvalidIdentity(t *testing.T) {
	balance, _ := money.Zero("BRL")

	tests := []struct {
		name     string
		id       string
		playerID string
		expected error
	}{
		{"empty wallet id", "", "player-1", ErrInvalidID},
		{"blank wallet id", "   ", "player-1", ErrInvalidID},
		{"empty player id", "wallet-1", "", ErrInvalidPlayerID},
		{"blank player id", "wallet-1", "   ", ErrInvalidPlayerID},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.id, tt.playerID, balance, testTime())

			if !errors.Is(err, tt.expected) {
				t.Errorf("New() error = %v, want %v", err, tt.expected)
			}
		})
	}
}

func TestNewRejectsNegativeBalance(t *testing.T) {
	balance, _ := money.Rehydrate(-1, "BRL")

	_, err := New("wallet-1", "player-1", balance, testTime())

	if !errors.Is(err, ErrInsufficientBalance) {
		t.Errorf("New() error = %v, want %v", err, ErrInsufficientBalance)
	}
}

func TestNewRejectsZeroTimestamp(t *testing.T) {
	balance, _ := money.Zero("BRL")

	_, err := New("wallet-1", "player-1", balance, time.Time{})

	if !errors.Is(err, ErrInvalidTimestamp) {
		t.Errorf("New() error = %v, want %v", err, ErrInvalidTimestamp)
	}
}

func TestCredit(t *testing.T) {
	createdAt := testTime()
	occurredAt := createdAt.Add(time.Minute)

	balance, _ := money.Parse("100.00", "BRL")
	amount, _ := money.Parse("50.00", "BRL")
	w, _ := New("wallet-1", "player-1", balance, createdAt)

	err := w.Credit(amount, occurredAt)
	if err != nil {
		t.Fatalf("Credit() error = %v", err)
	}

	if w.Balance().String() != "150.00" {
		t.Errorf("Balance() = %s, want 150.00", w.Balance())
	}

	if w.Version() != 2 {
		t.Errorf("Version() = %d, want 2", w.Version())
	}

	if !w.UpdatedAt().Equal(occurredAt) {
		t.Errorf("UpdatedAt() = %v, want %v", w.UpdatedAt(), occurredAt)
	}
}

func TestCreditZeroDoesNotChangeWallet(t *testing.T) {
	now := testTime()
	balance, _ := money.Parse("100.00", "BRL")
	zero, _ := money.Zero("BRL")
	w, _ := New("wallet-1", "player-1", balance, now)

	err := w.Credit(zero, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("Credit() error = %v", err)
	}

	if w.Balance().String() != "100.00" {
		t.Errorf("Balance() = %s, want 100.00", w.Balance())
	}

	if w.Version() != 1 {
		t.Errorf("Version() = %d, want 1", w.Version())
	}

	if !w.UpdatedAt().Equal(now) {
		t.Error("UpdatedAt() changed for zero credit")
	}
}

func TestCreditRejectsCurrencyMismatch(t *testing.T) {
	now := testTime()
	balance, _ := money.Parse("100.00", "BRL")
	amount, _ := money.Parse("10.00", "USD")
	w, _ := New("wallet-1", "player-1", balance, now)

	err := w.Credit(amount, now.Add(time.Minute))

	if !errors.Is(err, ErrCurrencyMismatch) {
		t.Errorf("Credit() error = %v, want %v", err, ErrCurrencyMismatch)
	}
}

func TestCreditRejectsNegativeAmount(t *testing.T) {
	now := testTime()
	balance, _ := money.Parse("100.00", "BRL")
	amount, _ := money.Rehydrate(-100, "BRL")
	w, _ := New("wallet-1", "player-1", balance, now)

	err := w.Credit(amount, now.Add(time.Minute))

	if !errors.Is(err, ErrInvalidAmount) {
		t.Errorf("Credit() error = %v, want %v", err, ErrInvalidAmount)
	}
}

func TestCreditRejectsOverflow(t *testing.T) {
	now := testTime()
	balance, _ := money.Rehydrate(math.MaxInt64, "BRL")
	one, _ := money.New(1, "BRL")
	w, _ := New("wallet-1", "player-1", balance, now)

	err := w.Credit(one, now.Add(time.Minute))

	if !errors.Is(err, money.ErrOverflow) {
		t.Errorf("Credit() error = %v, want %v", err, money.ErrOverflow)
	}

	if w.Balance().Amount() != math.MaxInt64 {
		t.Error("Balance() changed after failed credit")
	}

	if w.Version() != 1 {
		t.Errorf("Version() = %d, want 1", w.Version())
	}

	if !w.UpdatedAt().Equal(now) {
		t.Error("UpdatedAt() changed after failed credit")
	}
}

func TestCreditRejectsZeroTimestamp(t *testing.T) {
	now := testTime()
	balance, _ := money.Parse("100.00", "BRL")
	amount, _ := money.Parse("10.00", "BRL")
	w, _ := New("wallet-1", "player-1", balance, now)

	err := w.Credit(amount, time.Time{})

	if !errors.Is(err, ErrInvalidTimestamp) {
		t.Errorf("Credit() error = %v, want %v", err, ErrInvalidTimestamp)
	}

	if w.Balance().String() != "100.00" {
		t.Error("Balance() changed after invalid timestamp")
	}

	if w.Version() != 1 {
		t.Errorf("Version() = %d, want 1", w.Version())
	}
}

func TestDebit(t *testing.T) {
	createdAt := testTime()
	occurredAt := createdAt.Add(time.Minute)

	balance, _ := money.Parse("100.00", "BRL")
	amount, _ := money.Parse("30.00", "BRL")
	w, _ := New("wallet-1", "player-1", balance, createdAt)

	err := w.Debit(amount, occurredAt)
	if err != nil {
		t.Fatalf("Debit() error = %v", err)
	}

	if w.Balance().String() != "70.00" {
		t.Errorf("Balance() = %s, want 70.00", w.Balance())
	}

	if w.Version() != 2 {
		t.Errorf("Version() = %d, want 2", w.Version())
	}

	if !w.UpdatedAt().Equal(occurredAt) {
		t.Errorf("UpdatedAt() = %v, want %v", w.UpdatedAt(), occurredAt)
	}
}

func TestDebitEntireBalance(t *testing.T) {
	now := testTime()
	balance, _ := money.Parse("100.00", "BRL")
	amount, _ := money.Parse("100.00", "BRL")
	w, _ := New("wallet-1", "player-1", balance, now)

	err := w.Debit(amount, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("Debit() error = %v", err)
	}

	if !w.Balance().IsZero() {
		t.Errorf("Balance() = %s, want 0.00", w.Balance())
	}

	if w.Version() != 2 {
		t.Errorf("Version() = %d, want 2", w.Version())
	}
}

func TestDebitZeroDoesNotChangeWallet(t *testing.T) {
	now := testTime()
	balance, _ := money.Parse("100.00", "BRL")
	zero, _ := money.Zero("BRL")
	w, _ := New("wallet-1", "player-1", balance, now)

	err := w.Debit(zero, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("Debit() error = %v", err)
	}

	if w.Balance().String() != "100.00" {
		t.Errorf("Balance() = %s, want 100.00", w.Balance())
	}

	if w.Version() != 1 {
		t.Errorf("Version() = %d, want 1", w.Version())
	}

	if !w.UpdatedAt().Equal(now) {
		t.Error("UpdatedAt() changed for zero debit")
	}
}

func TestDebitRejectsInsufficientBalance(t *testing.T) {
	now := testTime()
	balance, _ := money.Parse("50.00", "BRL")
	amount, _ := money.Parse("80.00", "BRL")
	w, _ := New("wallet-1", "player-1", balance, now)

	err := w.Debit(amount, now.Add(time.Minute))

	if !errors.Is(err, ErrInsufficientBalance) {
		t.Errorf("Debit() error = %v, want %v", err, ErrInsufficientBalance)
	}

	if w.Balance().String() != "50.00" {
		t.Error("Balance() changed after rejected debit")
	}

	if w.Version() != 1 {
		t.Errorf("Version() = %d, want 1", w.Version())
	}

	if !w.UpdatedAt().Equal(now) {
		t.Error("UpdatedAt() changed after rejected debit")
	}
}

func TestDebitRejectsCurrencyMismatch(t *testing.T) {
	now := testTime()
	balance, _ := money.Parse("100.00", "BRL")
	amount, _ := money.Parse("10.00", "USD")
	w, _ := New("wallet-1", "player-1", balance, now)

	err := w.Debit(amount, now.Add(time.Minute))

	if !errors.Is(err, ErrCurrencyMismatch) {
		t.Errorf("Debit() error = %v, want %v", err, ErrCurrencyMismatch)
	}
}

func TestDebitRejectsNegativeAmount(t *testing.T) {
	now := testTime()
	balance, _ := money.Parse("100.00", "BRL")
	amount, _ := money.Rehydrate(-100, "BRL")
	w, _ := New("wallet-1", "player-1", balance, now)

	err := w.Debit(amount, now.Add(time.Minute))

	if !errors.Is(err, ErrInvalidAmount) {
		t.Errorf("Debit() error = %v, want %v", err, ErrInvalidAmount)
	}
}

func TestDebitRejectsZeroTimestamp(t *testing.T) {
	now := testTime()
	balance, _ := money.Parse("100.00", "BRL")
	amount, _ := money.Parse("10.00", "BRL")
	w, _ := New("wallet-1", "player-1", balance, now)

	err := w.Debit(amount, time.Time{})

	if !errors.Is(err, ErrInvalidTimestamp) {
		t.Errorf("Debit() error = %v, want %v", err, ErrInvalidTimestamp)
	}

	if w.Balance().String() != "100.00" {
		t.Error("Balance() changed after invalid timestamp")
	}

	if w.Version() != 1 {
		t.Errorf("Version() = %d, want 1", w.Version())
	}
}

func TestOperationAllowsEarlierTimestamp(t *testing.T) {
	now := testTime()
	earlier := now.Add(-time.Minute)

	balance, _ := money.Parse("100.00", "BRL")
	amount, _ := money.Parse("10.00", "BRL")
	w, _ := New("wallet-1", "player-1", balance, now)

	err := w.Credit(amount, earlier)
	if err != nil {
		t.Fatalf("Credit() error = %v", err)
	}

	if w.Balance().String() != "110.00" {
		t.Errorf("Balance() = %s, want 110.00", w.Balance())
	}

	if w.Version() != 2 {
		t.Errorf("Version() = %d, want 2", w.Version())
	}

	if !w.UpdatedAt().Equal(earlier) {
		t.Errorf("UpdatedAt() = %v, want %v", w.UpdatedAt(), earlier)
	}
}

func TestRehydrate(t *testing.T) {
	createdAt := testTime()
	updatedAt := createdAt.Add(time.Hour)
	balance, _ := money.Parse("250.50", "BRL")

	w, err := Rehydrate(
		"wallet-1",
		"player-1",
		balance,
		7,
		createdAt,
		updatedAt,
	)
	if err != nil {
		t.Fatalf("Rehydrate() error = %v", err)
	}

	if w.Balance().String() != "250.50" {
		t.Errorf("Balance() = %s, want 250.50", w.Balance())
	}

	if w.Version() != 7 {
		t.Errorf("Version() = %d, want 7", w.Version())
	}

	if !w.CreatedAt().Equal(createdAt) {
		t.Errorf("CreatedAt() = %v, want %v", w.CreatedAt(), createdAt)
	}

	if !w.UpdatedAt().Equal(updatedAt) {
		t.Errorf("UpdatedAt() = %v, want %v", w.UpdatedAt(), updatedAt)
	}
}

func TestRehydrateRejectsInvalidVersion(t *testing.T) {
	now := testTime()
	balance, _ := money.Zero("BRL")

	_, err := Rehydrate(
		"wallet-1",
		"player-1",
		balance,
		0,
		now,
		now,
	)

	if !errors.Is(err, ErrInvalidVersion) {
		t.Errorf("Rehydrate() error = %v, want %v", err, ErrInvalidVersion)
	}
}

func TestRehydrateRejectsNegativeBalance(t *testing.T) {
	now := testTime()
	balance, _ := money.Rehydrate(-1, "BRL")

	_, err := Rehydrate(
		"wallet-1",
		"player-1",
		balance,
		1,
		now,
		now,
	)

	if !errors.Is(err, ErrInsufficientBalance) {
		t.Errorf("Rehydrate() error = %v, want %v", err, ErrInsufficientBalance)
	}
}

func TestRehydrateRejectsUpdatedAtBeforeCreatedAt(t *testing.T) {
	createdAt := testTime()
	updatedAt := createdAt.Add(-time.Second)
	balance, _ := money.Zero("BRL")

	_, err := Rehydrate(
		"wallet-1",
		"player-1",
		balance,
		1,
		createdAt,
		updatedAt,
	)

	if !errors.Is(err, ErrInvalidTimestamp) {
		t.Errorf("Rehydrate() error = %v, want %v", err, ErrInvalidTimestamp)
	}
}
