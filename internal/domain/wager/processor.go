package wager

import (
	"strings"
	"time"

	"jungle_gaming_teste_tecnico/internal/domain/ledger"
	"jungle_gaming_teste_tecnico/internal/domain/money"
	"jungle_gaming_teste_tecnico/internal/domain/wagertransaction"
	"jungle_gaming_teste_tecnico/internal/domain/wallet"
)

type Processor struct{}

func NewProcessor() Processor {
	return Processor{}
}

func (Processor) Process(
	walletValue *wallet.Wallet,
	transaction *wagertransaction.WagerTransaction,
	reference *wagertransaction.WagerTransaction,
	ledgerEntryID string,
	occurredAt time.Time,
) (Result, error) {
	if err := validateWalletTransaction(walletValue, transaction); err != nil {
		return Result{}, err
	}

	workingWallet := *walletValue
	workingTransaction := *transaction

	result, err := process(
		&workingWallet,
		&workingTransaction,
		reference,
		ledgerEntryID,
		occurredAt,
	)
	if err != nil {
		return Result{}, err
	}

	*walletValue = workingWallet
	*transaction = workingTransaction

	return result, nil
}

func process(
	walletValue *wallet.Wallet,
	transaction *wagertransaction.WagerTransaction,
	reference *wagertransaction.WagerTransaction,
	ledgerEntryID string,
	occurredAt time.Time,
) (Result, error) {
	switch transaction.Kind() {
	case wagertransaction.KindBet:
		return processDebit(walletValue, transaction, ledgerEntryID, occurredAt)

	case wagertransaction.KindWin:
		if transaction.ReferenceExternalTransactionID() != "" {
			if err := validateWinReference(transaction, reference); err != nil {
				return Result{}, err
			}

			if err := resolveReference(transaction, reference, occurredAt); err != nil {
				return Result{}, err
			}
		}

		return processCredit(walletValue, transaction, ledgerEntryID, occurredAt)

	case wagertransaction.KindLoss:
		return processLoss(walletValue, transaction, occurredAt)

	case wagertransaction.KindRefund:
		return processRefund(
			walletValue,
			transaction,
			reference,
			ledgerEntryID,
			occurredAt,
		)

	case wagertransaction.KindRollback:
		return processRollback(
			walletValue,
			transaction,
			reference,
			ledgerEntryID,
			occurredAt,
		)

	default:
		return Result{}, ErrInvalidTransactionKind
	}
}

func processDebit(
	walletValue *wallet.Wallet,
	transaction *wagertransaction.WagerTransaction,
	ledgerEntryID string,
	occurredAt time.Time,
) (Result, error) {
	if !transaction.Money().IsPositive() {
		return Result{}, ErrInvalidAmount
	}

	if strings.TrimSpace(ledgerEntryID) == "" {
		return Result{}, ErrInvalidLedgerEntryID
	}

	before := walletValue.Balance()

	if err := walletValue.Debit(transaction.Money(), occurredAt); err != nil {
		return Result{}, err
	}

	entry, err := newLedgerEntry(
		ledgerEntryID,
		transaction,
		ledger.DirectionDebit,
		before,
		walletValue.Balance(),
		occurredAt,
	)
	if err != nil {
		return Result{}, err
	}

	if err := transaction.MarkProcessed(
		walletValue.Balance(),
		occurredAt,
	); err != nil {
		return Result{}, err
	}

	return newResult(walletValue.Balance(), &entry), nil
}

func processCredit(
	walletValue *wallet.Wallet,
	transaction *wagertransaction.WagerTransaction,
	ledgerEntryID string,
	occurredAt time.Time,
) (Result, error) {
	if !transaction.Money().IsPositive() {
		return Result{}, ErrInvalidAmount
	}

	if strings.TrimSpace(ledgerEntryID) == "" {
		return Result{}, ErrInvalidLedgerEntryID
	}

	before := walletValue.Balance()

	if err := walletValue.Credit(transaction.Money(), occurredAt); err != nil {
		return Result{}, err
	}

	entry, err := newLedgerEntry(
		ledgerEntryID,
		transaction,
		ledger.DirectionCredit,
		before,
		walletValue.Balance(),
		occurredAt,
	)
	if err != nil {
		return Result{}, err
	}

	if err := transaction.MarkProcessed(
		walletValue.Balance(),
		occurredAt,
	); err != nil {
		return Result{}, err
	}

	return newResult(walletValue.Balance(), &entry), nil
}

func processLoss(
	walletValue *wallet.Wallet,
	transaction *wagertransaction.WagerTransaction,
	occurredAt time.Time,
) (Result, error) {
	if !transaction.Money().IsZero() {
		return Result{}, ErrInvalidAmount
	}

	if err := transaction.MarkProcessed(
		walletValue.Balance(),
		occurredAt,
	); err != nil {
		return Result{}, err
	}

	return newResult(walletValue.Balance(), nil), nil
}

