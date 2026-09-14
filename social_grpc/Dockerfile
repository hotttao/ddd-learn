FROM golang:1.26-alpine AS builder

RUN apk add --no-cache ca-certificates git
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=off
WORKDIR /workspace

COPY social_grpc/go.mod social_grpc/go.sum social_grpc/
COPY hertz_infra/go.mod hertz_infra/go.sum hertz_infra/
COPY hertz_gen/go.mod hertz_gen/go.sum hertz_gen/
RUN cd social_grpc && GOWORK=off go mod download

COPY hertz_gen/ hertz_gen/
COPY hertz_infra/ hertz_infra/
COPY social_grpc/ social_grpc/
RUN cd social_grpc && GOWORK=off CGO_ENABLED=0 go build -o /out/social_grpc .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/social_grpc /app/social_grpc
EXPOSE 8091
ENTRYPOINT ["/app/social_grpc"]
