package money

import "errors"

var (
	ErrInvalidAmount    = errors.New("invalid money amount")
	ErrNegativeAmount   = errors.New("money amount cannot be negative")
	ErrInvalidCurrency  = errors.New("invalid currency")
	ErrCurrencyMismatch = errors.New("currency mismatch")
	ErrOverflow         = errors.New("money overflow")
)
