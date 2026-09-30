package httptransport

import (
	"encoding/json"
	"net/http"
)

func writeJSON(
	writer http.ResponseWriter,
	status int,
	value any,
) {
	writer.Header().Set(
		"Content-Type",
		"application/json",
	)

	writer.WriteHeader(status)

	_ = json.NewEncoder(writer).Encode(value)
}

func writeError(
	writer http.ResponseWriter,
	status int,
	code string,
	message string,
) {
	writeJSON(
		writer,
		status,
		errorResponse{
			Code:    code,
			Message: message,
		},
	)
}
