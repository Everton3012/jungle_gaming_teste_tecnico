package money

import (
	"errors"
	"math"
	"testing"
)

func TestParse(t *testing.T) {
	tests := []struct {
		name     string
		value    string
		currency string
		amount   int64
		expected string
	}{
		{"integer", "10", "BRL", 1000, "10.00"},
		{"one decimal", "10.1", "BRL", 1010, "10.10"},
		{"two decimals", "10.25", "BRL", 1025, "10.25"},
		{"zero", "0.00", "BRL", 0, "0.00"},
		{"minimum unit", "0.01", "BRL", 1, "0.01"},
		{"normalizes currency", "10.00", " brl ", 1000, "10.00"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, err := Parse(tt.value, tt.currency)
			if err != nil {
				t.Fatalf("Parse() error = %v", err)
			}

			if m.Amount() != tt.amount {
				t.Errorf("Amount() = %d, want %d", m.Amount(), tt.amount)
			}

			if m.String() != tt.expected {
				t.Errorf("String() = %s, want %s", m.String(), tt.expected)
			}

			if m.Currency() != "BRL" {
				t.Errorf("Currency() = %s, want BRL", m.Currency())
			}
		})
	}
}

func TestParseRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name  string
		value string
		err   error
	}{
		{"empty", "", ErrInvalidAmount},
		{"negative", "-1.00", ErrNegativeAmount},
		{"positive sign", "+1.00", ErrInvalidAmount},
		{"too many decimals", "1.001", ErrInvalidAmount},
		{"missing whole", ".50", ErrInvalidAmount},
		{"missing fraction", "1.", ErrInvalidAmount},
		{"multiple dots", "1.2.3", ErrInvalidAmount},
		{"scientific notation", "1e2", ErrInvalidAmount},
		{"letters", "abc", ErrInvalidAmount},
		{"comma", "10,25", ErrInvalidAmount},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(tt.value, "BRL")

			if !errors.Is(err, tt.err) {
				t.Errorf("Parse() error = %v, want %v", err, tt.err)
			}
		})
	}
}

func TestNewRejectsInvalidCurrency(t *testing.T) {
	tests := []string{
		"",
		"BR",
		"BRLL",
		"12A",
		"R$L",
		"@@@",
	}

	for _, currency := range tests {
		t.Run(currency, func(t *testing.T) {
			_, err := New(100, currency)

			if !errors.Is(err, ErrInvalidCurrency) {
				t.Errorf("New() error = %v, want %v", err, ErrInvalidCurrency)
			}
		})
	}
}

func TestNewRejectsNegativeAmount(t *testing.T) {
	_, err := New(-1, "BRL")

	if !errors.Is(err, ErrNegativeAmount) {
		t.Errorf("New() error = %v, want %v", err, ErrNegativeAmount)
	}
}

func TestZero(t *testing.T) {
	m, err := Zero("BRL")
	if err != nil {
		t.Fatalf("Zero() error = %v", err)
	}

	if !m.IsZero() {
		t.Errorf("Zero() amount = %s, want 0.00", m.String())
	}
}

func TestAdd(t *testing.T) {
	left, _ := New(1000, "BRL")
	right, _ := New(250, "BRL")

	result, err := left.Add(right)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	if result.Amount() != 1250 {
		t.Errorf("Add() amount = %d, want 1250", result.Amount())
	}
}

func TestAddWithNegativeMoney(t *testing.T) {
	left, _ := New(1000, "BRL")
	right, _ := Rehydrate(-250, "BRL")

	result, err := left.Add(right)
	if err != nil {
		t.Fatalf("Add() error = %v", err)
	}

	if result.Amount() != 750 {
		t.Errorf("Add() amount = %d, want 750", result.Amount())
	}
}

func TestAddRejectsCurrencyMismatch(t *testing.T) {
	brl, _ := New(1000, "BRL")
	usd, _ := New(1000, "USD")

	_, err := brl.Add(usd)

	if !errors.Is(err, ErrCurrencyMismatch) {
		t.Errorf("Add() error = %v, want %v", err, ErrCurrencyMismatch)
	}
}

func TestAddRejectsPositiveOverflow(t *testing.T) {
	max, _ := Rehydrate(math.MaxInt64, "BRL")
	one, _ := New(1, "BRL")

	_, err := max.Add(one)

	if !errors.Is(err, ErrOverflow) {
		t.Errorf("Add() error = %v, want %v", err, ErrOverflow)
	}
}

func TestAddRejectsNegativeOverflow(t *testing.T) {
	min, _ := Rehydrate(math.MinInt64, "BRL")
	negativeOne, _ := Rehydrate(-1, "BRL")

	_, err := min.Add(negativeOne)

	if !errors.Is(err, ErrOverflow) {
		t.Errorf("Add() error = %v, want %v", err, ErrOverflow)
	}
}

func TestSub(t *testing.T) {
	left, _ := New(1000, "BRL")
	right, _ := New(250, "BRL")

	result, err := left.Sub(right)
	if err != nil {
		t.Fatalf("Sub() error = %v", err)
	}

	if result.Amount() != 750 {
		t.Errorf("Sub() amount = %d, want 750", result.Amount())
	}
}

