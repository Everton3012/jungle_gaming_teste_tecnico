package ledger

import "errors"

var (
	ErrInvalidID            = errors.New("invalid ledger entry id")
	ErrInvalidWalletID      = errors.New("invalid wallet id")
	ErrInvalidTransactionID = errors.New("invalid transaction id")
	ErrInvalidDirection     = errors.New("invalid ledger direction")
	ErrInvalidAmount        = errors.New("invalid ledger amount")
	ErrInvalidBalance       = errors.New("invalid ledger balance")
	ErrInvalidTimestamp     = errors.New("invalid ledger timestamp")
)
