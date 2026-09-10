package crawl

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestCrawlContentContractPreservesQueryAndContentFields(t *testing.T) {
	request := &ListCrawlContentsRequest{
		OrganizationId: "org-g",
		Keyword:        "golang",
	}
	content := &CrawlContent{
		Id:            "content-1",
		Title:         "Kitex xDS",
		SourceKeyword: "golang",
		Platform:      "xhs",
	}

	requestBytes, err := proto.Marshal(request)
	require.NoError(t, err)
	decodedRequest := new(ListCrawlContentsRequest)
	require.NoError(t, proto.Unmarshal(requestBytes, decodedRequest))
	require.True(t, proto.Equal(request, decodedRequest))

	contentBytes, err := proto.Marshal(content)
	require.NoError(t, err)
	decodedContent := new(CrawlContent)
	require.NoError(t, proto.Unmarshal(contentBytes, decodedContent))
	require.True(t, proto.Equal(content, decodedContent))
}
