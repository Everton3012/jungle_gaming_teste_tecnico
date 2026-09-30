package wagertransaction

import (
	"errors"
	"testing"
	"time"

	"jungle_gaming_teste_tecnico/internal/domain/money"
)

func transactionTestTime() time.Time {
	return time.Date(2026, 9, 29, 21, 0, 0, 0, time.UTC)
}

func transactionMoney(t *testing.T, value string) money.Money {
	t.Helper()

	result, err := money.Parse(value, "BRL")
	if err != nil {
		t.Fatalf("money.Parse() error = %v", err)
	}

	return result
}

func validExternalInput(t *testing.T, kind Kind) ExternalInput {
	t.Helper()

	input := ExternalInput{
		ID:                    "transaction-1",
		ExternalTransactionID: "external-1",
		ProviderID:            "provider-1",
		IdempotencyKey:        "idempotency-1",
		PayloadHash:           "payload-hash-1",
		WalletID:              "wallet-1",
		PlayerID:              "player-1",
		RoundID:               "round-1",
		GameID:                "game-1",
		Kind:                  kind,
		Money:                 transactionMoney(t, "10.00"),
		CreatedAt:             transactionTestTime(),
	}

	if kind.RequiresReference() {
		input.ReferenceExternalTransactionID = "reference-external-1"
	}

	return input
}

func TestNewExternal(t *testing.T) {
	input := validExternalInput(t, KindBet)

	transaction, err := NewExternal(input)
	if err != nil {
		t.Fatalf("NewExternal() error = %v", err)
	}

	if transaction.ID() != input.ID {
		t.Errorf("ID() = %s, want %s", transaction.ID(), input.ID)
	}

	if transaction.ExternalTransactionID() != input.ExternalTransactionID {
		t.Errorf(
			"ExternalTransactionID() = %s, want %s",
			transaction.ExternalTransactionID(),
			input.ExternalTransactionID,
		)
	}

	if transaction.ProviderID() != input.ProviderID {
		t.Errorf(
			"ProviderID() = %s, want %s",
			transaction.ProviderID(),
			input.ProviderID,
		)
	}

	if transaction.IdempotencyKey() != input.IdempotencyKey {
		t.Errorf(
			"IdempotencyKey() = %s, want %s",
			transaction.IdempotencyKey(),
			input.IdempotencyKey,
		)
	}

	if transaction.PayloadHash() != input.PayloadHash {
		t.Errorf(
			"PayloadHash() = %s, want %s",
			transaction.PayloadHash(),
			input.PayloadHash,
		)
	}

	if transaction.WalletID() != input.WalletID {
		t.Errorf(
			"WalletID() = %s, want %s",
			transaction.WalletID(),
			input.WalletID,
		)
	}

	if transaction.PlayerID() != input.PlayerID {
		t.Errorf(
			"PlayerID() = %s, want %s",
			transaction.PlayerID(),
			input.PlayerID,
		)
	}

	if transaction.RoundID() != input.RoundID {
		t.Errorf(
			"RoundID() = %s, want %s",
			transaction.RoundID(),
			input.RoundID,
		)
	}

	if transaction.GameID() != input.GameID {
		t.Errorf(
			"GameID() = %s, want %s",
			transaction.GameID(),
			input.GameID,
		)
	}

	if transaction.Kind() != KindBet {
		t.Errorf("Kind() = %s, want %s", transaction.Kind(), KindBet)
	}

	if transaction.Money().String() != "10.00" {
		t.Errorf(
			"Money() = %s, want 10.00",
			transaction.Money().String(),
		)
	}

	if transaction.Status() != StatusPending {
		t.Errorf(
			"Status() = %s, want %s",
			transaction.Status(),
			StatusPending,
		)
	}

	if transaction.FailureCode() != "" {
		t.Errorf(
			"FailureCode() = %s, want empty",
			transaction.FailureCode(),
		)
	}

	if _, ok := transaction.ResultBalance(); ok {
		t.Error("ResultBalance() should not exist for a new transaction")
	}

	if !transaction.CreatedAt().Equal(input.CreatedAt) {
		t.Errorf(
			"CreatedAt() = %v, want %v",
			transaction.CreatedAt(),
			input.CreatedAt,
		)
	}

	if !transaction.UpdatedAt().Equal(input.CreatedAt) {
		t.Errorf(
			"UpdatedAt() = %v, want %v",
			transaction.UpdatedAt(),
			input.CreatedAt,
		)
	}
}