func TestSubAllowsNegativeInternalResult(t *testing.T) {
	left, _ := New(500, "BRL")
	right, _ := New(1000, "BRL")

	result, err := left.Sub(right)
	if err != nil {
		t.Fatalf("Sub() error = %v", err)
	}

	if result.Amount() != -500 {
		t.Errorf("Sub() amount = %d, want -500", result.Amount())
	}
}

func TestSubNegativeMoney(t *testing.T) {
	left, _ := New(1000, "BRL")
	right, _ := Rehydrate(-500, "BRL")

	result, err := left.Sub(right)
	if err != nil {
		t.Fatalf("Sub() error = %v", err)
	}

	if result.Amount() != 1500 {
		t.Errorf("Sub() amount = %d, want 1500", result.Amount())
	}
}

func TestSubRejectsCurrencyMismatch(t *testing.T) {
	brl, _ := New(1000, "BRL")
	usd, _ := New(500, "USD")

	_, err := brl.Sub(usd)

	if !errors.Is(err, ErrCurrencyMismatch) {
		t.Errorf("Sub() error = %v, want %v", err, ErrCurrencyMismatch)
	}
}

func TestSubRejectsNegativeOverflow(t *testing.T) {
	min, _ := Rehydrate(math.MinInt64, "BRL")
	one, _ := New(1, "BRL")

	_, err := min.Sub(one)

	if !errors.Is(err, ErrOverflow) {
		t.Errorf("Sub() error = %v, want %v", err, ErrOverflow)
	}
}

func TestSubRejectsPositiveOverflow(t *testing.T) {
	max, _ := Rehydrate(math.MaxInt64, "BRL")
	negativeOne, _ := Rehydrate(-1, "BRL")

	_, err := max.Sub(negativeOne)

	if !errors.Is(err, ErrOverflow) {
		t.Errorf("Sub() error = %v, want %v", err, ErrOverflow)
	}
}

func TestNegate(t *testing.T) {
	m, _ := New(1000, "BRL")

	negative, err := m.Negate()
	if err != nil {
		t.Fatalf("Negate() error = %v", err)
	}

	if negative.Amount() != -1000 {
		t.Errorf("Negate() amount = %d, want -1000", negative.Amount())
	}

	positive, err := negative.Negate()
	if err != nil {
		t.Fatalf("Negate() error = %v", err)
	}

	if positive.Amount() != 1000 {
		t.Errorf("Negate() amount = %d, want 1000", positive.Amount())
	}
}

func TestNegateRejectsOverflow(t *testing.T) {
	m, _ := Rehydrate(math.MinInt64, "BRL")

	_, err := m.Negate()

	if !errors.Is(err, ErrOverflow) {
		t.Errorf("Negate() error = %v, want %v", err, ErrOverflow)
	}
}

func TestCompare(t *testing.T) {
	ten, _ := New(1000, "BRL")
	twenty, _ := New(2000, "BRL")
	anotherTen, _ := New(1000, "BRL")

	result, err := ten.Compare(twenty)
	if err != nil || result != -1 {
		t.Errorf("Compare() = %d, %v, want -1, nil", result, err)
	}

	result, err = twenty.Compare(ten)
	if err != nil || result != 1 {
		t.Errorf("Compare() = %d, %v, want 1, nil", result, err)
	}

	result, err = ten.Compare(anotherTen)
	if err != nil || result != 0 {
		t.Errorf("Compare() = %d, %v, want 0, nil", result, err)
	}
}

func TestCompareRejectsCurrencyMismatch(t *testing.T) {
	brl, _ := New(1000, "BRL")
	usd, _ := New(1000, "USD")

	_, err := brl.Compare(usd)

	if !errors.Is(err, ErrCurrencyMismatch) {
		t.Errorf("Compare() error = %v, want %v", err, ErrCurrencyMismatch)
	}
}

func TestRehydrateAllowsNegativeAmount(t *testing.T) {
	m, err := Rehydrate(-1250, "BRL")
	if err != nil {
		t.Fatalf("Rehydrate() error = %v", err)
	}

	if m.Amount() != -1250 {
		t.Errorf("Amount() = %d, want -1250", m.Amount())
	}

	if m.String() != "-12.50" {
		t.Errorf("String() = %s, want -12.50", m.String())
	}
}

func TestParseMaximumInt64(t *testing.T) {
	m, err := Parse("92233720368547758.07", "BRL")
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	if m.Amount() != math.MaxInt64 {
		t.Errorf("Amount() = %d, want %d", m.Amount(), int64(math.MaxInt64))
	}
}

func TestParseRejectsOverflow(t *testing.T) {
	_, err := Parse("92233720368547758.08", "BRL")

	if !errors.Is(err, ErrOverflow) {
		t.Errorf("Parse() error = %v, want %v", err, ErrOverflow)
	}
}

func TestPredicates(t *testing.T) {
	zero, _ := New(0, "BRL")
	positive, _ := New(1, "BRL")
	negative, _ := Rehydrate(-1, "BRL")
	equal, _ := New(1, "BRL")
	different, _ := New(2, "BRL")

	if !zero.IsZero() {
		t.Error("zero should be zero")
	}

	if zero.IsPositive() {
		t.Error("zero should not be positive")
	}

	if !positive.IsPositive() {
		t.Error("positive should be positive")
	}

	if negative.IsPositive() {
		t.Error("negative should not be positive")
	}

	if !positive.Equal(equal) {
		t.Error("equal values should be equal")
	}

	if positive.Equal(different) {
		t.Error("different values should not be equal")
	}
}
