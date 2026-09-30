package money

import "errors"

var (
	ErrInvalidAmount    = errors.New("invalid money amount")
	ErrInvalidCurrency  = errors.New("invalid currency")
	ErrCurrencyMismatch = errors.New("currency mismatch")
	ErrNegativeAmount   = errors.New("negative money amount")
	ErrOverflow         = errors.New("money overflow")
)
