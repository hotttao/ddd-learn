package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/app/server"
	"github.com/cloudwego/hertz/pkg/common/utils"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Config struct {
	ListenAddr  string
	DatabaseDSN string
	KratosURL   string
	TalosURL    string
}

type TokenManager struct {
	db        *pgxpool.Pool
	kratosURL string
	talosURL  string
	client    *http.Client
}

type tokenRequest struct {
	Name   string   `json:"name"`
	Scopes []string `json:"scopes"`
	TTL    string   `json:"ttl"`
}

type tokenRecord struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Scopes     []string  `json:"scopes"`
	Status     string    `json:"status"`
	ExpireTime time.Time `json:"expire_time"`
	CreatedAt  time.Time `json:"created_at"`
	TalosKeyID string    `json:"-"`
}

type kratosSession struct {
	Identity struct {
		ID string `json:"id"`
	} `json:"identity"`
}

type talosIssuedKey struct {
	KeyID      string    `json:"key_id"`
	Name       string    `json:"name"`
	Scopes     []string  `json:"scopes"`
	Status     string    `json:"status"`
	ExpireTime time.Time `json:"expire_time"`
}

type talosIssueResponse struct {
	IssuedAPIKey talosIssuedKey `json:"issued_api_key"`
	Secret       string         `json:"secret"`
}

type talosRotateResponse struct {
	IssuedAPIKey talosIssuedKey `json:"issued_api_key"`
	Secret       string         `json:"secret"`
}

func main() {
	cfg := Config{
		ListenAddr:  env("TOKEN_MANAGER_ADDR", ":8090"),
		DatabaseDSN: env("TOKEN_MANAGER_DATABASE_DSN", "postgres://token_manager:token_manager@postgres:5432/token_manager?sslmode=disable"),
		KratosURL:   strings.TrimRight(env("KRATOS_PUBLIC_URL", "http://kratos:4433"), "/"),
		TalosURL:    strings.TrimRight(env("TALOS_URL", "http://talos:4420"), "/"),
	}

	ctx := context.Background()
	db, err := pgxpool.New(ctx, cfg.DatabaseDSN)
	if err != nil {
		log.Fatalf("connect token manager database: %v", err)
	}
	defer db.Close()
	if err := db.Ping(ctx); err != nil {
		log.Fatalf("ping token manager database: %v", err)
	}
	if err := ensureSchema(ctx, db); err != nil {
		log.Fatalf("ensure token manager schema: %v", err)
	}

	m := &TokenManager{db: db, kratosURL: cfg.KratosURL, talosURL: cfg.TalosURL, client: &http.Client{Timeout: 5 * time.Second}}
	h := server.Default(server.WithHostPorts(cfg.ListenAddr))
	h.GET("/health/ready", m.health)
	h.POST("/v1/auth/tokens", m.create)
	h.GET("/v1/auth/tokens", m.list)
	// Hertz 路由参数以斜线结束；动作名称使用最后一段路径表达。
	h.POST("/v1/auth/tokens/:id/revoke", m.revoke)
	h.POST("/v1/auth/tokens/:id/rotate", m.rotate)

	log.Printf("token-manager: listening on %s", cfg.ListenAddr)
	h.Spin()
}

func (m *TokenManager) health(_ context.Context, c *app.RequestContext) {
	c.JSON(http.StatusOK, utils.H{"status": "ok"})
}

func (m *TokenManager) create(ctx context.Context, c *app.RequestContext) {
	owner, err := m.currentUser(c)
	if err != nil {
		writeError(c, http.StatusUnauthorized, err)
		return
	}
	var req tokenRequest
	if err := c.BindAndValidate(&req); err != nil || strings.TrimSpace(req.Name) == "" {
		writeError(c, http.StatusBadRequest, errors.New("name is required"))
		return
	}
	if req.TTL == "" {
		req.TTL = "720h"
	}
	var issued talosIssueResponse
	err = m.talosJSON(ctx, http.MethodPost, "/v2alpha1/admin/issuedApiKeys", map[string]any{
		"name": req.Name, "actor_id": "User:" + owner, "scopes": req.Scopes, "ttl": req.TTL,
	}, &issued)
	if err != nil {
		writeError(c, http.StatusBadGateway, err)
		return
	}
	record := tokenRecord{ID: newID(), Name: req.Name, Scopes: issued.IssuedAPIKey.Scopes, Status: issued.IssuedAPIKey.Status, ExpireTime: issued.IssuedAPIKey.ExpireTime, CreatedAt: time.Now().UTC(), TalosKeyID: issued.IssuedAPIKey.KeyID}
	if err := m.insert(ctx, owner, record); err != nil {
		writeError(c, http.StatusInternalServerError, err)
		return
	}
	// Secret is deliberately returned only in this response; it is not inserted into PostgreSQL.
	c.JSON(http.StatusCreated, utils.H{"token": record, "secret": issued.Secret})
}