func TestNewExternalSupportsAllExternalKinds(t *testing.T) {
	kinds := []Kind{
		KindBet,
		KindWin,
		KindLoss,
		KindRefund,
		KindRollback,
	}

	for _, kind := range kinds {
		t.Run(string(kind), func(t *testing.T) {
			input := validExternalInput(t, kind)

			transaction, err := NewExternal(input)
			if err != nil {
				t.Fatalf("NewExternal() error = %v", err)
			}

			if transaction.Kind() != kind {
				t.Errorf(
					"Kind() = %s, want %s",
					transaction.Kind(),
					kind,
				)
			}

			if transaction.Status() != StatusPending {
				t.Errorf(
					"Status() = %s, want %s",
					transaction.Status(),
					StatusPending,
				)
			}
		})
	}
}

func TestNewExternalRejectsOpening(t *testing.T) {
	input := validExternalInput(t, KindBet)
	input.Kind = KindOpening

	_, err := NewExternal(input)

	if !errors.Is(err, ErrOpeningExternal) {
		t.Errorf(
			"NewExternal() error = %v, want %v",
			err,
			ErrOpeningExternal,
		)
	}
}

func TestNewExternalRejectsInvalidKind(t *testing.T) {
	input := validExternalInput(t, KindBet)
	input.Kind = Kind("INVALID")

	_, err := NewExternal(input)

	if !errors.Is(err, ErrInvalidKind) {
		t.Errorf(
			"NewExternal() error = %v, want %v",
			err,
			ErrInvalidKind,
		)
	}
}

func TestNewExternalRejectsInvalidRequiredFields(t *testing.T) {
	tests := []struct {
		name     string
		change   func(*ExternalInput)
		expected error
	}{
		{
			name: "id",
			change: func(input *ExternalInput) {
				input.ID = " "
			},
			expected: ErrInvalidID,
		},
		{
			name: "external transaction id",
			change: func(input *ExternalInput) {
				input.ExternalTransactionID = " "
			},
			expected: ErrInvalidExternalTransactionID,
		},
		{
			name: "provider id",
			change: func(input *ExternalInput) {
				input.ProviderID = " "
			},
			expected: ErrInvalidProviderID,
		},
		{
			name: "idempotency key",
			change: func(input *ExternalInput) {
				input.IdempotencyKey = " "
			},
			expected: ErrInvalidIdempotencyKey,
		},
		{
			name: "payload hash",
			change: func(input *ExternalInput) {
				input.PayloadHash = " "
			},
			expected: ErrInvalidPayloadHash,
		},
		{
			name: "wallet id",
			change: func(input *ExternalInput) {
				input.WalletID = " "
			},
			expected: ErrInvalidWalletID,
		},
		{
			name: "player id",
			change: func(input *ExternalInput) {
				input.PlayerID = " "
			},
			expected: ErrInvalidPlayerID,
		},
		{
			name: "round id",
			change: func(input *ExternalInput) {
				input.RoundID = " "
			},
			expected: ErrInvalidRoundID,
		},
		{
			name: "game id",
			change: func(input *ExternalInput) {
				input.GameID = " "
			},
			expected: ErrInvalidGameID,
		},
		{
			name: "timestamp",
			change: func(input *ExternalInput) {
				input.CreatedAt = time.Time{}
			},
			expected: ErrInvalidTimestamp,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := validExternalInput(t, KindBet)
			tt.change(&input)

			_, err := NewExternal(input)

			if !errors.Is(err, tt.expected) {
				t.Errorf(
					"NewExternal() error = %v, want %v",
					err,
					tt.expected,
				)
			}
		})
	}
}

