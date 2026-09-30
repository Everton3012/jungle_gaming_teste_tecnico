package wallet

import "errors"

var (
	ErrInvalidID           = errors.New("invalid wallet id")
	ErrInvalidPlayerID     = errors.New("invalid player id")
	ErrInvalidVersion      = errors.New("invalid wallet version")
	ErrInvalidTimestamp    = errors.New("invalid wallet timestamp")
	ErrInvalidAmount       = errors.New("invalid wallet amount")
	ErrCurrencyMismatch    = errors.New("wallet currency mismatch")
	ErrInsufficientBalance = errors.New("insufficient wallet balance")
)