func (m *TokenManager) list(ctx context.Context, c *app.RequestContext) {
	owner, err := m.currentUser(c)
	if err != nil {
		writeError(c, http.StatusUnauthorized, err)
		return
	}
	rows, err := m.db.Query(ctx, `SELECT id, name, scopes, status, expire_time, created_at, talos_key_id FROM api_tokens WHERE owner_identity_id=$1 ORDER BY created_at DESC`, owner)
	if err != nil {
		writeError(c, http.StatusInternalServerError, err)
		return
	}
	defer rows.Close()
	result := make([]tokenRecord, 0)
	for rows.Next() {
		var r tokenRecord
		if err := rows.Scan(&r.ID, &r.Name, &r.Scopes, &r.Status, &r.ExpireTime, &r.CreatedAt, &r.TalosKeyID); err != nil {
			writeError(c, http.StatusInternalServerError, err)
			return
		}
		result = append(result, r)
	}
	if err := rows.Err(); err != nil {
		writeError(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, utils.H{"tokens": result})
}

func (m *TokenManager) revoke(ctx context.Context, c *app.RequestContext) {
	owner, err := m.currentUser(c)
	if err != nil {
		writeError(c, http.StatusUnauthorized, err)
		return
	}
	r, err := m.findOwned(ctx, owner, c.Param("id"))
	if err != nil {
		writeError(c, http.StatusNotFound, err)
		return
	}
	if err := m.talosJSON(ctx, http.MethodPost, "/v2alpha1/admin/issuedApiKeys/"+r.TalosKeyID+":revoke", map[string]any{"reason": "REVOCATION_REASON_USER_REQUEST"}, nil); err != nil {
		writeError(c, http.StatusBadGateway, err)
		return
	}
	if _, err := m.db.Exec(ctx, `UPDATE api_tokens SET status='KEY_STATUS_REVOKED', updated_at=now() WHERE id=$1`, r.ID); err != nil {
		writeError(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusOK, utils.H{"id": r.ID, "status": "KEY_STATUS_REVOKED"})
}

func (m *TokenManager) rotate(ctx context.Context, c *app.RequestContext) {
	owner, err := m.currentUser(c)
	if err != nil {
		writeError(c, http.StatusUnauthorized, err)
		return
	}
	r, err := m.findOwned(ctx, owner, c.Param("id"))
	if err != nil {
		writeError(c, http.StatusNotFound, err)
		return
	}
	var rotated talosRotateResponse
	if err := m.talosJSON(ctx, http.MethodPost, "/v2alpha1/admin/issuedApiKeys/"+r.TalosKeyID+":rotate", map[string]any{}, &rotated); err != nil {
		writeError(c, http.StatusBadGateway, err)
		return
	}
	newRecord := tokenRecord{ID: newID(), Name: rotated.IssuedAPIKey.Name, Scopes: rotated.IssuedAPIKey.Scopes, Status: rotated.IssuedAPIKey.Status, ExpireTime: rotated.IssuedAPIKey.ExpireTime, CreatedAt: time.Now().UTC(), TalosKeyID: rotated.IssuedAPIKey.KeyID}
	if err := m.insert(ctx, owner, newRecord); err != nil {
		writeError(c, http.StatusInternalServerError, err)
		return
	}
	if _, err := m.db.Exec(ctx, `UPDATE api_tokens SET status='KEY_STATUS_REVOKED', updated_at=now() WHERE id=$1`, r.ID); err != nil {
		writeError(c, http.StatusInternalServerError, err)
		return
	}
	c.JSON(http.StatusCreated, utils.H{"token": newRecord, "secret": rotated.Secret, "replaced_token_id": r.ID})
}

func (m *TokenManager) currentUser(c *app.RequestContext) (string, error) {
	req, err := http.NewRequest(http.MethodGet, m.kratosURL+"/sessions/whoami", nil)
	if err != nil {
		return "", err
	}
	if cookie := string(c.Request.Header.Peek("Cookie")); cookie != "" {
		req.Header.Set("Cookie", cookie)
	}
	resp, err := m.client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("kratos session is not authenticated: status=%d", resp.StatusCode)
	}
	var session kratosSession
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return "", err
	}
	if session.Identity.ID == "" {
		return "", errors.New("kratos response has no identity id")
	}
	return session.Identity.ID, nil
}

func (m *TokenManager) talosJSON(ctx context.Context, method, path string, body any, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, m.talosURL+path, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := m.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("talos returned status=%d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

func (m *TokenManager) insert(ctx context.Context, owner string, r tokenRecord) error {
	_, err := m.db.Exec(ctx, `INSERT INTO api_tokens (id, owner_identity_id, talos_key_id, name, scopes, status, expire_time, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$8)`, r.ID, owner, r.TalosKeyID, r.Name, r.Scopes, r.Status, r.ExpireTime, r.CreatedAt)
	return err
}

func (m *TokenManager) findOwned(ctx context.Context, owner, id string) (tokenRecord, error) {
	var r tokenRecord
	err := m.db.QueryRow(ctx, `SELECT id, name, scopes, status, expire_time, created_at, talos_key_id FROM api_tokens WHERE id=$1 AND owner_identity_id=$2`, id, owner).Scan(&r.ID, &r.Name, &r.Scopes, &r.Status, &r.ExpireTime, &r.CreatedAt, &r.TalosKeyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, errors.New("token not found")
	}
	return r, err
}

func ensureSchema(ctx context.Context, db *pgxpool.Pool) error {
	_, err := db.Exec(ctx, `CREATE TABLE IF NOT EXISTS api_tokens (
		id text PRIMARY KEY, owner_identity_id text NOT NULL, talos_key_id text NOT NULL UNIQUE,
		name text NOT NULL, scopes text[] NOT NULL DEFAULT '{}', status text NOT NULL,
		expire_time timestamptz NOT NULL, created_at timestamptz NOT NULL, updated_at timestamptz NOT NULL
	)`)
	return err
}

func writeError(c *app.RequestContext, status int, err error) {
	c.JSON(status, utils.H{"error": err.Error()})
}
func newID() string { return fmt.Sprintf("tm_%d", time.Now().UnixNano()) }
func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}
