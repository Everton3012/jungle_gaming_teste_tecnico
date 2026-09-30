package wager

import (
	"errors"
	"testing"
	"time"

	"jungle_gaming_teste_tecnico/internal/domain/ledger"
	"jungle_gaming_teste_tecnico/internal/domain/money"
	"jungle_gaming_teste_tecnico/internal/domain/wagertransaction"
	"jungle_gaming_teste_tecnico/internal/domain/wallet"
)

var testTime = time.Date(
	2026,
	9,
	29,
	23,
	0,
	0,
	0,
	time.UTC,
)

func mustMoney(t *testing.T, amount string) money.Money {
	t.Helper()

	value, err := money.Parse(amount, "BRL")
	if err != nil {
		t.Fatalf("money.Parse() error = %v", err)
	}

	return value
}

func mustWallet(t *testing.T, balance string) wallet.Wallet {
	t.Helper()

	value, err := wallet.New(
		"wallet-1",
		"player-1",
		mustMoney(t, balance),
		testTime,
	)
	if err != nil {
		t.Fatalf("wallet.New() error = %v", err)
	}

	return value
}

func mustTransaction(
	t *testing.T,
	kind wagertransaction.Kind,
	amount string,
	referenceExternalID string,
) wagertransaction.WagerTransaction {
	t.Helper()

	value, err := wagertransaction.NewExternal(
		wagertransaction.ExternalInput{
			ID:                             "transaction-1",
			ExternalTransactionID:          "external-1",
			ProviderID:                     "provider-1",
			IdempotencyKey:                 "provider-1:external-1",
			PayloadHash:                    "hash-1",
			WalletID:                       "wallet-1",
			PlayerID:                       "player-1",
			RoundID:                        "round-1",
			GameID:                         "game-1",
			Kind:                           kind,
			Money:                          mustMoney(t, amount),
			ReferenceExternalTransactionID: referenceExternalID,
			CreatedAt:                      testTime,
		},
	)
	if err != nil {
		t.Fatalf(
			"wagertransaction.NewExternal() error = %v",
			err,
		)
	}

	return value
}

func mustProcessedReference(
	t *testing.T,
	kind wagertransaction.Kind,
	amount string,
	externalID string,
) wagertransaction.WagerTransaction {
	t.Helper()

	referenceExternalID := ""
	if kind.RequiresReference() {
		referenceExternalID = "origin-external"
	}

	value, err := wagertransaction.NewExternal(
		wagertransaction.ExternalInput{
			ID:                             "reference-1",
			ExternalTransactionID:          externalID,
			ProviderID:                     "provider-1",
			IdempotencyKey:                 "provider-1:" + externalID,
			PayloadHash:                    "reference-hash",
			WalletID:                       "wallet-1",
			PlayerID:                       "player-1",
			RoundID:                        "round-1",
			GameID:                         "game-1",
			Kind:                           kind,
			Money:                          mustMoney(t, amount),
			ReferenceExternalTransactionID: referenceExternalID,
			CreatedAt:                      testTime,
		},
	)
	if err != nil {
		t.Fatalf(
			"wagertransaction.NewExternal() error = %v",
			err,
		)
	}

	if err := value.MarkProcessed(
		mustMoney(t, "100.00"),
		testTime.Add(time.Second),
	); err != nil {
		t.Fatalf("MarkProcessed() error = %v", err)
	}

	return value
}

func TestProcessBet(t *testing.T) {
	walletValue := mustWallet(t, "100.00")
	transaction := mustTransaction(
		t,
		wagertransaction.KindBet,
		"25.00",
		"",
	)

	result, err := NewProcessor().Process(
		&walletValue,
		&transaction,
		nil,
		"ledger-1",
		testTime.Add(time.Second),
	)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	if result.Balance().String() != "75.00" {
		t.Fatalf(
			"balance = %s, want 75.00",
			result.Balance().String(),
		)
	}

	if walletValue.Version() != 2 {
		t.Fatalf(
			"version = %d, want 2",
			walletValue.Version(),
		)
	}

	if transaction.Status() != wagertransaction.StatusProcessed {
		t.Fatalf(
			"status = %s, want PROCESSED",
			transaction.Status(),
		)
	}

	entry, ok := result.LedgerEntry()
	if !ok {
		t.Fatal("expected ledger entry")
	}

	if entry.Direction() != ledger.DirectionDebit {
		t.Fatalf(
			"direction = %s, want DEBIT",
			entry.Direction(),
		)
	}

	if entry.BalanceBefore().String() != "100.00" ||
		entry.BalanceAfter().String() != "75.00" {
		t.Fatalf(
			"ledger balance = %s -> %s, want 100.00 -> 75.00",
			entry.BalanceBefore(),
			entry.BalanceAfter(),
		)
	}
}

