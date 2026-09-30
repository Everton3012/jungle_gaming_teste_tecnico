package wager

import (
	"errors"
	"testing"
	"time"

	"jungle_gaming_teste_tecnico/internal/domain/wagertransaction"
)

func TestReferenceStatusMissing(t *testing.T) {
	transaction := newReferenceTransaction(t, wagertransaction.KindRefund)

	if got := ReferenceStatus(&transaction, nil); got != ReferenceMissing {
		t.Fatalf("ReferenceStatus() = %s, want %s", got, ReferenceMissing)
	}
}

func TestReferenceStatusWaitingForPending(t *testing.T) {
	transaction := newReferenceTransaction(t, wagertransaction.KindRefund)
	reference := newBetReference(t)

	if got := ReferenceStatus(&transaction, &reference); got != ReferenceWaiting {
		t.Fatalf("ReferenceStatus() = %s, want %s", got, ReferenceWaiting)
	}
}

func TestReferenceStatusWaitingForPendingReference(t *testing.T) {
	transaction := newReferenceTransaction(t, wagertransaction.KindRefund)
	reference := newBetReference(t)

	if err := reference.MarkPendingReference(time.Now()); err != nil {
		t.Fatalf("MarkPendingReference() error = %v", err)
	}

	if got := ReferenceStatus(&transaction, &reference); got != ReferenceWaiting {
		t.Fatalf("ReferenceStatus() = %s, want %s", got, ReferenceWaiting)
	}
}

func TestReferenceStatusReady(t *testing.T) {
	transaction := newReferenceTransaction(t, wagertransaction.KindRefund)
	reference := newBetReference(t)

	balance := mustMoney(t, "20.00")

	if err := reference.MarkProcessed(balance, time.Now()); err != nil {
		t.Fatalf("MarkProcessed() error = %v", err)
	}

	if got := ReferenceStatus(&transaction, &reference); got != ReferenceReady {
		t.Fatalf("ReferenceStatus() = %s, want %s", got, ReferenceReady)
	}
}

func TestReferenceStatusRejected(t *testing.T) {
	transaction := newReferenceTransaction(t, wagertransaction.KindRefund)
	reference := newBetReference(t)

	code, err := wagertransaction.NewFailureCode("INSUFFICIENT_FUNDS")
	if err != nil {
		t.Fatalf("NewFailureCode() error = %v", err)
	}

	if err := reference.MarkRejected(
		code,
		mustMoney(t, "100.00"),
		time.Now(),
	); err != nil {
		t.Fatalf("MarkRejected() error = %v", err)
	}

	if got := ReferenceStatus(&transaction, &reference); got != ReferenceRejected {
		t.Fatalf("ReferenceStatus() = %s, want %s", got, ReferenceRejected)
	}
}

func TestReferenceStatusFailed(t *testing.T) {
	transaction := newReferenceTransaction(t, wagertransaction.KindRefund)
	reference := newBetReference(t)

	code, err := wagertransaction.NewFailureCode(
		"PERMANENT_INFRASTRUCTURE_FAILURE",
	)
	if err != nil {
		t.Fatalf("NewFailureCode() error = %v", err)
	}

	if err := reference.MarkFailed(code, time.Now()); err != nil {
		t.Fatalf("MarkFailed() error = %v", err)
	}

	if got := ReferenceStatus(&transaction, &reference); got != ReferenceFailed {
		t.Fatalf("ReferenceStatus() = %s, want %s", got, ReferenceFailed)
	}
}

func TestWaitForReference(t *testing.T) {
	transaction := newReferenceTransaction(t, wagertransaction.KindRefund)

	if err := WaitForReference(&transaction, time.Now()); err != nil {
		t.Fatalf("WaitForReference() error = %v", err)
	}

	if transaction.Status() != wagertransaction.StatusPendingReference {
		t.Fatalf(
			"Status() = %s, want %s",
			transaction.Status(),
			wagertransaction.StatusPendingReference,
		)
	}
}

func TestWaitForReferenceIsIdempotentWhilePending(t *testing.T) {
	transaction := newReferenceTransaction(t, wagertransaction.KindRefund)

	if err := WaitForReference(&transaction, time.Now()); err != nil {
		t.Fatalf("first WaitForReference() error = %v", err)
	}

	if err := WaitForReference(&transaction, time.Now()); err != nil {
		t.Fatalf("second WaitForReference() error = %v", err)
	}

	if transaction.Status() != wagertransaction.StatusPendingReference {
		t.Fatalf(
			"Status() = %s, want %s",
			transaction.Status(),
			wagertransaction.StatusPendingReference,
		)
	}
}

func TestWaitForReferenceRejectsKindWithoutRequiredReference(t *testing.T) {
	transaction := newReferenceTransaction(t, wagertransaction.KindBet)

	err := WaitForReference(&transaction, time.Now())

	if !errors.Is(err, ErrReferenceNotRequired) {
		t.Fatalf(
			"WaitForReference() error = %v, want %v",
			err,
			ErrReferenceNotRequired,
		)
	}
}

func TestWaitForReferenceRejectsNilTransaction(t *testing.T) {
	err := WaitForReference(nil, time.Now())

	if !errors.Is(err, ErrReferenceRequired) {
		t.Fatalf(
			"WaitForReference() error = %v, want %v",
			err,
			ErrReferenceRequired,
		)
	}
}

func newReferenceTransaction(
	t *testing.T,
	kind wagertransaction.Kind,
) wagertransaction.WagerTransaction {
	t.Helper()

	referenceExternalID := ""
	if kind.RequiresReference() {
		referenceExternalID = "reference-1"
	}

	transaction, err := wagertransaction.NewExternal(
		wagertransaction.ExternalInput{
			ID:                             "transaction-1",
			ExternalTransactionID:          "external-1",
			ProviderID:                     "provider-1",
			IdempotencyKey:                 "idempotency-1",
			PayloadHash:                    "hash-1",
			WalletID:                       "wallet-1",
			PlayerID:                       "player-1",
			RoundID:                        "round-1",
			GameID:                         "game-1",
			Kind:                           kind,
			Money:                          mustMoney(t, "10.00"),
			ReferenceExternalTransactionID: referenceExternalID,
			CreatedAt:                      time.Now(),
		},
	)
	if err != nil {
		t.Fatalf("NewExternal() error = %v", err)
	}

	return transaction
}

func newBetReference(t *testing.T) wagertransaction.WagerTransaction {
	t.Helper()

	transaction, err := wagertransaction.NewExternal(
		wagertransaction.ExternalInput{
			ID:                    "reference-internal-1",
			ExternalTransactionID: "reference-1",
			ProviderID:            "provider-1",
			IdempotencyKey:        "reference-idempotency-1",
			PayloadHash:           "reference-hash-1",
			WalletID:              "wallet-1",
			PlayerID:              "player-1",
			RoundID:               "round-1",
			GameID:                "game-1",
			Kind:                  wagertransaction.KindBet,
			Money:                 mustMoney(t, "10.00"),
			CreatedAt:             time.Now(),
		},
	)
	if err != nil {
		t.Fatalf("NewExternal() error = %v", err)
	}

	return transaction
}
