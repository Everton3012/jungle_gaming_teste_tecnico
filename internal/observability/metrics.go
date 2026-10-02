package observability

import (
	"sync/atomic"
	"time"
)

type Metrics struct {
	HTTPRequests              atomic.Uint64
	WagerProcessed            atomic.Uint64
	WagerRejected             atomic.Uint64
	WagerPending              atomic.Uint64
	IdempotentReplays         atomic.Uint64
	SQSRedeliveries           atomic.Uint64
	InboxDuplicates           atomic.Uint64
	OutboxPublishRetries      atomic.Uint64
	ConcurrencyConflicts      atomic.Uint64
	ReconciliationDivergences atomic.Uint64
	ProcessingCount           atomic.Uint64
	ProcessingNanoseconds     atomic.Uint64
}

var Default = &Metrics{}

func (m *Metrics) ObserveProcessing(start time.Time) {
	if m == nil {
		return
	}
	m.ProcessingCount.Add(1)
	duration := time.Since(start)
	if duration > 0 {
		m.ProcessingNanoseconds.Add(uint64(duration))
	}
}