func TestProcessBetRejectsInsufficientBalanceWithoutMutation(
	t *testing.T,
) {
	walletValue := mustWallet(t, "20.00")
	transaction := mustTransaction(
		t,
		wagertransaction.KindBet,
		"25.00",
		"",
	)

	_, err := NewProcessor().Process(
		&walletValue,
		&transaction,
		nil,
		"ledger-1",
		testTime.Add(time.Second),
	)
	if !errors.Is(err, wallet.ErrInsufficientBalance) {
		t.Fatalf(
			"Process() error = %v, want %v",
			err,
			wallet.ErrInsufficientBalance,
		)
	}

	if walletValue.Balance().String() != "20.00" ||
		walletValue.Version() != 1 {
		t.Fatal("wallet mutated after failure")
	}

	if transaction.Status() != wagertransaction.StatusPending {
		t.Fatalf(
			"transaction status = %s, want PENDING",
			transaction.Status(),
		)
	}
}

func TestProcessWin(t *testing.T) {
	walletValue := mustWallet(t, "100.00")
	transaction := mustTransaction(
		t,
		wagertransaction.KindWin,
		"40.00",
		"",
	)

	result, err := NewProcessor().Process(
		&walletValue,
		&transaction,
		nil,
		"ledger-1",
		testTime.Add(time.Second),
	)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	entry, ok := result.LedgerEntry()
	if !ok {
		t.Fatal("expected ledger entry")
	}

	if result.Balance().String() != "140.00" ||
		entry.Direction() != ledger.DirectionCredit {
		t.Fatal("unexpected WIN result")
	}
}

func TestProcessWinWithBetReference(t *testing.T) {
	walletValue := mustWallet(t, "100.00")

	transaction := mustTransaction(
		t,
		wagertransaction.KindWin,
		"40.00",
		"bet-external",
	)

	reference := mustProcessedReference(
		t,
		wagertransaction.KindBet,
		"25.00",
		"bet-external",
	)

	_, err := NewProcessor().Process(
		&walletValue,
		&transaction,
		&reference,
		"ledger-1",
		testTime.Add(2*time.Second),
	)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	if transaction.ReferenceTransactionID() != reference.ID() {
		t.Fatalf(
			"reference id = %s, want %s",
			transaction.ReferenceTransactionID(),
			reference.ID(),
		)
	}
}

func TestProcessLoss(t *testing.T) {
	walletValue := mustWallet(t, "100.00")

	transaction := mustTransaction(
		t,
		wagertransaction.KindLoss,
		"0.00",
		"",
	)

	result, err := NewProcessor().Process(
		&walletValue,
		&transaction,
		nil,
		"",
		testTime.Add(time.Second),
	)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	if result.Balance().String() != "100.00" {
		t.Fatalf(
			"balance = %s, want 100.00",
			result.Balance(),
		)
	}

	if walletValue.Version() != 1 {
		t.Fatalf(
			"version = %d, want 1",
			walletValue.Version(),
		)
	}

	if _, ok := result.LedgerEntry(); ok {
		t.Fatal("LOSS must not create ledger entry")
	}

	if transaction.Status() != wagertransaction.StatusProcessed {
		t.Fatalf(
			"status = %s, want PROCESSED",
			transaction.Status(),
		)
	}
}

func TestProcessLossRejectsPositiveAmount(t *testing.T) {
	walletValue := mustWallet(t, "100.00")

	transaction := mustTransaction(
		t,
		wagertransaction.KindLoss,
		"1.00",
		"",
	)

	_, err := NewProcessor().Process(
		&walletValue,
		&transaction,
		nil,
		"",
		testTime.Add(time.Second),
	)
	if !errors.Is(err, ErrInvalidAmount) {
		t.Fatalf(
			"Process() error = %v, want %v",
			err,
			ErrInvalidAmount,
		)
	}
}

func TestProcessRefund(t *testing.T) {
	walletValue := mustWallet(t, "75.00")

	transaction := mustTransaction(
		t,
		wagertransaction.KindRefund,
		"25.00",
		"bet-external",
	)

	reference := mustProcessedReference(
		t,
		wagertransaction.KindBet,
		"25.00",
		"bet-external",
	)

	result, err := NewProcessor().Process(
		&walletValue,
		&transaction,
		&reference,
		"ledger-1",
		testTime.Add(2*time.Second),
	)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	entry, ok := result.LedgerEntry()
	if !ok || entry.Direction() != ledger.DirectionCredit {
		t.Fatal("REFUND must create CREDIT ledger entry")
	}

	if result.Balance().String() != "100.00" {
		t.Fatalf(
			"balance = %s, want 100.00",
			result.Balance(),
		)
	}

	if transaction.ReferenceTransactionID() != reference.ID() {
		t.Fatalf(
			"reference id = %s, want %s",
			transaction.ReferenceTransactionID(),
			reference.ID(),
		)
	}
}

