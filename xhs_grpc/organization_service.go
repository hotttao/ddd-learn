package main

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

	"github.com/cloudwego/kitex/pkg/remote/trans/nphttp2/codes"
	"github.com/cloudwego/kitex/pkg/remote/trans/nphttp2/status"

	serverjwt "media_agent/hertz_infra/serverhertz/jwt"
	organization "media_agent/xhs_grpc/kitex_gen/xhs_service/organization"
)

const maxKetoResponseSize = 1 << 20

type membership struct {
	organizationID string
	role           string
}

type membershipReader interface {
	listMemberships(context.Context, string) ([]membership, error)
}

type organizationService struct {
	memberships membershipReader
}

func (s *organizationService) ListMyOrganizations(
	ctx context.Context,
	_ *organization.ListMyOrganizationsRequest,
) (*organization.ListMyOrganizationsResponse, error) {
	principal, ok := serverjwt.PrincipalFromContext(ctx)
	if !ok || principal.Subject == "" {
		return nil, status.Err(codes.Unauthenticated, "unauthenticated")
	}

	memberships, err := s.memberships.listMemberships(ctx, "User:"+principal.Subject)
	if err != nil {
		return nil, status.Errorf(codes.Unavailable, "list organization memberships: %v", err)
	}

	rolesByOrganization := make(map[string]map[string]struct{})
	for _, item := range memberships {
		roles := rolesByOrganization[item.organizationID]
		if roles == nil {
			roles = make(map[string]struct{})
			rolesByOrganization[item.organizationID] = roles
		}
		roles[item.role] = struct{}{}
	}

	organizationIDs := make([]string, 0, len(rolesByOrganization))
	for organizationID := range rolesByOrganization {
		organizationIDs = append(organizationIDs, organizationID)
	}
	sort.Strings(organizationIDs)

	response := &organization.ListMyOrganizationsResponse{}
	for _, organizationID := range organizationIDs {
		roles := make([]string, 0, len(rolesByOrganization[organizationID]))
		for role := range rolesByOrganization[organizationID] {
			roles = append(roles, role)
		}
		sort.Strings(roles)
		response.Organizations = append(response.Organizations, &organization.OrganizationMembership{
			Id:    organizationID,
			Roles: roles,
		})
	}
	return response, nil
}

type ketoMembershipReader struct {
	checkEndpoint         string
	relationshipsEndpoint string
	client                *http.Client
}

func newKetoMembershipReader(baseURL string) (*ketoMembershipReader, error) {
	base, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return nil, fmt.Errorf("invalid Keto read URL %q", baseURL)
	}
	return &ketoMembershipReader{
		checkEndpoint:         strings.TrimRight(base.String(), "/") + "/relation-tuples/check/openapi",
		relationshipsEndpoint: strings.TrimRight(base.String(), "/") + "/relation-tuples",
		client:                &http.Client{Timeout: 2 * time.Second},
	}, nil
}

func (r *ketoMembershipReader) check(
	ctx context.Context,
	subject, namespace, object, relation string,
) (bool, error) {
	payload, err := json.Marshal(map[string]string{
		"subject_id": subject,
		"namespace":  namespace,
		"object":     object,
		"relation":   relation,
	})
	if err != nil {
		return false, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.checkEndpoint, bytes.NewReader(payload))
	if err != nil {
		return false, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxKetoResponseSize))
		return false, fmt.Errorf("Keto returned HTTP %d", resp.StatusCode)
	}
	var result struct {
		Allowed bool `json:"allowed"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxKetoResponseSize)).Decode(&result); err != nil {
		return false, err
	}
	return result.Allowed, nil
}

func (r *ketoMembershipReader) listMemberships(ctx context.Context, subject string) ([]membership, error) {
	result := make([]membership, 0)
	pageToken := ""
	seenPageTokens := make(map[string]struct{})

	for {
		endpoint, err := url.Parse(r.relationshipsEndpoint)
		if err != nil {
			return nil, err
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
			return nil, err
		}
		resp, err := r.client.Do(req)
		if err != nil {
			return nil, err
		}

		var page struct {
			RelationTuples []struct {
				Namespace string `json:"namespace"`
				Object    string `json:"object"`
				Relation  string `json:"relation"`
				SubjectID string `json:"subject_id"`
			} `json:"relation_tuples"`
			NextPageToken string `json:"next_page_token"`
		}
		if resp.StatusCode != http.StatusOK {
			_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxKetoResponseSize))
			_ = resp.Body.Close()
			return nil, fmt.Errorf("Keto returned HTTP %d", resp.StatusCode)
		}
		decodeErr := json.NewDecoder(io.LimitReader(resp.Body, maxKetoResponseSize)).Decode(&page)
		_ = resp.Body.Close()
		if decodeErr != nil {
			return nil, decodeErr
		}

		for _, tuple := range page.RelationTuples {
			if tuple.Namespace != "Organization" || tuple.SubjectID != subject {
				continue
			}
			if tuple.Relation != "members" && tuple.Relation != "admins" {
				continue
			}
			result = append(result, membership{organizationID: tuple.Object, role: tuple.Relation})
		}

		pageToken = page.NextPageToken
		if pageToken == "" {
			return result, nil
		}
		if _, exists := seenPageTokens[pageToken]; exists {
			return nil, fmt.Errorf("Keto returned repeated page token")
		}
		seenPageTokens[pageToken] = struct{}{}
	}
}
