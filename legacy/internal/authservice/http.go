package authservice

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/protocol"
	"github.com/hotttao/ddd-learn/internal/security"
)

const sessionCookieName = "ory_kratos_session"

type Handler struct {
	service      *Service
	cookieSecure bool
}

func NewHandler(service *Service, cookieSecure bool) *Handler {
	return &Handler{service: service, cookieSecure: cookieSecure}
}

func (h *Handler) Register(router *server.Hertz) {
	router.GET("/healthz", h.health)
	router.POST("/v1/login", h.login)
	router.GET("/sessions/whoami", h.whoami)
	router.POST("/internal/tokens", h.internalToken)
	router.GET("/.well-known/jwks.json", h.jwks)
	router.POST("/v1/authorize", h.authorize)
}

func (h *Handler) health(_ context.Context, c *app.RequestContext) {
	c.JSON(http.StatusOK, map[string]string{"status": "ok", "service": "auth-service"})
}

func (h *Handler) login(_ context.Context, c *app.RequestContext) {
	var request struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := c.BindAndValidate(&request); err != nil {
		writeError(c, http.StatusBadRequest, "invalid_request", "username and password are required")
		return
	}
	session, err := h.service.Login(request.Username, request.Password)
	if err != nil {
		writeError(c, http.StatusUnauthorized, "invalid_credentials", err.Error())
		return
	}
	maxAge := int(time.Until(session.ExpiresAt).Seconds())
	c.SetCookie(sessionCookieName, session.Token, maxAge, "/", "", protocol.CookieSameSiteLaxMode, h.cookieSecure, true)
	c.JSON(http.StatusOK, map[string]any{"session": session, "session_token": session.Token})
}

func (h *Handler) whoami(_ context.Context, c *app.RequestContext) {
	session, err := h.sessionFromRequest(c)
	if err != nil {
		writeError(c, http.StatusUnauthorized, "invalid_session", err.Error())
		return
	}
	c.JSON(http.StatusOK, session)
}

func (h *Handler) internalToken(_ context.Context, c *app.RequestContext) {
	session, err := h.sessionFromRequest(c)
	if err != nil {
		writeError(c, http.StatusUnauthorized, "invalid_session", err.Error())
		return
	}
	raw, expiresAt, err := h.service.IssueInternalToken(session)
	if err != nil {
		writeError(c, http.StatusInternalServerError, "token_issue_failed", "could not issue internal token")
		return
	}
	c.JSON(http.StatusOK, map[string]any{
		"access_token": raw, "token_type": "Bearer", "expires_in": int(time.Until(expiresAt).Seconds()),
	})
}

func (h *Handler) jwks(_ context.Context, c *app.RequestContext) {
	c.JSON(http.StatusOK, h.service.JWKS())
}

func (h *Handler) authorize(_ context.Context, c *app.RequestContext) {
	raw, err := security.BearerToken(string(c.GetHeader("Authorization")))
	if err != nil {
		writeError(c, http.StatusUnauthorized, "invalid_token", err.Error())
		return
	}
	claims, err := h.service.VerifyInternalToken(raw)
	if err != nil {
		writeError(c, http.StatusUnauthorized, "invalid_token", err.Error())
		return
	}
	var request struct {
		Action   string `json:"action"`
		Resource string `json:"resource"`
		TenantID string `json:"tenant_id"`
	}
	if err := c.BindAndValidate(&request); err != nil || strings.TrimSpace(request.Action) == "" || strings.TrimSpace(request.Resource) == "" {
		writeError(c, http.StatusBadRequest, "invalid_request", "action, resource and tenant_id are required")
		return
	}
	decision := h.service.Authorize(claims, request.Action, request.TenantID)
	c.JSON(http.StatusOK, decision)
}

func (h *Handler) sessionFromRequest(c *app.RequestContext) (Session, error) {
	token := string(c.Cookie(sessionCookieName))
	if token == "" {
		token = string(c.GetHeader("X-Session-Token"))
	}
	if token == "" {
		return Session{}, errors.New("session cookie or X-Session-Token is required")
	}
	return h.service.Session(token)
}

func writeError(c *app.RequestContext, status int, code, message string) {
	c.JSON(status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}
