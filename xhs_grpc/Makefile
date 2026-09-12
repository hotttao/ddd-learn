KITEX_GOCACHE ?= /tmp/ddd-learn-kitex-cache
KITEX_RUN := GOCACHE=$(KITEX_GOCACHE) go run github.com/cloudwego/kitex/tool/cmd/kitex
DESCRIPTOR_OUT := ../deployments/gateway/009_istio_proxyless/transcoder/xhs.pb

.PHONY: kitex-gen
# kitex-gen：从共享 XHS Protobuf 契约生成本服务的 Model、Client 和 Server。
kitex-gen:
	$(KITEX_RUN) -module media_agent/xhs_grpc -type protobuf -I ../idl \
		--protobuf 'Mxhs_service/crawl.proto=media_agent/xhs_grpc/kitex_gen/xhs_service/crawl' \
		../idl/xhs_service/crawl.proto
	$(KITEX_RUN) -module media_agent/xhs_grpc -type protobuf -I ../idl \
		--protobuf 'Mxhs_service/organization.proto=media_agent/xhs_grpc/kitex_gen/xhs_service/organization' \
		../idl/xhs_service/organization.proto
	$(KITEX_RUN) -module media_agent/xhs_grpc -type protobuf -I ../idl -I . \
		--protobuf 'Mhealth.proto=media_agent/xhs_grpc/kitex_gen/grpc/health/v1' \
		health.proto

.PHONY: test
test:
	GOWORK=off GOCACHE=/tmp/ddd-learn-xhs-grpc-cache go test ./kitex_gen/...

.PHONY: run
run:
	GOWORK=off GOCACHE=/tmp/ddd-learn-xhs-grpc-cache go run .

.PHONY: image
image:
	docker build -f Dockerfile -t xhs_grpc:0.0.1 ..

.PHONY: descriptor
# descriptor：从 XHS Protobuf IDL 生成包含 imports 的 Envoy Transcoder 描述文件。
descriptor:
	mkdir -p $(dir $(DESCRIPTOR_OUT))
	protoc -I ../idl \
		--include_imports \
		--include_source_info \
		--descriptor_set_out=$(DESCRIPTOR_OUT) \
		../idl/xhs_service/crawl.proto \
		../idl/xhs_service/organization.proto
