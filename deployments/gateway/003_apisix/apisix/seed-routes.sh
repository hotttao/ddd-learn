#!/bin/sh
set -eu

admin_url=http://apisix:9180/apisix/admin
admin_key="${APISIX_ADMIN_KEY}"

until curl -fsS -H "X-API-KEY: ${admin_key}" "${admin_url}/routes" >/dev/null; do
  echo "waiting for APISIX Admin API"
  sleep 2
done

curl -fsS -X PUT "${admin_url}/upstreams/1" \
  -H "X-API-KEY: ${admin_key}" \
  -H "Content-Type: application/json" \
  --data '{
    "type": "roundrobin",
    "nodes": {
      "xhs_service:8082": 1,
      "xhs_service_2:8082": 1
    },
    "checks": {
      "active": {
        "type": "http",
        "http_path": "/health",
        "healthy": {
          "interval": 2,
          "successes": 1
        },
        "unhealthy": {
          "interval": 2,
          "http_failures": 2,
          "timeouts": 2,
          "http_statuses": [500, 502, 503, 504]
        }
      }
    }
  }'

curl -fsS -X PUT "${admin_url}/routes/1" \
  -H "X-API-KEY: ${admin_key}" \
  -H "Content-Type: application/json" \
  --data '{
    "name": "xhs-api",
    "uri": "/v1/xhs/*",
    "host": "192.168.2.41",
    "priority": 10,
    "methods": ["GET", "POST", "PUT"],
    "plugins": {
      "limit-count": {
        "count": 3,
        "time_window": 60,
        "key_type": "var",
        "key": "remote_addr",
        "policy": "local",
        "rejected_code": 429,
        "rejected_msg": "APISIX route rate limit exceeded",
        "show_limit_quota_header": true
      },
      "forward-auth": {
        "uri": "http://oathkeeper:4456/decisions",
        "request_method": "GET",
        "request_headers": [
          "Authorization",
          "Cookie",
          "X-Forwarded-Method",
          "X-Forwarded-Proto",
          "X-Forwarded-Host",
          "X-Forwarded-Uri"
        ],
        "upstream_headers": ["Authorization"],
        "timeout": 3000,
        "keepalive": true
      }
    },
    "upstream_id": 1
  }'

curl -fsS -X PUT "${admin_url}/upstreams/2" \
  -H "X-API-KEY: ${admin_key}" \
  -H "Content-Type: application/json" \
  --data '{
    "type": "roundrobin",
    "nodes": {
      "ui:80": 1
    },
    "checks": {
      "active": {
        "type": "http",
        "http_path": "/health",
        "healthy": {
          "interval": 2,
          "successes": 1
        },
        "unhealthy": {
          "interval": 2,
          "http_failures": 2,
          "timeouts": 2,
          "http_statuses": [500, 502, 503, 504]
        }
      }
    }
  }'

curl -fsS -X PUT "${admin_url}/upstreams/3" \
  -H "X-API-KEY: ${admin_key}" \
  -H "Content-Type: application/json" \
  --data '{
    "type": "roundrobin",
    "nodes": {
      "kratos:4433": 1
    }
  }'

curl -fsS -X PUT "${admin_url}/routes/2" \
  -H "X-API-KEY: ${admin_key}" \
  -H "Content-Type: application/json" \
  --data '{
    "name": "ui",
    "uri": "/*",
    "host": "192.168.2.41",
    "priority": 1,
    "methods": ["GET", "HEAD"],
    "upstream_id": 2
  }'

curl -fsS -X PUT "${admin_url}/routes/3" \
  -H "X-API-KEY: ${admin_key}" \
  -H "Content-Type: application/json" \
  --data '{
    "name": "kratos-public-api",
    "uri": "/kratos/*",
    "host": "192.168.2.41",
    "priority": 5,
    "methods": ["GET", "HEAD", "POST"],
    "plugins": {
      "proxy-rewrite": {
        "regex_uri": ["^/kratos/(.*)", "/$1"]
      }
    },
    "upstream_id": 3
  }'

curl -fsS -X PUT "${admin_url}/consumers/alice" \
  -H "X-API-KEY: ${admin_key}" \
  -H "Content-Type: application/json" \
  --data '{
    "username": "alice",
    "plugins": {
      "key-auth": {
        "key": "alice-api-key"
      }
    }
  }'

curl -fsS -X PUT "${admin_url}/routes/4" \
  -H "X-API-KEY: ${admin_key}" \
  -H "Content-Type: application/json" \
  --data '{
    "name": "consumer-demo",
    "uri": "/consumer-demo/*",
    "host": "192.168.2.41",
    "priority": 20,
    "methods": ["GET", "HEAD"],
    "plugins": {
      "key-auth": {}
    },
    "upstream_id": 2
  }'

echo "APISIX consumer alice and route 4 are ready"