func TestNewExternalRequiresReferenceForRefundAndRollback(t *testing.T) {
	kinds := []Kind{
		KindRefund,
		KindRollback,
	}

	for _, kind := range kinds {
		t.Run(string(kind), func(t *testing.T) {
			input := validExternalInput(t, kind)
			input.ReferenceExternalTransactionID = ""

			_, err := NewExternal(input)

			if !errors.Is(err, ErrInvalidExternalTransactionID) {
				t.Errorf(
					"NewExternal() error = %v, want %v",
					err,
					ErrInvalidExternalTransactionID,
				)
			}
		})
	}
}

func TestMarkPendingReference(t *testing.T) {
	transaction, _ := NewExternal(
		validExternalInput(t, KindRefund),
	)

	updatedAt := transactionTestTime().Add(time.Minute)

	err := transaction.MarkPendingReference(updatedAt)
	if err != nil {
		t.Fatalf("MarkPendingReference() error = %v", err)
	}

	if transaction.Status() != StatusPendingReference {
		t.Errorf(
			"Status() = %s, want %s",
			transaction.Status(),
			StatusPendingReference,
		)
	}

	if !transaction.UpdatedAt().Equal(updatedAt) {
		t.Errorf(
			"UpdatedAt() = %v, want %v",
			transaction.UpdatedAt(),
			updatedAt,
		)
	}
}

func TestResolveReference(t *testing.T) {
	transaction, _ := NewExternal(
		validExternalInput(t, KindRefund),
	)

	err := transaction.MarkPendingReference(
		transactionTestTime().Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("MarkPendingReference() error = %v", err)
	}

	updatedAt := transactionTestTime().Add(2 * time.Minute)

	err = transaction.ResolveReference(
		"reference-transaction-1",
		updatedAt,
	)
	if err != nil {
		t.Fatalf("ResolveReference() error = %v", err)
	}

	if transaction.ReferenceTransactionID() != "reference-transaction-1" {
		t.Errorf(
			"ReferenceTransactionID() = %s, want reference-transaction-1",
			transaction.ReferenceTransactionID(),
		)
	}

	if transaction.Status() != StatusPending {
		t.Errorf(
			"Status() = %s, want %s",
			transaction.Status(),
			StatusPending,
		)
	}

	if !transaction.UpdatedAt().Equal(updatedAt) {
		t.Errorf(
			"UpdatedAt() = %v, want %v",
			transaction.UpdatedAt(),
			updatedAt,
		)
	}
}

func TestResolveReferenceRejectsEmptyReference(t *testing.T) {
	transaction, _ := NewExternal(
		validExternalInput(t, KindRefund),
	)

	_ = transaction.MarkPendingReference(
		transactionTestTime().Add(time.Minute),
	)

	err := transaction.ResolveReference(
		" ",
		transactionTestTime().Add(2*time.Minute),
	)

	if !errors.Is(err, ErrInvalidID) {
		t.Errorf(
			"ResolveReference() error = %v, want %v",
			err,
			ErrInvalidID,
		)
	}

	if transaction.Status() != StatusPendingReference {
		t.Errorf(
			"Status() = %s, want %s",
			transaction.Status(),
			StatusPendingReference,
		)
	}
}

func TestMarkProcessed(t *testing.T) {
	transaction, _ := NewExternal(
		validExternalInput(t, KindBet),
	)

	resultBalance := transactionMoney(t, "90.00")
	updatedAt := transactionTestTime().Add(time.Minute)

	err := transaction.MarkProcessed(resultBalance, updatedAt)
	if err != nil {
		t.Fatalf("MarkProcessed() error = %v", err)
	}

	if transaction.Status() != StatusProcessed {
		t.Errorf(
			"Status() = %s, want %s",
			transaction.Status(),
			StatusProcessed,
		)
	}

	if transaction.FailureCode() != "" {
		t.Errorf(
			"FailureCode() = %s, want empty",
			transaction.FailureCode(),
		)
	}

	result, ok := transaction.ResultBalance()
	if !ok {
		t.Fatal("ResultBalance() should exist")
	}

	if result.String() != "90.00" {
		t.Errorf(
			"ResultBalance() = %s, want 90.00",
			result.String(),
		)
	}

	if !transaction.UpdatedAt().Equal(updatedAt) {
		t.Errorf(
			"UpdatedAt() = %v, want %v",
			transaction.UpdatedAt(),
			updatedAt,
		)
	}
}

