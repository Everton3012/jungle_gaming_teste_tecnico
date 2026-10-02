package auth

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	appconfig "jungle_gaming_teste_tecnico/internal/config"
)

var (
	ErrMissingCredentials = errors.New("authorization bearer token is required")
	ErrInvalidToken       = errors.New("invalid access token")
	ErrExpiredToken       = errors.New("access token expired")
	ErrForbidden          = errors.New("authenticated client is not authorized")
	ErrKeySetUnavailable  = errors.New("OIDC key set unavailable")
)

type Principal struct {
	ClientID   string
	ProviderID string
	Internal   bool
}

type contextKey struct{}

type Verifier struct {
	issuer         string
	jwksURL        string
	walletClientID string
	providers      map[string]struct{}
	httpClient     *http.Client

	mu        sync.RWMutex
	keys      map[string]*rsa.PublicKey
	keysUntil time.Time
}

type jwtHeader struct {
	Algorithm string `json:"alg"`
	KeyID     string `json:"kid"`
}

type jwtClaims struct {
	Issuer          string      `json:"iss"`
	AuthorizedParty string      `json:"azp"`
	Expiration      json.Number `json:"exp"`
	NotBefore       json.Number `json:"nbf"`
	IssuedAt        json.Number `json:"iat"`
}

type jwksDocument struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	KeyType   string `json:"kty"`
	KeyID     string `json:"kid"`
	Algorithm string `json:"alg"`
	Use       string `json:"use"`
	Modulus   string `json:"n"`
	Exponent  string `json:"e"`
}

func NewVerifier(cfg appconfig.Config) (*Verifier, error) {
	issuer := strings.TrimSpace(cfg.Auth.Issuer)
	jwksURL := strings.TrimSpace(cfg.Auth.JWKSURL)
	walletClientID := strings.TrimSpace(cfg.Auth.WalletClientID)

	if issuer == "" || jwksURL == "" || walletClientID == "" {
		return nil, errors.New("OIDC configuration is incomplete")
	}

	providers := make(map[string]struct{}, len(cfg.Auth.ProviderClients))
	for _, provider := range cfg.Auth.ProviderClients {
		provider = strings.TrimSpace(provider)
		if provider != "" {
			providers[provider] = struct{}{}
		}
	}
	if len(providers) == 0 {
		return nil, errors.New("at least one provider OAuth client is required")
	}

	return &Verifier{
		issuer:         issuer,
		jwksURL:        jwksURL,
		walletClientID: walletClientID,
		providers:      providers,
		httpClient: &http.Client{
			Timeout: 3 * time.Second,
		},
		keys: make(map[string]*rsa.PublicKey),
	}, nil
}

func (v *Verifier) AuthenticateRequest(ctx context.Context, request *http.Request) (Principal, error) {
	if request == nil {
		return Principal{}, ErrMissingCredentials
	}

	header := strings.TrimSpace(request.Header.Get("Authorization"))
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
		return Principal{}, ErrMissingCredentials
	}

	return v.Verify(ctx, parts[1])
}

