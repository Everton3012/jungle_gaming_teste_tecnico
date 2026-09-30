package wagertransaction

import "strings"

type FailureCode string

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
