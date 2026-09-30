package wager

import (
	"time"

	"jungle_gaming_teste_tecnico/internal/domain/wagertransaction"
)

type ReferenceState string

const (
	ReferenceMissing  ReferenceState = "MISSING"
	ReferenceWaiting  ReferenceState = "WAITING"
	ReferenceReady    ReferenceState = "READY"
	ReferenceRejected ReferenceState = "REJECTED"
	ReferenceFailed   ReferenceState = "FAILED"
)

func ReferenceStatus(
	transaction *wagertransaction.WagerTransaction,
	reference *wagertransaction.WagerTransaction,
) ReferenceState {
	if reference == nil {
		return ReferenceMissing
	}

	switch reference.Status() {
	case wagertransaction.StatusPending,
		wagertransaction.StatusPendingReference:
		return ReferenceWaiting

	case wagertransaction.StatusProcessed:
		return ReferenceReady

	case wagertransaction.StatusRejected:
		return ReferenceRejected

	case wagertransaction.StatusFailed:
		return ReferenceFailed

	default:
		return ReferenceMissing
	}
}

func WaitForReference(
	transaction *wagertransaction.WagerTransaction,
	updatedAt time.Time,
) error {
	if transaction == nil {
		return ErrReferenceRequired
	}

	if !transaction.Kind().RequiresReference() {
		return ErrReferenceNotRequired
	}

	switch transaction.Status() {
	case wagertransaction.StatusPending:
		return transaction.MarkPendingReference(updatedAt)

	case wagertransaction.StatusPendingReference:
		return nil

	default:
		return wagertransaction.ErrInvalidTransition
	}
}
