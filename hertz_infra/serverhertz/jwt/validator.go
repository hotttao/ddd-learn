package jwt

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	jose "github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
)

var ErrInvalidToken = errors.New("invalid internal JWT")

// Principal is the authenticated identity carried by a validated internal JWT.
// Dynamic roles and resource permissions intentionally do not belong here.
type Principal struct {
	Subject               string
	SessionID             string
	AuthenticationMethods []string
	ClientID              string
	TokenID               string
	IssuedAt              time.Time
	ExpiresAt             time.Time
}

type principalContextKey struct{}

// WithPrincipal stores an authenticated principal in the request context.
func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, principalContextKey{}, principal)
}

// PrincipalFromContext returns only principals installed after successful JWT validation.
func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey{}).(Principal)
	return principal, ok
}

type privateClaims struct {
	SessionID             string   `json:"sid,omitempty"`
	AuthenticationMethods []string `json:"amr,omitempty"`
	ClientID              string   `json:"client_id,omitempty"`
	ServiceActor          string   `json:"service_actor,omitempty"`
}

// Validator maintains a cached public JWK set and refreshes it for rotations.
type Validator struct {
	config            Config
	allowedAlgorithms []jose.SignatureAlgorithm
	source            keySetSource
	now               func() time.Time

	refreshMu   sync.Mutex
	keysMu      sync.RWMutex
	keys        jose.JSONWebKeySet
	refreshedAt time.Time
}

func NewValidator(ctx context.Context, config Config) (*Validator, error) {
	allowed, err := config.validate()
	if err != nil {
		return nil, err
	}
	if config.RefreshInterval <= 0 {
		config.RefreshInterval = 5 * time.Minute
	}
	if config.HTTPTimeout <= 0 {
		config.HTTPTimeout = 2 * time.Second
	}
	if config.ClockSkew < 0 {
		return nil, fmt.Errorf("internaljwt: clock skew cannot be negative")
	}

	v := &Validator{
		config:            config,
		allowedAlgorithms: allowed,
		source: newURLKeySetSource(config.JWKSURL, &http.Client{
			Timeout: config.HTTPTimeout,
		}),
		now: time.Now,
	}
	if err := v.refresh(ctx, true); err != nil {
		return nil, fmt.Errorf("internaljwt: initial JWKS load: %w", err)
	}
	return v, nil
}

// Validate verifies signature, issuer, audience and mandatory lifetime claims.
func (v *Validator) Validate(ctx context.Context, raw string) (Principal, error) {
	token, err := jwt.ParseSigned(raw, v.allowedAlgorithms)
	if err != nil {
		return Principal{}, invalidToken("parse", err)
	}
	if len(token.Headers) != 1 {
		return Principal{}, invalidToken("header", fmt.Errorf("expected exactly one signature"))
	}
	header := token.Headers[0]
	if header.KeyID == "" {
		return Principal{}, invalidToken("header", fmt.Errorf("kid is required"))
	}

	keys, err := v.candidates(ctx, header.KeyID)
	if err != nil {
		return Principal{}, invalidToken("key", err)
	}

	var lastErr error
	for i := range keys {
		key := &keys[i]
		if key.Algorithm != "" && key.Algorithm != string(header.Algorithm) {
			lastErr = fmt.Errorf("JWK algorithm does not match token")
			continue
		}
		if key.Use != "" && key.Use != "sig" {
			lastErr = fmt.Errorf("JWK is not a signing key")
			continue
		}

		var claims jwt.Claims
		var extra privateClaims
		if err := token.Claims(key.Key, &claims, &extra); err != nil {
			lastErr = err
			continue
		}
		principal, err := v.validateClaims(claims, extra)
		if err != nil {
			return Principal{}, invalidToken("claims", err)
		}
		return principal, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("no usable key for kid %q", header.KeyID)
	}
	return Principal{}, invalidToken("signature", lastErr)
}

func (v *Validator) validateClaims(claims jwt.Claims, extra privateClaims) (Principal, error) {
	if claims.Subject == "" {
		return Principal{}, fmt.Errorf("sub is required")
	}
	if claims.Expiry == nil {
		return Principal{}, fmt.Errorf("exp is required")
	}
	if claims.IssuedAt == nil {
		return Principal{}, fmt.Errorf("iat is required")
	}
	if claims.NotBefore == nil {
		return Principal{}, fmt.Errorf("nbf is required")
	}
	if claims.ID == "" {
		return Principal{}, fmt.Errorf("jti is required")
	}

	now := v.now().UTC()
	expected := jwt.Expected{
		Issuer:      v.config.Issuer,
		AnyAudience: jwt.Audience(v.config.Audiences),
		Time:        now,
	}
	if err := claims.ValidateWithLeeway(expected, v.config.ClockSkew); err != nil {
		return Principal{}, err
	}
	subject, err := normalizedSubject(claims.Subject, extra)
	if err != nil {
		return Principal{}, err
	}
	return Principal{
		Subject:               subject,
		SessionID:             extra.SessionID,
		AuthenticationMethods: append([]string(nil), extra.AuthenticationMethods...),
		ClientID:              extra.ClientID,
		TokenID:               claims.ID,
		IssuedAt:              claims.IssuedAt.Time(),
		ExpiresAt:             claims.Expiry.Time(),
	}, nil
}

func normalizedSubject(tokenSubject string, claims privateClaims) (string, error) {
	for _, method := range claims.AuthenticationMethods {
		if method == "service_token" {
			if claims.ServiceActor == "" {
				return "", fmt.Errorf("service_actor is required for service_token")
			}
			return claims.ServiceActor, nil
		}
	}
	return tokenSubject, nil
}

func (v *Validator) candidates(ctx context.Context, kid string) ([]jose.JSONWebKey, error) {
	_ = v.refresh(ctx, false)
	keys := v.keysForID(kid)
	if len(keys) > 0 {
		return keys, nil
	}
	if err := v.refresh(ctx, true); err != nil {
		return nil, fmt.Errorf("refresh JWKS for unknown kid: %w", err)
	}
	keys = v.keysForID(kid)
	if len(keys) == 0 {
		return nil, fmt.Errorf("unknown kid %q", kid)
	}
	return keys, nil
}

func (v *Validator) keysForID(kid string) []jose.JSONWebKey {
	v.keysMu.RLock()
	defer v.keysMu.RUnlock()
	return append([]jose.JSONWebKey(nil), v.keys.Key(kid)...)
}

func (v *Validator) refresh(ctx context.Context, required bool) error {
	v.refreshMu.Lock()
	defer v.refreshMu.Unlock()

	v.keysMu.RLock()
	hasKeys := len(v.keys.Keys) > 0
	stale := v.refreshedAt.IsZero() || v.now().Sub(v.refreshedAt) >= v.config.RefreshInterval
	v.keysMu.RUnlock()
	if !required && !stale {
		return nil
	}

	set, err := v.source.Load(ctx)
	if err != nil {
		if !required && hasKeys {
			return nil
		}
		return err
	}
	v.keysMu.Lock()
	v.keys = set
	v.refreshedAt = v.now()
	v.keysMu.Unlock()
	return nil
}

// BearerToken extracts a single RFC 6750 Bearer credential.
func BearerToken(header string) (string, error) {
	parts := strings.Fields(header)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || parts[1] == "" {
		return "", invalidToken("authorization", fmt.Errorf("expected Bearer token"))
	}
	return parts[1], nil
}

func invalidToken(part string, err error) error {
	return fmt.Errorf("%w: %s: %v", ErrInvalidToken, part, err)
}
