package main

import (
	"context"

	"github.com/cloudwego/kitex/client"
	kitexmetadata "github.com/cloudwego/kitex/pkg/remote/trans/nphttp2/metadata"
	"github.com/cloudwego/kitex/transport"
	"github.com/kitex-contrib/xds/xdssuite"
	social "media_agent/social_grpc/kitex_gen/social_service"
	crawl "media_agent/xhs_grpc/kitex_gen/xhs_service/crawl"
	crawlservice "media_agent/xhs_grpc/kitex_gen/xhs_service/crawl/crawlservice"
)

// contentProvider hides the platform-specific API from the Social aggregate.
// A real XHSProvider can replace mockXHSProvider without changing the RPC layer.
type contentProvider interface {
	Search(ctx context.Context, organizationID, keyword string) ([]*social.SocialContent, error)
}

// mockXHSProvider is deliberately local to this learning step. It models the
// response shape of XHS while xDS and the real downstream Kitex client remain
// separate later steps.
type mockXHSProvider struct{}

func (mockXHSProvider) Search(_ context.Context, organizationID, keyword string) ([]*social.SocialContent, error) {
	return []*social.SocialContent{{
		Id:            "xhs-mock-1",
		Title:         "Social Provider Aggregation",
		SourceKeyword: keyword,
		Platform:      "xhs",
	}}, nil
}

type xhsProvider struct {
	client crawlservice.Client
}

func newXHSProvider(address string) (*xhsProvider, error) {
	xhsClient, err := crawlservice.NewClient(
		address,
		client.WithTransportProtocol(transport.GRPC),
		xdssuite.NewClientOption(),
	)
	if err != nil {
		return nil, err
	}
	return &xhsProvider{client: xhsClient}, nil
}

func (p *xhsProvider) Search(ctx context.Context, organizationID, keyword string) ([]*social.SocialContent, error) {
	ctx = forwardAuthorization(ctx)
	response, err := p.client.ListCrawlContents(ctx, &crawl.ListCrawlContentsRequest{
		OrganizationId: organizationID,
		Keyword:        keyword,
	})
	if err != nil {
		return nil, err
	}
	contents := make([]*social.SocialContent, 0, len(response.GetContents()))
	for _, item := range response.GetContents() {
		contents = append(contents, &social.SocialContent{
			Id:            item.GetId(),
			Title:         item.GetTitle(),
			SourceKeyword: item.GetSourceKeyword(),
			Platform:      item.GetPlatform(),
		})
	}
	return contents, nil
}

// forwardAuthorization carries the gateway-issued internal JWT to the next
// Kitex hop. Social has already validated this token; XHS validates the same
// token again at its own service boundary.
func forwardAuthorization(ctx context.Context) context.Context {
	incoming, ok := kitexmetadata.FromIncomingContext(ctx)
	if !ok {
		return ctx
	}
	authorization := incoming.Get("authorization")
	if len(authorization) == 0 {
		return ctx
	}
	return kitexmetadata.AppendToOutgoingContext(ctx, "authorization", authorization[0])
}
