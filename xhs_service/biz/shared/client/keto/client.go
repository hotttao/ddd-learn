package keto

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	configpb "media_agent/hertz_gen/config"
)

const maxResponseSize = 1 << 20

type Client struct {
	endpoint string
	http     *http.Client
}

func New(cfg *configpb.KetoConfig) (*Client, error) {
	if cfg == nil || !cfg.GetEnabled() {
		return nil, fmt.Errorf("keto client: configuration is disabled")
	}
	base, err := url.Parse(cfg.GetReadUrl())
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return nil, fmt.Errorf("keto client: invalid read URL %q", cfg.GetReadUrl())
	}
	timeout := time.Duration(cfg.GetRequestTimeoutMs()) * time.Millisecond
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &Client{
		endpoint: strings.TrimRight(base.String(), "/") + "/relation-tuples/check/openapi",
		http:     &http.Client{Timeout: timeout},
	}, nil
}

func (c *Client) Check(ctx context.Context, subject, namespace, object, relation string) (bool, error) {
	payload, err := json.Marshal(map[string]string{
		"namespace":  namespace,
		"object":     object,
		"relation":   relation,
		"subject_id": subject,
	})
	if err != nil {
		return false, fmt.Errorf("keto client: encode check request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return false, fmt.Errorf("keto client: create check request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return false, fmt.Errorf("keto client: check permission: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseSize))
		return false, fmt.Errorf("keto client: check permission returned HTTP %d", resp.StatusCode)
	}
	var result struct {
		Allowed bool `json:"allowed"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseSize)).Decode(&result); err != nil {
		return false, fmt.Errorf("keto client: decode check response: %w", err)
	}
	return result.Allowed, nil
}
