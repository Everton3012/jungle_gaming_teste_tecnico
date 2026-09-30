package wager_test

import (
	"context"
	"sync"
	"testing"
	"time"

	applicationwager "jungle_gaming_teste_tecnico/internal/application/wager"
)

func TestServiceConcurrentRefundsOnlyOneReversesBet(t *testing.T) {
	env := newTestEnvironment(t)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	const (
		walletID = "wallet-concurrent-refunds"
		playerID = "player-concurrent-refunds"

		betID         = "transaction-concurrent-refunds-bet"
		betExternalID = "external-concurrent-refunds-bet"

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

	bet := newBet(
		t,
		betID,
		betExternalID,
		"idempotency-concurrent-refunds-bet",
		"hash-concurrent-refunds-bet",
		walletID,
		playerID,
		amount,
	)

	betResult, err := env.service.Process(
		ctx,
		applicationwager.ProcessInput{
			Transaction:   bet,
			LedgerEntryID: "ledger-concurrent-refunds-bet",
			OccurredAt:    time.Now().UTC(),
		},
	)
	if err != nil {
		t.Fatalf("process bet: %v", err)
	}

	if betResult.Balance != 8000 {
		t.Fatalf(
			"balance after bet = %d, want 8000",
			betResult.Balance,
		)
	}

	refundA := newRefund(
		t,
		"transaction-concurrent-refund-a",
		"external-concurrent-refund-a",
		"idempotency-concurrent-refund-a",
		"hash-concurrent-refund-a",
		walletID,
		playerID,
		betExternalID,
		amount,
	)

	refundB := newRefund(
		t,
		"transaction-concurrent-refund-b",
		"external-concurrent-refund-b",
		"idempotency-concurrent-refund-b",
		"hash-concurrent-refund-b",
		walletID,
		playerID,
		betExternalID,
		amount,
	)

	type execution struct {
		result applicationwager.ProcessResult
		err    error
	}

	start := make(chan struct{})
	results := make(chan execution, 2)

	var wg sync.WaitGroup
	wg.Add(2)

	run := func(transactionIndex int) {
		defer wg.Done()

		<-start

		var result applicationwager.ProcessResult
		var err error

		switch transactionIndex {
		case 0:
			result, err = env.service.Process(
				ctx,
				applicationwager.ProcessInput{
					Transaction: refundA,
					LedgerEntryID: "ledger-concurrent-" +
						"refund-a",
					OccurredAt: time.Now().UTC(),
				},
			)

		case 1:
			result, err = env.service.Process(
				ctx,
				applicationwager.ProcessInput{
					Transaction: refundB,
					LedgerEntryID: "ledger-concurrent-" +
						"refund-b",
					OccurredAt: time.Now().UTC(),
				},
			)
		}

		results <- execution{
			result: result,
			err:    err,
		}
	}

	go run(0)
	go run(1)

	close(start)

	wg.Wait()
	close(results)

	successes := 0
	failures := 0

	for execution := range results {
		if execution.err != nil {
			failures++
			continue
		}

		successes++
	}

	if successes != 1 {
		t.Fatalf(
			"successful refunds = %d, want 1",
			successes,
		)
	}

	if failures != 1 {
		t.Fatalf(
			"failed refunds = %d, want 1",
			failures,
		)
	}

	walletEntity, err := env.walletRepository.FindByID(
		ctx,
		walletID,
	)
	if err != nil {
		t.Fatalf("find wallet: %v", err)
	}

	if walletEntity.Balance().Amount() != initialBalance {
		t.Fatalf(
			"final balance = %d, want %d",
			walletEntity.Balance().Amount(),
			initialBalance,
		)
	}

	if got := countRows(
		t,
		env,
		"wager_transactions",
	); got != 2 {
		t.Fatalf(
			"wager transaction count = %d, want 2",
			got,
		)
	}

	if got := countRows(
		t,
		env,
		"ledger_entries",
	); got != 2 {
		t.Fatalf(
			"ledger entry count = %d, want 2",
			got,
		)
	}
}

func TestServiceConcurrentRefundAndRollbackOnlyOneReversesBet(
	t *testing.T,
) {
	env := newTestEnvironment(t)

	ctx, cancel := context.WithTimeout(
		context.Background(),
		10*time.Second,
	)
	defer cancel()

	const (
		walletID = "wallet-refund-rollback-race"
		playerID = "player-refund-rollback-race"

		betID         = "transaction-refund-rollback-bet"
		betExternalID = "external-refund-rollback-bet"

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

	bet := newBet(
		t,
		betID,
		betExternalID,
		"idempotency-refund-rollback-bet",
		"hash-refund-rollback-bet",
		walletID,
		playerID,
		amount,
	)

	betResult, err := env.service.Process(
		ctx,
		applicationwager.ProcessInput{
			Transaction:   bet,
			LedgerEntryID: "ledger-refund-rollback-bet",
			OccurredAt:    time.Now().UTC(),
		},
	)
	if err != nil {
		t.Fatalf("process bet: %v", err)
	}

	if betResult.Balance != 8000 {
		t.Fatalf(
			"balance after bet = %d, want 8000",
			betResult.Balance,
		)
	}

	refund := newRefund(
		t,
		"transaction-refund-rollback-refund",
		"external-refund-rollback-refund",
		"idempotency-refund-rollback-refund",
		"hash-refund-rollback-refund",
		walletID,
		playerID,
		betExternalID,
		amount,
	)

	rollback := newRollback(
		t,
		"transaction-refund-rollback-rollback",
		"external-refund-rollback-rollback",
		"idempotency-refund-rollback-rollback",
		"hash-refund-rollback-rollback",
		walletID,
		playerID,
		betExternalID,
		amount,
	)

	type execution struct {
		result applicationwager.ProcessResult
		err    error
	}

	start := make(chan struct{})
	results := make(chan execution, 2)

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()

		<-start

		result, err := env.service.Process(
			ctx,
			applicationwager.ProcessInput{
				Transaction: refund,
				LedgerEntryID: "ledger-refund-" +
					"rollback-refund",
				OccurredAt: time.Now().UTC(),
			},
		)

		results <- execution{
			result: result,
			err:    err,
		}
	}()

	go func() {
		defer wg.Done()

		<-start

		result, err := env.service.Process(
			ctx,
			applicationwager.ProcessInput{
				Transaction: rollback,
				LedgerEntryID: "ledger-refund-" +
					"rollback-rollback",
				OccurredAt: time.Now().UTC(),
			},
		)

		results <- execution{
			result: result,
			err:    err,
		}
	}()

	close(start)

	wg.Wait()
	close(results)

	successes := 0
	failures := 0

	for execution := range results {
		if execution.err != nil {
			failures++
			continue
		}

		successes++
	}

	if successes != 1 {
		t.Fatalf(
			"successful reversals = %d, want 1",
			successes,
		)
	}

	if failures != 1 {
		t.Fatalf(
			"failed reversals = %d, want 1",
			failures,
		)
	}

	walletEntity, err := env.walletRepository.FindByID(
		ctx,
		walletID,
	)
	if err != nil {
		t.Fatalf("find wallet: %v", err)
	}

	if walletEntity.Balance().Amount() != initialBalance {
		t.Fatalf(
			"final balance = %d, want %d",
			walletEntity.Balance().Amount(),
			initialBalance,
		)
	}

	if got := countRows(
		t,
		env,
		"wager_transactions",
	); got != 2 {
		t.Fatalf(
			"wager transaction count = %d, want 2",
			got,
		)
	}

	if got := countRows(
		t,
		env,
		"ledger_entries",
	); got != 2 {
		t.Fatalf(
			"ledger entry count = %d, want 2",
			got,
		)
	}
}
