package idempotency_test

import (
	"errors"
	"testing"

	"jungle_gaming_teste_tecnico/internal/application/idempotency"
)

func TestCanonicalizeDeterministic(t *testing.T) {
	first := []byte(`{
		"providerId": "provider-1",
		"externalTransactionId": "tx-1",
		"amount": 2000,
		"currency": "BRL"
	}`)

	second := []byte(`{
		"currency": "BRL",
		"amount": 2000,
		"externalTransactionId": "tx-1",
		"providerId": "provider-1"
	}`)

	firstCanonical, err := idempotency.Canonicalize(first)
	if err != nil {
		t.Fatalf("canonicalize first payload: %v", err)
	}

	secondCanonical, err := idempotency.Canonicalize(second)
	if err != nil {
		t.Fatalf("canonicalize second payload: %v", err)
	}

	if string(firstCanonical) != string(secondCanonical) {
		t.Fatalf(
			"canonical payloads differ:\nfirst:  %s\nsecond: %s",
			firstCanonical,
			secondCanonical,
		)
	}
}

func TestHashIgnoresObjectFieldOrder(t *testing.T) {
	first := []byte(`{
		"walletId": "wallet-1",
		"amount": 2000,
		"currency": "BRL",
		"kind": "BET"
	}`)

	second := []byte(`{
		"kind": "BET",
		"currency": "BRL",
		"amount": 2000,
		"walletId": "wallet-1"
	}`)

	firstHash, err := idempotency.Hash(first)
	if err != nil {
		t.Fatalf("hash first payload: %v", err)
	}

	secondHash, err := idempotency.Hash(second)
	if err != nil {
		t.Fatalf("hash second payload: %v", err)
	}

	if firstHash != secondHash {
		t.Fatalf(
			"equivalent payloads generated different hashes:\n%s\n%s",
			firstHash,
			secondHash,
		)
	}
}

func TestHashIgnoresWhitespace(t *testing.T) {
	compact := []byte(
		`{"walletId":"wallet-1","amount":2000,"currency":"BRL"}`,
	)

	formatted := []byte(`
	{
		"walletId": "wallet-1",
		"amount": 2000,
		"currency": "BRL"
	}
	`)

	compactHash, err := idempotency.Hash(compact)
	if err != nil {
		t.Fatalf("hash compact payload: %v", err)
	}

	formattedHash, err := idempotency.Hash(formatted)
	if err != nil {
		t.Fatalf("hash formatted payload: %v", err)
	}

	if compactHash != formattedHash {
		t.Fatal("whitespace changed canonical hash")
	}
}

func TestHashPreservesArrayOrder(t *testing.T) {
	first := []byte(`{
		"items": ["BET", "WIN", "REFUND"]
	}`)

	second := []byte(`{
		"items": ["REFUND", "WIN", "BET"]
	}`)

	firstHash, err := idempotency.Hash(first)
	if err != nil {
		t.Fatalf("hash first payload: %v", err)
	}

	secondHash, err := idempotency.Hash(second)
	if err != nil {
		t.Fatalf("hash second payload: %v", err)
	}

	if firstHash == secondHash {
		t.Fatal("different array order generated same hash")
	}
}

func TestHashNestedObjectsAreDeterministic(t *testing.T) {
	first := []byte(`{
		"transaction": {
			"id": "tx-1",
			"money": {
				"amount": 2000,
				"currency": "BRL"
			}
		},
		"provider": {
			"id": "provider-1"
		}
	}`)

	second := []byte(`{
		"provider": {
			"id": "provider-1"
		},
		"transaction": {
			"money": {
				"currency": "BRL",
				"amount": 2000
			},
			"id": "tx-1"
		}
	}`)

	firstHash, err := idempotency.Hash(first)
	if err != nil {
		t.Fatalf("hash first payload: %v", err)
	}

	secondHash, err := idempotency.Hash(second)
	if err != nil {
		t.Fatalf("hash second payload: %v", err)
	}

	if firstHash != secondHash {
		t.Fatal("nested field ordering changed canonical hash")
	}
}

func TestHashDifferentPayloadProducesDifferentHash(t *testing.T) {
	first := []byte(`{
		"walletId": "wallet-1",
		"amount": 2000,
		"currency": "BRL"
	}`)

	second := []byte(`{
		"walletId": "wallet-1",
		"amount": 3000,
		"currency": "BRL"
	}`)

	firstHash, err := idempotency.Hash(first)
	if err != nil {
		t.Fatalf("hash first payload: %v", err)
	}

	secondHash, err := idempotency.Hash(second)
	if err != nil {
		t.Fatalf("hash second payload: %v", err)
	}

	if firstHash == secondHash {
		t.Fatal("different payloads generated same hash")
	}
}

func TestHashPreservesLargeIntegerPrecision(t *testing.T) {
	first := []byte(`{
		"amount": 9007199254740993
	}`)

	second := []byte(`{
		"amount": 9007199254740992
	}`)

	firstHash, err := idempotency.Hash(first)
	if err != nil {
		t.Fatalf("hash first payload: %v", err)
	}

	secondHash, err := idempotency.Hash(second)
	if err != nil {
		t.Fatalf("hash second payload: %v", err)
	}

	if firstHash == secondHash {
		t.Fatal("large integer precision was lost")
	}
}

func TestHTTPAndSQSPayloadProduceSameHash(t *testing.T) {
	httpPayload := []byte(`{
		"externalTransactionId": "tx-123",
		"walletId": "wallet-123",
		"playerId": "player-123",
		"roundId": "round-123",
		"gameId": "game-123",
		"kind": "BET",
		"amount": 2500,
		"currency": "BRL"
	}`)

	sqsPayload := []byte(`{
		"currency": "BRL",
		"amount": 2500,
		"kind": "BET",
		"gameId": "game-123",
		"roundId": "round-123",
		"playerId": "player-123",
		"walletId": "wallet-123",
		"externalTransactionId": "tx-123"
	}`)

	httpHash, err := idempotency.Hash(httpPayload)
	if err != nil {
		t.Fatalf("hash HTTP payload: %v", err)
	}

	sqsHash, err := idempotency.Hash(sqsPayload)
	if err != nil {
		t.Fatalf("hash SQS payload: %v", err)
	}

	if httpHash != sqsHash {
		t.Fatalf(
			"HTTP/SQS hashes differ:\nHTTP: %s\nSQS:  %s",
			httpHash,
			sqsHash,
		)
	}
}

func TestCanonicalizeRejectsEmptyPayload(t *testing.T) {
	_, err := idempotency.Canonicalize(nil)

	if !errors.Is(err, idempotency.ErrEmptyPayload) {
		t.Fatalf(
			"expected ErrEmptyPayload, got %v",
			err,
		)
	}
}

func TestCanonicalizeRejectsInvalidJSON(t *testing.T) {
	payload := []byte(`{
		"amount":
	}`)

	_, err := idempotency.Canonicalize(payload)

	if !errors.Is(err, idempotency.ErrInvalidJSON) {
		t.Fatalf(
			"expected ErrInvalidJSON, got %v",
			err,
		)
	}
}

func TestCanonicalizeRejectsTrailingJSON(t *testing.T) {
	payload := []byte(
		`{"amount":2000} {"amount":3000}`,
	)

	_, err := idempotency.Canonicalize(payload)

	if !errors.Is(err, idempotency.ErrTrailingJSONData) {
		t.Fatalf(
			"expected ErrTrailingJSONData, got %v",
			err,
		)
	}
}
