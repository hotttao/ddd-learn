# gateway/001：Traefik RateLimit

## 当前步骤

本步骤在 UI 静态路由上增加 Traefik `RateLimit` Middleware，观察 Gateway 的流量治理行为。它不改变 Kratos、Oathkeeper、Keto 或 `xhs_service` 的认证鉴权逻辑。

## 配置

`traefik/dynamic.yml` 定义：

```yaml
ui-rate-limit:
  rateLimit:
    average: 5
    burst: 2
```

并将它挂到 UI Router：

```text
Browser → ui Router → ui-rate-limit → ui Service → Nginx
```

含义是每个来源平均每秒允许 5 个请求，并额外允许 2 个突发请求。超过限制时，
Traefik 直接返回 `429 Too Many Requests`，请求不会到达 Nginx。

`/kratos` 和 `/v1/xhs` 使用各自更具体的 Router，不会经过这个 UI 限流 Middleware：

```text
/kratos/*  → kratos-public
/v1/xhs/*  → oathkeeper-forward-auth → xhs-api → xhs_service
/*         → ui-rate-limit → ui → Nginx
```

## 验证

```bash
for i in $(seq 1 12); do
  curl -s -o /dev/null -w "%{http_code}\n" \
    http://192.168.2.41:8080/
done
```

短时间连续请求应同时看到 `200` 和 `429`。Traefik Dashboard 和 Access Log 可以
观察 `ui-rate-limit@file`、请求状态以及被限流的请求。

限流只回答“请求速度是否超过入口策略”，不回答“用户是否有业务权限”。即使请求
没有被限流，`/v1/xhs` 仍然要经过 Oathkeeper 和 `xhs_service` 的 Keto 检查。
