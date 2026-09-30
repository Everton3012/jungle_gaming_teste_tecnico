package wagertransaction

import "strings"

type FailureCode string

const (
	FailureCodeInsufficientFunds FailureCode = "INSUFFICIENT_FUNDS"
	FailureCodeReferenceNotFound FailureCode = "REFERENCE_NOT_FOUND"
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
