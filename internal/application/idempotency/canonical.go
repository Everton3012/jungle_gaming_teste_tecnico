package idempotency

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

var (
	ErrEmptyPayload     = errors.New("payload is empty")
	ErrInvalidJSON      = errors.New("payload is not valid JSON")
	ErrTrailingJSONData = errors.New("payload contains trailing JSON data")
)

func Canonicalize(payload []byte) ([]byte, error) {
	if len(bytes.TrimSpace(payload)) == 0 {
		return nil, ErrEmptyPayload
	}

	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()

	var value any

	if err := decoder.Decode(&value); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidJSON, err)
	}

	var trailing any

	err := decoder.Decode(&trailing)

	switch {
	case errors.Is(err, io.EOF):

	case err == nil:
		return nil, ErrTrailingJSONData

	default:
		return nil, fmt.Errorf("%w: %v", ErrTrailingJSONData, err)
	}

	canonical, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal canonical JSON: %w", err)
	}

	return canonical, nil
}

func Hash(payload []byte) (string, error) {
	canonical, err := Canonicalize(payload)
	if err != nil {
		return "", err
	}

	sum := sha256.Sum256(canonical)

	return hex.EncodeToString(sum[:]), nil
}
