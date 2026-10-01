package wager

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	domainledger "jungle_gaming_teste_tecnico/internal/domain/ledger"
	domainmoney "jungle_gaming_teste_tecnico/internal/domain/money"
	domainoutbox "jungle_gaming_teste_tecnico/internal/domain/outbox"
	domaintransaction "jungle_gaming_teste_tecnico/internal/domain/wagertransaction"
	outboxpostgres "jungle_gaming_teste_tecnico/internal/infrastructure/postgres/outbox"
)

const (
	eventTypeWagerTransactionProcessed        = "WagerTransactionProcessed"
	eventTypeWagerTransactionRejected         = "WagerTransactionRejected"
	eventTypeWalletBalanceChanged             = "WalletBalanceChanged"
	eventTypeWagerTransactionPendingReference = "WagerTransactionPendingReference"
	integrationEventVersion                   = 1
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
	TransactionID                  string     `json:"transactionId"`
	ProviderID                     string     `json:"providerId,omitempty"`
	ExternalTransactionID          string     `json:"externalTransactionId,omitempty"`
	WalletID                       string     `json:"walletId"`
	PlayerID                       string     `json:"playerId"`
	RoundID                        string     `json:"roundId,omitempty"`
	GameID                         string     `json:"gameId,omitempty"`
	Kind                           string     `json:"kind"`
	Money                          eventMoney `json:"money"`
	ResultBalance                  eventMoney `json:"resultBalance"`
	ReferenceExternalTransactionID string     `json:"referenceExternalTransactionId,omitempty"`
	ReferenceTransactionID         string     `json:"referenceTransactionId,omitempty"`
}

type wagerTransactionRejectedData struct {
	TransactionID                  string     `json:"transactionId"`
	ProviderID                     string     `json:"providerId"`
	ExternalTransactionID          string     `json:"externalTransactionId"`
	WalletID                       string     `json:"walletId"`
	PlayerID                       string     `json:"playerId"`
	RoundID                        string     `json:"roundId"`
	GameID                         string     `json:"gameId"`
	Kind                           string     `json:"kind"`
	Money                          eventMoney `json:"money"`
	ResultBalance                  eventMoney `json:"resultBalance"`
	FailureCode                    string     `json:"failureCode"`
	ReferenceExternalTransactionID string     `json:"referenceExternalTransactionId,omitempty"`
	ReferenceTransactionID         string     `json:"referenceTransactionId,omitempty"`
}

