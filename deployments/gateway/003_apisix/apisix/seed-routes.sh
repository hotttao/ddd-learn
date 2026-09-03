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
    "methods": ["GET", "POST", "PUT"],
    "upstream_id": 1
  }'

echo "APISIX upstream 1 and route 1 are ready"