func TestProcessRefundRejectsNonBetReference(t *testing.T) {
	walletValue := mustWallet(t, "100.00")

	transaction := mustTransaction(
		t,
		wagertransaction.KindRefund,
		"25.00",
		"win-external",
	)

	reference := mustProcessedReference(
		t,
		wagertransaction.KindWin,
		"25.00",
		"win-external",
	)

	_, err := NewProcessor().Process(
		&walletValue,
		&transaction,
		&reference,
		"ledger-1",
		testTime.Add(2*time.Second),
	)
	if !errors.Is(err, ErrInvalidReferenceKind) {
		t.Fatalf(
			"Process() error = %v, want %v",
			err,
			ErrInvalidReferenceKind,
		)
	}
}

func TestProcessRefundRejectsPartialAmount(t *testing.T) {
	walletValue := mustWallet(t, "100.00")

	transaction := mustTransaction(
		t,
		wagertransaction.KindRefund,
		"20.00",
		"bet-external",
	)

	reference := mustProcessedReference(
		t,
		wagertransaction.KindBet,
		"25.00",
		"bet-external",
	)

	_, err := NewProcessor().Process(
		&walletValue,
		&transaction,
		&reference,
		"ledger-1",
		testTime.Add(2*time.Second),
	)
	if !errors.Is(err, ErrInvalidReferenceAmount) {
		t.Fatalf(
			"Process() error = %v, want %v",
			err,
			ErrInvalidReferenceAmount,
		)
	}
}

func TestProcessRollbackBet(t *testing.T) {
	walletValue := mustWallet(t, "75.00")

	transaction := mustTransaction(
		t,
		wagertransaction.KindRollback,
		"25.00",
		"bet-external",
	)

	reference := mustProcessedReference(
		t,
		wagertransaction.KindBet,
		"25.00",
		"bet-external",
	)

	result, err := NewProcessor().Process(
		&walletValue,
		&transaction,
		&reference,
		"ledger-1",
		testTime.Add(2*time.Second),
	)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	entry, ok := result.LedgerEntry()
	if !ok || entry.Direction() != ledger.DirectionCredit {
		t.Fatal("rollback of BET must create CREDIT ledger entry")
	}

	if result.Balance().String() != "100.00" {
		t.Fatalf(
			"balance = %s, want 100.00",
			result.Balance(),
		)
	}
}

func TestProcessRollbackWin(t *testing.T) {
	walletValue := mustWallet(t, "140.00")

	transaction := mustTransaction(
		t,
		wagertransaction.KindRollback,
		"40.00",
		"win-external",
	)

	reference := mustProcessedReference(
		t,
		wagertransaction.KindWin,
		"40.00",
		"win-external",
	)

	result, err := NewProcessor().Process(
		&walletValue,
		&transaction,
		&reference,
		"ledger-1",
		testTime.Add(2*time.Second),
	)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	entry, ok := result.LedgerEntry()
	if !ok || entry.Direction() != ledger.DirectionDebit {
		t.Fatal("rollback of WIN must create DEBIT ledger entry")
	}

	if result.Balance().String() != "100.00" {
		t.Fatalf(
			"balance = %s, want 100.00",
			result.Balance(),
		)
	}
}

func TestProcessRollbackRefund(t *testing.T) {
	walletValue := mustWallet(t, "100.00")

	transaction := mustTransaction(
		t,
		wagertransaction.KindRollback,
		"25.00",
		"refund-external",
	)

	reference := mustProcessedReference(
		t,
		wagertransaction.KindRefund,
		"25.00",
		"refund-external",
	)

	result, err := NewProcessor().Process(
		&walletValue,
		&transaction,
		&reference,
		"ledger-1",
		testTime.Add(2*time.Second),
	)
	if err != nil {
		t.Fatalf("Process() error = %v", err)
	}

	entry, ok := result.LedgerEntry()
	if !ok || entry.Direction() != ledger.DirectionDebit {
		t.Fatal("rollback of REFUND must create DEBIT ledger entry")
	}

	if result.Balance().String() != "75.00" {
		t.Fatalf(
			"balance = %s, want 75.00",
			result.Balance(),
		)
	}
}

