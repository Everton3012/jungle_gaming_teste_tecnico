package httptransport

type moneyRequest struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

type wagerTransactionRequest struct {
	ProviderID                     string       `json:"providerId"`
	ExternalTransactionID          string       `json:"externalTransactionId"`
	PlayerID                       string       `json:"playerId"`
	WalletID                       string       `json:"walletId"`
	RoundID                        string       `json:"roundId"`
	GameID                         string       `json:"gameId"`
	Kind                           string       `json:"kind"`
	Money                          moneyRequest `json:"money"`
	ReferenceExternalTransactionID string       `json:"referenceExternalTransactionId,omitempty"`
}

type moneyResponse struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

type wagerTransactionResponse struct {
	TransactionID    string        `json:"transactionId"`
	Status           string        `json:"status"`
	Balance          moneyResponse `json:"balance"`
	IdempotentReplay bool          `json:"idempotentReplay"`
	FailureCode      string        `json:"failureCode,omitempty"`
}

type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
