KITEX_GOCACHE ?= /tmp/ddd-learn-kitex-cache
KITEX_RUN := GOCACHE=$(KITEX_GOCACHE) go run github.com/cloudwego/kitex/tool/cmd/kitex
DESCRIPTOR_OUT := ../deployments/gateway/009_istio_proxyless/transcoder/xhs.pb
TRANSCODER_FILTER_OUT := ../deployments/gateway/009_istio_proxyless/ingress/xhs-grpc-transcoder.yaml

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

.PHONY: transcoder-filter
# transcoder-filter：将 descriptor 内联到只作用于 Istio Ingress 的 EnvoyFilter。
transcoder-filter: descriptor
	mkdir -p $(dir $(TRANSCODER_FILTER_OUT))
	{ \
		echo '# 第五步：在 Istio Ingress Envoy 中启用 XHS gRPC-JSON Transcoder。'; \
		echo '# 该文件由 make transcoder-filter 生成，不要手工修改 descriptor 内容。'; \
		echo '# apiVersion/kind：声明这是 Istio 的 EnvoyFilter，而不是 Gateway API 路由。'; \
		echo 'apiVersion: networking.istio.io/v1alpha3'; \
		echo 'kind: EnvoyFilter'; \
		echo 'metadata:'; \
		echo '  # metadata.name 是 EnvoyFilter 在 ddd-learn namespace 中的唯一名称。'; \
		echo '  name: xhs-grpc-json-transcoder'; \
		echo '  # 该配置只在 ddd-learn namespace 内生效。'; \
		echo '  namespace: ddd-learn'; \
		echo 'spec:'; \
		echo '  # 只匹配 Istio Gateway Controller 为 istio-ingress 创建的数据面 Pod。'; \
		echo '  # 因此 Ory、xhs Pod 和 Ambient ztunnel 不会加载这个 HTTP Filter。'; \
		echo '  workloadSelector:'; \
		echo '    labels:'; \
		echo '      gateway.networking.k8s.io/gateway-name: istio-ingress'; \
		echo '  # 对匹配到的 Gateway Envoy 修改 HTTP Filter Chain。'; \
		echo '  configPatches:'; \
		echo '    # HTTP_FILTER 表示修改 HTTP Connection Manager 中的 HTTP 层过滤器。'; \
		echo '    - applyTo: HTTP_FILTER'; \
		echo '      match:'; \
		echo '        # 只匹配 Gateway 上下文，不影响 sidecar 或其他 mesh listener。'; \
		echo '        context: GATEWAY'; \
		echo '        listener:'; \
		echo '          filterChain:'; \
		echo '            filter:'; \
		echo '              # 目标是 HTTP Connection Manager，它承载 HTTP/HTTP2 路由和过滤器。'; \
		echo '              name: envoy.filters.network.http_connection_manager'; \
		echo '              subFilter:'; \
		echo '                # 以 router 为锚点，保证 Transcoder 插入到最终路由转发之前。'; \
		echo '                name: envoy.filters.http.router'; \
		echo '      patch:'; \
		echo '        # INSERT_BEFORE 确保 JSON 请求先被转换成 gRPC，再交给 router。'; \
		echo '        operation: INSERT_BEFORE'; \
		echo '        value:'; \
		echo '          # Envoy 内置的 gRPC-JSON Transcoder HTTP Filter。'; \
		echo '          name: envoy.filters.http.grpc_json_transcoder'; \
		echo '          typed_config:'; \
		echo '            "@type": type.googleapis.com/envoy.extensions.filters.http.grpc_json_transcoder.v3.GrpcJsonTranscoder'; \
		echo '            # descriptor 的 base64 内容由 deployments/.../transcoder/xhs.pb 生成。'; \
		echo '            # 它包含 service、RPC、message 以及 google.api.http 映射。'; \
		echo '            proto_descriptor_bin: ' $$(base64 -w0 $(DESCRIPTOR_OUT)); \
		echo '            # 只开放 XHS 的两个 service，避免该 Filter 处理无关 protobuf service。'; \
		echo '            services:'; \
		echo '              - xhs_service.crawl.CrawlService'; \
		echo '              - xhs_service.organization.OrganizationService'; \
		echo '            # 保留 proto 字段原名，例如 organization_id，而不是转换成 organizationId。'; \
		echo '            print_options:'; \
		echo '              preserve_proto_field_names: true'; \
	} > $(TRANSCODER_FILTER_OUT)
