package wagertransaction

import (
	"strings"
	"time"

	"jungle_gaming_teste_tecnico/internal/domain/money"
)

type ExternalInput struct {
	ID                             string
	ExternalTransactionID          string
	ProviderID                     string
	IdempotencyKey                 string
	PayloadHash                    string
	WalletID                       string
	PlayerID                       string
	RoundID                        string
	GameID                         string
	Kind                           Kind
	Money                          money.Money
	ReferenceExternalTransactionID string
	CreatedAt                      time.Time
}

type RehydrateInput struct {
	ID                             string
	ExternalTransactionID          string
	ProviderID                     string
	IdempotencyKey                 string
	PayloadHash                    string
	WalletID                       string
	PlayerID                       string
	RoundID                        string
	GameID                         string
	Kind                           Kind
	Money                          money.Money
	ReferenceExternalTransactionID string
	ReferenceTransactionID         string
	Status                         Status
	FailureCode                    FailureCode
	ResultBalance                  *money.Money
	CreatedAt                      time.Time
	UpdatedAt                      time.Time
}

type WagerTransaction struct {
	id                             string
	externalTransactionID          string
	providerID                     string
	idempotencyKey                 string
	payloadHash                    string
	walletID                       string
	playerID                       string
	roundID                        string
	gameID                         string
	kind                           Kind
	money                          money.Money
	referenceExternalTransactionID string
	referenceTransactionID         string
	status                         Status
	failureCode                    FailureCode
	resultBalance                  *money.Money
	createdAt                      time.Time
	updatedAt                      time.Time
}

type OpeningInput struct {
	ID        string
	WalletID  string
	PlayerID  string
	Money     money.Money
	CreatedAt time.Time
}

func NewOpening(input OpeningInput) (WagerTransaction, error) {
	if strings.TrimSpace(input.ID) == "" {
		return WagerTransaction{}, ErrInvalidID
	}

	if strings.TrimSpace(input.WalletID) == "" {
		return WagerTransaction{}, ErrInvalidWalletID
	}

	if strings.TrimSpace(input.PlayerID) == "" {
		return WagerTransaction{}, ErrInvalidPlayerID
	}

	if input.Money.Currency() == "" {
		return WagerTransaction{}, money.ErrInvalidCurrency
	}

	if !input.Money.IsPositive() {
		return WagerTransaction{}, ErrInvalidOpeningAmount
	}

	if input.CreatedAt.IsZero() {
		return WagerTransaction{}, ErrInvalidTimestamp
	}

	createdAt := input.CreatedAt.UTC()
	resultBalance := input.Money

	return WagerTransaction{
		id:            strings.TrimSpace(input.ID),
		walletID:      strings.TrimSpace(input.WalletID),
		playerID:      strings.TrimSpace(input.PlayerID),
		kind:          KindOpening,
		money:         input.Money,
		status:        StatusProcessed,
		resultBalance: &resultBalance,
		createdAt:     createdAt,
		updatedAt:     createdAt,
	}, nil
}

func NewExternal(input ExternalInput) (WagerTransaction, error) {
	if err := validateExternalInput(input); err != nil {
		return WagerTransaction{}, err
	}

	createdAt := input.CreatedAt.UTC()

	return WagerTransaction{
		id:                             strings.TrimSpace(input.ID),
		externalTransactionID:          strings.TrimSpace(input.ExternalTransactionID),
		providerID:                     strings.TrimSpace(input.ProviderID),
		idempotencyKey:                 strings.TrimSpace(input.IdempotencyKey),
		payloadHash:                    strings.TrimSpace(input.PayloadHash),
		walletID:                       strings.TrimSpace(input.WalletID),
		playerID:                       strings.TrimSpace(input.PlayerID),
		roundID:                        strings.TrimSpace(input.RoundID),
		gameID:                         strings.TrimSpace(input.GameID),
		kind:                           input.Kind,
		money:                          input.Money,
		referenceExternalTransactionID: strings.TrimSpace(input.ReferenceExternalTransactionID),
		status:                         StatusPending,
		createdAt:                      createdAt,
		updatedAt:                      createdAt,
	}, nil
}