func processRefund(
	walletValue *wallet.Wallet,
	transaction *wagertransaction.WagerTransaction,
	reference *wagertransaction.WagerTransaction,
	ledgerEntryID string,
	occurredAt time.Time,
) (Result, error) {
	if err := validateReference(transaction, reference); err != nil {
		return Result{}, err
	}

	if reference.Kind() != wagertransaction.KindBet {
		return Result{}, ErrInvalidReferenceKind
	}

	if err := validateReferenceAmount(transaction, reference); err != nil {
		return Result{}, err
	}

	if err := resolveReference(transaction, reference, occurredAt); err != nil {
		return Result{}, err
	}

	return processCredit(
		walletValue,
		transaction,
		ledgerEntryID,
		occurredAt,
	)
}

func processRollback(
	walletValue *wallet.Wallet,
	transaction *wagertransaction.WagerTransaction,
	reference *wagertransaction.WagerTransaction,
	ledgerEntryID string,
	occurredAt time.Time,
) (Result, error) {
	if err := validateReference(transaction, reference); err != nil {
		return Result{}, err
	}

	if err := validateReferenceAmount(transaction, reference); err != nil {
		return Result{}, err
	}

	if err := resolveReference(transaction, reference, occurredAt); err != nil {
		return Result{}, err
	}

	switch reference.Kind() {
	case wagertransaction.KindBet:
		return processCredit(
			walletValue,
			transaction,
			ledgerEntryID,
			occurredAt,
		)

	case wagertransaction.KindWin, wagertransaction.KindRefund:
		return processDebit(
			walletValue,
			transaction,
			ledgerEntryID,
			occurredAt,
		)

	default:
		return Result{}, ErrInvalidReferenceKind
	}
}

func validateWalletTransaction(
	walletValue *wallet.Wallet,
	transaction *wagertransaction.WagerTransaction,
) error {
	if walletValue == nil || transaction == nil {
		return ErrWalletMismatch
	}

	if transaction.WalletID() != walletValue.ID() {
		return ErrWalletMismatch
	}

	if transaction.PlayerID() != walletValue.PlayerID() {
		return ErrPlayerMismatch
	}

	if transaction.Money().Currency() != walletValue.Currency() {
		return ErrCurrencyMismatch
	}

	return nil
}

func validateWinReference(
	transaction *wagertransaction.WagerTransaction,
	reference *wagertransaction.WagerTransaction,
) error {
	if err := validateReference(transaction, reference); err != nil {
		return err
	}

	if reference.Kind() != wagertransaction.KindBet {
		return ErrInvalidReferenceKind
	}

	return nil
}

func validateReference(
	transaction *wagertransaction.WagerTransaction,
	reference *wagertransaction.WagerTransaction,
) error {
	if reference == nil {
		return ErrReferenceRequired
	}

	if reference.Status() != wagertransaction.StatusProcessed {
		return ErrReferenceNotProcessed
	}

	if transaction.ReferenceExternalTransactionID() != reference.ExternalTransactionID() ||
		transaction.ProviderID() != reference.ProviderID() ||
		transaction.PlayerID() != reference.PlayerID() ||
		transaction.WalletID() != reference.WalletID() ||
		transaction.RoundID() != reference.RoundID() ||
		transaction.Money().Currency() != reference.Money().Currency() {
		return ErrReferenceMismatch
	}

	return nil
}

func validateReferenceAmount(
	transaction *wagertransaction.WagerTransaction,
	reference *wagertransaction.WagerTransaction,
) error {
	if !transaction.Money().Equal(reference.Money()) {
		return ErrInvalidReferenceAmount
	}

	return nil
}

func resolveReference(
	transaction *wagertransaction.WagerTransaction,
	reference *wagertransaction.WagerTransaction,
	occurredAt time.Time,
) error {
	if transaction.Status() == wagertransaction.StatusPending {
		if err := transaction.MarkPendingReference(occurredAt); err != nil {
			return err
		}
	}

	if transaction.Status() != wagertransaction.StatusPendingReference {
		return wagertransaction.ErrInvalidTransition
	}

	return transaction.ResolveReference(
		reference.ID(),
		occurredAt,
	)
}

func newLedgerEntry(
	ledgerEntryID string,
	transaction *wagertransaction.WagerTransaction,
	direction ledger.Direction,
	before money.Money,
	after money.Money,
	occurredAt time.Time,
) (ledger.Entry, error) {
	return ledger.New(
		strings.TrimSpace(ledgerEntryID),
		transaction.WalletID(),
		transaction.ID(),
		direction,
		transaction.Money(),
		before,
		after,
		occurredAt,
	)
}
