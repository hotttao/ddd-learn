FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git ca-certificates
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=off
WORKDIR /workspace
COPY hertz_gen/go.mod hertz_gen/go.sum hertz_gen/
COPY hertz_infra/go.mod hertz_infra/go.sum hertz_infra/
COPY xhs_service/go.mod xhs_service/go.sum xhs_service/
RUN cd xhs_service && GOWORK=off go mod download
COPY hertz_gen/ hertz_gen/
COPY hertz_infra/ hertz_infra/
COPY xhs_service/ xhs_service/
RUN cd xhs_service && GOWORK=off CGO_ENABLED=0 go build -o /out/xhs_service .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/xhs_service /app/xhs_service
COPY xhs_service/migrations/ /app/migrations/
COPY xhs_service/conf/ /app/conf/
COPY xhs_service/.env /app/.env
EXPOSE 8082
ENTRYPOINT ["/app/xhs_service"]
