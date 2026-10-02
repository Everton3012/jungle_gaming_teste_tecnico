package auth

import (
	"encoding/json"
	"errors"
	"net/http"
)

type Requirement int

const (
	RequireAny Requirement = iota
	RequireProvider
	RequireInternal
)

func (v *Verifier) Middleware(requirement Requirement, next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		principal, err := v.AuthenticateRequest(request.Context(), request)
		if err != nil {
			status := http.StatusUnauthorized
			code := "UNAUTHORIZED"
			message := "valid OAuth2 bearer token is required"
			if errors.Is(err, ErrForbidden) {
				status = http.StatusForbidden
				code = "FORBIDDEN"
				message = "authenticated client is not authorized"
			} else if errors.Is(err, ErrKeySetUnavailable) {
				status = http.StatusServiceUnavailable
				code = "IDENTITY_PROVIDER_UNAVAILABLE"
				message = "identity provider is temporarily unavailable"
			}
			writeAuthError(writer, status, code, message)
			return
		}

		switch requirement {
		case RequireProvider:
			if principal.Internal || principal.ProviderID == "" {
				writeAuthError(writer, http.StatusForbidden, "FORBIDDEN", "provider credentials are required")
				return
			}
		case RequireInternal:
			if !principal.Internal {
				writeAuthError(writer, http.StatusForbidden, "FORBIDDEN", "internal wallet-service credentials are required")
				return
			}
		}

		next.ServeHTTP(writer, request.WithContext(WithPrincipal(request.Context(), principal)))
	})
}

func writeAuthError(writer http.ResponseWriter, status int, code string, message string) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(map[string]string{"code": code, "message": message})
}
