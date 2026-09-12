FROM golang:1.26-alpine AS builder

RUN apk add --no-cache ca-certificates git
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=off
WORKDIR /workspace

COPY hertz_gen/go.mod hertz_gen/go.sum hertz_gen/
COPY hertz_infra/go.mod hertz_infra/go.sum hertz_infra/
COPY xhs_grpc/go.mod xhs_grpc/go.sum xhs_grpc/
RUN cd xhs_grpc && GOWORK=off go mod download

COPY hertz_gen/ hertz_gen/
COPY hertz_infra/ hertz_infra/
COPY xhs_grpc/ xhs_grpc/
RUN cd xhs_grpc && GOWORK=off CGO_ENABLED=0 go build -o /out/xhs_grpc .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/xhs_grpc /app/xhs_grpc
EXPOSE 8090
ENTRYPOINT ["/app/xhs_grpc"]
