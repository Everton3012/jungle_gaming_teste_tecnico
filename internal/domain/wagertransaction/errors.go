package wagertransaction

import "errors"

var (
	ErrInvalidID                    = errors.New("invalid transaction id")
	ErrInvalidExternalTransactionID = errors.New("invalid external transaction id")
	ErrInvalidProviderID            = errors.New("invalid provider id")
	ErrInvalidIdempotencyKey        = errors.New("invalid idempotency key")
	ErrInvalidPayloadHash           = errors.New("invalid payload hash")
	ErrInvalidWalletID              = errors.New("invalid wallet id")
	ErrInvalidPlayerID              = errors.New("invalid player id")
	ErrInvalidRoundID               = errors.New("invalid round id")
	ErrInvalidGameID                = errors.New("invalid game id")
	ErrInvalidKind                  = errors.New("invalid transaction kind")
	ErrInvalidStatus                = errors.New("invalid transaction status")
	ErrInvalidTimestamp             = errors.New("invalid transaction timestamp")
	ErrInvalidFailureCode           = errors.New("invalid failure code")
	ErrInvalidOpeningAmount         = errors.New("opening amount must be positive")
	ErrInvalidTransition            = errors.New("invalid transaction state transition")
	ErrTerminalTransaction          = errors.New("transaction is terminal")
	ErrOpeningExternal              = errors.New("opening cannot be an external transaction")
	ErrExternalKindRequired         = errors.New("external transaction kind required")
)
