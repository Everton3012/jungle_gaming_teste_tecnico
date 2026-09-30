package wager_test

import (
	"context"
	"testing"
	"time"

	applicationwager "jungle_gaming_teste_tecnico/internal/application/wager"
	domaintransaction "jungle_gaming_teste_tecnico/internal/domain/wagertransaction"
)

func newRollback(
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
			Kind:                           domaintransaction.KindRollback,
			Money:                          mustMoney(t, amount),
			ReferenceExternalTransactionID: referenceExternalTransactionID,
			CreatedAt:                      time.Now().UTC(),
		},
	)
	if err != nil {
		t.Fatalf("create rollback: %v", err)
	}

	return &entity
}

func TestServiceRollbackBeforeBetPendingThenResolved(t *testing.T) {
	env := newTestEnvironment(t)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	const (
		walletID = "wallet-rollback-before-bet"
		playerID = "player-rollback-before-bet"

		betID         = "transaction-rollback-reference-bet"
		betExternalID = "external-rollback-reference-bet"

		rollbackID         = "transaction-pending-rollback"
		rollbackExternalID = "external-pending-rollback"

		initialBalance = int64(10000)
		amount         = int64(2000)
	)

	createWallet(
		t,
		env,
		walletID,
		playerID,
		initialBalance,
	)

	/*
		ROLLBACK chega antes da BET referenciada.

		Não deve existir movimentação financeira.
		A operação deve ser persistida como PENDING_REFERENCE.
	*/
	rollback := newRollback(
		t,
		rollbackID,
		rollbackExternalID,
		"idempotency-pending-rollback",
		"hash-pending-rollback",
		walletID,
		playerID,
		betExternalID,
		amount,
	)

	pendingResult, err := env.service.Process(
		ctx,
		applicationwager.ProcessInput{
			Transaction:   rollback,
			LedgerEntryID: "ledger-rollback-before-reference",
			OccurredAt:    time.Now().UTC(),
		},
	)
	if err != nil {
		t.Fatalf(
			"process rollback before reference: %v",
			err,
		)
	}

	if pendingResult.Transaction == nil {
		t.Fatal("pending rollback transaction must not be nil")
	}

	if pendingResult.Replayed {
		t.Fatal("first pending rollback must not be replay")
	}

	if pendingResult.Transaction.Status() !=
		domaintransaction.StatusPendingReference {

		t.Fatalf(
			"rollback status = %s, want %s",
			pendingResult.Transaction.Status(),
			domaintransaction.StatusPendingReference,
		)
	}

	if pendingResult.Balance != initialBalance {
		t.Fatalf(
			"balance while rollback pending = %d, want %d",
			pendingResult.Balance,
			initialBalance,
		)
	}

	/*
		Confirma que PENDING_REFERENCE realmente foi persistido.
	*/
	persistedPending, err :=
		env.transactionRepository.FindByID(
			ctx,
			rollbackID,
		)
	if err != nil {
		t.Fatalf(
			"find persisted pending rollback: %v",
			err,
		)
	}

	if persistedPending.Status() !=
		domaintransaction.StatusPendingReference {

		t.Fatalf(
			"persisted rollback status = %s, want %s",
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
		Nenhum débito/crédito deve ter ocorrido ainda.
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

	if walletBeforeReference.Balance().Amount() != initialBalance {
		t.Fatalf(
			"wallet balance before reference = %d, want %d",
			walletBeforeReference.Balance().Amount(),
			initialBalance,
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
		A BET referenciada chega depois.

		10000 - 2000 = 8000.
	*/
	bet := newBet(
		t,
		betID,
		betExternalID,
		"idempotency-rollback-reference-bet",
		"hash-rollback-reference-bet",
		walletID,
		playerID,
		amount,
	)

	betResult, err := env.service.Process(
		ctx,
		applicationwager.ProcessInput{
			Transaction:   bet,
			LedgerEntryID: "ledger-rollback-reference-bet",
			OccurredAt:    time.Now().UTC(),
		},
	)
	if err != nil {
		t.Fatalf(
			"process rollback referenced bet: %v",
			err,
		)
	}

	if betResult.Transaction == nil {
		t.Fatal("processed bet transaction must not be nil")
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
		Retoma o ROLLBACK.

		Como a referência é uma BET (DEBIT),
		o ROLLBACK deve produzir o movimento contrário: CREDIT.

		8000 + 2000 = 10000.
	*/
	resumedResult, err :=
		env.service.ResumePendingReference(
			ctx,
			rollbackID,
			"ledger-pending-rollback-resolved",
			time.Now().UTC(),
		)
	if err != nil {
		t.Fatalf(
			"resume pending rollback: %v",
			err,
		)
	}

	if resumedResult.Transaction == nil {
		t.Fatal("resumed rollback transaction must not be nil")
	}

	if resumedResult.Replayed {
		t.Fatal(
			"first successful rollback resolution must not be replay",
		)
	}

	if resumedResult.Transaction.Status() !=
		domaintransaction.StatusProcessed {

		t.Fatalf(
			"resolved rollback status = %s, want %s",
			resumedResult.Transaction.Status(),
			domaintransaction.StatusProcessed,
		)
	}

	if resumedResult.Balance != initialBalance {
		t.Fatalf(
			"balance after rollback = %d, want %d",
			resumedResult.Balance,
			initialBalance,
		)
	}

	if resumedResult.Currency != "BRL" {
		t.Fatalf(
			"currency after rollback = %s, want BRL",
			resumedResult.Currency,
		)
	}

	/*
		Confirma resolução persistida.
	*/
	persistedRollback, err :=
		env.transactionRepository.FindByID(
			ctx,
			rollbackID,
		)
	if err != nil {
		t.Fatalf(
			"find resolved rollback: %v",
			err,
		)
	}

	if persistedRollback.Status() !=
		domaintransaction.StatusProcessed {

		t.Fatalf(
			"persisted rollback status = %s, want %s",
			persistedRollback.Status(),
			domaintransaction.StatusProcessed,
		)
	}

	if persistedRollback.ReferenceTransactionID() != betID {
		t.Fatalf(
			"resolved reference transaction id = %q, want %q",
			persistedRollback.ReferenceTransactionID(),
			betID,
		)
	}

	/*
		Estado financeiro final.
	*/
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

	if finalWallet.Balance().Amount() != initialBalance {
		t.Fatalf(
			"final wallet balance = %d, want %d",
			finalWallet.Balance().Amount(),
			initialBalance,
		)
	}

	/*
		Exatamente:

		2 transactions:
		  BET
		  ROLLBACK

		2 ledger entries:
		  BET      -> DEBIT
		  ROLLBACK -> CREDIT
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
		Replay do mesmo ROLLBACK.

		Não pode gerar segundo crédito.
	*/
	replayRollback := newRollback(
		t,
		"transaction-pending-rollback-replay",
		rollbackExternalID,
		"idempotency-pending-rollback",
		"hash-pending-rollback",
		walletID,
		playerID,
		betExternalID,
		amount,
	)

	replayResult, err := env.service.Process(
		ctx,
		applicationwager.ProcessInput{
			Transaction: replayRollback,
			LedgerEntryID: "ledger-pending-rollback-" +
				"replay",
			OccurredAt: time.Now().UTC(),
		},
	)
	if err != nil {
		t.Fatalf(
			"replay resolved rollback: %v",
			err,
		)
	}

	if !replayResult.Replayed {
		t.Fatal("resolved rollback replay must be replay")
	}

	if replayResult.Transaction == nil {
		t.Fatal("replayed rollback transaction must not be nil")
	}

	if replayResult.Transaction.Status() !=
		domaintransaction.StatusProcessed {

		t.Fatalf(
			"replay status = %s, want %s",
			replayResult.Transaction.Status(),
			domaintransaction.StatusProcessed,
		)
	}

	if replayResult.Balance != initialBalance {
		t.Fatalf(
			"replay balance = %d, want %d",
			replayResult.Balance,
			initialBalance,
		)
	}

	walletAfterReplay, err :=
		env.walletRepository.FindByID(
			ctx,
			walletID,
		)
	if err != nil {
		t.Fatalf(
			"find wallet after rollback replay: %v",
			err,
		)
	}

	if walletAfterReplay.Balance().Amount() != initialBalance {
		t.Fatalf(
			"wallet balance after replay = %d, want %d",
			walletAfterReplay.Balance().Amount(),
			initialBalance,
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
