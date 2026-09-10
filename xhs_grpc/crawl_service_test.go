package main

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	serverjwt "media_agent/hertz_infra/serverhertz/jwt"
	crawl "media_agent/xhs_grpc/kitex_gen/xhs_service/crawl"
)

func TestListCrawlContentsFiltersByKeyword(t *testing.T) {
	service := newCrawlService()
	ctx := serverjwt.WithPrincipal(context.Background(), serverjwt.Principal{Subject: "alice"})

	response, err := service.ListCrawlContents(ctx, &crawl.ListCrawlContentsRequest{
		OrganizationId: "org-g",
		Keyword:        "kitex",
	})
	require.NoError(t, err)
	require.Len(t, response.GetContents(), 0)

	response, err = service.ListCrawlContents(ctx, &crawl.ListCrawlContentsRequest{
		OrganizationId: "org-g",
		Keyword:        "golang",
	})
	require.NoError(t, err)
	require.Len(t, response.GetContents(), 1)
	require.Equal(t, "xhs", response.GetContents()[0].GetPlatform())
}

func TestCrawlMethodsRequirePrincipal(t *testing.T) {
	service := newCrawlService()
	_, err := service.ListCrawlContents(context.Background(), &crawl.ListCrawlContentsRequest{OrganizationId: "org-g"})
	require.EqualError(t, err, "unauthenticated")
}
