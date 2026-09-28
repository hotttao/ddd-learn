FROM golang:1.26-alpine AS builder
RUN apk add --no-cache ca-certificates
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=off
WORKDIR /workspace
COPY token_manager/go.mod token_manager/
RUN cd token_manager && GOWORK=off go mod download
COPY token_manager/ token_manager/
RUN cd token_manager && GOWORK=off CGO_ENABLED=0 go build -o /out/token_manager .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/token_manager /app/token_manager
EXPOSE 8090
ENTRYPOINT ["/app/token_manager"]
