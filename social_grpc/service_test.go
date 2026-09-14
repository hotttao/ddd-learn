package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	serverjwt "media_agent/hertz_infra/serverhertz/jwt"
	social "media_agent/social_grpc/kitex_gen/social_service"
)

func TestSearchContentsAggregatesProviders(t *testing.T) {
	service := newSocialService(mockXHSProvider{}, mockXHSProvider{})

	ctx := serverjwt.WithPrincipal(context.Background(), serverjwt.Principal{Subject: "alice-id"})
	response, err := service.SearchContents(ctx, &social.SearchContentsRequest{
		OrganizationId: "G",
		Keyword:        "golang",
	})

	require.NoError(t, err)
	require.Len(t, response.Contents, 2)
	require.Equal(t, "xhs", response.Contents[0].Platform)
	require.Equal(t, "golang", response.Contents[0].SourceKeyword)
}

func TestListMyOrganizationsRequiresPrincipal(t *testing.T) {
	service := newSocialService(mockXHSProvider{})

	_, err := service.ListMyOrganizations(context.Background(), &social.ListMyOrganizationsRequest{})

	require.ErrorContains(t, err, "unauthenticated")
}

func TestSearchContentsRequiresOrganization(t *testing.T) {
	service := newSocialService(mockXHSProvider{})

	_, err := service.SearchContents(context.Background(), &social.SearchContentsRequest{})

	require.ErrorContains(t, err, "organization_id is required")
}
