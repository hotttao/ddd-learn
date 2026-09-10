KITEX_GOCACHE ?= /tmp/ddd-learn-kitex-cache
KITEX_RUN := GOCACHE=$(KITEX_GOCACHE) go run github.com/cloudwego/kitex/tool/cmd/kitex

.PHONY: kitex-gen
# kitex-gen：从共享 XHS Protobuf 契约生成本服务的 Model、Client 和 Server。
kitex-gen:
	$(KITEX_RUN) -module media_agent/xhs_grpc -type protobuf -I ../idl \
		--protobuf 'Mxhs_service/crawl.proto=media_agent/xhs_grpc/kitex_gen/xhs_service/crawl' \
		../idl/xhs_service/crawl.proto
	$(KITEX_RUN) -module media_agent/xhs_grpc -type protobuf -I ../idl \
		--protobuf 'Mxhs_service/organization.proto=media_agent/xhs_grpc/kitex_gen/xhs_service/organization' \
		../idl/xhs_service/organization.proto

.PHONY: test
test:
	GOWORK=off GOCACHE=/tmp/ddd-learn-xhs-grpc-cache go test ./kitex_gen/...
