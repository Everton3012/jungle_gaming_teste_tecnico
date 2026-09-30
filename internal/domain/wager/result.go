package wager

import (
	"jungle_gaming_teste_tecnico/internal/domain/ledger"
	"jungle_gaming_teste_tecnico/internal/domain/money"
)

type Result struct {
	balance     money.Money
	ledgerEntry *ledger.Entry
}

func newResult(balance money.Money, ledgerEntry *ledger.Entry) Result {
	return Result{balance: balance, ledgerEntry: ledgerEntry}
}

func (r Result) Balance() money.Money {
	return r.balance
}

func (r Result) LedgerEntry() (ledger.Entry, bool) {
	if r.ledgerEntry == nil {
		return ledger.Entry{}, false
	}

	return *r.ledgerEntry, true
}
