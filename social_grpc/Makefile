KITEX_GOCACHE ?= /tmp/ddd-learn-social-grpc-kitex-cache
KITEX_RUN := GOCACHE=$(KITEX_GOCACHE) go run github.com/cloudwego/kitex/tool/cmd/kitex

.PHONY: kitex-gen
# 从共享 Social Protobuf 契约生成 Model、Client 和 Server。
kitex-gen:
	$(KITEX_RUN) -module media_agent/social_grpc -type protobuf -I ../idl \
		--protobuf 'Msocial_service/social.proto=media_agent/social_grpc/kitex_gen/social_service' \
		../idl/social_service/social.proto

.PHONY: test
test:
	GOWORK=off GOCACHE=/tmp/ddd-learn-social-grpc-cache go test ./...

.PHONY: image
# 镜像构建使用仓库根目录作为 Docker context，以复制 social_grpc 的完整模块。
image:
	docker build -f Dockerfile -t social_grpc:0.0.1 ..

DESCRIPTOR_OUT := ../deployments/gateway/009_istio_proxyless/transcoder/social.pb
TRANSCODER_FILTER_OUT := ../deployments/gateway/009_istio_proxyless/ingress/social-grpc-transcoder.yaml

.PHONY: descriptor
# descriptor：生成 Social HTTP/JSON 转 gRPC 所需的 Protobuf 描述文件。
descriptor:
	mkdir -p $(dir $(DESCRIPTOR_OUT))
	protoc -I ../idl \
		--include_imports \
		--include_source_info \
		--descriptor_set_out=$(DESCRIPTOR_OUT) \
		../idl/social_service/social.proto

.PHONY: transcoder-filter
# transcoder-filter：生成只作用于 Istio Ingress Gateway 的 Social Transcoder EnvoyFilter。
transcoder-filter: descriptor
	mkdir -p $(dir $(TRANSCODER_FILTER_OUT))
	{ \
		echo '# 第六步：在 Istio Ingress Envoy 中启用 Social gRPC-JSON Transcoder。'; \
		echo '# 该文件由 make transcoder-filter 生成，不要手工修改 descriptor 内容。'; \
		echo 'apiVersion: networking.istio.io/v1alpha3'; \
		echo 'kind: EnvoyFilter'; \
		echo 'metadata:'; \
		echo '  name: social-grpc-json-transcoder'; \
		echo '  namespace: ddd-learn-proxyless'; \
		echo 'spec:'; \
		echo '  workloadSelector:'; \
		echo '    labels:'; \
		echo '      gateway.networking.k8s.io/gateway-name: istio-ingress'; \
		echo '  configPatches:'; \
		echo '    - applyTo: HTTP_FILTER'; \
		echo '      match:'; \
		echo '        context: GATEWAY'; \
		echo '        listener:'; \
		echo '          filterChain:'; \
		echo '            filter:'; \
		echo '              name: envoy.filters.network.http_connection_manager'; \
		echo '              subFilter:'; \
		echo '                name: envoy.filters.http.router'; \
		echo '      patch:'; \
		echo '        operation: INSERT_BEFORE'; \
		echo '        value:'; \
		echo '          name: envoy.filters.http.grpc_json_transcoder'; \
		echo '          typed_config:'; \
		echo '            "@type": type.googleapis.com/envoy.extensions.filters.http.grpc_json_transcoder.v3.GrpcJsonTranscoder'; \
		echo '            proto_descriptor_bin: ' $$(base64 -w0 $(DESCRIPTOR_OUT)); \
		echo '            services:'; \
		echo '              - social_service.SocialService'; \
		echo '            match_incoming_request_route: true'; \
		echo '            print_options:'; \
		echo '              preserve_proto_field_names: true'; \
	} > $(TRANSCODER_FILTER_OUT)
