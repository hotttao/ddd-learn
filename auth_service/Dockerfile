FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git ca-certificates
ENV GOPROXY=https://goproxy.cn,direct
ENV GOSUMDB=off
WORKDIR /workspace
COPY hertz_gen/go.mod hertz_gen/go.sum hertz_gen/
COPY hertz_infra/go.mod hertz_infra/go.sum hertz_infra/
COPY auth_service/go.mod auth_service/go.sum auth_service/
RUN cd auth_service && GOWORK=off go mod download
COPY hertz_gen/ hertz_gen/
COPY hertz_infra/ hertz_infra/
COPY auth_service/ auth_service/
RUN cd auth_service && GOWORK=off CGO_ENABLED=0 go build -o /out/auth_service .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/auth_service /app/auth_service
COPY auth_service/migrations/ /app/migrations/
COPY auth_service/conf/ /app/conf/
COPY auth_service/.env /app/.env
EXPOSE 8081
ENTRYPOINT ["/app/auth_service"]
