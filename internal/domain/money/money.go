package money

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

const scale int64 = 100

type Money struct {
	amount   int64
	currency string
}

func New(amount int64, currency string) (Money, error) {
	if amount < 0 {
		return Money{}, ErrNegativeAmount
	}

	normalizedCurrency, err := normalizeCurrency(currency)
	if err != nil {
		return Money{}, err
	}

	return Money{
		amount:   amount,
		currency: normalizedCurrency,
	}, nil
}

func Parse(value, currency string) (Money, error) {
	value = strings.TrimSpace(value)

	if value == "" {
		return Money{}, ErrInvalidAmount
	}

	if strings.ContainsAny(value, "eE") {
		return Money{}, ErrInvalidAmount
	}

	if strings.HasPrefix(value, "-") {
		return Money{}, ErrNegativeAmount
	}

	if strings.HasPrefix(value, "+") {
		return Money{}, ErrInvalidAmount
	}

	parts := strings.Split(value, ".")
	if len(parts) > 2 {
		return Money{}, ErrInvalidAmount
	}

	whole := parts[0]
	if whole == "" || !onlyDigits(whole) {
		return Money{}, ErrInvalidAmount
	}

	fraction := ""

	if len(parts) == 2 {
		fraction = parts[1]

		if fraction == "" || len(fraction) > 2 || !onlyDigits(fraction) {
			return Money{}, ErrInvalidAmount
		}
	}

	switch len(fraction) {
	case 0:
		fraction = "00"
	case 1:
		fraction += "0"
	}

	wholeValue, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return Money{}, ErrOverflow
	}

	fractionValue, err := strconv.ParseInt(fraction, 10, 64)
	if err != nil {
		return Money{}, ErrInvalidAmount
	}

	if wholeValue > (math.MaxInt64-fractionValue)/scale {
		return Money{}, ErrOverflow
	}

	return New(wholeValue*scale+fractionValue, currency)
}

func (m Money) Add(other Money) (Money, error) {
	if m.currency != other.currency {
		return Money{}, ErrCurrencyMismatch
	}

	if other.amount > math.MaxInt64-m.amount {
		return Money{}, ErrOverflow
	}

	return Money{
		amount:   m.amount + other.amount,
		currency: m.currency,
	}, nil
}

func (m Money) Sub(other Money) (Money, error) {
	if m.currency != other.currency {
		return Money{}, ErrCurrencyMismatch
	}

	if other.amount > m.amount {
		return Money{}, ErrNegativeAmount
	}

	return Money{
		amount:   m.amount - other.amount,
		currency: m.currency,
	}, nil
}

func (m Money) IsZero() bool {
	return m.amount == 0
}

func (m Money) IsPositive() bool {
	return m.amount > 0
}

func (m Money) Equal(other Money) bool {
	return m.amount == other.amount && m.currency == other.currency
}

func (m Money) Amount() int64 {
	return m.amount
}

func (m Money) Currency() string {
	return m.currency
}

func (m Money) String() string {
	whole := m.amount / scale
	fraction := m.amount % scale

	return fmt.Sprintf("%d.%02d", whole, fraction)
}

func normalizeCurrency(currency string) (string, error) {
	normalized := strings.ToUpper(strings.TrimSpace(currency))

	if len(normalized) != 3 {
		return "", ErrInvalidCurrency
	}

	for _, char := range normalized {
		if char < 'A' || char > 'Z' {
			return "", ErrInvalidCurrency
		}
	}

	return normalized, nil
}

func onlyDigits(value string) bool {
	if value == "" {
		return false
	}

	for _, char := range value {
		if char < '0' || char > '9' {
			return false
		}
	}

	return true
}
