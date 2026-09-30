package wager_test

import (
	"context"
	"testing"
	"time"

	applicationwager "jungle_gaming_teste_tecnico/internal/application/wager"
	domaintransaction "jungle_gaming_teste_tecnico/internal/domain/wagertransaction"
)

func newRefund(
	t *testing.T,
	id string,
	externalID string,
	idempotencyKey string,
	payloadHash string,
	walletID string,
	playerID string,
	referenceExternalTransactionID string,
	amount int64,
) *domaintransaction.WagerTransaction {
	t.Helper()

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
			ReferenceExternalTransactionID: referenceExternalTransactionID,
			CreatedAt:                      time.Now().UTC(),
		},
	)
	if err != nil {
		t.Fatalf("create refund: %v", err)
	}

	return &entity
}

func TestServiceRefundBeforeBetPendingThenResolved(t *testing.T) {
	env := newTestEnvironment(t)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	const (
		walletID = "wallet-refund-before-bet"
		playerID = "player-refund-before-bet"

		betID         = "transaction-reference-bet"
		betExternalID = "external-reference-bet"

		refundID         = "transaction-pending-refund"
		refundExternalID = "external-pending-refund"

		amount = int64(2000)
	)

	createWallet(
		t,
		env,
		walletID,
		playerID,
		10000,
	)

	/*
		REFUND chega antes da BET referenciada.

		A operação não pode ser perdida nem rejeitada imediatamente.
		Ela deve ser persistida como PENDING_REFERENCE.
	*/
	refund := newRefund(
		t,
		refundID,
		refundExternalID,
		"idempotency-pending-refund",
		"hash-pending-refund",
		walletID,
		playerID,
		betExternalID,
		amount,
	)

	pendingResult, err := env.service.Process(
		ctx,
		applicationwager.ProcessInput{
			Transaction:   refund,
			LedgerEntryID: "ledger-refund-before-reference",
			OccurredAt:    time.Now().UTC(),
		},
	)
	if err != nil {
		t.Fatalf(
			"process refund before reference: %v",
			err,
		)
	}

	if pendingResult.Transaction == nil {
		t.Fatal(
			"pending refund transaction must not be nil",
		)
	}

	if pendingResult.Replayed {
		t.Fatal(
			"first pending refund execution must not be replay",
		)
	}

	if pendingResult.Transaction.Status() !=
		domaintransaction.StatusPendingReference {

		t.Fatalf(
			"refund status = %s, want %s",
			pendingResult.Transaction.Status(),
			domaintransaction.StatusPendingReference,
		)
	}

	if pendingResult.Balance != 10000 {
		t.Fatalf(
			"balance while refund is pending = %d, want 10000",
			pendingResult.Balance,
		)
	}

	/*
		A operação pendente deve existir fisicamente no PostgreSQL.
	*/
	persistedPending, err :=
		env.transactionRepository.FindByID(
			ctx,
			refundID,
		)
	if err != nil {
		t.Fatalf(
			"find persisted pending refund: %v",
			err,
		)
	}

	if persistedPending.Status() !=
		domaintransaction.StatusPendingReference {

		t.Fatalf(
			"persisted refund status = %s, want %s",
			persistedPending.Status(),
			domaintransaction.StatusPendingReference,
		)
	}

	if persistedPending.ReferenceExternalTransactionID() !=
		betExternalID {

		t.Fatalf(
			"reference external transaction id = %q, want %q",
			persistedPending.ReferenceExternalTransactionID(),
			betExternalID,
		)
	}

	/*
		Nenhuma movimentação financeira pode ocorrer enquanto
		a referência não existir.
	*/
	walletBeforeReference, err :=
		env.walletRepository.FindByID(
			ctx,
			walletID,
		)
	if err != nil {
		t.Fatalf(
			"find wallet before reference: %v",
			err,
		)
	}

	if walletBeforeReference.Balance().Amount() != 10000 {
		t.Fatalf(
			"wallet balance before reference = %d, want 10000",
			walletBeforeReference.Balance().Amount(),
		)
	}

	if got := countRows(
		t,
		env,
		"wager_transactions",
	); got != 1 {
		t.Fatalf(
			"transaction count before reference = %d, want 1",
			got,
		)
	}

	if got := countRows(
		t,
		env,
		"ledger_entries",
	); got != 0 {
		t.Fatalf(
			"ledger count before reference = %d, want 0",
			got,
		)
	}

	/*
		Agora a BET referenciada chega normalmente.
	*/
	bet := newBet(
		t,
		betID,
		betExternalID,
		"idempotency-reference-bet",
		"hash-reference-bet",
		walletID,
		playerID,
		amount,
	)

	betResult, err := env.service.Process(
		ctx,
		applicationwager.ProcessInput{
			Transaction:   bet,
			LedgerEntryID: "ledger-reference-bet",
			OccurredAt:    time.Now().UTC(),
		},
	)
	if err != nil {
		t.Fatalf(
			"process referenced bet: %v",
			err,
		)
	}

	if betResult.Transaction == nil {
		t.Fatal(
			"processed bet transaction must not be nil",
		)
	}

	if betResult.Transaction.Status() !=
		domaintransaction.StatusProcessed {

		t.Fatalf(
			"bet status = %s, want %s",
			betResult.Transaction.Status(),
			domaintransaction.StatusProcessed,
		)
	}

	if betResult.Balance != 8000 {
		t.Fatalf(
			"balance after bet = %d, want 8000",
			betResult.Balance,
		)
	}

	/*
		Simula o worker retomando a operação persistida.

		O service deve localizar a BET através de
		(providerId, referenceExternalTransactionId),
		resolver a referência e processar o REFUND.
	*/
	resumedResult, err :=
		env.service.ResumePendingReference(
			ctx,
			refundID,
			"ledger-pending-refund-resolved",
			time.Now().UTC(),
		)
	if err != nil {
		t.Fatalf(
			"resume pending refund: %v",
			err,
		)
	}

	if resumedResult.Transaction == nil {
		t.Fatal(
			"resumed refund transaction must not be nil",
		)
	}

	if resumedResult.Replayed {
		t.Fatal(
			"first successful pending-reference resolution must not be replay",
		)
	}

	if resumedResult.Transaction.Status() !=
		domaintransaction.StatusProcessed {

		t.Fatalf(
			"resolved refund status = %s, want %s",
			resumedResult.Transaction.Status(),
			domaintransaction.StatusProcessed,
		)
	}

	if resumedResult.Balance != 10000 {
		t.Fatalf(
			"balance after refund = %d, want 10000",
			resumedResult.Balance,
		)
	}

	if resumedResult.Currency != "BRL" {
		t.Fatalf(
			"currency after refund = %s, want BRL",
			resumedResult.Currency,
		)
	}

	/*
		Valida estado final persistido.
	*/
	persistedRefund, err :=
		env.transactionRepository.FindByID(
			ctx,
			refundID,
		)
	if err != nil {
		t.Fatalf(
			"find resolved refund: %v",
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

	if persistedRefund.ReferenceTransactionID() != betID {
		t.Fatalf(
			"resolved reference transaction id = %q, want %q",
			persistedRefund.ReferenceTransactionID(),
			betID,
		)
	}

	finalWallet, err :=
		env.walletRepository.FindByID(
			ctx,
			walletID,
		)
	if err != nil {
		t.Fatalf(
			"find final wallet: %v",
			err,
		)
	}

	if finalWallet.Balance().Amount() != 10000 {
		t.Fatalf(
			"final wallet balance = %d, want 10000",
			finalWallet.Balance().Amount(),
		)
	}

	/*
		Temos exatamente:

		1 BET
		1 REFUND

		e exatamente:

		1 ledger DEBIT da BET
		1 ledger CREDIT do REFUND
	*/
	if got := countRows(
		t,
		env,
		"wager_transactions",
	); got != 2 {
		t.Fatalf(
			"final transaction count = %d, want 2",
			got,
		)
	}

	if got := countRows(
		t,
		env,
		"ledger_entries",
	); got != 2 {
		t.Fatalf(
			"final ledger count = %d, want 2",
			got,
		)
	}

	/*
		Replay HTTP/SQS equivalente da mesma operação.

		Não pode creditar novamente.
	*/
	replayRefund := newRefund(
		t,
		"transaction-pending-refund-replay",
		refundExternalID,
		"idempotency-pending-refund",
		"hash-pending-refund",
		walletID,
		playerID,
		betExternalID,
		amount,
	)

	replayResult, err := env.service.Process(
		ctx,
		applicationwager.ProcessInput{
			Transaction: replayRefund,
			LedgerEntryID: "ledger-pending-refund-" +
				"replay",
			OccurredAt: time.Now().UTC(),
		},
	)
	if err != nil {
		t.Fatalf(
			"replay resolved refund: %v",
			err,
		)
	}

	if !replayResult.Replayed {
		t.Fatal(
			"resolved refund replay must be marked as replay",
		)
	}

	if replayResult.Transaction == nil {
		t.Fatal(
			"replayed refund transaction must not be nil",
		)
	}

	if replayResult.Transaction.Status() !=
		domaintransaction.StatusProcessed {

		t.Fatalf(
			"replay status = %s, want %s",
			replayResult.Transaction.Status(),
			domaintransaction.StatusProcessed,
		)
	}

	if replayResult.Balance != 10000 {
		t.Fatalf(
			"replay balance = %d, want 10000",
			replayResult.Balance,
		)
	}

	walletAfterReplay, err :=
		env.walletRepository.FindByID(
			ctx,
			walletID,
		)
	if err != nil {
		t.Fatalf(
			"find wallet after replay: %v",
			err,
		)
	}

	if walletAfterReplay.Balance().Amount() != 10000 {
		t.Fatalf(
			"wallet balance after replay = %d, want 10000",
			walletAfterReplay.Balance().Amount(),
		)
	}

	if got := countRows(
		t,
		env,
		"wager_transactions",
	); got != 2 {
		t.Fatalf(
			"transaction count after replay = %d, want 2",
			got,
		)
	}

	if got := countRows(
		t,
		env,
		"ledger_entries",
	); got != 2 {
		t.Fatalf(
			"ledger count after replay = %d, want 2",
			got,
		)
	}
}
