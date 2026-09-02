# 002_internal_jwt：Internal JWT

本目录是 Internal JWT 模块的独立部署目录。它从
`deployments/auth/001_auth` 复制基础配置作为起点，之后独立维护 Kratos、Traefik、
PostgreSQL、Courier、Mailpit、Oathkeeper、JWKS 和鉴权路由配置。当前 002 的
PostgreSQL 使用自己的 `auth-002-postgres` volume，不读取或修改 001 的认证数据。

复制配置表示继承上一步的部署基线，不表示两个 Compose 项目运行时共享容器或
配置文件。后续本模块的配置和认证数据均只作用于本目录。

## 本模块的边界
```text
Browser / API Client
        │ Kratos Session Cookie 或外部凭证
        ▼
Traefik :8080
        │ ForwardAuth / Decision API
        ▼
Oathkeeper :4456
        │ cookie_session -> allow -> id_token
        │ Authorization: Bearer <short-lived internal JWT>
        ▼
xhs_service :8082
        │ GET http://oathkeeper:4456/.well-known/jwks.json
        │ JWKS 公钥本地缓存 + claims 校验
        ▼
Mock response
```

各组件职责如下：

| 组件 | 本模块职责 | 不负责的内容 |
| --- | --- | --- |
| Kratos | 验证用户 Session，提供 `identity.id` | 签发 Internal JWT、业务权限判断 |
| Oathkeeper | 匹配 Access Rule，认证外部凭证，签发 Internal JWT | 业务数据、真实抓取、业务数据库 |
| Traefik | 接收业务请求，调用 Oathkeeper Decision API，并转发允许的请求 | 解析或自行伪造用户身份 |
| `xhs_service` | 验证 Internal JWT，提供三个业务接口 | 真实抓取实现 |
| JWKS | 提供签名和验签所需的密钥材料 | 用户和 Session 存储 |

本模块采用 Oathkeeper Decision API 模式。Traefik 将 API Router 下的
`/v1/<.*>` 请求统一交给 Oathkeeper；`<.*>` 是 regexp 匹配策略下的路径表达式。
两个端口的实际过程如下：

### 4455：Reverse Proxy

```text
Client
  │ 业务请求 + Cookie
  ▼
Oathkeeper :4455
  │ 验证 Session、签发 Internal JWT
  │ 直接代理业务请求
  ▼
xhs_service :8082
```

### 4456：Decision API（本模块采用）

```text
Client
  │ 业务请求 + Cookie
  ▼
Traefik :8080
  │ GET /decisions
  ▼
Oathkeeper :4456
  │ 验证 Session、签发 Internal JWT
  │ 返回 200 + Authorization: Bearer <JWT>
  ▼
Traefik -> xhs_service :8082
          原始业务请求 + Internal JWT
```

`xhs_service` 通过 Oathkeeper API 获取验签公钥：

```text
GET http://oathkeeper:4456/.well-known/jwks.json
```

4455 是 Oathkeeper 直接代理业务的入口；4456 是只返回鉴权决策的 API 入口。
本模块使用 4456，4455 不参与业务调用链。

Oathkeeper 的宽匹配 Rule 只负责确认 Session 有效并签发 Internal JWT。新增
`/v1/` 下的业务接口通常不需要修改 `rules.yaml`；服务仍必须自行验证 JWT，
并根据接口、用户、组织和资源关系判断是否允许执行。每个业务服务应拥有自己
稳定的 URL 前缀，例如 `xhs_service` 使用 `/v1/xhs`。只有新增不同 Host、
不同 API 前缀，或需要额外认证条件时，才需要增加 Oathkeeper Rule。

Traefik 的 Router 与前缀保持一一对应：

```text
/kratos/<path> -> kratos-public -> Kratos :4433
               不经过 Oathkeeper

/v1/xhs/<path> -> xhs-api -> oathkeeper-forward-auth
             -> Oathkeeper :4456/decisions
             -> xhs_service :8082
```

## 业务接口契约

接口先验证 `Authorization: Bearer <Internal JWT>`，通过后返回 Mock 数据。
真实抓取、关键词持久化和内容数据库不在本模块实现范围内。

| 方法 | 路径 | 用途 | Mock 结果 |
| --- | --- | --- | --- |
| `POST` | `/v1/xhs/organizations/:organization_id/crawl/tasks` | 启动抓取任务 | 返回 Mock 任务 ID 和 `pending` 状态 |
| `GET` | `/v1/xhs/organizations/:organization_id/crawl/contents` | 查看抓取内容 | 返回固定内容列表 |
| `PUT` | `/v1/xhs/organizations/:organization_id/crawl/keywords` | 修改抓取关键词 | 返回请求中的关键词或固定关键词列表 |

步骤 6 使用保留的组织 ID `forbidden` 模拟业务权限拒绝。请求已经通过 Internal
JWT 认证，但 `xhs_service` 的 Mock PermissionChecker 会返回拒绝，接口响应 `403`。
该约定只用于本模块验收，下一模块接入 Keto 后删除。

## 教学用户初始化

`kratos-seed` 使用 Kratos Admin API 初始化两个开发用户。它不是 SQL migration：
密码由 Kratos 负责哈希，脚本只提交一次性的明文密码，并按邮箱检查是否已经存在。

| 用户 | 邮箱 | 开发密码 | `metadata_admin` |
| --- | --- | --- | --- |
| Alice | `alice@example.com` | `Alice-password-2026` | `organization_id=G`, `role=admin` |
| Bob | `bob@example.com` | `Bob-password-2026` | `organization_id=G`, `role=member` |

启动 Compose 后，初始化服务会自动执行；也可以手动重跑：

```shell
docker compose -f deployments/auth/002_internal_jwt/docker-compose.yml run --rm kratos-seed
```

本阶段只初始化 Identity，不根据 `role` 判断业务权限。普通成员和管理员的
权限关系在下一模块接入 Keto 后建立。

## 信任边界

```text
客户端提供的 Cookie / Authorization
        ↓ 仅交给 Oathkeeper 验证
Oathkeeper 验证后的 Subject
        ↓ 使用私钥签发 Internal JWT
Traefik 转发 Internal JWT
        ↓ 使用公钥验证签名和标准 claims
xhs_service 使用可信 Subject 执行业务接口
```

客户端提交的 `X-User-ID`、`X-Subject`、`X-Role` 等身份 Header 不可信，
不能直接作为业务身份。`xhs_service` 必须验证 JWT 的签名、`iss`、`aud`、
`exp` 和 `sub`，不能因为请求经过 Traefik 就自动信任。

## 配置目录约定

```text
deployments/
├── 001_auth/
│   ├── docker-compose.yml       # 002 的复制来源
│   ├── kratos/
│   └── traefik/
└── 002_internal_jwt/            # 从 001 复制后独立演进，使用自己的认证 volume
    ├── README.md
    ├── docker-compose.yml
    ├── kratos/
    └── traefik/
    # 后续步骤新增以下目录
    ├── oathkeeper/
    │   ├── config.yaml
    │   └── rules.yaml
    └── jwks/
        └── id_token.jwks.json   # 开发环境私钥；公钥由 Oathkeeper 发布
```

`id_token.jwks.json` 是开发环境私钥，只挂载给 Oathkeeper。业务服务只通过
`/.well-known/jwks.json` 获取公钥。生产环境必须通过 Secret、Vault 或其他
密钥管理系统提供私钥，不能把私钥提交到代码仓库。
