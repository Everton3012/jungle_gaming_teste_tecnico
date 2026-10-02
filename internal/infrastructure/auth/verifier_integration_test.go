package auth

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	appconfig "jungle_gaming_teste_tecnico/internal/config"
)

func TestVerifierAgainstRealKeycloak(t *testing.T) {
	const baseURL = "http://localhost:8085"
	client := &http.Client{Timeout: 750 * time.Millisecond}
	request, _ := http.NewRequest(http.MethodGet, baseURL+"/realms/jungle/.well-known/openid-configuration", nil)
	response, err := client.Do(request)
	if err != nil {
		t.Skipf("real Keycloak is not available: %v", err)
	}
	_ = response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Skipf("real Keycloak is not ready: HTTP %d", response.StatusCode)
	}

	verifier, err := NewVerifier(appconfig.Config{Auth: appconfig.AuthConfig{
		Issuer:          baseURL + "/realms/jungle",
		JWKSURL:         baseURL + "/realms/jungle/protocol/openid-connect/certs",
		WalletClientID:  "wallet-service",
		ProviderClients: []string{"provider-a", "provider-b"},
	}})
	if err != nil {
		t.Fatalf("create verifier: %v", err)
	}

	providerToken := keycloakToken(t, baseURL, "provider-a", "provider-a-secret")
	principal, err := verifier.Verify(context.Background(), providerToken)
	if err != nil {
		t.Fatalf("verify provider token: %v", err)
	}
	if principal.ProviderID != "provider-a" || principal.Internal {
		t.Fatalf("unexpected provider principal: %+v", principal)
	}

	walletToken := keycloakToken(t, baseURL, "wallet-service", "wallet-service-secret")
	principal, err = verifier.Verify(context.Background(), walletToken)
	if err != nil {
		t.Fatalf("verify wallet-service token: %v", err)
	}
	if !principal.Internal || principal.ProviderID != "" {
		t.Fatalf("unexpected internal principal: %+v", principal)
	}

	if _, err := verifier.Verify(context.Background(), "not-a-jwt"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("invalid token error = %v, want ErrInvalidToken", err)
	}
}

func keycloakToken(t *testing.T, baseURL, clientID, clientSecret string) string {
	t.Helper()

	form := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
	}
	request, err := http.NewRequest(http.MethodPost, baseURL+"/realms/jungle/protocol/openid-connect/token", strings.NewReader(form.Encode()))
	if err != nil {
		t.Fatalf("create token request: %v", err)
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	response, err := (&http.Client{Timeout: 10 * time.Second}).Do(request)
	if err != nil {
		t.Fatalf("request token: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("token endpoint status = %d, want 200", response.StatusCode)
	}

	var payload struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatalf("decode token response: %v", err)
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		t.Fatal("token endpoint returned an empty access_token")
	}
	return payload.AccessToken
}