func TestPendingReferenceCannotBeProcessedBeforeResolution(t *testing.T) {
	transaction, _ := NewExternal(
		validExternalInput(t, KindRefund),
	)

	_ = transaction.MarkPendingReference(
		transactionTestTime().Add(time.Minute),
	)

	resultBalance := transactionMoney(t, "100.00")

	err := transaction.MarkProcessed(
		resultBalance,
		transactionTestTime().Add(2*time.Minute),
	)

	if !errors.Is(err, ErrInvalidTransition) {
		t.Errorf(
			"MarkProcessed() error = %v, want %v",
			err,
			ErrInvalidTransition,
		)
	}

	if transaction.Status() != StatusPendingReference {
		t.Errorf(
			"Status() = %s, want %s",
			transaction.Status(),
			StatusPendingReference,
		)
	}
}

func TestMarkProcessedRejectsCurrencyMismatch(t *testing.T) {
	transaction, _ := NewExternal(
		validExternalInput(t, KindBet),
	)

	resultBalance, _ := money.Parse("90.00", "USD")

	err := transaction.MarkProcessed(
		resultBalance,
		transactionTestTime().Add(time.Minute),
	)

	if !errors.Is(err, money.ErrCurrencyMismatch) {
		t.Errorf(
			"MarkProcessed() error = %v, want %v",
			err,
			money.ErrCurrencyMismatch,
		)
	}

	if transaction.Status() != StatusPending {
		t.Errorf(
			"Status() = %s, want %s",
			transaction.Status(),
			StatusPending,
		)
	}
}

func TestMarkRejected(t *testing.T) {
	transaction, _ := NewExternal(
		validExternalInput(t, KindBet),
	)

	failureCode, _ := NewFailureCode("INSUFFICIENT_FUNDS")
	resultBalance := transactionMoney(t, "5.00")

	err := transaction.MarkRejected(
		failureCode,
		resultBalance,
		transactionTestTime().Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("MarkRejected() error = %v", err)
	}

	if transaction.Status() != StatusRejected {
		t.Errorf(
			"Status() = %s, want %s",
			transaction.Status(),
			StatusRejected,
		)
	}

	if transaction.FailureCode() != failureCode {
		t.Errorf(
			"FailureCode() = %s, want %s",
			transaction.FailureCode(),
			failureCode,
		)
	}

	result, ok := transaction.ResultBalance()
	if !ok {
		t.Fatal("ResultBalance() should exist")
	}

	if result.String() != "5.00" {
		t.Errorf(
			"ResultBalance() = %s, want 5.00",
			result.String(),
		)
	}
}

func TestMarkRejectedFromPendingReference(t *testing.T) {
	transaction, _ := NewExternal(
		validExternalInput(t, KindRefund),
	)

	_ = transaction.MarkPendingReference(
		transactionTestTime().Add(time.Minute),
	)

	failureCode, _ := NewFailureCode("REFERENCE_REJECTED")
	resultBalance := transactionMoney(t, "100.00")

	err := transaction.MarkRejected(
		failureCode,
		resultBalance,
		transactionTestTime().Add(2*time.Minute),
	)
	if err != nil {
		t.Fatalf("MarkRejected() error = %v", err)
	}

	if transaction.Status() != StatusRejected {
		t.Errorf(
			"Status() = %s, want %s",
			transaction.Status(),
			StatusRejected,
		)
	}
}

