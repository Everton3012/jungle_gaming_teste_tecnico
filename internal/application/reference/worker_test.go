package reference

import (
	"context"
	"errors"
	"testing"
	"time"

	applicationwager "jungle_gaming_teste_tecnico/internal/application/wager"
	domainmoney "jungle_gaming_teste_tecnico/internal/domain/money"
	domaintransaction "jungle_gaming_teste_tecnico/internal/domain/wagertransaction"
)

type fakeRepository struct {
	transactions []domaintransaction.WagerTransaction
	err          error
	calls        int
	lastLimit    int
}

func (f *fakeRepository) FindPendingReferences(
	ctx context.Context,
	limit int,
) ([]domaintransaction.WagerTransaction, error) {
	f.calls++
	f.lastLimit = limit

	if f.err != nil {
		return nil, f.err
	}

	result := make(
		[]domaintransaction.WagerTransaction,
		len(f.transactions),
	)

	copy(result, f.transactions)

	return result, nil
}

type resumeCall struct {
	transactionID string
	ledgerEntryID string
	occurredAt    time.Time
}

type fakeService struct {
	calls   []resumeCall
	results map[string]applicationwager.ProcessResult
	errors  map[string]error
}

func (f *fakeService) ResumePendingReference(
	ctx context.Context,
	transactionID string,
	ledgerEntryID string,
	occurredAt time.Time,
) (applicationwager.ProcessResult, error) {
	f.calls = append(
		f.calls,
		resumeCall{
			transactionID: transactionID,
			ledgerEntryID: ledgerEntryID,
			occurredAt:    occurredAt,
		},
	)

	if err, ok := f.errors[transactionID]; ok {
		return applicationwager.ProcessResult{}, err
	}

	if result, ok := f.results[transactionID]; ok {
		return result, nil
	}

	return applicationwager.ProcessResult{}, nil
}

func newPendingTransaction(
	t *testing.T,
	id string,
	referenceExternalID string,
) domaintransaction.WagerTransaction {
	t.Helper()

	value, err := domainmoney.New(
		1000,
		"BRL",
	)
	if err != nil {
		t.Fatalf("create money: %v", err)
	}

	now := time.Date(
		2026,
		time.September,
		30,
		12,
		0,
		0,
		0,
		time.UTC,
	)

	transaction, err := domaintransaction.NewExternal(
		domaintransaction.ExternalInput{
			ID:                             id,
			ExternalTransactionID:          "external-" + id,
			ProviderID:                     "provider-test",
			IdempotencyKey:                 "idempotency-" + id,
			PayloadHash:                    "hash-" + id,
			WalletID:                       "wallet-test",
			PlayerID:                       "player-test",
			RoundID:                        "round-test",
			GameID:                         "game-test",
			Kind:                           domaintransaction.KindRefund,
			Money:                          value,
			ReferenceExternalTransactionID: referenceExternalID,
			CreatedAt:                      now,
		},
	)
	if err != nil {
		t.Fatalf(
			"create transaction: %v",
			err,
		)
	}

	if err := transaction.MarkPendingReference(
		now.Add(time.Second),
	); err != nil {
		t.Fatalf(
			"mark pending reference: %v",
			err,
		)
	}

	return transaction
}

func validConfig() Config {
	return Config{
		BatchSize:    10,
		PollInterval: time.Second,
		RetryBackoff: 100 * time.Millisecond,
	}
}

func TestNewWorkerRejectsNilRepository(
	t *testing.T,
) {
	service := &fakeService{}

	_, err := NewWorker(
		nil,
		service,
		validConfig(),
	)

	if !errors.Is(
		err,
		ErrPendingRepositoryRequired,
	) {
		t.Fatalf(
			"expected ErrPendingRepositoryRequired, got %v",
			err,
		)
	}
}

func TestNewWorkerRejectsNilService(
	t *testing.T,
) {
	repository := &fakeRepository{}

	_, err := NewWorker(
		repository,
		nil,
		validConfig(),
	)

	if !errors.Is(
		err,
		ErrWagerServiceRequired,
	) {
		t.Fatalf(
			"expected ErrWagerServiceRequired, got %v",
			err,
		)
	}
}

