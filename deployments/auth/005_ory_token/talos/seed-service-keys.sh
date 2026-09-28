#!/bin/sh

set -eu

talos_url="${TALOS_URL:-http://talos:4420}"

wait_for_talos() {
  i=0
  while [ "$i" -lt 30 ]; do
    if curl -fsS "$talos_url/health/ready" >/dev/null; then
      return 0
    fi
    i=$((i + 1))
    sleep 1
  done

  echo "Talos is not ready: $talos_url" >&2
  exit 1
}

issue_key() {
  name="$1"
  actor_id="$2"
  scopes="$3"

  echo "Issuing API key: name=$name actor_id=$actor_id scopes=$scopes"
  response="$(
    curl -fsS -X POST "$talos_url/v2alpha1/admin/issuedApiKeys" \
      -H "Content-Type: application/json" \
      -d "{\"name\":\"$name\",\"actor_id\":\"$actor_id\",\"scopes\":$scopes,\"ttl\":\"720h\"}"
  )"

  echo "Save this response securely; the secret is returned only once:"
  echo "$response"

  secret="$(printf '%s' "$response" | sed -n 's/.*"secret":"\([^"]*\)".*/\1/p')"
  if [ -z "$secret" ]; then
    echo "Talos response did not contain secret" >&2
    exit 1
  fi

  echo "Verification result:"
  curl -fsS -X POST "$talos_url/v2alpha1/admin/apiKeys:verify" \
    -H "Content-Type: application/json" \
    -d "{\"credential\":\"$secret\"}"
  echo
}

wait_for_talos

# 本地教学服务主体；生产环境应通过受控管理流程创建，不把 Secret 打到日志。
issue_key "social-service-to-xhs" "Service:social-service" "[\"xhs.read\"]"
issue_key "xhs-service-internal" "Service:xhs-service" "[\"xhs.read\",\"xhs.crawl.start\"]"