func TestMarkFailed(t *testing.T) {
	transaction, _ := NewExternal(
		validExternalInput(t, KindBet),
	)

	failureCode, _ := NewFailureCode("PROCESSING_FAILED")

	err := transaction.MarkFailed(
		failureCode,
		transactionTestTime().Add(time.Minute),
	)
	if err != nil {
		t.Fatalf("MarkFailed() error = %v", err)
	}

	if transaction.Status() != StatusFailed {
		t.Errorf(
			"Status() = %s, want %s",
			transaction.Status(),
			StatusFailed,
		)
	}

	if transaction.FailureCode() != failureCode {
		t.Errorf(
			"FailureCode() = %s, want %s",
			transaction.FailureCode(),
			failureCode,
		)
	}

	if _, ok := transaction.ResultBalance(); ok {
		t.Error("ResultBalance() should not exist for failed transaction")
	}
}

func TestRejectAndFailRequireFailureCode(t *testing.T) {
	t.Run("rejected", func(t *testing.T) {
		transaction, _ := NewExternal(
			validExternalInput(t, KindBet),
		)

		err := transaction.MarkRejected(
			"",
			transactionMoney(t, "100.00"),
			transactionTestTime().Add(time.Minute),
		)

		if !errors.Is(err, ErrInvalidFailureCode) {
			t.Errorf(
				"MarkRejected() error = %v, want %v",
				err,
				ErrInvalidFailureCode,
			)
		}
	})

	t.Run("failed", func(t *testing.T) {
		transaction, _ := NewExternal(
			validExternalInput(t, KindBet),
		)

		err := transaction.MarkFailed(
			"",
			transactionTestTime().Add(time.Minute),
		)

		if !errors.Is(err, ErrInvalidFailureCode) {
			t.Errorf(
				"MarkFailed() error = %v, want %v",
				err,
				ErrInvalidFailureCode,
			)
		}
	})
}

func TestTerminalTransactionsCannotTransition(t *testing.T) {
	statuses := []Status{
		StatusProcessed,
		StatusRejected,
		StatusFailed,
	}

	for _, status := range statuses {
		t.Run(string(status), func(t *testing.T) {
			transaction := rehydratedTransactionForStatus(
				t,
				status,
			)

			err := transaction.MarkPendingReference(
				transactionTestTime().Add(2 * time.Hour),
			)

			if !errors.Is(err, ErrTerminalTransaction) {
				t.Errorf(
					"MarkPendingReference() error = %v, want %v",
					err,
					ErrTerminalTransaction,
				)
			}
		})
	}
}

func TestTransitionRejectsZeroTimestamp(t *testing.T) {
	transaction, _ := NewExternal(
		validExternalInput(t, KindRefund),
	)

	err := transaction.MarkPendingReference(time.Time{})

	if !errors.Is(err, ErrInvalidTimestamp) {
		t.Errorf(
			"MarkPendingReference() error = %v, want %v",
			err,
			ErrInvalidTimestamp,
		)
	}

	if transaction.Status() != StatusPending {
		t.Errorf(
			"Status() = %s, want %s",
			transaction.Status(),
			StatusPending,
		)
	}
}

func TestNewFailureCode(t *testing.T) {
	code, err := NewFailureCode("INSUFFICIENT_FUNDS")
	if err != nil {
		t.Fatalf("NewFailureCode() error = %v", err)
	}

	if code.String() != "INSUFFICIENT_FUNDS" {
		t.Errorf(
			"String() = %s, want INSUFFICIENT_FUNDS",
			code.String(),
		)
	}
}

func TestNewFailureCodeRejectsEmptyValue(t *testing.T) {
	_, err := NewFailureCode(" ")

	if !errors.Is(err, ErrInvalidFailureCode) {
		t.Errorf(
			"NewFailureCode() error = %v, want %v",
			err,
			ErrInvalidFailureCode,
		)
	}
}

func TestKindProperties(t *testing.T) {
	if !KindBet.IsValid() {
		t.Error("BET should be valid")
	}

	if !KindOpening.IsValid() {
		t.Error("OPENING should be valid")
	}

	if Kind("INVALID").IsValid() {
		t.Error("INVALID should not be valid")
	}

	if KindOpening.IsExternal() {
		t.Error("OPENING should not be external")
	}

	if !KindBet.IsExternal() {
		t.Error("BET should be external")
	}

	if !KindRefund.RequiresReference() {
		t.Error("REFUND should require reference")
	}

	if !KindRollback.RequiresReference() {
		t.Error("ROLLBACK should require reference")
	}

	if KindBet.RequiresReference() {
		t.Error("BET should not require reference")
	}
}

