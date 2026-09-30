package ledger

import (
	"strings"
	"time"

	"jungle_gaming_teste_tecnico/internal/domain/money"
)

type Entry struct {
	id            string
	walletID      string
	transactionID string
	direction     Direction
	amount        money.Money
	balanceBefore money.Money
	balanceAfter  money.Money
	createdAt     time.Time
}

func New(
	id string,
	walletID string,
	transactionID string,
	direction Direction,
	amount money.Money,
	balanceBefore money.Money,
	balanceAfter money.Money,
	createdAt time.Time,
) (Entry, error) {
	if err := validate(
		id,
		walletID,
		transactionID,
		direction,
		amount,
		balanceBefore,
		balanceAfter,
		createdAt,
	); err != nil {
		return Entry{}, err
	}

	return Entry{
		id:            strings.TrimSpace(id),
		walletID:      strings.TrimSpace(walletID),
		transactionID: strings.TrimSpace(transactionID),
		direction:     direction,
		amount:        amount,
		balanceBefore: balanceBefore,
		balanceAfter:  balanceAfter,
		createdAt:     createdAt.UTC(),
	}, nil
}

func Rehydrate(
	id string,
	walletID string,
	transactionID string,
	direction Direction,
	amount money.Money,
	balanceBefore money.Money,
	balanceAfter money.Money,
	createdAt time.Time,
) (Entry, error) {
	return New(
		id,
		walletID,
		transactionID,
		direction,
		amount,
		balanceBefore,
		balanceAfter,
		createdAt,
	)
}

func validate(
	id string,
	walletID string,
	transactionID string,
	direction Direction,
	amount money.Money,
	balanceBefore money.Money,
	balanceAfter money.Money,
	createdAt time.Time,
) error {
	if strings.TrimSpace(id) == "" {
		return ErrInvalidID
	}

	if strings.TrimSpace(walletID) == "" {
		return ErrInvalidWalletID
	}

	if strings.TrimSpace(transactionID) == "" {
		return ErrInvalidTransactionID
	}

	if !direction.IsValid() {
		return ErrInvalidDirection
	}

	if amount.Currency() == "" ||
		balanceBefore.Currency() == "" ||
		balanceAfter.Currency() == "" {
		return money.ErrInvalidCurrency
	}

	if amount.Amount() <= 0 {
		return ErrInvalidAmount
	}

	if balanceBefore.Amount() < 0 || balanceAfter.Amount() < 0 {
		return ErrInvalidBalance
	}

	if amount.Currency() != balanceBefore.Currency() ||
		amount.Currency() != balanceAfter.Currency() {
		return money.ErrCurrencyMismatch
	}

	if createdAt.IsZero() {
		return ErrInvalidTimestamp
	}

	return validateBalanceEquation(
		direction,
		amount,
		balanceBefore,
		balanceAfter,
	)
}

func validateBalanceEquation(
	direction Direction,
	amount money.Money,
	balanceBefore money.Money,
	balanceAfter money.Money,
) error {
	var expected money.Money
	var err error

	switch direction {
	case DirectionCredit:
		expected, err = balanceBefore.Add(amount)
	case DirectionDebit:
		expected, err = balanceBefore.Sub(amount)
	default:
		return ErrInvalidDirection
	}

	if err != nil {
		return ErrInvalidBalance
	}

	comparison, err := expected.Compare(balanceAfter)
	if err != nil {
		return err
	}

	if comparison != 0 {
		return ErrInvalidBalance
	}

	return nil
}

func (e Entry) ID() string {
	return e.id
}

func (e Entry) WalletID() string {
	return e.walletID
}

func (e Entry) TransactionID() string {
	return e.transactionID
}

func (e Entry) Direction() Direction {
	return e.direction
}

func (e Entry) Amount() money.Money {
	return e.amount
}

func (e Entry) BalanceBefore() money.Money {
	return e.balanceBefore
}

func (e Entry) BalanceAfter() money.Money {
	return e.balanceAfter
}

func (e Entry) CreatedAt() time.Time {
	return e.createdAt
}
