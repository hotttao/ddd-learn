package main

import (
	"context"
	"time"

	"github.com/cloudwego/kitex/pkg/remote/trans/nphttp2/codes"
	kitexmetadata "github.com/cloudwego/kitex/pkg/remote/trans/nphttp2/metadata"
	"github.com/cloudwego/kitex/pkg/remote/trans/nphttp2/status"
)

const faultMetadataKey = "x-ddd-fault-mode"

func injectFault(ctx context.Context) error {
	metadata, ok := kitexmetadata.FromIncomingContext(ctx)
	if !ok {
		return nil
	}
	values := metadata.Get(faultMetadataKey)
	if len(values) == 0 {
		return nil
	}
	switch values[0] {
	case "unavailable":
		return status.Err(codes.Unavailable, "xhs test fault: unavailable")
	case "delay":
		timer := time.NewTimer(6 * time.Second)
		defer timer.Stop()
		select {
		case <-timer.C:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	default:
		return nil
	}
}
