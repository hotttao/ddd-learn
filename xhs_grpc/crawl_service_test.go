package main

import (
	"context"
	"testing"

	"github.com/cloudwego/kitex/pkg/remote/trans/nphttp2/codes"
	"github.com/cloudwego/kitex/pkg/remote/trans/nphttp2/status"
	"github.com/stretchr/testify/require"
	serverjwt "media_agent/hertz_infra/serverhertz/jwt"
	crawl "media_agent/xhs_grpc/kitex_gen/xhs_service/crawl"
)

type fakePermissionChecker struct {
	allowed  bool
	relation string
}

func (f *fakePermissionChecker) check(_ context.Context, _, _, _, relation string) (bool, error) {
	f.relation = relation
	return f.allowed, nil
}

func TestListCrawlContentsFiltersByKeyword(t *testing.T) {
	permissions := &fakePermissionChecker{allowed: true}
	service := newCrawlService(permissions)
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
	require.Equal(t, "view_content", permissions.relation)
}

func TestCrawlMethodsRequirePrincipal(t *testing.T) {
	service := newCrawlService(&fakePermissionChecker{allowed: true})
	_, err := service.ListCrawlContents(context.Background(), &crawl.ListCrawlContentsRequest{OrganizationId: "org-g"})
	require.Equal(t, codes.Unauthenticated, status.Code(err))
	require.ErrorContains(t, err, "unauthenticated")
}

func TestUpdateKeywordsRequiresModifyPermission(t *testing.T) {
	permissions := &fakePermissionChecker{allowed: false}
	service := newCrawlService(permissions)
	ctx := serverjwt.WithPrincipal(context.Background(), serverjwt.Principal{Subject: "bob"})

	_, err := service.UpdateKeywords(ctx, &crawl.UpdateKeywordsRequest{
		OrganizationId: "G",
		Keywords:       &crawl.KeywordInput{Values: []string{"istio"}},
	})

	require.ErrorContains(t, err, "permission denied")
	require.Equal(t, codes.PermissionDenied, status.Code(err))
	require.Equal(t, "modify_keywords", permissions.relation)
}