func TestStatusProperties(t *testing.T) {
	terminal := []Status{
		StatusProcessed,
		StatusRejected,
		StatusFailed,
	}

	for _, status := range terminal {
		if !status.IsTerminal() {
			t.Errorf("%s should be terminal", status)
		}
	}

	nonTerminal := []Status{
		StatusPending,
		StatusPendingReference,
	}

	for _, status := range nonTerminal {
		if status.IsTerminal() {
			t.Errorf("%s should not be terminal", status)
		}
	}

	if Status("INVALID").IsValid() {
		t.Error("INVALID should not be valid")
	}
}

func TestRehydrateProcessed(t *testing.T) {
	resultBalance := transactionMoney(t, "90.00")

	input := RehydrateInput{
		ID:                    "transaction-1",
		ExternalTransactionID: "external-1",
		ProviderID:            "provider-1",
		IdempotencyKey:        "idempotency-1",
		PayloadHash:           "payload-hash-1",
		WalletID:              "wallet-1",
		PlayerID:              "player-1",
		RoundID:               "round-1",
		GameID:                "game-1",
		Kind:                  KindBet,
		Money:                 transactionMoney(t, "10.00"),
		Status:                StatusProcessed,
		ResultBalance:         &resultBalance,
		CreatedAt:             transactionTestTime(),
		UpdatedAt:             transactionTestTime().Add(time.Minute),
	}

	transaction, err := Rehydrate(input)
	if err != nil {
		t.Fatalf("Rehydrate() error = %v", err)
	}

	if transaction.Status() != StatusProcessed {
		t.Errorf(
			"Status() = %s, want %s",
			transaction.Status(),
			StatusProcessed,
		)
	}

	result, ok := transaction.ResultBalance()
	if !ok {
		t.Fatal("ResultBalance() should exist")
	}

	if result.String() != "90.00" {
		t.Errorf(
			"ResultBalance() = %s, want 90.00",
			result.String(),
		)
	}
}

func TestRehydrateRejectsInvalidState(t *testing.T) {
	input := RehydrateInput{
		ID:                    "transaction-1",
		ExternalTransactionID: "external-1",
		ProviderID:            "provider-1",
		IdempotencyKey:        "idempotency-1",
		PayloadHash:           "payload-hash-1",
		WalletID:              "wallet-1",
		PlayerID:              "player-1",
		RoundID:               "round-1",
		GameID:                "game-1",
		Kind:                  KindBet,
		Money:                 transactionMoney(t, "10.00"),
		Status:                StatusProcessed,
		CreatedAt:             transactionTestTime(),
		UpdatedAt:             transactionTestTime().Add(time.Minute),
	}

	_, err := Rehydrate(input)

	if !errors.Is(err, ErrInvalidStatus) {
		t.Errorf(
			"Rehydrate() error = %v, want %v",
			err,
			ErrInvalidStatus,
		)
	}
}

func TestRehydrateRejectsTimestampInconsistency(t *testing.T) {
	input := RehydrateInput{
		ID:                    "transaction-1",
		ExternalTransactionID: "external-1",
		ProviderID:            "provider-1",
		IdempotencyKey:        "idempotency-1",
		PayloadHash:           "payload-hash-1",
		WalletID:              "wallet-1",
		PlayerID:              "player-1",
		RoundID:               "round-1",
		GameID:                "game-1",
		Kind:                  KindBet,
		Money:                 transactionMoney(t, "10.00"),
		Status:                StatusPending,
		CreatedAt:             transactionTestTime(),
		UpdatedAt:             transactionTestTime().Add(-time.Minute),
	}

	_, err := Rehydrate(input)

	if !errors.Is(err, ErrInvalidTimestamp) {
		t.Errorf(
			"Rehydrate() error = %v, want %v",
			err,
			ErrInvalidTimestamp,
		)
	}
}

