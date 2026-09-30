package wallet

import (
	"strings"
	"time"

	"jungle_gaming_teste_tecnico/internal/domain/money"
)

const initialVersion uint64 = 1

type Wallet struct {
	id        string
	playerID  string
	balance   money.Money
	version   uint64
	createdAt time.Time
	updatedAt time.Time
}

func New(
	id string,
	playerID string,
	initialBalance money.Money,
	createdAt time.Time,
) (Wallet, error) {
	if strings.TrimSpace(id) == "" {
		return Wallet{}, ErrInvalidID
	}

	if strings.TrimSpace(playerID) == "" {
		return Wallet{}, ErrInvalidPlayerID
	}

	if initialBalance.Currency() == "" {
		return Wallet{}, money.ErrInvalidCurrency
	}

	if initialBalance.Amount() < 0 {
		return Wallet{}, ErrInsufficientBalance
	}

	if createdAt.IsZero() {
		return Wallet{}, ErrInvalidTimestamp
	}

	createdAt = createdAt.UTC()

	return Wallet{
		id:        strings.TrimSpace(id),
		playerID:  strings.TrimSpace(playerID),
		balance:   initialBalance,
		version:   initialVersion,
		createdAt: createdAt,
		updatedAt: createdAt,
	}, nil
}

func Rehydrate(
	id string,
	playerID string,
	balance money.Money,
	version uint64,
	createdAt time.Time,
	updatedAt time.Time,
) (Wallet, error) {
	if strings.TrimSpace(id) == "" {
		return Wallet{}, ErrInvalidID
	}

	if strings.TrimSpace(playerID) == "" {
		return Wallet{}, ErrInvalidPlayerID
	}

	if balance.Currency() == "" {
		return Wallet{}, money.ErrInvalidCurrency
	}

	if balance.Amount() < 0 {
		return Wallet{}, ErrInsufficientBalance
	}

	if version < initialVersion {
		return Wallet{}, ErrInvalidVersion
	}

	if createdAt.IsZero() || updatedAt.IsZero() {
		return Wallet{}, ErrInvalidTimestamp
	}

	createdAt = createdAt.UTC()
	updatedAt = updatedAt.UTC()

	if updatedAt.Before(createdAt) {
		return Wallet{}, ErrInvalidTimestamp
	}

	return Wallet{
		id:        strings.TrimSpace(id),
		playerID:  strings.TrimSpace(playerID),
		balance:   balance,
		version:   version,
		createdAt: createdAt,
		updatedAt: updatedAt,
	}, nil
}

func (w *Wallet) Credit(amount money.Money, occurredAt time.Time) error {
	if amount.Currency() != w.balance.Currency() {
		return ErrCurrencyMismatch
	}

	if amount.Amount() < 0 {
		return ErrInvalidAmount
	}

	if amount.IsZero() {
		return nil
	}

	if occurredAt.IsZero() {
		return ErrInvalidTimestamp
	}

	newBalance, err := w.balance.Add(amount)
	if err != nil {
		return err
	}

	w.balance = newBalance
	w.version++
	w.updatedAt = occurredAt.UTC()

	return nil
}

func (w *Wallet) Debit(amount money.Money, occurredAt time.Time) error {
	if amount.Currency() != w.balance.Currency() {
		return ErrCurrencyMismatch
	}

	if amount.Amount() < 0 {
		return ErrInvalidAmount
	}

	if amount.IsZero() {
		return nil
	}

	if occurredAt.IsZero() {
		return ErrInvalidTimestamp
	}

	comparison, err := w.balance.Compare(amount)
	if err != nil {
		return err
	}

	if comparison < 0 {
		return ErrInsufficientBalance
	}

	newBalance, err := w.balance.Sub(amount)
	if err != nil {
		return err
	}

	w.balance = newBalance
	w.version++
	w.updatedAt = occurredAt.UTC()

	return nil
}

func (w Wallet) ID() string {
	return w.id
}

func (w Wallet) PlayerID() string {
	return w.playerID
}

func (w Wallet) Balance() money.Money {
	return w.balance
}

func (w Wallet) Currency() string {
	return w.balance.Currency()
}

func (w Wallet) Version() uint64 {
	return w.version
}

func (w Wallet) CreatedAt() time.Time {
	return w.createdAt
}

func (w Wallet) UpdatedAt() time.Time {
	return w.updatedAt
}
