package httptransport

import (
	"time"

	domainledger "jungle_gaming_teste_tecnico/internal/domain/ledger"
	domainmoney "jungle_gaming_teste_tecnico/internal/domain/money"
	domaintransaction "jungle_gaming_teste_tecnico/internal/domain/wagertransaction"
	domainwallet "jungle_gaming_teste_tecnico/internal/domain/wallet"
)

func presentMoney(value domainmoney.Money) moneyResponse {
	return moneyResponse{Amount: value.String(), Currency: value.Currency()}
}

func presentWallet(wallet *domainwallet.Wallet) walletResponse {
	return walletResponse{
		ID:       wallet.ID(),
		PlayerID: wallet.PlayerID(),
		Balance:  presentMoney(wallet.Balance()),
		Version:  wallet.Version(),
	}
}

func presentLedgerEntry(entry *domainledger.Entry) ledgerEntryResponse {
	return ledgerEntryResponse{
		ID:            entry.ID(),
		WalletID:      entry.WalletID(),
		TransactionID: entry.TransactionID(),
		Direction:     string(entry.Direction()),
		Money:         presentMoney(entry.Amount()),
		BalanceBefore: presentMoney(entry.BalanceBefore()),
		BalanceAfter:  presentMoney(entry.BalanceAfter()),
		CreatedAt:     entry.CreatedAt().UTC().Format(time.RFC3339Nano),
	}
}

func presentTransaction(transaction *domaintransaction.WagerTransaction) transactionDetailResponse {
	response := transactionDetailResponse{
		ID:                             transaction.ID(),
		ExternalTransactionID:          transaction.ExternalTransactionID(),
		ProviderID:                     transaction.ProviderID(),
		WalletID:                       transaction.WalletID(),
		PlayerID:                       transaction.PlayerID(),
		RoundID:                        transaction.RoundID(),
		GameID:                         transaction.GameID(),
		Kind:                           string(transaction.Kind()),
		Money:                          presentMoney(transaction.Money()),
		ReferenceExternalTransactionID: transaction.ReferenceExternalTransactionID(),
		ReferenceTransactionID:         transaction.ReferenceTransactionID(),
		Status:                         string(transaction.Status()),
		FailureCode:                    transaction.FailureCode().String(),
		CreatedAt:                      transaction.CreatedAt().UTC().Format(time.RFC3339Nano),
		UpdatedAt:                      transaction.UpdatedAt().UTC().Format(time.RFC3339Nano),
	}
	if balance, ok := transaction.ResultBalance(); ok {
		value := presentMoney(balance)
		response.ResultBalance = &value
	}
	return response
}