type wagerTransactionPendingReferenceData struct {
	TransactionID                  string     `json:"transactionId"`
	ProviderID                     string     `json:"providerId"`
	ExternalTransactionID          string     `json:"externalTransactionId"`
	WalletID                       string     `json:"walletId"`
	PlayerID                       string     `json:"playerId"`
	RoundID                        string     `json:"roundId"`
	GameID                         string     `json:"gameId"`
	Kind                           string     `json:"kind"`
	Money                          eventMoney `json:"money"`
	ReferenceExternalTransactionID string     `json:"referenceExternalTransactionId"`
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

type eventMetadata struct {
	CorrelationID string
	CausationID   string
}

func persistProcessedEvents(
	ctx context.Context,
	repository *outboxpostgres.Repository,
	transaction *domaintransaction.WagerTransaction,
	ledgerEntry *domainledger.Entry,
	walletVersion uint64,
	occurredAt time.Time,
	metadata eventMetadata,
) error {
	if transaction == nil {
		return ErrTransactionRequired
	}

	balance, ok := transaction.ResultBalance()
	if !ok {
		return fmt.Errorf("transaction %s has no result balance", transaction.ID())
	}

	data := wagerTransactionProcessedData{
		TransactionID:                  transaction.ID(),
		ProviderID:                     transaction.ProviderID(),
		ExternalTransactionID:          transaction.ExternalTransactionID(),
		WalletID:                       transaction.WalletID(),
		PlayerID:                       transaction.PlayerID(),
		RoundID:                        transaction.RoundID(),
		GameID:                         transaction.GameID(),
		Kind:                           string(transaction.Kind()),
		Money:                          moneyForEvent(transaction.Money()),
		ResultBalance:                  moneyForEvent(balance),
		ReferenceExternalTransactionID: transaction.ReferenceExternalTransactionID(),
		ReferenceTransactionID:         transaction.ReferenceTransactionID(),
	}

	if err := persistIntegrationEvent(
		ctx,
		repository,
		"wager-transaction-processed-"+transaction.ID(),
		eventTypeWagerTransactionProcessed,
		"WAGER_TRANSACTION",
		transaction.ID(),
		occurredAt,
		metadata,
		data,
	); err != nil {
		return err
	}

	if ledgerEntry == nil {
		return nil
	}

	balanceData := walletBalanceChangedData{
		WalletID:      ledgerEntry.WalletID(),
		TransactionID: ledgerEntry.TransactionID(),
		Direction:     string(ledgerEntry.Direction()),
		Money:         moneyForEvent(ledgerEntry.Amount()),
		BalanceBefore: moneyForEvent(ledgerEntry.BalanceBefore()),
		BalanceAfter:  moneyForEvent(ledgerEntry.BalanceAfter()),
		WalletVersion: walletVersion,
	}

	return persistIntegrationEvent(
		ctx,
		repository,
		"wallet-balance-changed-"+transaction.ID(),
		eventTypeWalletBalanceChanged,
		"WALLET",
		transaction.WalletID(),
		occurredAt,
		metadata,
		balanceData,
	)
}

func persistRejectedEvent(
	ctx context.Context,
	repository *outboxpostgres.Repository,
	transaction *domaintransaction.WagerTransaction,
	occurredAt time.Time,
	metadata eventMetadata,
) error {
	if transaction == nil {
		return ErrTransactionRequired
	}

	balance, ok := transaction.ResultBalance()
	if !ok {
		return fmt.Errorf("rejected transaction %s has no result balance", transaction.ID())
	}

	data := wagerTransactionRejectedData{
		TransactionID:                  transaction.ID(),
		ProviderID:                     transaction.ProviderID(),
		ExternalTransactionID:          transaction.ExternalTransactionID(),
		WalletID:                       transaction.WalletID(),
		PlayerID:                       transaction.PlayerID(),
		RoundID:                        transaction.RoundID(),
		GameID:                         transaction.GameID(),
		Kind:                           string(transaction.Kind()),
		Money:                          moneyForEvent(transaction.Money()),
		ResultBalance:                  moneyForEvent(balance),
		FailureCode:                    transaction.FailureCode().String(),
		ReferenceExternalTransactionID: transaction.ReferenceExternalTransactionID(),
		ReferenceTransactionID:         transaction.ReferenceTransactionID(),
	}

	return persistIntegrationEvent(
		ctx,
		repository,
		"wager-transaction-rejected-"+transaction.ID(),
		eventTypeWagerTransactionRejected,
		"WAGER_TRANSACTION",
		transaction.ID(),
		occurredAt,
		metadata,
		data,
	)
}

func persistPendingReferenceEvent(
	ctx context.Context,
	repository *outboxpostgres.Repository,
	transaction *domaintransaction.WagerTransaction,
	occurredAt time.Time,
	metadata eventMetadata,
) error {
	if transaction == nil {
		return ErrTransactionRequired
	}

	data := wagerTransactionPendingReferenceData{
		TransactionID:                  transaction.ID(),
		ProviderID:                     transaction.ProviderID(),
		ExternalTransactionID:          transaction.ExternalTransactionID(),
		WalletID:                       transaction.WalletID(),
		PlayerID:                       transaction.PlayerID(),
		RoundID:                        transaction.RoundID(),
		GameID:                         transaction.GameID(),
		Kind:                           string(transaction.Kind()),
		Money:                          moneyForEvent(transaction.Money()),
		ReferenceExternalTransactionID: transaction.ReferenceExternalTransactionID(),
	}

	return persistIntegrationEvent(
		ctx,
		repository,
		"wager-transaction-pending-reference-"+transaction.ID(),
		eventTypeWagerTransactionPendingReference,
		"WAGER_TRANSACTION",
		transaction.ID(),
		occurredAt,
		metadata,
		data,
	)
}

func persistIntegrationEvent(
	ctx context.Context,
	repository *outboxpostgres.Repository,
	eventID string,
	eventType string,
	aggregateType string,
	aggregateID string,
	occurredAt time.Time,
	metadata eventMetadata,
	data interface{},
) error {
	if repository == nil {
		return errors.New("outbox repository is required")
	}

	correlationID := strings.TrimSpace(metadata.CorrelationID)
	if correlationID == "" {
		correlationID = aggregateID
	}

	envelope := integrationEventEnvelope{
		EventID:       eventID,
		EventType:     eventType,
		AggregateID:   aggregateID,
		CorrelationID: correlationID,
		CausationID:   strings.TrimSpace(metadata.CausationID),
		OccurredAt:    occurredAt.UTC().Format(time.RFC3339Nano),
		Version:       integrationEventVersion,
		Data:          data,
	}

	payload, err := json.Marshal(envelope)
	if err != nil {
		return fmt.Errorf("marshal integration event %s: %w", eventType, err)
	}

	event, err := domainoutbox.NewEvent(domainoutbox.NewEventInput{
		ID:            eventID,
		EventType:     eventType,
		AggregateType: aggregateType,
		AggregateID:   aggregateID,
		Payload:       payload,
		CreatedAt:     occurredAt,
	})
	if err != nil {
		return fmt.Errorf("create integration event %s: %w", eventType, err)
	}

	if err := repository.Create(ctx, event); err != nil {
		return fmt.Errorf("persist integration event %s: %w", eventType, err)
	}

	return nil
}

func moneyForEvent(value domainmoney.Money) eventMoney {
	return eventMoney{
		Amount:   formatMinorUnits(value.Amount()),
		Currency: value.Currency(),
	}
}

func formatMinorUnits(amount int64) string {
	sign := ""
	if amount < 0 {
		sign = "-"
		amount = -amount
	}

	return fmt.Sprintf("%s%d.%02d", sign, amount/100, amount%100)
}
