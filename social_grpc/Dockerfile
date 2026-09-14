FROM golang:1.26-alpine AS builder

RUN apk add --no-cache ca-certificates git
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=off
WORKDIR /workspace

COPY social_grpc/go.mod social_grpc/go.sum social_grpc/
RUN cd social_grpc && GOWORK=off go mod download

COPY social_grpc/ social_grpc/
RUN cd social_grpc && GOWORK=off CGO_ENABLED=0 go build -o /out/social_grpc .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/social_grpc /app/social_grpc
EXPOSE 8091
ENTRYPOINT ["/app/social_grpc"]
