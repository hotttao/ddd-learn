package jwt

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

func TestValidatorValidate(t *testing.T) {
	now := time.Date(2026, 8, 29, 3, 0, 0, 0, time.UTC)
	trusted := mustRSAKey(t)
	untrusted := mustRSAKey(t)
	v := testValidator(now, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{publicJWK("trusted", trusted)}})

	tests := []struct {
		name    string
		key     *rsa.PrivateKey
		kid     string
		claims  jwt.Claims
		wantErr bool
	}{
		{name: "valid", key: trusted, kid: "trusted", claims: validClaims(now)},
		{name: "expired", key: trusted, kid: "trusted", claims: withExpiry(validClaims(now), now.Add(-time.Minute)), wantErr: true},
		{name: "wrong audience", key: trusted, kid: "trusted", claims: withAudience(validClaims(now), "external-api"), wantErr: true},
		{name: "wrong issuer", key: trusted, kid: "trusted", claims: withIssuer(validClaims(now), "someone-else"), wantErr: true},
		{name: "wrong signature", key: untrusted, kid: "trusted", claims: validClaims(now), wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			raw := signToken(t, test.key, test.kid, test.claims)
			principal, err := v.Validate(context.Background(), raw)
			if test.wantErr {
				if !errors.Is(err, ErrInvalidToken) {
					t.Fatalf("Validate() error = %v, want ErrInvalidToken", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Validate() error = %v", err)
			}
			if principal.Subject != "identity:alice" || principal.TokenID != "token-1" {
				t.Fatalf("Validate() principal = %#v", principal)
			}
		})
	}
}

func TestValidatorRejectsAlgorithmDowngrade(t *testing.T) {
	now := time.Date(2026, 8, 29, 3, 0, 0, 0, time.UTC)
	key := mustRSAKey(t)
	v := testValidator(now, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{publicJWK("trusted", key)}})

	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","kid":"trusted"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"iss":"oathkeeper","sub":"identity:alice"}`))
	_, err := v.Validate(context.Background(), header+"."+payload+".")
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("Validate() error = %v, want ErrInvalidToken", err)
	}
}

func TestValidatorRefreshesUnknownKeyID(t *testing.T) {
	now := time.Date(2026, 8, 29, 3, 0, 0, 0, time.UTC)
	oldKey := mustRSAKey(t)
	newKey := mustRSAKey(t)
	source := &rotatingSource{sets: []jose.JSONWebKeySet{
		{Keys: []jose.JSONWebKey{publicJWK("old", oldKey)}},
		{Keys: []jose.JSONWebKey{publicJWK("new", newKey)}},
	}}
	v := testValidatorWithSource(now, source)
	if err := v.refresh(context.Background(), true); err != nil {
		t.Fatalf("initial refresh: %v", err)
	}

	raw := signToken(t, newKey, "new", validClaims(now))
	principal, err := v.Validate(context.Background(), raw)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if principal.Subject != "identity:alice" {
		t.Fatalf("Validate() subject = %q", principal.Subject)
	}
	if source.Calls() != 2 {
		t.Fatalf("JWKS source calls = %d, want 2", source.Calls())
	}
}

func TestValidatorNormalizesServiceActor(t *testing.T) {
	now := time.Date(2026, 8, 29, 3, 0, 0, 0, time.UTC)
	key := mustRSAKey(t)
	v := testValidator(now, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{publicJWK("trusted", key)}})

	raw := signTokenWithPrivateClaims(t, key, "trusted", validClaims(now), privateClaims{
		AuthenticationMethods: []string{"service_token"},
		ServiceActor:          "service:xhs-cli",
	})
	principal, err := v.Validate(context.Background(), raw)
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if principal.Subject != "service:xhs-cli" {
		t.Fatalf("Validate() subject = %q", principal.Subject)
	}
}

func TestValidatorRejectsServiceTokenWithoutActor(t *testing.T) {
	now := time.Date(2026, 8, 29, 3, 0, 0, 0, time.UTC)
	key := mustRSAKey(t)
	v := testValidator(now, jose.JSONWebKeySet{Keys: []jose.JSONWebKey{publicJWK("trusted", key)}})

	raw := signTokenWithPrivateClaims(t, key, "trusted", validClaims(now), privateClaims{
		AuthenticationMethods: []string{"service_token"},
	})
	_, err := v.Validate(context.Background(), raw)
	if !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("Validate() error = %v, want ErrInvalidToken", err)
	}
}