func Rehydrate(input RehydrateInput) (WagerTransaction, error) {
	if err := validateRehydrateInput(input); err != nil {
		return WagerTransaction{}, err
	}

	resultBalance := cloneMoney(input.ResultBalance)

	return WagerTransaction{
		id:                             strings.TrimSpace(input.ID),
		externalTransactionID:          strings.TrimSpace(input.ExternalTransactionID),
		providerID:                     strings.TrimSpace(input.ProviderID),
		idempotencyKey:                 strings.TrimSpace(input.IdempotencyKey),
		payloadHash:                    strings.TrimSpace(input.PayloadHash),
		walletID:                       strings.TrimSpace(input.WalletID),
		playerID:                       strings.TrimSpace(input.PlayerID),
		roundID:                        strings.TrimSpace(input.RoundID),
		gameID:                         strings.TrimSpace(input.GameID),
		kind:                           input.Kind,
		money:                          input.Money,
		referenceExternalTransactionID: strings.TrimSpace(input.ReferenceExternalTransactionID),
		referenceTransactionID:         strings.TrimSpace(input.ReferenceTransactionID),
		status:                         input.Status,
		failureCode:                    input.FailureCode,
		resultBalance:                  resultBalance,
		createdAt:                      input.CreatedAt.UTC(),
		updatedAt:                      input.UpdatedAt.UTC(),
	}, nil
}

func (t *WagerTransaction) MarkPendingReference(updatedAt time.Time) error {
	if err := t.prepareTransition(updatedAt); err != nil {
		return err
	}

	if t.status != StatusPending {
		return ErrInvalidTransition
	}

	t.status = StatusPendingReference
	t.updatedAt = updatedAt.UTC()

	return nil
}

func (t *WagerTransaction) ResolveReference(
	referenceTransactionID string,
	updatedAt time.Time,
) error {
	if err := t.prepareTransition(updatedAt); err != nil {
		return err
	}

	if t.status != StatusPendingReference {
		return ErrInvalidTransition
	}

	referenceTransactionID = strings.TrimSpace(referenceTransactionID)
	if referenceTransactionID == "" {
		return ErrInvalidID
	}

	t.referenceTransactionID = referenceTransactionID
	t.status = StatusPending
	t.updatedAt = updatedAt.UTC()

	return nil
}

func (t *WagerTransaction) MarkProcessed(
	resultBalance money.Money,
	updatedAt time.Time,
) error {
	if err := t.prepareTransition(updatedAt); err != nil {
		return err
	}

	if t.status != StatusPending {
		return ErrInvalidTransition
	}

	if resultBalance.Currency() != t.money.Currency() {
		return money.ErrCurrencyMismatch
	}

	t.status = StatusProcessed
	t.failureCode = ""
	t.resultBalance = &resultBalance
	t.updatedAt = updatedAt.UTC()

	return nil
}

func (t *WagerTransaction) MarkRejected(
	failureCode FailureCode,
	resultBalance money.Money,
	updatedAt time.Time,
) error {
	if err := t.prepareTransition(updatedAt); err != nil {
		return err
	}

	if t.status != StatusPending && t.status != StatusPendingReference {
		return ErrInvalidTransition
	}

	if strings.TrimSpace(failureCode.String()) == "" {
		return ErrInvalidFailureCode
	}

	if resultBalance.Currency() != t.money.Currency() {
		return money.ErrCurrencyMismatch
	}

	t.status = StatusRejected
	t.failureCode = failureCode
	t.resultBalance = &resultBalance
	t.updatedAt = updatedAt.UTC()

	return nil
}

func (t *WagerTransaction) MarkFailed(
	failureCode FailureCode,
	updatedAt time.Time,
) error {
	if err := t.prepareTransition(updatedAt); err != nil {
		return err
	}

	if t.status != StatusPending && t.status != StatusPendingReference {
		return ErrInvalidTransition
	}

	if strings.TrimSpace(failureCode.String()) == "" {
		return ErrInvalidFailureCode
	}

	t.status = StatusFailed
	t.failureCode = failureCode
	t.updatedAt = updatedAt.UTC()

	return nil
}

func (t WagerTransaction) prepareTransition(updatedAt time.Time) error {
	if t.status.IsTerminal() {
		return ErrTerminalTransaction
	}

	if updatedAt.IsZero() {
		return ErrInvalidTimestamp
	}

	return nil
}

func validateExternalInput(input ExternalInput) error {
	if strings.TrimSpace(input.ID) == "" {
		return ErrInvalidID
	}

	if strings.TrimSpace(input.ExternalTransactionID) == "" {
		return ErrInvalidExternalTransactionID
	}

	if strings.TrimSpace(input.ProviderID) == "" {
		return ErrInvalidProviderID
	}

	if strings.TrimSpace(input.IdempotencyKey) == "" {
		return ErrInvalidIdempotencyKey
	}

	if strings.TrimSpace(input.PayloadHash) == "" {
		return ErrInvalidPayloadHash
	}

	if strings.TrimSpace(input.WalletID) == "" {
		return ErrInvalidWalletID
	}

	if strings.TrimSpace(input.PlayerID) == "" {
		return ErrInvalidPlayerID
	}

	if strings.TrimSpace(input.RoundID) == "" {
		return ErrInvalidRoundID
	}

	if strings.TrimSpace(input.GameID) == "" {
		return ErrInvalidGameID
	}

	if !input.Kind.IsValid() {
		return ErrInvalidKind
	}

	if input.Kind == KindOpening {
		return ErrOpeningExternal
	}

	if !input.Kind.IsExternal() {
		return ErrExternalKindRequired
	}

	if input.Money.Currency() == "" {
		return money.ErrInvalidCurrency
	}

	if input.CreatedAt.IsZero() {
		return ErrInvalidTimestamp
	}

	if input.Kind.RequiresReference() &&
		strings.TrimSpace(input.ReferenceExternalTransactionID) == "" {
		return ErrInvalidExternalTransactionID
	}

	return nil
}

