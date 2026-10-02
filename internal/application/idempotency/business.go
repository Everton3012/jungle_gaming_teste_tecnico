package idempotency

import (
	"encoding/json"
	"strings"

	domainmoney "jungle_gaming_teste_tecnico/internal/domain/money"
)

type BusinessOperation struct {
	ProviderID                     string
	ExternalTransactionID          string
	PlayerID                       string
	WalletID                       string
	RoundID                        string
	GameID                         string
	Kind                           string
	Money                          domainmoney.Money
	ReferenceExternalTransactionID string
}

type canonicalBusinessOperation struct {
	ProviderID                     string         `json:"providerId"`
	ExternalTransactionID          string         `json:"externalTransactionId"`
	PlayerID                       string         `json:"playerId"`
	WalletID                       string         `json:"walletId"`
	RoundID                        string         `json:"roundId"`
	GameID                         string         `json:"gameId"`
	Kind                           string         `json:"kind"`
	Money                          canonicalMoney `json:"money"`
	ReferenceExternalTransactionID string         `json:"referenceExternalTransactionId,omitempty"`
}

type canonicalMoney struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

func HashBusiness(operation BusinessOperation) (string, error) {
	payload, err := json.Marshal(canonicalBusinessOperation{
		ProviderID:            strings.TrimSpace(operation.ProviderID),
		ExternalTransactionID: strings.TrimSpace(operation.ExternalTransactionID),
		PlayerID:              strings.TrimSpace(operation.PlayerID),
		WalletID:              strings.TrimSpace(operation.WalletID),
		RoundID:               strings.TrimSpace(operation.RoundID),
		GameID:                strings.TrimSpace(operation.GameID),
		Kind:                  strings.ToUpper(strings.TrimSpace(operation.Kind)),
		Money: canonicalMoney{
			Amount:   operation.Money.String(),
			Currency: operation.Money.Currency(),
		},
		ReferenceExternalTransactionID: strings.TrimSpace(operation.ReferenceExternalTransactionID),
	})
	if err != nil {
		return "", err
	}
	return Hash(payload)
}