func (v *Verifier) Verify(ctx context.Context, token string) (Principal, error) {
	token = strings.TrimSpace(token)
	segments := strings.Split(token, ".")
	if len(segments) != 3 {
		return Principal{}, ErrInvalidToken
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(segments[0])
	if err != nil {
		return Principal{}, ErrInvalidToken
	}

	var header jwtHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return Principal{}, ErrInvalidToken
	}
	if header.Algorithm != "RS256" || strings.TrimSpace(header.KeyID) == "" {
		return Principal{}, ErrInvalidToken
	}

	claimsBytes, err := base64.RawURLEncoding.DecodeString(segments[1])
	if err != nil {
		return Principal{}, ErrInvalidToken
	}

	decoder := json.NewDecoder(strings.NewReader(string(claimsBytes)))
	decoder.UseNumber()
	var claims jwtClaims
	if err := decoder.Decode(&claims); err != nil {
		return Principal{}, ErrInvalidToken
	}

	if claims.Issuer != v.issuer {
		return Principal{}, ErrInvalidToken
	}

	now := time.Now().UTC().Unix()
	exp, err := numberInt64(claims.Expiration)
	if err != nil || exp <= now {
		return Principal{}, ErrExpiredToken
	}
	if claims.NotBefore != "" {
		nbf, err := numberInt64(claims.NotBefore)
		if err != nil || nbf > now+5 {
			return Principal{}, ErrInvalidToken
		}
	}

	key, err := v.key(ctx, header.KeyID, false)
	if err != nil {
		return Principal{}, err
	}

	signature, err := base64.RawURLEncoding.DecodeString(segments[2])
	if err != nil {
		return Principal{}, ErrInvalidToken
	}

	hash := sha256.Sum256([]byte(segments[0] + "." + segments[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, hash[:], signature); err != nil {

		key, refreshErr := v.key(ctx, header.KeyID, true)
		if refreshErr != nil {
			return Principal{}, refreshErr
		}
		if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, hash[:], signature); err != nil {
			return Principal{}, ErrInvalidToken
		}
	}

	clientID := strings.TrimSpace(claims.AuthorizedParty)
	if clientID == "" {
		return Principal{}, ErrInvalidToken
	}

	if clientID == v.walletClientID {
		return Principal{ClientID: clientID, Internal: true}, nil
	}

	if _, ok := v.providers[clientID]; ok {
		return Principal{ClientID: clientID, ProviderID: clientID}, nil
	}

	return Principal{}, ErrForbidden
}

func (v *Verifier) key(ctx context.Context, kid string, forceRefresh bool) (*rsa.PublicKey, error) {
	if !forceRefresh {
		v.mu.RLock()
		key := v.keys[kid]
		valid := key != nil && time.Now().Before(v.keysUntil)
		v.mu.RUnlock()
		if valid {
			return key, nil
		}
	}

	if err := v.refreshKeys(ctx); err != nil {
		return nil, err
	}

	v.mu.RLock()
	key := v.keys[kid]
	v.mu.RUnlock()
	if key == nil {
		return nil, ErrInvalidToken
	}

	return key, nil
}

func (v *Verifier) refreshKeys(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, v.jwksURL, nil)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrKeySetUnavailable, err)
	}

	response, err := v.httpClient.Do(request)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrKeySetUnavailable, err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: HTTP %d", ErrKeySetUnavailable, response.StatusCode)
	}

	var document jwksDocument
	if err := json.NewDecoder(response.Body).Decode(&document); err != nil {
		return fmt.Errorf("%w: decode JWKS: %v", ErrKeySetUnavailable, err)
	}

	keys := make(map[string]*rsa.PublicKey)
	for _, item := range document.Keys {
		if item.KeyType != "RSA" || item.KeyID == "" || item.Modulus == "" || item.Exponent == "" {
			continue
		}
		key, err := rsaKey(item)
		if err != nil {
			continue
		}
		keys[item.KeyID] = key
	}
	if len(keys) == 0 {
		return fmt.Errorf("%w: no RSA signing keys", ErrKeySetUnavailable)
	}

	v.mu.Lock()
	v.keys = keys
	v.keysUntil = time.Now().Add(5 * time.Minute)
	v.mu.Unlock()
	return nil
}

func rsaKey(value jwk) (*rsa.PublicKey, error) {
	modulus, err := base64.RawURLEncoding.DecodeString(value.Modulus)
	if err != nil || len(modulus) == 0 {
		return nil, ErrInvalidToken
	}
	exponentBytes, err := base64.RawURLEncoding.DecodeString(value.Exponent)
	if err != nil || len(exponentBytes) == 0 || len(exponentBytes) > 4 {
		return nil, ErrInvalidToken
	}

	exponent := 0
	for _, b := range exponentBytes {
		exponent = exponent<<8 | int(b)
	}
	if exponent <= 1 {
		return nil, ErrInvalidToken
	}

	return &rsa.PublicKey{N: new(big.Int).SetBytes(modulus), E: exponent}, nil
}

func numberInt64(value json.Number) (int64, error) {
	if value == "" {
		return 0, ErrInvalidToken
	}
	return value.Int64()
}

func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, contextKey{}, principal)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(contextKey{}).(Principal)
	return principal, ok
}
