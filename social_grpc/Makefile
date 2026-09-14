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
