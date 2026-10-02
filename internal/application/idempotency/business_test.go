package idempotency

import (
	"testing"

	domainmoney "jungle_gaming_teste_tecnico/internal/domain/money"
)

func TestHashBusinessIsTransportIndependent(t *testing.T) {
	money, err := domainmoney.Parse("25.00", "brl")
	if err != nil {
		t.Fatalf("parse money: %v", err)
	}

	base := BusinessOperation{
		ProviderID:            " provider-a ",
		ExternalTransactionID: " transaction-123 ",
		PlayerID:              " player-1 ",
		WalletID:              " wallet-1 ",
		RoundID:               " round-1 ",
		GameID:                " game-1 ",
		Kind:                  " bet ",
		Money:                 money,
	}

	first, err := HashBusiness(base)
	if err != nil {
		t.Fatalf("hash first payload: %v", err)
	}

	second, err := HashBusiness(BusinessOperation{
		ProviderID:            "provider-a",
		ExternalTransactionID: "transaction-123",
		PlayerID:              "player-1",
		WalletID:              "wallet-1",
		RoundID:               "round-1",
		GameID:                "game-1",
		Kind:                  "BET",
		Money:                 money,
	})
	if err != nil {
		t.Fatalf("hash second payload: %v", err)
	}

	if first != second {
		t.Fatalf("equivalent business hashes differ: %q != %q", first, second)
	}
}

func TestHashBusinessChangesWhenBusinessContentChanges(t *testing.T) {
	money, err := domainmoney.Parse("25.00", "BRL")
	if err != nil {
		t.Fatalf("parse money: %v", err)
	}

	base := BusinessOperation{
		ProviderID:            "provider-a",
		ExternalTransactionID: "transaction-123",
		PlayerID:              "player-1",
		WalletID:              "wallet-1",
		RoundID:               "round-1",
		GameID:                "game-1",
		Kind:                  "BET",
		Money:                 money,
	}

	first, err := HashBusiness(base)
	if err != nil {
		t.Fatalf("hash base payload: %v", err)
	}

	base.RoundID = "round-2"
	second, err := HashBusiness(base)
	if err != nil {
		t.Fatalf("hash changed payload: %v", err)
	}

	if first == second {
		t.Fatalf("different business content produced the same hash: %q", first)
	}
}
