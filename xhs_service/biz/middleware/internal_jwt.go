package middleware

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/protocol/consts"

	configpb "media_agent/hertz_gen/config"
	"media_agent/hertz_infra/internaljwt"
)

// InternalJWT authenticates requests selected by RequirePrefix.
type InternalJWT struct {
	validator *internaljwt.Validator
}

func NewInternalJWT(ctx context.Context, cfg *configpb.InternalJWTConfig) (*InternalJWT, error) {
	if cfg == nil || !cfg.GetEnabled() {
		return nil, nil
	}
	validator, err := internaljwt.NewValidator(ctx, internaljwt.Config{
		Issuer:            cfg.GetIssuer(),
		Audiences:         cfg.GetAudiences(),
		JWKSURL:           cfg.GetJwksUrl(),
		AllowedAlgorithms: cfg.GetAllowedAlgorithms(),
		RefreshInterval:   time.Duration(cfg.GetRefreshIntervalSeconds()) * time.Second,
		HTTPTimeout:       time.Duration(cfg.GetHttpTimeoutMs()) * time.Millisecond,
		ClockSkew:         time.Duration(cfg.GetClockSkewSeconds()) * time.Second,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize internal JWT middleware: %w", err)
	}
	return &InternalJWT{validator: validator}, nil
}

// RequirePrefix validates requests under prefix and leaves other routes public.
func (m *InternalJWT) RequirePrefix(prefix string) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		if !strings.HasPrefix(string(c.Request.URI().Path()), prefix) {
			c.Next(ctx)
			return
		}
		stripUntrustedIdentityHeaders(c)
		raw, err := internaljwt.BearerToken(string(c.GetHeader("Authorization")))
		if err != nil {
			abortUnauthorized(c)
			return
		}
		principal, err := m.validator.Validate(ctx, raw)
		if err != nil {
			abortUnauthorized(c)
			return
		}
		c.Next(internaljwt.WithPrincipal(ctx, principal))
	}
}

func PrincipalFromContext(ctx context.Context) (internaljwt.Principal, bool) {
	return internaljwt.PrincipalFromContext(ctx)
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
