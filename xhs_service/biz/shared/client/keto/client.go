package keto

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	configpb "media_agent/hertz_gen/config"
	domain "media_agent/xhs_service/biz/domain/organization"
)

const maxResponseSize = 1 << 20

type Client struct {
	checkEndpoint         string
	relationshipsEndpoint string
	http                  *http.Client
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
		checkEndpoint:         strings.TrimRight(base.String(), "/") + "/relation-tuples/check/openapi",
		relationshipsEndpoint: strings.TrimRight(base.String(), "/") + "/relation-tuples",
		http:                  &http.Client{Timeout: timeout},
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
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.checkEndpoint, bytes.NewReader(payload))
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

func (c *Client) ListMemberships(ctx context.Context, subject string) ([]domain.Membership, error) {
	memberships := make([]domain.Membership, 0)
	pageToken := ""
	seenPageTokens := make(map[string]struct{})

	for {
		endpoint, err := url.Parse(c.relationshipsEndpoint)
		if err != nil {
			return nil, fmt.Errorf("keto client: parse relationships endpoint: %w", err)
		}
		query := endpoint.Query()
		query.Set("namespace", "Organization")
		query.Set("subject_id", subject)
		query.Set("page_size", "250")
		if pageToken != "" {
			query.Set("page_token", pageToken)
		}
		endpoint.RawQuery = query.Encode()

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
		if err != nil {
			return nil, fmt.Errorf("keto client: create relationships request: %w", err)
		}
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, fmt.Errorf("keto client: list memberships: %w", err)
		}
		if resp.StatusCode != http.StatusOK {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxResponseSize))
			resp.Body.Close()
			return nil, fmt.Errorf("keto client: list memberships returned HTTP %d", resp.StatusCode)
		}

		var result struct {
			RelationTuples []struct {
				Object    string `json:"object"`
				Relation  string `json:"relation"`
				SubjectID string `json:"subject_id"`
				Namespace string `json:"namespace"`
			} `json:"relation_tuples"`
			NextPageToken string `json:"next_page_token"`
		}
		decodeErr := json.NewDecoder(io.LimitReader(resp.Body, maxResponseSize)).Decode(&result)
		resp.Body.Close()
		if decodeErr != nil {
			return nil, fmt.Errorf("keto client: decode memberships response: %w", decodeErr)
		}
		for _, tuple := range result.RelationTuples {
			if tuple.Namespace != "Organization" || tuple.SubjectID != subject {
				continue
			}
			if tuple.Relation != "members" && tuple.Relation != "admins" {
				continue
			}
			memberships = append(memberships, domain.Membership{OrganizationID: tuple.Object, Role: tuple.Relation})
		}

		pageToken = result.NextPageToken
		if pageToken == "" {
			break
		}
		if _, exists := seenPageTokens[pageToken]; exists {
			return nil, fmt.Errorf("keto client: repeated relationships page token")
		}
		seenPageTokens[pageToken] = struct{}{}
	}

	sort.Slice(memberships, func(i, j int) bool {
		if memberships[i].OrganizationID == memberships[j].OrganizationID {
			return memberships[i].Role < memberships[j].Role
		}
		return memberships[i].OrganizationID < memberships[j].OrganizationID
	})
	return memberships, nil
}
