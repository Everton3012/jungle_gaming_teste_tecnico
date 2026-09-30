package opening

import "errors"

var (
	ErrInvalidWalletID      = errors.New("invalid wallet id")
	ErrInvalidPlayerID      = errors.New("invalid player id")
	ErrInvalidTransactionID = errors.New("invalid opening transaction id")
	ErrInvalidLedgerID      = errors.New("invalid opening ledger id")
	ErrInvalidTimestamp     = errors.New("invalid opening timestamp")
)