func TestProcessRollbackRejectsInsufficientBalanceWithoutMutation(
	t *testing.T,
) {
	walletValue := mustWallet(t, "10.00")

	transaction := mustTransaction(
		t,
		wagertransaction.KindRollback,
		"25.00",
		"win-external",
	)

	reference := mustProcessedReference(
		t,
		wagertransaction.KindWin,
		"25.00",
		"win-external",
	)

	_, err := NewProcessor().Process(
		&walletValue,
		&transaction,
		&reference,
		"ledger-1",
		testTime.Add(2*time.Second),
	)
	if !errors.Is(err, wallet.ErrInsufficientBalance) {
		t.Fatalf(
			"Process() error = %v, want %v",
			err,
			wallet.ErrInsufficientBalance,
		)
	}

	if walletValue.Balance().String() != "10.00" ||
		walletValue.Version() != 1 {
		t.Fatal("wallet mutated after failed rollback")
	}

	if transaction.Status() != wagertransaction.StatusPending ||
		transaction.ReferenceTransactionID() != "" {
		t.Fatal("transaction mutated after failed rollback")
	}
}

func TestProcessRequiresPositiveAmount(t *testing.T) {
	kinds := []wagertransaction.Kind{
		wagertransaction.KindBet,
		wagertransaction.KindWin,
	}

	for _, kind := range kinds {
		t.Run(string(kind), func(t *testing.T) {
			walletValue := mustWallet(t, "100.00")

			transaction := mustTransaction(
				t,
				kind,
				"0.00",
				"",
			)

			_, err := NewProcessor().Process(
				&walletValue,
				&transaction,
				nil,
				"ledger-1",
				testTime.Add(time.Second),
			)
			if !errors.Is(err, ErrInvalidAmount) {
				t.Fatalf(
					"Process() error = %v, want %v",
					err,
					ErrInvalidAmount,
				)
			}
		})
	}
}

func TestProcessRejectsWalletMismatch(t *testing.T) {
	transaction := mustTransaction(
		t,
		wagertransaction.KindBet,
		"25.00",
		"",
	)

	otherWallet, err := wallet.New(
		"wallet-2",
		"player-1",
		mustMoney(t, "100.00"),
		testTime,
	)
	if err != nil {
		t.Fatalf("wallet.New() error = %v", err)
	}

	_, err = NewProcessor().Process(
		&otherWallet,
		&transaction,
		nil,
		"ledger-1",
		testTime.Add(time.Second),
	)
	if !errors.Is(err, ErrWalletMismatch) {
		t.Fatalf(
			"Process() error = %v, want %v",
			err,
			ErrWalletMismatch,
		)
	}
}

func TestProcessRejectsMissingReference(t *testing.T) {
	walletValue := mustWallet(t, "100.00")

	transaction := mustTransaction(
		t,
		wagertransaction.KindRefund,
		"25.00",
		"bet-external",
	)

	_, err := NewProcessor().Process(
		&walletValue,
		&transaction,
		nil,
		"ledger-1",
		testTime.Add(time.Second),
	)
	if !errors.Is(err, ErrReferenceRequired) {
		t.Fatalf(
			"Process() error = %v, want %v",
			err,
			ErrReferenceRequired,
		)
	}
}

func TestProcessRejectsPendingReference(t *testing.T) {
	walletValue := mustWallet(t, "100.00")

	transaction := mustTransaction(
		t,
		wagertransaction.KindRefund,
		"25.00",
		"bet-external",
	)

	reference := mustTransaction(
		t,
		wagertransaction.KindBet,
		"25.00",
		"",
	)

	_, err := NewProcessor().Process(
		&walletValue,
		&transaction,
		&reference,
		"ledger-1",
		testTime.Add(time.Second),
	)
	if !errors.Is(err, ErrReferenceNotProcessed) {
		t.Fatalf(
			"Process() error = %v, want %v",
			err,
			ErrReferenceNotProcessed,
		)
	}
}

func TestProcessRejectsBlankLedgerIDWithoutMutation(
	t *testing.T,
) {
	walletValue := mustWallet(t, "100.00")

	transaction := mustTransaction(
		t,
		wagertransaction.KindBet,
		"25.00",
		"",
	)

	_, err := NewProcessor().Process(
		&walletValue,
		&transaction,
		nil,
		" ",
		testTime.Add(time.Second),
	)
	if !errors.Is(err, ErrInvalidLedgerEntryID) {
		t.Fatalf(
			"Process() error = %v, want %v",
			err,
			ErrInvalidLedgerEntryID,
		)
	}

	if walletValue.Balance().String() != "100.00" ||
		walletValue.Version() != 1 {
		t.Fatal("wallet mutated after invalid ledger id")
	}
}
