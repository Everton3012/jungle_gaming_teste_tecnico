package wagertransaction

import "strings"

type FailureCode string

const (
	FailureCodeInsufficientFunds         FailureCode = "INSUFFICIENT_FUNDS"
	FailureCodeReversalInsufficientFunds FailureCode = "REVERSAL_INSUFFICIENT_FUNDS"
	FailureCodeReferenceNotFound         FailureCode = "REFERENCE_NOT_FOUND"
	FailureCodeReferenceRejected         FailureCode = "REFERENCE_REJECTED"
	FailureCodeReferenceFailed           FailureCode = "REFERENCE_FAILED"
	FailureCodeInvalidReference          FailureCode = "INVALID_REFERENCE"
	FailureCodeInvalidAmount             FailureCode = "INVALID_AMOUNT"
	FailureCodeWalletMismatch            FailureCode = "WALLET_MISMATCH"
	FailureCodePlayerMismatch            FailureCode = "PLAYER_MISMATCH"
)

func NewFailureCode(value string) (FailureCode, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", ErrInvalidFailureCode
	}

	return FailureCode(value), nil
}

func (f FailureCode) String() string {
	return string(f)
}
