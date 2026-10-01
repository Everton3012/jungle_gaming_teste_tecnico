package httptransport

import "net/http"

func NewRouter(
	wagerHandler *WagerHandler,
	walletHandler *WalletHandler,
) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc(
		"/wallets",
		walletHandler.Create,
	)

	mux.HandleFunc(
		"/wagering/transactions",
		wagerHandler.ProcessTransaction,
	)

	return mux
}
