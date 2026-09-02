# gateway/001：Traefik Docker 当前实现

## 目标

在已经完成的 auth/Keto 架构上使用 Traefik 作为单机 Docker Gateway，验证：

- UI 静态文件如何由 Nginx 提供；
- `/kratos`、`/v1/xhs` 和 `/` 如何分别路由；
- Traefik 如何执行 ForwardAuth、负载均衡、健康检查和 RateLimit；
- 如何通过访问日志和 Prometheus 指标观察网关请求；
- Gateway 变化不改变 Oathkeeper、`xhs_service` 和 Keto 的职责。

## 当前拓扑

```text
Browser :8080
    │
    ▼
Traefik
    ├── /kratos/*  → Kratos Public API
    ├── /v1/xhs/*  → Oathkeeper ForwardAuth → xhs_service / xhs_service_2
    └── /*         → ui → Nginx → ui_example/dist

Traefik Dashboard :8081
```

当前部署目录从 `deployments/auth/003_keto` 复制而来，使用独立的 Compose 项目名
`ddd-learn-gateway-001-traefik` 和 PostgreSQL Volume `gateway-001-postgres`。

## 已实现步骤

### 1. 认证架构部署基线

复制 auth/003 的 Kratos、Oathkeeper、Keto、PostgreSQL、Mailpit、Traefik 和
`xhs_service` 配置，保留原有认证和权限行为。构建上下文路径已调整为仓库根目录：

```yaml
build:
  context: ../../..
  dockerfile: xhs_service/Dockerfile
```

### 2. Traefik Dashboard

静态配置开启 Dashboard：

```yaml
api:
  dashboard: true
  insecure: true
```

端口映射：

```text
192.168.2.41:8080 → Traefik :80
192.168.2.41:8081 → Traefik :8080 Dashboard
```

访问：

```text
http://192.168.2.41:8081/dashboard/
```

### 3. File Provider 路由

当前使用 `traefik/dynamic.yml`，不依赖 Docker labels：

```text
PathPrefix(`/kratos`) → kratos-public → StripPrefix(`/kratos`) → kratos:4433
PathPrefix(`/v1/xhs`) → xhs-api → ForwardAuth → xhs_service
PathPrefix(`/`)        → ui → ui:80
```

UI Router 设置 `priority: 1`，因此不会截获更具体的 `/kratos` 和 `/v1/xhs` 请求。

### 4. xhs_service 负载均衡

Compose 中运行两个相同的业务实例：

```text
xhs_service:8082
xhs_service_2:8082
```

Traefik 的 `xhs-api` Service 配置两个 backend，并对 `/health` 执行健康检查：

```yaml
healthCheck:
  path: /health
  interval: 5s
  timeout: 2s
```

停止 `xhs_service_2` 后，Traefik Dashboard 中的状态从 `UP` 变为 `DOWN`，业务
请求继续由认证链路处理，不会因为单个实例停止而直接固定转发到失效实例。

### 5. UI 静态文件镜像

`ui_example` 使用多阶段 Dockerfile 构建镜像 `ui_example:0.0.1`：

```text
国内 Node 镜像
  → registry.npmmirror.com 安装 Yarn 依赖
  → yarn build
  → 国内 Nginx 镜像
  → /usr/share/nginx/html
```

Nginx 负责静态文件和 React Router fallback；Traefik 不直接读取 `dist` 目录。

### 6. UI 接入 Compose 和 Traefik

Compose 使用已经构建好的 UI 镜像：

```yaml
ui:
  image: ui_example:0.0.1
  expose:
    - "80"
```

Traefik 将根路径转发到 `http://ui:80`。由于 UI 和 API 共用 `:8080`，浏览器请求
`/kratos` 和 `/v1/xhs` 时不需要 Vite 开发服务器代理。

### 7. Kratos 统一浏览器入口

静态 UI 接入后，Kratos 的 Flow 页面和回调地址统一改为：

```text
http://192.168.2.41:8080
```

因此登录、注册、恢复密码、修改密码和验证 Flow 不再跳转到开发端口 `:5173`。
`ui_example/vite.config.ts` 中的 `:5173` 仅保留给 Vite 开发模式。

### 8. UI Router RateLimit

当前 RateLimit 只挂在 UI Router：

```yaml
ui-rate-limit:
  rateLimit:
    average: 5
    burst: 2
```

超过限制时 Traefik 返回 `429`，请求不会到达 Nginx。`/kratos` 和 `/v1/xhs` 不经过
这个 Middleware；业务 API 仍由 Oathkeeper 和 Keto 负责认证鉴权。

### 9. 网关可观测性

Traefik 同时启用两类观测数据：

- Access Log：以 JSON 输出到容器标准输出，记录请求方法、入口、路由、状态码和耗时；
- Prometheus Metrics：通过独立的 `metrics` EntryPoint 暴露在宿主机 `8082` 端口。

配置关系如下：

```text
Browser/API request
  → Traefik Router / Middleware / Service
  → JSON Access Log → docker logs
  → Prometheus Metrics → :8082/metrics
```

当前没有额外部署 Prometheus 或 OpenTelemetry Collector；本步骤先验证 Traefik
自身能够产生并暴露观测数据，后续网关或可观测性模块再接入采集系统。

## 启动和验证

在仓库根目录执行：

```bash
docker compose \
  -f deployments/gateway/001_traefik_docker/docker-compose.yml \
  up -d
```

访问 UI：

```text
http://192.168.2.41:8080/
```

验证路由边界：

```bash
curl http://192.168.2.41:8080/
curl http://192.168.2.41:8080/login
curl http://192.168.2.41:8080/v1/xhs/organizations/G/crawl/contents
```

预期结果：

- `/`：HTTP 200，返回静态首页；
- `/login`：HTTP 200，返回 SPA 的 `index.html`；
- `/v1/xhs/...`：未登录时由 Oathkeeper 返回 HTTP 401；
- 登录后，Alice/Bob 的业务结果仍由 Keto 决定，分别保持原有 `200/403` 矩阵。

验证限流：

```bash
for i in $(seq 1 12); do
  curl -s -o /dev/null -w "%{http_code}\n" \
    http://192.168.2.41:8080/
done
```

短时间连续请求应同时看到 `200` 和 `429`。

验证可观测性：

```bash
curl http://192.168.2.41:8082/metrics
docker logs ddd-learn-gateway-001-traefik-traefik --since 1m
```

访问 `/`、`/login` 和 `/v1/xhs/...` 后，Metrics 响应中应能看到 Traefik
请求计数和耗时指标；容器日志中应能看到对应的 JSON Access Log。
