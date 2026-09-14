package main

import (
	"context"

	"github.com/cloudwego/kitex/pkg/remote/trans/nphttp2/codes"
	"github.com/cloudwego/kitex/pkg/remote/trans/nphttp2/status"
	serverjwt "media_agent/hertz_infra/serverhertz/jwt"
	social "media_agent/social_grpc/kitex_gen/social_service"
)

type socialService struct {
	providers []contentProvider
}

func newSocialService(providers ...contentProvider) *socialService {
	return &socialService{providers: providers}
}

func (s *socialService) ListMyOrganizations(
	ctx context.Context,
	_ *social.ListMyOrganizationsRequest,
) (*social.ListMyOrganizationsResponse, error) {
	if _, ok := serverjwt.PrincipalFromContext(ctx); !ok {
		return nil, status.Err(codes.Unauthenticated, "unauthenticated")
	}
	// Organization membership will be read from the authenticated principal
	// and Keto in the integration step. This response is a stable mock for now.
	return &social.ListMyOrganizationsResponse{Organizations: []*social.OrganizationMembership{{
		Id:    "G",
		Roles: []string{"members"},
	}}}, nil
}

func (s *socialService) SearchContents(
	ctx context.Context,
	req *social.SearchContentsRequest,
) (*social.SearchContentsResponse, error) {
	if req.GetOrganizationId() == "" {
		return nil, status.Err(codes.InvalidArgument, "organization_id is required")
	}
	if len(s.providers) == 0 {
		return nil, status.Err(codes.Unavailable, "no content provider configured")
	}

	response := &social.SearchContentsResponse{}
	for _, provider := range s.providers {
		contents, err := provider.Search(ctx, req.GetOrganizationId(), req.GetKeyword())
		if err != nil {
			return nil, status.Errorf(codes.Unavailable, "search content provider: %v", err)
		}
		response.Contents = append(response.Contents, contents...)
	}
	return response, nil
}