func TestNewWorkerRejectsInvalidBatchSize(
	t *testing.T,
) {
	repository := &fakeRepository{}
	service := &fakeService{}

	config := validConfig()
	config.BatchSize = 0

	_, err := NewWorker(
		repository,
		service,
		config,
	)

	if !errors.Is(err, ErrInvalidBatchSize) {
		t.Fatalf(
			"expected ErrInvalidBatchSize, got %v",
			err,
		)
	}
}

func TestNewWorkerRejectsInvalidPollInterval(
	t *testing.T,
) {
	repository := &fakeRepository{}
	service := &fakeService{}

	config := validConfig()
	config.PollInterval = 0

	_, err := NewWorker(
		repository,
		service,
		config,
	)

	if !errors.Is(err, ErrInvalidPollInterval) {
		t.Fatalf(
			"expected ErrInvalidPollInterval, got %v",
			err,
		)
	}
}

func TestNewWorkerRejectsInvalidRetryBackoff(
	t *testing.T,
) {
	repository := &fakeRepository{}
	service := &fakeService{}

	config := validConfig()
	config.RetryBackoff = 0

	_, err := NewWorker(
		repository,
		service,
		config,
	)

	if !errors.Is(err, ErrInvalidRetryBackoff) {
		t.Fatalf(
			"expected ErrInvalidRetryBackoff, got %v",
			err,
		)
	}
}

func TestWorkerProcessesPendingReferences(
	t *testing.T,
) {
	first := newPendingTransaction(
		t,
		"transaction-1",
		"reference-1",
	)

	second := newPendingTransaction(
		t,
		"transaction-2",
		"reference-2",
	)

	repository := &fakeRepository{
		transactions: []domaintransaction.WagerTransaction{
			first,
			second,
		},
	}

	service := &fakeService{
		results: make(
			map[string]applicationwager.ProcessResult,
		),
		errors: make(map[string]error),
	}

	worker, err := NewWorker(
		repository,
		service,
		validConfig(),
	)
	if err != nil {
		t.Fatalf("create worker: %v", err)
	}

	fixedTime := time.Date(
		2026,
		time.September,
		30,
		15,
		0,
		0,
		0,
		time.UTC,
	)

	worker.now = func() time.Time {
		return fixedTime
	}

	processed, err := worker.ProcessBatch(
		context.Background(),
	)
	if err != nil {
		t.Fatalf(
			"process batch: %v",
			err,
		)
	}

	if processed != 2 {
		t.Fatalf(
			"processed = %d, want 2",
			processed,
		)
	}

	if repository.calls != 1 {
		t.Fatalf(
			"repository calls = %d, want 1",
			repository.calls,
		)
	}

	if repository.lastLimit != 10 {
		t.Fatalf(
			"repository limit = %d, want 10",
			repository.lastLimit,
		)
	}

	if len(service.calls) != 2 {
		t.Fatalf(
			"service calls = %d, want 2",
			len(service.calls),
		)
	}

	if service.calls[0].transactionID !=
		"transaction-1" {
		t.Fatalf(
			"first transaction = %q",
			service.calls[0].transactionID,
		)
	}

	if service.calls[0].ledgerEntryID !=
		"reference-transaction-1" {
		t.Fatalf(
			"first ledger ID = %q",
			service.calls[0].ledgerEntryID,
		)
	}

	if !service.calls[0].occurredAt.Equal(
		fixedTime,
	) {
		t.Fatalf(
			"occurredAt = %v, want %v",
			service.calls[0].occurredAt,
			fixedTime,
		)
	}
}

