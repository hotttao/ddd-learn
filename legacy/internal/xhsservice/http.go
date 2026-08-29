package xhsservice

import (
	"context"
	"net/http"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/hotttao/ddd-learn/internal/security"
)

const (
	claimsContextKey = "identity.claims"
	tokenContextKey  = "identity.raw_token"
)

type TokenVerifier interface {
	Verify(ctx context.Context, raw string) (*security.Claims, error)
}

type Handler struct {
	service    *Service
	verifier   TokenVerifier
	authorizer Authorizer
}

func NewHandler(service *Service, verifier TokenVerifier, authorizer Authorizer) *Handler {
	return &Handler{service: service, verifier: verifier, authorizer: authorizer}
}

func (h *Handler) Register(router *server.Hertz) {
	router.GET("/healthz", h.health)
	api := router.Group("/v1", h.authenticate)
	api.POST("/crawl/tasks", h.startCrawl)
	api.GET("/crawl/contents", h.contents)
	api.GET("/crawl/keywords", h.keyword)
	api.PUT("/crawl/keywords", h.updateKeyword)
}

func (h *Handler) health(_ context.Context, c *app.RequestContext) {
	c.JSON(http.StatusOK, map[string]string{"status": "ok", "service": "xhs-service"})
}

func (h *Handler) authenticate(ctx context.Context, c *app.RequestContext) {
	raw, err := security.BearerToken(string(c.GetHeader("Authorization")))
	if err != nil {
		abortError(c, http.StatusUnauthorized, "invalid_token", err.Error())
		return
	}
	claims, err := h.verifier.Verify(ctx, raw)
	if err != nil {
		abortError(c, http.StatusUnauthorized, "invalid_token", err.Error())
		return
	}
	c.Set(claimsContextKey, claims)
	c.Set(tokenContextKey, raw)
	c.Next(ctx)
}

func (h *Handler) startCrawl(ctx context.Context, c *app.RequestContext) {
	claims, raw := identity(c)
	if !h.authorize(ctx, c, raw, claims, "crawl:start", "organization:"+claims.TenantID) {
		return
	}
	task := h.service.StartCrawl(claims.Subject)
	c.JSON(http.StatusAccepted, task)
}

func (h *Handler) contents(ctx context.Context, c *app.RequestContext) {
	claims, raw := identity(c)
	if !h.authorize(ctx, c, raw, claims, "content:read", "organization:"+claims.TenantID) {
		return
	}
	c.JSON(http.StatusOK, map[string]any{"items": h.service.Contents()})
}

func (h *Handler) keyword(ctx context.Context, c *app.RequestContext) {
	claims, raw := identity(c)
	if !h.authorize(ctx, c, raw, claims, "content:read", "organization:"+claims.TenantID) {
		return
	}
	c.JSON(http.StatusOK, map[string]string{"keyword": h.service.Keyword()})
}

func (h *Handler) updateKeyword(ctx context.Context, c *app.RequestContext) {
	claims, raw := identity(c)
	if !h.authorize(ctx, c, raw, claims, "keyword:update", "organization:"+claims.TenantID) {
		return
	}
	var request struct {
		Keyword string `json:"keyword"`
	}
	if err := c.BindAndValidate(&request); err != nil || strings.TrimSpace(request.Keyword) == "" {
		writeError(c, http.StatusBadRequest, "invalid_request", "keyword is required")
		return
	}
	keyword, err := h.service.UpdateKeyword(request.Keyword)
	if err != nil {
		writeError(c, http.StatusBadRequest, "invalid_keyword", err.Error())
		return
	}
	c.JSON(http.StatusOK, map[string]string{"keyword": keyword, "updated_by": claims.Subject})
}

func (h *Handler) authorize(ctx context.Context, c *app.RequestContext, raw string, claims *security.Claims, action, resource string) bool {
	decision, err := h.authorizer.Decide(ctx, raw, action, resource, claims.TenantID)
	if err != nil {
		writeError(c, http.StatusServiceUnavailable, "authorization_unavailable", err.Error())
		return false
	}
	if !decision.Allowed {
		writeError(c, http.StatusForbidden, "permission_denied", decision.Reason)
		return false
	}
	return true
}

func identity(c *app.RequestContext) (*security.Claims, string) {
	claimsValue, _ := c.Get(claimsContextKey)
	tokenValue, _ := c.Get(tokenContextKey)
	return claimsValue.(*security.Claims), tokenValue.(string)
}

func abortError(c *app.RequestContext, status int, code, message string) {
	c.AbortWithStatusJSON(status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func writeError(c *app.RequestContext, status int, code, message string) {
	c.JSON(status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
