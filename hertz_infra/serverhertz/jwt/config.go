// Package jwt verifies short-lived JWTs issued by the trusted edge.
// It contains no business authorization rules; services still decide which
// routes require a principal and which resource permissions to check.
package jwt

import (
	"fmt"
	"time"

	jose "github.com/go-jose/go-jose/v4"
)

// Config defines the trust boundary for internal JWT validation.
type Config struct {
	Issuer            string
	Audiences         []string
	JWKSURL           string
	AllowedAlgorithms []string
	RefreshInterval   time.Duration
	HTTPTimeout       time.Duration
	ClockSkew         time.Duration
}

func (c Config) validate() ([]jose.SignatureAlgorithm, error) {
	if c.Issuer == "" {
		return nil, fmt.Errorf("internaljwt: issuer is required")
	}
	if len(c.Audiences) == 0 {
		return nil, fmt.Errorf("internaljwt: at least one audience is required")
	}
	for _, audience := range c.Audiences {
		if audience == "" {
			return nil, fmt.Errorf("internaljwt: audience cannot be empty")
		}
	}
	if c.JWKSURL == "" {
		return nil, fmt.Errorf("internaljwt: jwks URL is required")
	}
	if len(c.AllowedAlgorithms) == 0 {
		return nil, fmt.Errorf("internaljwt: at least one signing algorithm is required")
	}

	allowed := make([]jose.SignatureAlgorithm, 0, len(c.AllowedAlgorithms))
	seen := make(map[jose.SignatureAlgorithm]struct{}, len(c.AllowedAlgorithms))
	for _, name := range c.AllowedAlgorithms {
		algorithm := jose.SignatureAlgorithm(name)
		switch algorithm {
		case jose.RS256, jose.RS384, jose.RS512,
			jose.PS256, jose.PS384, jose.PS512,
			jose.ES256, jose.ES384, jose.ES512,
			jose.EdDSA:
		default:
			return nil, fmt.Errorf("internaljwt: signing algorithm %q is not allowed", name)
		}
		if _, ok := seen[algorithm]; ok {
			continue
		}
		seen[algorithm] = struct{}{}
		allowed = append(allowed, algorithm)
	}
	return allowed, nil
}
