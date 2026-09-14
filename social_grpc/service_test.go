package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	social "media_agent/social_grpc/kitex_gen/social_service"
)

func TestSearchContentsAggregatesProviders(t *testing.T) {
	service := newSocialService(mockXHSProvider{}, mockXHSProvider{})

	response, err := service.SearchContents(context.Background(), &social.SearchContentsRequest{
		OrganizationId: "G",
		Keyword:        "golang",
	})

	require.NoError(t, err)
	require.Len(t, response.Contents, 2)
	require.Equal(t, "xhs", response.Contents[0].Platform)
	require.Equal(t, "golang", response.Contents[0].SourceKeyword)
}

func TestSearchContentsRequiresOrganization(t *testing.T) {
	service := newSocialService(mockXHSProvider{})

	_, err := service.SearchContents(context.Background(), &social.SearchContentsRequest{})

	require.ErrorContains(t, err, "organization_id is required")
}
