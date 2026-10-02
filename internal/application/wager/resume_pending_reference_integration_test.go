package wager_test

import (
	"context"
	"testing"
	"time"

	applicationwager "jungle_gaming_teste_tecnico/internal/application/wager"
	domaintransaction "jungle_gaming_teste_tecnico/internal/domain/wagertransaction"
)

func newPendingRefund(
	t *testing.T,
	id string,
	externalID string,
	idempotencyKey string,
	payloadHash string,
	walletID string,
	playerID string,
	referenceExternalID string,
	amount int64,
) *domaintransaction.WagerTransaction {
	t.Helper()

	now := time.Now().UTC().Truncate(time.Microsecond)

	entity, err := domaintransaction.NewExternal(
		domaintransaction.ExternalInput{
			ID:                             id,
			ExternalTransactionID:          externalID,
			ProviderID:                     "provider-test",
			IdempotencyKey:                 idempotencyKey,
			PayloadHash:                    payloadHash,
			WalletID:                       walletID,
			PlayerID:                       playerID,
			RoundID:                        "round-test",
			GameID:                         "game-test",
			Kind:                           domaintransaction.KindRefund,
			Money:                          mustMoney(t, amount),
			ReferenceExternalTransactionID: referenceExternalID,
			CreatedAt:                      now,
		},
	)
	if err != nil {
		t.Fatalf("create refund: %v", err)
	}

	if err := entity.MarkPendingReference(now); err != nil {
		t.Fatalf("mark refund pending reference: %v", err)
	}

	return &entity
}

func TestServiceResumePendingReference(t *testing.T) {
	env := newTestEnvironment(t)

	ctx := context.Background()

	createWallet(
		t,
		env,
		"wallet-resume",
		"player-resume",
		10000,
	)

	pendingRefund := newPendingRefund(
		t,
		"transaction-refund-resume",
		"external-refund-resume",
		"idempotency-refund-resume",
		"hash-refund-resume",
		"wallet-resume",
		"player-resume",
		"external-bet-resume",
		2000,
	)

	if err := env.transactionRepository.Create(
		ctx,
		pendingRefund,
	); err != nil {
		t.Fatalf(
			"persist pending refund: %v",
			err,
		)
	}

	persistedPending, err :=
		env.transactionRepository.FindByID(
			ctx,
			"transaction-refund-resume",
		)
	if err != nil {
		t.Fatalf(
			"find pending refund: %v",
			err,
		)
	}

	if persistedPending.Status() !=
		domaintransaction.StatusPendingReference {
		t.Fatalf(
			"pending status = %s, want %s",
			persistedPending.Status(),
			domaintransaction.StatusPendingReference,
		)
	}

	bet := newBet(
		t,
		"transaction-bet-resume",
		"external-bet-resume",
		"idempotency-bet-resume",
		"hash-bet-resume",
		"wallet-resume",
		"player-resume",
		2000,
	)

	betResult, err := env.service.Process(
		ctx,
		applicationwager.ProcessInput{
			Transaction:   bet,
			LedgerEntryID: "ledger-bet-resume",
			OccurredAt:    time.Now().UTC(),
		},
	)
	if err != nil {
		t.Fatalf(
			"process referenced bet: %v",
			err,
		)
	}

	if betResult.Balance != 8000 {
		t.Fatalf(
			"balance after bet = %d, want 8000",
			betResult.Balance,
		)
	}

	resumeResult, err :=
		env.service.ResumePendingReference(
			ctx,
			"transaction-refund-resume",
			"ledger-refund-resume",
			time.Now().UTC(),
		)
	if err != nil {
		t.Fatalf(
			"resume pending refund: %v",
			err,
		)
	}

	if resumeResult.Replayed {
		t.Fatal(
			"first resume must not be replay",
		)
	}

	if resumeResult.Balance != 10000 {
		t.Fatalf(
			"balance after refund = %d, want 10000",
			resumeResult.Balance,
		)
	}

	if resumeResult.Transaction.Status() !=
		domaintransaction.StatusProcessed {
		t.Fatalf(
			"refund status = %s, want %s",
			resumeResult.Transaction.Status(),
			domaintransaction.StatusProcessed,
		)
	}

	if resumeResult.Transaction.ReferenceTransactionID() !=
		"transaction-bet-resume" {
		t.Fatalf(
			"reference transaction ID = %q, want %q",
			resumeResult.Transaction.ReferenceTransactionID(),
			"transaction-bet-resume",
		)
	}

	walletEntity, err :=
		env.walletRepository.FindByID(
			ctx,
			"wallet-resume",
		)
	if err != nil {
		t.Fatalf(
			"find wallet: %v",
			err,
		)
	}

	if walletEntity.Balance().Amount() != 10000 {
		t.Fatalf(
			"persisted wallet balance = %d, want 10000",
			walletEntity.Balance().Amount(),
		)
	}

	persistedRefund, err :=
		env.transactionRepository.FindByID(
			ctx,
			"transaction-refund-resume",
		)
	if err != nil {
		t.Fatalf(
			"find persisted refund: %v",
			err,
		)
	}

	if persistedRefund.Status() !=
		domaintransaction.StatusProcessed {
		t.Fatalf(
			"persisted refund status = %s, want %s",
			persistedRefund.Status(),
			domaintransaction.StatusProcessed,
		)
	}

	if persistedRefund.ReferenceTransactionID() !=
		"transaction-bet-resume" {
		t.Fatalf(
			"persisted reference ID = %q, want %q",
			persistedRefund.ReferenceTransactionID(),
			"transaction-bet-resume",
		)
	}

	if countRows(t, env, "wager_transactions") != 2 {
		t.Fatal(
			"expected exactly two wager transactions",
		)
	}

	if countRows(t, env, "ledger_entries") != 2 {
		t.Fatal(
			"expected exactly two ledger entries",
		)
	}

	secondResult, err :=
		env.service.ResumePendingReference(
			ctx,
			"transaction-refund-resume",
			"ledger-refund-resume-second",
			time.Now().UTC(),
		)
	if err != nil {
		t.Fatalf(
			"second resume: %v",
			err,
		)
	}

	if !secondResult.Replayed {
		t.Fatal(
			"second resume must be replay",
		)
	}

	if secondResult.Balance != 10000 {
		t.Fatalf(
			"replay balance = %d, want 10000",
			secondResult.Balance,
		)
	}

	walletAfterReplay, err :=
		env.walletRepository.FindByID(
			ctx,
			"wallet-resume",
		)
	if err != nil {
		t.Fatalf(
			"find wallet after replay: %v",
			err,
		)
	}

	if walletAfterReplay.Balance().Amount() != 10000 {
		t.Fatalf(
			"wallet credited twice: balance=%d",
			walletAfterReplay.Balance().Amount(),
		)
	}

	if countRows(t, env, "wager_transactions") != 2 {
		t.Fatal(
			"replay created another wager transaction",
		)
	}

	if countRows(t, env, "ledger_entries") != 2 {
		t.Fatal(
			"replay created another ledger entry",
		)
	}
}
