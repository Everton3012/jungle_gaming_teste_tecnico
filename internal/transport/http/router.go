package httptransport

import "net/http"

func NewRouter(
	wagerHandler *WagerHandler,
) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc(
		"/wagering/transactions",
		wagerHandler.ProcessTransaction,
	)

	return mux
}
