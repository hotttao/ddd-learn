# gateway/001：UI 静态镜像

## 当前步骤

本目录继承 `deployments/auth/003_keto` 的认证和业务部署基线。本步骤只制作
`ui_example` 的静态文件镜像，不把 UI 容器接入 Compose，也不修改 Traefik 路由。

## 构建过程

构建上下文是 `ui_example`，不是仓库根目录：

```bash
docker build \
  -f ui_example/Dockerfile \
  -t ddd-learn-ui-example:gateway-001 \
  ui_example
```

Dockerfile 使用多阶段构建：

```text
国内 Node 镜像
  → Yarn 使用 registry.npmmirror.com 安装依赖
  → yarn build
  → 国内 Nginx 镜像
  → /usr/share/nginx/html
```

## 文件职责

| 文件 | 作用 |
| --- | --- |
| `ui_example/Dockerfile` | 编译前端并制作 Nginx runtime 镜像 |
| `ui_example/nginx.conf` | 提供静态文件和 React Router fallback |
| `ui_example/.dockerignore` | 排除 `node_modules`、`dist` 和缓存 |

Nginx 提供两个访问行为：

- `/` 返回编译后的 `index.html`；
- 不存在的前端路径回退到 `index.html`，交给 React Router 处理；
- `/health` 返回 `200 ok`，供后续 Gateway 健康检查使用。

## 镜像验证

使用临时容器验证镜像，不启动本模块的 Compose：

```bash
docker run --rm -d \
  --name ddd-learn-ui-image-check \
  -p 127.0.0.1:18080:80 \
  ddd-learn-ui-example:gateway-001

curl http://127.0.0.1:18080/
curl http://127.0.0.1:18080/dashboard
curl http://127.0.0.1:18080/health

docker stop ddd-learn-ui-image-check
```

当前步骤的边界是“镜像可以独立提供静态文件”。下一步才把 `ui` 服务加入
`deployments/gateway/001_traefik_docker/docker-compose.yml`，并由 Traefik 将根路径
转发到它；`/kratos` 和 `/v1/xhs` 仍然保持原有路由优先级。
