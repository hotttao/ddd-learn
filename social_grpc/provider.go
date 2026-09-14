package main

import (
	"context"

	social "media_agent/social_grpc/kitex_gen/social_service"
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
