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

type transactionDetailResponse struct {
	ID                             string         `json:"id"`
	ExternalTransactionID          string         `json:"externalTransactionId,omitempty"`
	ProviderID                     string         `json:"providerId,omitempty"`
	WalletID                       string         `json:"walletId"`
	PlayerID                       string         `json:"playerId"`
	RoundID                        string         `json:"roundId,omitempty"`
	GameID                         string         `json:"gameId,omitempty"`
	Kind                           string         `json:"kind"`
	Money                          moneyResponse  `json:"money"`
	ReferenceExternalTransactionID string         `json:"referenceExternalTransactionId,omitempty"`
	ReferenceTransactionID         string         `json:"referenceTransactionId,omitempty"`
	Status                         string         `json:"status"`
	FailureCode                    string         `json:"failureCode,omitempty"`
	ResultBalance                  *moneyResponse `json:"resultBalance,omitempty"`
	CreatedAt                      string         `json:"createdAt"`
	UpdatedAt                      string         `json:"updatedAt"`
}

type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type createWalletRequest struct {
	PlayerID       string       `json:"playerId"`
	InitialBalance moneyRequest `json:"initialBalance"`
}

type walletResponse struct {
	ID       string        `json:"id"`
	PlayerID string        `json:"playerId"`
	Balance  moneyResponse `json:"balance"`
	Version  uint64        `json:"version"`
}

type ledgerEntryResponse struct {
	ID            string        `json:"id"`
	WalletID      string        `json:"walletId"`
	TransactionID string        `json:"transactionId"`
	Direction     string        `json:"direction"`
	Money         moneyResponse `json:"money"`
	BalanceBefore moneyResponse `json:"balanceBefore"`
	BalanceAfter  moneyResponse `json:"balanceAfter"`
	CreatedAt     string        `json:"createdAt"`
}

type ledgerPageResponse struct {
	Items      []ledgerEntryResponse `json:"items"`
	NextCursor string                `json:"nextCursor,omitempty"`
}

type reconciliationResponse struct {
	WalletID          string        `json:"walletId"`
	StoredBalance     moneyResponse `json:"storedBalance"`
	CalculatedBalance moneyResponse `json:"calculatedBalance"`
	Difference        moneyResponse `json:"difference"`
	Consistent        bool          `json:"consistent"`
	CheckedEntries    int           `json:"checkedEntries"`
}

type healthResponse struct {
	Status   string `json:"status"`
	Postgres string `json:"postgres,omitempty"`
	SQS      string `json:"sqs,omitempty"`
}
