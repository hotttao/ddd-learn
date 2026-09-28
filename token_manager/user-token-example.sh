#!/bin/sh

# 使用方式：
#   KRATOS_SESSION_COOKIE='ory_kratos_session=...' \
#   ./token_manager/user-token-example.sh create
#
# Session Cookie 应从浏览器登录后的开发者工具中复制；不要把它提交到代码库。
set -eu

api_url="${TOKEN_MANAGER_URL:-http://192.168.2.41:8090}"
cookie="${KRATOS_SESSION_COOKIE:?请设置 KRATOS_SESSION_COOKIE}"
action="${1:-list}"

case "$action" in
  create)
    curl -fsS -X POST "$api_url/v1/auth/tokens" \
      -H "Cookie: $cookie" \
      -H 'Content-Type: application/json' \
      -d '{"name":"local-cli","scopes":["xhs.read"],"ttl":"720h"}'
    ;;
  list)
    curl -fsS "$api_url/v1/auth/tokens" -H "Cookie: $cookie"
    ;;
  revoke|rotate)
    token_id="${2:?请提供 Token Manager 返回的 token id}"
    curl -fsS -X POST "$api_url/v1/auth/tokens/$token_id/$action" \
      -H "Cookie: $cookie"
    ;;
  *)
    echo "usage: $0 {create|list|revoke|rotate} [token-manager-id]" >&2
    exit 2
    ;;
esac
echo
