package xhsservice

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

type Decision struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason"`
}

type Authorizer interface {
	Decide(ctx context.Context, token, action, resource, tenantID string) (Decision, error)
}

type HTTPAuthorizer struct {
	url    string
	client *http.Client
}

func NewHTTPAuthorizer(url string, client *http.Client) *HTTPAuthorizer {
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	return &HTTPAuthorizer{url: url, client: client}
}

func (a *HTTPAuthorizer) Decide(ctx context.Context, token, action, resource, tenantID string) (Decision, error) {
	body, err := json.Marshal(map[string]string{"action": action, "resource": resource, "tenant_id": tenantID})
	if err != nil {
		return Decision{}, fmt.Errorf("encode authorization request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.url, bytes.NewReader(body))
	if err != nil {
		return Decision{}, fmt.Errorf("create authorization request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Workload-Identity", "xhs-service")
	resp, err := a.client.Do(req)
	if err != nil {
		return Decision{}, fmt.Errorf("call authorization service: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Decision{}, fmt.Errorf("authorization service returned status %d", resp.StatusCode)
	}
	var decision Decision
	if err := json.NewDecoder(resp.Body).Decode(&decision); err != nil {
		return Decision{}, fmt.Errorf("decode authorization decision: %w", err)
	}
	return decision, nil
}
