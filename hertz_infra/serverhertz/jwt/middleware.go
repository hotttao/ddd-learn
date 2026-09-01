package jwt

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	configpb "media_agent/hertz_gen/config"
)

// NewMiddleware builds the shared Hertz middleware for Internal JWT
// authentication. A disabled configuration returns nil.
func NewMiddleware(ctx context.Context, cfg *configpb.InternalJWTConfig) (app.HandlerFunc, error) {
	if cfg == nil || !cfg.GetEnabled() {
		return nil, nil
	}
	prefixes, err := protectedPrefixes(cfg.GetProtectedPrefixes())
	if err != nil {
		return nil, err
	}
	validator, err := NewValidator(ctx, Config{
		Issuer:            cfg.GetIssuer(),
		Audiences:         cfg.GetAudiences(),
		JWKSURL:           cfg.GetJwksUrl(),
		AllowedAlgorithms: cfg.GetAllowedAlgorithms(),
		RefreshInterval:   time.Duration(cfg.GetRefreshIntervalSeconds()) * time.Second,
		HTTPTimeout:       time.Duration(cfg.GetHttpTimeoutMs()) * time.Millisecond,
		ClockSkew:         time.Duration(cfg.GetClockSkewSeconds()) * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize internal JWT validator: %w", err)
	}

	return func(ctx context.Context, c *app.RequestContext) {
		if !matchesProtectedPrefix(string(c.Request.URI().Path()), prefixes) {
			c.Next(ctx)
			return
		}

		stripUntrustedIdentityHeaders(c)
		raw, err := BearerToken(string(c.GetHeader("Authorization")))
		if err != nil {
			abortUnauthorized(c)
			return
		}
		principal, err := validator.Validate(ctx, raw)
		if err != nil {
			abortUnauthorized(c)
			return
		}
		c.Next(WithPrincipal(ctx, principal))
	}, nil
}

func protectedPrefixes(values []string) ([]string, error) {
	if len(values) == 0 {
		return nil, fmt.Errorf("internaljwt: at least one protected prefix is required")
	}
	prefixes := make([]string, 0, len(values))
	for _, value := range values {
		prefix := strings.TrimSpace(value)
		if prefix == "" || !strings.HasPrefix(prefix, "/") {
			return nil, fmt.Errorf("internaljwt: protected prefix %q must start with /", value)
		}
		if prefix != "/" {
			prefix = strings.TrimRight(prefix, "/")
		}
		prefixes = append(prefixes, prefix)
	}
	return prefixes, nil
}

func matchesProtectedPrefix(path string, prefixes []string) bool {
	for _, prefix := range prefixes {
		if prefix == "/" || path == prefix || strings.HasPrefix(path, prefix+"/") {
			return true
		}
	}
	return false
}

func stripUntrustedIdentityHeaders(c *app.RequestContext) {
	for _, header := range []string{"X-User-ID", "X-Role", "X-Subject"} {
		c.Request.Header.Del(header)
	}
}

func abortUnauthorized(c *app.RequestContext) {
	c.AbortWithStatusJSON(consts.StatusUnauthorized, map[string]any{
		"error": map[string]string{
			"code":    "invalid_internal_token",
			"message": "authentication required",
		},
	})
}