func rehydratedTransactionForStatus(
	t *testing.T,
	status Status,
) WagerTransaction {
	t.Helper()

	input := RehydrateInput{
		ID:                    "transaction-1",
		ExternalTransactionID: "external-1",
		ProviderID:            "provider-1",
		IdempotencyKey:        "idempotency-1",
		PayloadHash:           "payload-hash-1",
		WalletID:              "wallet-1",
		PlayerID:              "player-1",
		RoundID:               "round-1",
		GameID:                "game-1",
		Kind:                  KindBet,
		Money:                 transactionMoney(t, "10.00"),
		Status:                status,
		CreatedAt:             transactionTestTime(),
		UpdatedAt:             transactionTestTime().Add(time.Hour),
	}

	switch status {
	case StatusProcessed:
		resultBalance := transactionMoney(t, "90.00")
		input.ResultBalance = &resultBalance

	case StatusRejected:
		code, _ := NewFailureCode("REJECTED")
		resultBalance := transactionMoney(t, "100.00")
		input.FailureCode = code
		input.ResultBalance = &resultBalance

	case StatusFailed:
		code, _ := NewFailureCode("FAILED")
		input.FailureCode = code
	}

	transaction, err := Rehydrate(input)
	if err != nil {
		t.Fatalf("Rehydrate() error = %v", err)
	}

	return transaction
}
func TestNewOpening(t *testing.T) {
	amount := transactionMoney(t, "100.00")
	createdAt := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)

	transaction, err := NewOpening(OpeningInput{
		ID:        "opening-1",
		WalletID:  "wallet-1",
		PlayerID:  "player-1",
		Money:     amount,
		CreatedAt: createdAt,
	})
	if err != nil {
		t.Fatalf("NewOpening() error = %v", err)
	}

	if transaction.Kind() != KindOpening {
		t.Fatalf("Kind() = %s", transaction.Kind())
	}

	if transaction.Status() != StatusProcessed {
		t.Fatalf("Status() = %s", transaction.Status())
	}

	if transaction.ExternalTransactionID() != "" {
		t.Fatalf(
			"ExternalTransactionID() = %q",
			transaction.ExternalTransactionID(),
		)
	}

	if transaction.ProviderID() != "" {
		t.Fatalf("ProviderID() = %q", transaction.ProviderID())
	}

	if transaction.IdempotencyKey() != "" {
		t.Fatalf("IdempotencyKey() = %q", transaction.IdempotencyKey())
	}

	if transaction.PayloadHash() != "" {
		t.Fatalf("PayloadHash() = %q", transaction.PayloadHash())
	}

	resultBalance, ok := transaction.ResultBalance()
	if !ok {
		t.Fatal("ResultBalance() must exist")
	}

	if !resultBalance.Equal(amount) {
		t.Fatalf("ResultBalance() = %s", resultBalance.String())
	}
}

func TestNewOpeningRejectsZeroAmount(t *testing.T) {
	_, err := NewOpening(OpeningInput{
		ID:        "opening-1",
		WalletID:  "wallet-1",
		PlayerID:  "player-1",
		Money:     transactionMoney(t, "0.00"),
		CreatedAt: time.Now(),
	})

	if !errors.Is(err, ErrInvalidOpeningAmount) {
		t.Fatalf("NewOpening() error = %v", err)
	}
}

func TestNewOpeningRejectsNegativeAmount(t *testing.T) {
	amount, err := money.Rehydrate(-100, "BRL")
	if err != nil {
		t.Fatalf("money.Rehydrate() error = %v", err)
	}

	_, err = NewOpening(OpeningInput{
		ID:        "opening-1",
		WalletID:  "wallet-1",
		PlayerID:  "player-1",
		Money:     amount,
		CreatedAt: time.Now(),
	})

	if !errors.Is(err, ErrInvalidOpeningAmount) {
		t.Fatalf("NewOpening() error = %v", err)
	}
}
