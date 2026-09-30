package wager

import "errors"

var (
	ErrInvalidLedgerEntryID   = errors.New("invalid ledger entry id")
	ErrInvalidTransactionKind = errors.New("invalid transaction kind")
	ErrInvalidAmount          = errors.New("invalid wager amount")
	ErrWalletMismatch         = errors.New("transaction wallet mismatch")
	ErrPlayerMismatch         = errors.New("transaction player mismatch")
	ErrCurrencyMismatch       = errors.New("transaction currency mismatch")
	ErrReferenceRequired      = errors.New("reference transaction required")
	ErrReferenceNotRequired   = errors.New("transaction does not require a reference")
	ErrReferenceNotProcessed  = errors.New("reference transaction not processed")
	ErrReferenceRejected      = errors.New("reference transaction was rejected")
	ErrReferenceFailed        = errors.New("reference transaction failed")
	ErrReferenceMismatch      = errors.New("reference transaction mismatch")
	ErrInvalidReferenceKind   = errors.New("invalid reference transaction kind")
	ErrInvalidReferenceAmount = errors.New("reference transaction amount mismatch")
)
