# ddd-learn

用于学习 Gateway、认证鉴权和 Service Mesh 的最小 Hertz 微服务。当前阶段只包含可独立构建的业务服务和 Docker 镜像，不包含具体 Gateway。

## 服务

```text
Client
  │ Session Cookie / Session Token
  ▼
auth-service
  ├── 模拟 Kratos Session 与 /sessions/whoami
  ├── 把 Session 转换成 5 分钟 Internal JWT
  ├── 发布 Public JWKS
  └── 模拟 OpenFGA + OPA 的授权决策
        │
        │ Internal JWT
        ▼
xhs-service
  ├── 使用 JWKS 本地验证 JWT
  ├── 调用 auth-service 获取动态授权决策
  └── 执行业务操作
```

`auth-service` 是本地实验替身，目的是让后续所有 Gateway 实验共享同一组接口。生产架构中分别替换为 Kratos、Oathkeeper、OpenFGA 和 OPA。

## 测试用户与权限

| 用户 | 密码 | 组织 | 权限 |
| --- | --- | --- | --- |
| Alice | `alice-pass` | `org-g` | 启动抓取、查看内容、修改关键词 |
| Bob | `bob-pass` | `org-g` | 启动抓取、查看内容 |

## API

### auth-service

| 方法 | 路径 | 用途 |
| --- | --- | --- |
| `POST` | `/v1/login` | 登录并取得 Session Cookie / Session Token |
| `GET` | `/sessions/whoami` | 验证 Session |
| `POST` | `/internal/tokens` | 把有效 Session 转换为 Internal JWT |
| `GET` | `/.well-known/jwks.json` | 发布 JWT 验证公钥 |
| `POST` | `/v1/authorize` | 测试用授权决策接口 |
| `GET` | `/healthz` | 健康检查 |

### xhs-service

| 方法 | 路径 | 权限 |
| --- | --- | --- |
| `POST` | `/v1/crawl/tasks` | `crawl:start` |
| `GET` | `/v1/crawl/contents` | `content:read` |
| `GET` | `/v1/crawl/keywords` | `content:read` |
| `PUT` | `/v1/crawl/keywords` | `keyword:update` |
| `GET` | `/healthz` | 无 |

## 本地运行

```bash
go run ./cmd/auth-service
```

另一个终端：

```bash
go run ./cmd/xhs-service
```

登录 Alice：

```bash
curl -s -X POST http://localhost:8081/v1/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"alice","password":"alice-pass"}'
```

从响应取得 `session_token`，换取 Internal JWT：

```bash
curl -s -X POST http://localhost:8081/internal/tokens \
  -H 'X-Session-Token: <SESSION_TOKEN>'
```

调用业务接口：

```bash
curl -s -X PUT http://localhost:8082/v1/crawl/keywords \
  -H 'Authorization: Bearer <INTERNAL_JWT>' \
  -H 'Content-Type: application/json' \
  -d '{"keyword":"技术"}'
```

将 Alice 换成 Bob 后，读取内容返回 `200`，修改关键词返回 `403`。

## Docker 镜像

分别构建：

```bash
docker build -f build/package/auth-service.Dockerfile -t ddd-learn/auth-service:dev .
docker build -f build/package/xhs-service.Dockerfile -t ddd-learn/xhs-service:dev .
```

一起构建并启动：

```bash
docker compose -f compose.services.yaml up --build
```

两个镜像都使用多阶段构建、非 root 用户和 HTTP 健康检查。

## 安全边界

- `xhs-service` 不信任客户端身份 Header，只接受签名正确、Issuer 和 Audience 匹配的 Internal JWT。
- Internal JWT 有效期默认 5 分钟；业务权限不写入 JWT，而是在请求时调用授权服务。
- JWKS 在本地缓存，未知 `kid` 时刷新。
- 当前签名密钥在 `auth-service` 启动时临时生成，只适合单实例实验；生产环境必须使用共享密钥存储、KMS 或 Oathkeeper 的持久化 Private JWKS。
- `X-Workload-Identity` 当前只是后续 mTLS/SPIFFE 实验的占位信息，不能作为生产服务身份凭证。
- 授权服务不可用时，`xhs-service` 返回 `503`，不会放行请求。