func validateRehydrateInput(input RehydrateInput) error {
	if strings.TrimSpace(input.ID) == "" {
		return ErrInvalidID
	}

	if !input.Kind.IsValid() {
		return ErrInvalidKind
	}

	if !input.Status.IsValid() {
		return ErrInvalidStatus
	}

	if strings.TrimSpace(input.WalletID) == "" {
		return ErrInvalidWalletID
	}

	if strings.TrimSpace(input.PlayerID) == "" {
		return ErrInvalidPlayerID
	}

	if input.Money.Currency() == "" {
		return money.ErrInvalidCurrency
	}

	if input.CreatedAt.IsZero() || input.UpdatedAt.IsZero() {
		return ErrInvalidTimestamp
	}

	if input.UpdatedAt.Before(input.CreatedAt) {
		return ErrInvalidTimestamp
	}

	if input.Kind.IsExternal() {
		if err := validateExternalRehydration(input); err != nil {
			return err
		}
	}

	if input.Status == StatusRejected || input.Status == StatusFailed {
		if strings.TrimSpace(input.FailureCode.String()) == "" {
			return ErrInvalidFailureCode
		}
	}

	if input.Status == StatusProcessed && input.ResultBalance == nil {
		return ErrInvalidStatus
	}

	if input.ResultBalance != nil &&
		input.ResultBalance.Currency() != input.Money.Currency() {
		return money.ErrCurrencyMismatch
	}

	return nil
}

func validateExternalRehydration(input RehydrateInput) error {
	if strings.TrimSpace(input.ExternalTransactionID) == "" {
		return ErrInvalidExternalTransactionID
	}

	if strings.TrimSpace(input.ProviderID) == "" {
		return ErrInvalidProviderID
	}

	if strings.TrimSpace(input.IdempotencyKey) == "" {
		return ErrInvalidIdempotencyKey
	}

	if strings.TrimSpace(input.PayloadHash) == "" {
		return ErrInvalidPayloadHash
	}

	if strings.TrimSpace(input.RoundID) == "" {
		return ErrInvalidRoundID
	}

	if strings.TrimSpace(input.GameID) == "" {
		return ErrInvalidGameID
	}

	if input.Kind.RequiresReference() &&
		strings.TrimSpace(input.ReferenceExternalTransactionID) == "" {
		return ErrInvalidExternalTransactionID
	}

	return nil
}

func cloneMoney(value *money.Money) *money.Money {
	if value == nil {
		return nil
	}

	copy := *value
	return &copy
}

func (t WagerTransaction) ID() string {
	return t.id
}

func (t WagerTransaction) ExternalTransactionID() string {
	return t.externalTransactionID
}

func (t WagerTransaction) ProviderID() string {
	return t.providerID
}

func (t WagerTransaction) IdempotencyKey() string {
	return t.idempotencyKey
}

func (t WagerTransaction) PayloadHash() string {
	return t.payloadHash
}

func (t WagerTransaction) WalletID() string {
	return t.walletID
}

func (t WagerTransaction) PlayerID() string {
	return t.playerID
}

func (t WagerTransaction) RoundID() string {
	return t.roundID
}

func (t WagerTransaction) GameID() string {
	return t.gameID
}

func (t WagerTransaction) Kind() Kind {
	return t.kind
}

func (t WagerTransaction) Money() money.Money {
	return t.money
}

func (t WagerTransaction) ReferenceExternalTransactionID() string {
	return t.referenceExternalTransactionID
}

func (t WagerTransaction) ReferenceTransactionID() string {
	return t.referenceTransactionID
}

func (t WagerTransaction) Status() Status {
	return t.status
}

func (t WagerTransaction) FailureCode() FailureCode {
	return t.failureCode
}

func (t WagerTransaction) ResultBalance() (money.Money, bool) {
	if t.resultBalance == nil {
		return money.Money{}, false
	}

	return *t.resultBalance, true
}

func (t WagerTransaction) CreatedAt() time.Time {
	return t.createdAt
}

func (t WagerTransaction) UpdatedAt() time.Time {
	return t.updatedAt
}