func TestBearerToken(t *testing.T) {
	token, err := BearerToken("Bearer abc.def.ghi")
	if err != nil || token != "abc.def.ghi" {
		t.Fatalf("BearerToken() = %q, %v", token, err)
	}
	for _, header := range []string{"", "Basic abc", "Bearer", "Bearer one two"} {
		if _, err := BearerToken(header); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("BearerToken(%q) error = %v", header, err)
		}
	}
}

func testValidator(now time.Time, set jose.JSONWebKeySet) *Validator {
	v := testValidatorWithSource(now, &rotatingSource{sets: []jose.JSONWebKeySet{set}})
	v.keys = set
	v.refreshedAt = now
	return v
}

func testValidatorWithSource(now time.Time, source keySetSource) *Validator {
	return &Validator{
		config: Config{
			Issuer:          "oathkeeper",
			Audiences:       []string{"internal-api"},
			RefreshInterval: time.Hour,
			ClockSkew:       0,
		},
		allowedAlgorithms: []jose.SignatureAlgorithm{jose.RS256},
		source:            source,
		now:               func() time.Time { return now },
	}
}

func validClaims(now time.Time) jwt.Claims {
	return jwt.Claims{
		Issuer:    "oathkeeper",
		Subject:   "identity:alice",
		Audience:  jwt.Audience{"internal-api"},
		Expiry:    jwt.NewNumericDate(now.Add(5 * time.Minute)),
		NotBefore: jwt.NewNumericDate(now),
		IssuedAt:  jwt.NewNumericDate(now),
		ID:        "token-1",
	}
}

func withExpiry(claims jwt.Claims, expiry time.Time) jwt.Claims {
	claims.Expiry = jwt.NewNumericDate(expiry)
	return claims
}

func withAudience(claims jwt.Claims, audience string) jwt.Claims {
	claims.Audience = jwt.Audience{audience}
	return claims
}

func withIssuer(claims jwt.Claims, issuer string) jwt.Claims {
	claims.Issuer = issuer
	return claims
}

func signToken(t *testing.T, key *rsa.PrivateKey, kid string, claims jwt.Claims) string {
	t.Helper()
	return signTokenWithPrivateClaims(t, key, kid, claims, privateClaims{
		SessionID:             "session-1",
		AuthenticationMethods: []string{"session"},
	})
}

func signTokenWithPrivateClaims(t *testing.T, key *rsa.PrivateKey, kid string, claims jwt.Claims, private privateClaims) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{
		Algorithm: jose.RS256,
		Key: jose.JSONWebKey{
			Key:       key,
			KeyID:     kid,
			Algorithm: string(jose.RS256),
			Use:       "sig",
		},
	}, (&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		t.Fatalf("NewSigner(): %v", err)
	}
	raw, err := jwt.Signed(signer).
		Claims(claims).
		Claims(private).
		Serialize()
	if err != nil {
		t.Fatalf("Serialize(): %v", err)
	}
	return raw
}

func publicJWK(kid string, key *rsa.PrivateKey) jose.JSONWebKey {
	return jose.JSONWebKey{
		Key:       &key.PublicKey,
		KeyID:     kid,
		Algorithm: string(jose.RS256),
		Use:       "sig",
	}
}

func mustRSAKey(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("GenerateKey(): %v", err)
	}
	return key
}

type rotatingSource struct {
	mu   sync.Mutex
	sets []jose.JSONWebKeySet
	n    int
}

func (s *rotatingSource) Load(context.Context) (jose.JSONWebKeySet, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.sets) == 0 {
		return jose.JSONWebKeySet{}, fmt.Errorf("no key sets")
	}
	index := s.n
	if index >= len(s.sets) {
		index = len(s.sets) - 1
	}
	s.n++
	return s.sets[index], nil
}

func (s *rotatingSource) Calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.n
}
