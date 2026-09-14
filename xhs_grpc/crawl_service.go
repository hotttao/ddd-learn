package main

import (
	"context"
	"strings"

	"github.com/cloudwego/kitex/pkg/remote/trans/nphttp2/codes"
	"github.com/cloudwego/kitex/pkg/remote/trans/nphttp2/status"

	serverjwt "media_agent/hertz_infra/serverhertz/jwt"
	crawl "media_agent/xhs_grpc/kitex_gen/xhs_service/crawl"
)

type crawlService struct {
	contents    []content
	keywords    []string
	permissions permissionChecker
}

type permissionChecker interface {
	check(context.Context, string, string, string, string) (bool, error)
}

type content struct {
	id            string
	title         string
	sourceKeyword string
	platform      string
}

func newCrawlService(permissions permissionChecker) *crawlService {
	return &crawlService{
		permissions: permissions,
		keywords:    []string{"golang", "kitex"},
		contents: []content{{
			id: "xhs-1", title: "Kitex Proxyless xDS", sourceKeyword: "golang", platform: "xhs",
		}},
	}
}

func (s *crawlService) StartCrawlTask(ctx context.Context, req *crawl.StartCrawlTaskRequest) (*crawl.StartCrawlTaskResponse, error) {
	if err := s.authorize(ctx, req.GetOrganizationId(), "start_crawl"); err != nil {
		return nil, err
	}
	if req.GetOrganizationId() == "" || len(req.GetTask().GetKeywords()) == 0 {
		return nil, status.Err(codes.InvalidArgument, "organization_id and keywords are required")
	}
	return &crawl.StartCrawlTaskResponse{TaskId: "task-mock-1", Status: "pending"}, nil
}

func (s *crawlService) ListCrawlContents(ctx context.Context, req *crawl.ListCrawlContentsRequest) (*crawl.ListCrawlContentsResponse, error) {
	if err := s.authorize(ctx, req.GetOrganizationId(), "view_content"); err != nil {
		return nil, err
	}
	if req.GetOrganizationId() == "" {
		return nil, status.Err(codes.InvalidArgument, "organization_id is required")
	}
	response := &crawl.ListCrawlContentsResponse{}
	for _, item := range s.contents {
		if req.GetKeyword() != "" && !strings.EqualFold(req.GetKeyword(), item.sourceKeyword) {
			continue
		}
		response.Contents = append(response.Contents, &crawl.CrawlContent{
			Id: item.id, Title: item.title, SourceKeyword: item.sourceKeyword, Platform: item.platform,
		})
	}
	return response, nil
}

func (s *crawlService) GetKeywords(ctx context.Context, req *crawl.GetKeywordsRequest) (*crawl.GetKeywordsResponse, error) {
	if err := s.authorize(ctx, req.GetOrganizationId(), "view_content"); err != nil {
		return nil, err
	}
	return &crawl.GetKeywordsResponse{Keywords: append([]string(nil), s.keywords...)}, nil
}

func (s *crawlService) UpdateKeywords(ctx context.Context, req *crawl.UpdateKeywordsRequest) (*crawl.UpdateKeywordsResponse, error) {
	if err := s.authorize(ctx, req.GetOrganizationId(), "modify_keywords"); err != nil {
		return nil, err
	}
	if req.GetOrganizationId() == "" {
		return nil, status.Err(codes.InvalidArgument, "organization_id is required")
	}
	s.keywords = append([]string(nil), req.GetKeywords().GetValues()...)
	return &crawl.UpdateKeywordsResponse{Keywords: append([]string(nil), s.keywords...)}, nil
}

func (s *crawlService) authorize(ctx context.Context, organizationID, permission string) error {
	principal, ok := serverjwt.PrincipalFromContext(ctx)
	if !ok || principal.Subject == "" {
		return status.Err(codes.Unauthenticated, "unauthenticated")
	}
	allowed, err := s.permissions.check(ctx, "User:"+principal.Subject, "Organization", organizationID, permission)
	if err != nil {
		return status.Errorf(codes.Unavailable, "check %s permission: %v", permission, err)
	}
	if !allowed {
		return status.Errorf(codes.PermissionDenied, "permission denied: %s", permission)
	}
	return nil
}
