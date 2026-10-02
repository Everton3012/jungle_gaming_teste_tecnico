package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"
	"time"

	appconfig "jungle_gaming_teste_tecnico/internal/config"
)

func TestVerifierRejectsExpiredTokenBeforeKeyLookup(t *testing.T) {
	verifier, err := NewVerifier(appconfig.Config{Auth: appconfig.AuthConfig{
		Issuer:          "https://issuer.example/realms/jungle",
		JWKSURL:         "https://issuer.example/realms/jungle/protocol/openid-connect/certs",
		WalletClientID:  "wallet-service",
		ProviderClients: []string{"provider-a"},
	}})
	if err != nil {
		t.Fatalf("create verifier: %v", err)
	}

	header, err := json.Marshal(map[string]string{
		"alg": "RS256",
		"kid": "test-key",
	})
	if err != nil {
		t.Fatalf("marshal header: %v", err)
	}
	claims, err := json.Marshal(map[string]any{
		"iss": "https://issuer.example/realms/jungle",
		"azp": "provider-a",
		"exp": time.Now().UTC().Add(-time.Minute).Unix(),
	})
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}

	token := base64.RawURLEncoding.EncodeToString(header) + "." +
		base64.RawURLEncoding.EncodeToString(claims) + "." +
		base64.RawURLEncoding.EncodeToString([]byte("signature-not-reached"))

	_, err = verifier.Verify(context.Background(), token)
	if !errors.Is(err, ErrExpiredToken) {
		t.Fatalf("verify expired token error = %v, want ErrExpiredToken", err)
	}
}
