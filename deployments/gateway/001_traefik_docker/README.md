# gateway/001：UI 接入 Traefik

## 当前步骤

本步骤把已构建的 `ui_example` Nginx 镜像接入当前 Traefik Docker 部署。UI 只负责
提供静态文件，Traefik 负责根据路径把请求转发到 UI、Kratos 或 `xhs_service`。

## 启动

前置条件：已构建镜像 `ui_example:0.0.1`。

```bash
docker compose \
  -f deployments/gateway/001_traefik_docker/docker-compose.yml \
  up -d ui traefik
```

访问 UI：

```text
http://192.168.2.41:8080/
```

## 路由边界

```text
/kratos/*  → kratos-public → Kratos Public API
/v1/xhs/*  → xhs-api → Oathkeeper ForwardAuth → xhs_service
/*         → ui → Nginx 静态文件
```

UI Router 使用 `priority: 1`，因此不会抢占更具体的 `/kratos` 和 `/v1/xhs` Router。
浏览器请求 API 时仍然使用同源路径，不需要在 Nginx 中重复配置 API 代理。

## 验证

```bash
curl http://192.168.2.41:8080/
curl http://192.168.2.41:8080/login
curl http://192.168.2.41:8080/v1/xhs/organizations/G/crawl/contents
```

预期结果：

- `/` 返回静态 `index.html`；
- `/login` 返回 `index.html`，由 React Router 处理前端路由；
- `/v1/xhs/...` 先经过 Oathkeeper，未登录时返回 `401`；
- `/kratos/...` 不会被 UI Router 截获。

Traefik Dashboard 可以观察三个 Router：`ui@file`、`kratos-public@file`、
`xhs-api@file`，访问地址为：

```text
http://192.168.2.41:8081/dashboard/
```
