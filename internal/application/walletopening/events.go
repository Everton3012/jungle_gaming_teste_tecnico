package walletopening

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	domainledger "jungle_gaming_teste_tecnico/internal/domain/ledger"
	domainmoney "jungle_gaming_teste_tecnico/internal/domain/money"
	domainoutbox "jungle_gaming_teste_tecnico/internal/domain/outbox"
	domaintransaction "jungle_gaming_teste_tecnico/internal/domain/wagertransaction"
	domainwallet "jungle_gaming_teste_tecnico/internal/domain/wallet"
	outboxpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/outbox"
)

const (
	eventTypeWagerTransactionProcessed = "WagerTransactionProcessed"
	eventTypeWalletBalanceChanged      = "WalletBalanceChanged"
	integrationEventVersion            = 1
)

type integrationEventEnvelope struct {
	EventID       string      `json:"eventId"`
	EventType     string      `json:"eventType"`
	AggregateID   string      `json:"aggregateId"`
	CorrelationID string      `json:"correlationId"`
	CausationID   string      `json:"causationId,omitempty"`
	OccurredAt    string      `json:"occurredAt"`
	Version       int         `json:"version"`
	Data          interface{} `json:"data"`
}

type eventMoney struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

type wagerTransactionProcessedData struct {
	TransactionID string     `json:"transactionId"`
	WalletID      string     `json:"walletId"`
	PlayerID      string     `json:"playerId"`
	Kind          string     `json:"kind"`
	Money         eventMoney `json:"money"`
	ResultBalance eventMoney `json:"resultBalance"`
}

type walletBalanceChangedData struct {
	WalletID      string     `json:"walletId"`
	TransactionID string     `json:"transactionId"`
	Direction     string     `json:"direction"`
	Money         eventMoney `json:"money"`
	BalanceBefore eventMoney `json:"balanceBefore"`
	BalanceAfter  eventMoney `json:"balanceAfter"`
	WalletVersion uint64     `json:"walletVersion"`
}

func persistOpeningEvents(
	ctx context.Context,
	repository *outboxpostgres.Repository,
	wallet *domainwallet.Wallet,
	transaction *domaintransaction.WagerTransaction,
	ledgerEntry *domainledger.Entry,
	occurredAt time.Time,
) error {
	if repository == nil {
		return errors.New("outbox repository is required")
	}

	if wallet == nil {
		return errors.New("wallet is required")
	}

	if transaction == nil {
		return errors.New("opening transaction is required")
	}

	if ledgerEntry == nil {
		return errors.New("opening ledger entry is required")
	}

	resultBalance, ok := transaction.ResultBalance()
	if !ok {
		return errors.New(
			"opening transaction has no result balance",
		)
	}

	correlationID := transaction.ID()

	processedData := wagerTransactionProcessedData{
		TransactionID: transaction.ID(),
		WalletID:      transaction.WalletID(),
		PlayerID:      transaction.PlayerID(),
		Kind:          string(transaction.Kind()),
		Money:         moneyForEvent(transaction.Money()),
		ResultBalance: moneyForEvent(resultBalance),
	}

	if err := persistIntegrationEvent(
		ctx,
		repository,
		"wager-transaction-processed-"+transaction.ID(),
		eventTypeWagerTransactionProcessed,
		"WAGER_TRANSACTION",
		transaction.ID(),
		correlationID,
		occurredAt,
		processedData,
	); err != nil {
		return err
	}

	balanceData := walletBalanceChangedData{
		WalletID:      ledgerEntry.WalletID(),
		TransactionID: ledgerEntry.TransactionID(),
		Direction:     string(ledgerEntry.Direction()),
		Money:         moneyForEvent(ledgerEntry.Amount()),
		BalanceBefore: moneyForEvent(ledgerEntry.BalanceBefore()),
		BalanceAfter:  moneyForEvent(ledgerEntry.BalanceAfter()),
		WalletVersion: wallet.Version(),
	}

	if err := persistIntegrationEvent(
		ctx,
		repository,
		"wallet-balance-changed-"+transaction.ID(),
		eventTypeWalletBalanceChanged,
		"WALLET",
		wallet.ID(),
		correlationID,
		occurredAt,
		balanceData,
	); err != nil {
		return err
	}

	return nil
}

func persistIntegrationEvent(
	ctx context.Context,
	repository *outboxpostgres.Repository,
	eventID string,
	eventType string,
	aggregateType string,
	aggregateID string,
	correlationID string,
	occurredAt time.Time,
	data interface{},
) error {
	envelope := integrationEventEnvelope{
		EventID:       eventID,
		EventType:     eventType,
		AggregateID:   aggregateID,
		CorrelationID: correlationID,
		OccurredAt:    occurredAt.UTC().Format(time.RFC3339Nano),
		Version:       integrationEventVersion,
		Data:          data,
	}

	payload, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf(
			"marshal integration event %s: %w",
			eventType,
			err,
		)
	}

	event, err :=
		domainoutbox.NewEvent(
			domainoutbox.NewEventInput{
				ID:            eventID,
				EventType:     eventType,
				AggregateType: aggregateType,
				AggregateID:   aggregateID,
				Payload:       payload,
				CreatedAt:     occurredAt,
			},
		)
	if err != nil {
		return fmt.Errorf(
			"create integration event %s: %w",
			eventType,
			err,
		)
	}

	if err := repository.Create(
		ctx,
		event,
	); err != nil {
		return fmt.Errorf(
			"persist integration event %s: %w",
			eventType,
			err,
		)
	}

	return nil
}

func moneyForEvent(
	value domainmoney.Money,
) eventMoney {
	return eventMoney{
		Amount: formatMinorUnits(
			value.Amount(),
		),
		Currency: value.Currency(),
	}
}

func formatMinorUnits(
	amount int64,
) string {
	sign := ""

	if amount < 0 {
		sign = "-"
		amount = -amount
	}

	return fmt.Sprintf(
		"%s%d.%02d",
		sign,
		amount/100,
		amount%100,
	)
}
