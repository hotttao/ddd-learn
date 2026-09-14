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