func TestWorkerKeepsMissingReferencePending(
	t *testing.T,
) {
	transaction := newPendingTransaction(
		t,
		"transaction-missing",
		"reference-missing",
	)

	repository := &fakeRepository{
		transactions: []domaintransaction.WagerTransaction{
			transaction,
		},
	}

	service := &fakeService{
		results: make(
			map[string]applicationwager.ProcessResult,
		),
		errors: map[string]error{
			"transaction-missing": applicationwager.ErrReferenceRequired,
		},
	}

	worker, err := NewWorker(
		repository,
		service,
		validConfig(),
	)
	if err != nil {
		t.Fatalf("create worker: %v", err)
	}

	processed, err := worker.ProcessBatch(
		context.Background(),
	)
	if err != nil {
		t.Fatalf(
			"process batch: %v",
			err,
		)
	}

	if processed != 0 {
		t.Fatalf(
			"processed = %d, want 0",
			processed,
		)
	}

	if len(service.calls) != 1 {
		t.Fatalf(
			"service calls = %d, want 1",
			len(service.calls),
		)
	}
}

func TestWorkerIgnoresAlreadyResolvedTransaction(
	t *testing.T,
) {
	transaction := newPendingTransaction(
		t,
		"transaction-resolved",
		"reference-resolved",
	)

	repository := &fakeRepository{
		transactions: []domaintransaction.WagerTransaction{
			transaction,
		},
	}

	service := &fakeService{
		results: make(
			map[string]applicationwager.ProcessResult,
		),
		errors: map[string]error{
			"transaction-resolved": applicationwager.ErrNotPendingReference,
		},
	}

	worker, err := NewWorker(
		repository,
		service,
		validConfig(),
	)
	if err != nil {
		t.Fatalf("create worker: %v", err)
	}

	processed, err := worker.ProcessBatch(
		context.Background(),
	)
	if err != nil {
		t.Fatalf(
			"process batch: %v",
			err,
		)
	}

	if processed != 0 {
		t.Fatalf(
			"processed = %d, want 0",
			processed,
		)
	}
}

func TestWorkerReturnsUnexpectedServiceError(
	t *testing.T,
) {
	transaction := newPendingTransaction(
		t,
		"transaction-error",
		"reference-error",
	)

	expectedErr := errors.New(
		"database unavailable",
	)

	repository := &fakeRepository{
		transactions: []domaintransaction.WagerTransaction{
			transaction,
		},
	}

	service := &fakeService{
		results: make(
			map[string]applicationwager.ProcessResult,
		),
		errors: map[string]error{
			"transaction-error": expectedErr,
		},
	}

	worker, err := NewWorker(
		repository,
		service,
		validConfig(),
	)
	if err != nil {
		t.Fatalf("create worker: %v", err)
	}

	processed, err := worker.ProcessBatch(
		context.Background(),
	)

	if processed != 0 {
		t.Fatalf(
			"processed = %d, want 0",
			processed,
		)
	}

	if !errors.Is(err, expectedErr) {
		t.Fatalf(
			"expected service error, got %v",
			err,
		)
	}
}

func TestWorkerReturnsRepositoryError(
	t *testing.T,
) {
	expectedErr := errors.New(
		"repository unavailable",
	)

	repository := &fakeRepository{
		err: expectedErr,
	}

	service := &fakeService{}

	worker, err := NewWorker(
		repository,
		service,
		validConfig(),
	)
	if err != nil {
		t.Fatalf("create worker: %v", err)
	}

	processed, err := worker.ProcessBatch(
		context.Background(),
	)

	if processed != 0 {
		t.Fatalf(
			"processed = %d, want 0",
			processed,
		)
	}

	if !errors.Is(err, expectedErr) {
		t.Fatalf(
			"expected repository error, got %v",
			err,
		)
	}
}

func TestWorkerStopsWhenContextIsCancelled(
	t *testing.T,
) {
	repository := &fakeRepository{}
	service := &fakeService{}

	config := validConfig()
	config.PollInterval = time.Hour

	worker, err := NewWorker(
		repository,
		service,
		config,
	)
	if err != nil {
		t.Fatalf("create worker: %v", err)
	}

	ctx, cancel := context.WithCancel(
		context.Background(),
	)

	done := make(chan error, 1)

	go func() {
		done <- worker.Run(ctx)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf(
				"worker returned error: %v",
				err,
			)
		}

	case <-time.After(time.Second):
		t.Fatal(
			"worker did not stop after context cancellation",
		)
	}
}
