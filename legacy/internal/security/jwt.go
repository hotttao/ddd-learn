package security

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	PrincipalType string `json:"principal_type"`
	TenantID      string `json:"tenant_id,omitempty"`
	ClientID      string `json:"client_id,omitempty"`
	SessionID     string `json:"sid,omitempty"`
	AAL           string `json:"aal,omitempty"`
	Scope         string `json:"scope,omitempty"`
	jwt.RegisteredClaims
}

type Subject struct {
	ID            string
	PrincipalType string
	TenantID      string
	ClientID      string
	SessionID     string
	AAL           string
	Scope         string
}

type Signer struct {
	privateKey *rsa.PrivateKey
	kid        string
	issuer     string
	audience   string
	ttl        time.Duration
	now        func() time.Time
}

func NewEphemeralSigner(issuer, audience string, ttl time.Duration) (*Signer, error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("generate RSA key: %w", err)
	}
	fingerprint := sha256.Sum256(key.PublicKey.N.Bytes())
	kid := "ddd-learn-" + base64.RawURLEncoding.EncodeToString(fingerprint[:8])
	return NewSigner(key, kid, issuer, audience, ttl), nil
}

func NewSigner(key *rsa.PrivateKey, kid, issuer, audience string, ttl time.Duration) *Signer {
	return &Signer{
		privateKey: key,
		kid:        kid,
		issuer:     issuer,
		audience:   audience,
		ttl:        ttl,
		now:        time.Now,
	}
}

func (s *Signer) Issue(subject Subject) (string, time.Time, error) {
	now := s.now().UTC()
	expiresAt := now.Add(s.ttl)
	claims := Claims{
		PrincipalType: subject.PrincipalType,
		TenantID:      subject.TenantID,
		ClientID:      subject.ClientID,
		SessionID:     subject.SessionID,
		AAL:           subject.AAL,
		Scope:         subject.Scope,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   subject.ID,
			Audience:  jwt.ClaimStrings{s.audience},
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			ID:        randomID(),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = s.kid
	signed, err := token.SignedString(s.privateKey)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign JWT: %w", err)
	}
	return signed, expiresAt, nil
}

func (s *Signer) Verify(raw string) (*Claims, error) {
	return parseToken(raw, s.issuer, s.audience, func(token *jwt.Token) (any, error) {
		kid, _ := token.Header["kid"].(string)
		if kid != s.kid {
			return nil, fmt.Errorf("unknown kid %q", kid)
		}
		return &s.privateKey.PublicKey, nil
	})
}

type JWK struct {
	KeyType string `json:"kty"`
	Use     string `json:"use"`
	Alg     string `json:"alg"`
	KeyID   string `json:"kid"`
	N       string `json:"n"`
	E       string `json:"e"`
}

type JWKS struct {
	Keys []JWK `json:"keys"`
}

func (s *Signer) PublicJWKS() JWKS {
	publicKey := s.privateKey.PublicKey
	return JWKS{Keys: []JWK{{
		KeyType: "RSA",
		Use:     "sig",
		Alg:     "RS256",
		KeyID:   s.kid,
		N:       base64.RawURLEncoding.EncodeToString(publicKey.N.Bytes()),
		E:       encodeExponent(publicKey.E),
	}}}
}

type JWKSVerifier struct {
	url      string
	issuer   string
	audience string
	client   *http.Client

	mu   sync.RWMutex
	keys map[string]*rsa.PublicKey
}

func NewJWKSVerifier(url, issuer, audience string, client *http.Client) *JWKSVerifier {
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	return &JWKSVerifier{url: url, issuer: issuer, audience: audience, client: client, keys: map[string]*rsa.PublicKey{}}
}

func (v *JWKSVerifier) Verify(ctx context.Context, raw string) (*Claims, error) {
	return parseToken(raw, v.issuer, v.audience, func(token *jwt.Token) (any, error) {
		kid, _ := token.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("JWT header is missing kid")
		}
		if key := v.get(kid); key != nil {
			return key, nil
		}
		if err := v.refresh(ctx); err != nil {
			return nil, err
		}
		if key := v.get(kid); key != nil {
			return key, nil
		}
		return nil, fmt.Errorf("unknown kid %q", kid)
	})
}

func (v *JWKSVerifier) get(kid string) *rsa.PublicKey {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return v.keys[kid]
}

func (v *JWKSVerifier) refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, v.url, nil)
	if err != nil {
		return fmt.Errorf("create JWKS request: %w", err)
	}
	resp, err := v.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch JWKS: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch JWKS: unexpected status %d", resp.StatusCode)
	}
	var document JWKS
	if err := json.NewDecoder(resp.Body).Decode(&document); err != nil {
		return fmt.Errorf("decode JWKS: %w", err)
	}
	keys := make(map[string]*rsa.PublicKey, len(document.Keys))
	for _, item := range document.Keys {
		if item.KeyType != "RSA" || item.Use != "sig" || item.Alg != "RS256" || item.KeyID == "" {
			continue
		}
		key, err := item.publicKey()
		if err != nil {
			return fmt.Errorf("decode JWK %q: %w", item.KeyID, err)
		}
		keys[item.KeyID] = key
	}
	if len(keys) == 0 {
		return errors.New("JWKS has no usable RS256 signing key")
	}
	v.mu.Lock()
	v.keys = keys
	v.mu.Unlock()
	return nil
}

func (j JWK) publicKey() (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(j.N)
	if err != nil {
		return nil, fmt.Errorf("decode modulus: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(j.E)
	if err != nil {
		return nil, fmt.Errorf("decode exponent: %w", err)
	}
	if len(eBytes) == 0 || len(eBytes) > 4 {
		return nil, errors.New("invalid RSA exponent")
	}
	var padded [4]byte
	copy(padded[4-len(eBytes):], eBytes)
	exponent := int(binary.BigEndian.Uint32(padded[:]))
	if exponent < 3 {
		return nil, errors.New("invalid RSA exponent")
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(nBytes), E: exponent}, nil
}

func parseToken(raw, issuer, audience string, keyFunc jwt.Keyfunc) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(
		raw,
		claims,
		keyFunc,
		jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}),
		jwt.WithIssuer(issuer),
		jwt.WithAudience(audience),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
	)
	if err != nil {
		return nil, fmt.Errorf("verify JWT: %w", err)
	}
	if !token.Valid || claims.Subject == "" || claims.PrincipalType == "" {
		return nil, errors.New("JWT is missing required identity claims")
	}
	return claims, nil
}

func BearerToken(header string) (string, error) {
	scheme, token, ok := strings.Cut(strings.TrimSpace(header), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") || strings.TrimSpace(token) == "" {
		return "", errors.New("missing bearer token")
	}
	return strings.TrimSpace(token), nil
}

func encodeExponent(exponent int) string {
	var data [4]byte
	binary.BigEndian.PutUint32(data[:], uint32(exponent))
	first := 0
	for first < len(data)-1 && data[first] == 0 {
		first++
	}
	return base64.RawURLEncoding.EncodeToString(data[first:])
}

func randomID() string {
	value := make([]byte, 16)
	if _, err := rand.Read(value); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(value)
}
