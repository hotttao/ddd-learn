# 005_ory_token：Ory Talos API Token

## 当前状态

本目录用于在 auth/003_keto 的基础上增加类似 LLM API Key 的 API Token：

```text
用户通过 Kratos 登录
    ↓
Token Manager 代表用户创建 API Token
    ↓
Talos 保存并验证 API Token
    ↓
CLI / 内部服务携带 API Token
    ↓
Oathkeeper 认证
    ↓
Keto 授权
    ↓
xhs_service
```

当前已经创建 Docker Compose、Talos 配置和 Token Manager 服务；Token Manager 的网关接入
留到后续步骤。
本目录不会修改 deployments/auth/003_keto。

## 组件职责

```text
Kratos          用户身份和登录 Session
Token Manager   用户、管理员的 Token 管理接口
Talos           API Key 签发、校验、派生、轮换和撤销
Oathkeeper      API Token 认证和下游身份传递
Keto            用户、组织、服务和 Agent 的业务权限
xhs_service     业务接口
```

Hydra 不属于本实验主流程。Hydra 适合 OAuth2/OIDC 授权和 Access Token；本实验
需要的是用户申请、保存、撤销和轮换的 API Key/PAT。

## 主体模型

```text
User:<kratos-identity-id>
Service:<service-name>
Agent:<agent-id>
```

Talos 证明 API Token 属于哪个主体；Keto 判断该主体是否拥有目标组织和业务权限。

Kratos 和 Keto 使用 PostgreSQL；Talos OSS 按当前版本限制使用独立 SQLite：

```text
PostgreSQL
├── ory  → Kratos
└── keto → Keto

SQLite Volume
└── Talos
```

## 端口规划

| 宿主机端口 | 服务 | 用途 |
| ---: | --- | --- |
| 4433 | Kratos Public API | 用户登录、注册、Session |
| 4434 | Kratos Admin API | 身份查询和初始化 |
| 4420 | Talos Public API | API Key 校验和派生 |
| 4422 | Talos Metrics API | Prometheus 指标 |
| 4456 | Oathkeeper Decision API | Traefik 认证决策 |
| 4466 | Keto Read API | 业务权限检查 |
| 4467 | Keto Write API | Relation Tuple 管理 |
| 8090 | Token Manager | 用户和管理员 Token 管理 API |
| 8080 | Traefik | 对外统一入口 |
| 8025 | Mailpit | 教学邮件查看 |

Talos Public API 和 Admin API 使用同一个 HTTP 监听端口，通过不同的 API 路径区分。
健康检查使用 talos:4420/health/alive 和 /health/ready；Prometheus 指标使用独立
的 4422 端口。
Talos Admin API 没有内置认证，不能直接暴露给浏览器。只有 Token Manager 可以
访问 Admin API，并且 Token Manager 必须验证当前用户的 Kratos Session 和 Keto 权限。

## Token 创建和使用

用户登录后：

```http
POST http://192.168.2.41:8080/v1/auth/tokens
Cookie: ory_kratos_session=<session>
Content-Type: application/json

{
  "name": "alice-cli",
  "scopes": ["xhs.read", "xhs.crawl.start"],
  "expires_in": "720h",
  "organization_id": "G"
}
```

Token Manager 使用 User:<identity-id> 调用 Talos，完整 Secret 只返回一次。

CLI 或服务随后直接调用：

```http
GET http://192.168.2.41:8080/v1/xhs/content
Authorization: Bearer <api-token>
```

API Token 是不透明凭证，不能从字符串中解析 actor_id。认证组件必须调用 Talos
校验接口，使用返回的 actor_id、Scope 和 metadata。

## 生命周期接口

```http
GET    /v1/auth/tokens
POST   /v1/auth/tokens
POST   /v1/auth/tokens/{id}:revoke
POST   /v1/auth/tokens/{id}:rotate
```

查询接口只返回名称、Scope、状态、创建时间、最后使用时间和过期时间。撤销一个
API Token 不影响 Kratos Session 和其他 Token。

## 分步部署

### Step 0：复制 003 基础

复制 deployments/auth/003_keto 到本目录，保持原有 Kratos、Oathkeeper、Keto、
xhs_service 和 Traefik 结构。本步骤不增加 Talos。

### Step 1：部署 Talos

增加 talos、talos-migrate、SQLite 数据卷和 talos/config.yaml，验证 Public、Admin、
Health 和 Metrics 接口。Talos OSS 不能使用 PostgreSQL；商业版再单独切换。

### Step 2：创建服务 API Token

手动执行 talos-seed，初始化 Service:social-service 和 Service:xhs-service。

本步骤只验证 Talos 签发和校验 API Token：

```text
Talos issuedApiKeys
    ↓
API Token
    ↓
Talos apiKeys:verify
    ↓
actor_id、scopes、expires_at
```

此时 Oathkeeper 尚未接入 Talos，因此 API Token 还不能直接访问 xhs_service。
访问业务接口放在 Step 5。

执行：

```shell
docker compose -f deployments/auth/005_ory_token/docker-compose.yml run --rm talos-seed
```

脚本会把完整 Secret 输出一次。只用于本地教学，不能把日志中的 Secret 用于生产。

### Step 3：实现 Token Manager

Token Manager 监听 `8090`，提供用户 API Token 的创建、查询、撤销和轮换。它通过 Kratos
`sessions/whoami` 识别用户，通过 Talos Admin API 管理密钥；PostgreSQL 中只保存元数据和
Talos `key_id`，Secret 只在创建或轮换响应中返回一次。

Token Manager 元数据示例：

```json
{
  "id": "tm_1740000000000000000",
  "owner_identity_id": "kratos-identity-id-alice",
  "talos_key_id": "01JEXAMPLEKEYID00000000001",
  "name": "alice-cli",
  "scopes": ["xhs.read", "xhs.crawl.start"],
  "status": "KEY_STATUS_ACTIVE"
}
```

Talos 中对应的 API Key 元数据使用同一个 `key_id`，但实际 Secret 只在签发或轮换
响应中出现一次：

```json
{
  "key_id": "01JEXAMPLEKEYID00000000001",
  "actor_id": "User:kratos-identity-id-alice",
  "status": "KEY_STATUS_ACTIVE",
  "scopes": ["xhs.read", "xhs.crawl.start"]
}
```

`token_manager.id` 是业务管理 ID，`talos_key_id` 是关联 Talos 的外部 ID，`secret` 才是
CLI 或服务实际使用的凭证；`key_id` 不能代替 `secret`。

接口：

- `POST http://192.168.2.41:8090/v1/auth/tokens`
- `GET http://192.168.2.41:8090/v1/auth/tokens`
- `POST http://192.168.2.41:8090/v1/auth/tokens/{id}/revoke`
- `POST http://192.168.2.41:8090/v1/auth/tokens/{id}/rotate`

用户侧可以使用仓库中的 `token_manager/user-token-example.sh` 调用这些接口。脚本要求
通过 `KRATOS_SESSION_COOKIE` 传入登录后的 `ory_kratos_session`，不会在脚本中保存账号密码。

### Step 4：接入用户 API Token

让 Alice 和 Bob 通过 Kratos Session 创建自己的 API Token，验证主体、组织、Scope
和生命周期。

### Step 5：接入 Oathkeeper

Oathkeeper 使用内置 `bearer_token` Authenticator，把 `Authorization: Bearer <api-token>`
转交给 Token Manager 的内网校验接口；Token Manager 调用 Talos `apiKeys:verify`，再由
Oathkeeper 的 `id_token` Mutator 生成下游 Internal JWT。校验接口没有 Traefik 路由，
不会直接暴露给浏览器。

验证 Decision API：

```shell
curl -i http://192.168.2.41:4456/decisions \
  -H 'Authorization: Bearer <talos-api-token>' \
  -H 'X-Forwarded-Method: GET' \
  -H 'X-Forwarded-Uri: /v1/xhs/content' \
  -H 'X-Forwarded-Host: 192.168.2.41:8080'
```

`200` 且响应包含 `Authorization` Header，表示 Talos API Token 已转换为 Internal JWT。

保留两条认证链路：

```text
Kratos Cookie → Oathkeeper → Internal JWT → xhs_service
API Token      → Talos      → Internal JWT → xhs_service
```

### Step 6：接入 Keto

验证 Alice、Bob 和内部服务的权限矩阵。

### Step 7：增加派生短期 Token

使用 Talos 将长期 API Key 派生为短期 JWT 或 Macaroon，比较本地验签和实时
撤销的差异。

### Step 8：完成审计和生命周期验证

验证 Token 创建、查询、轮换、撤销、过期、用户禁用和管理员操作审计。

## 目录规划

```text
deployments/auth/005_ory_token/
├── README.md
├── docker-compose.yml
├── talos/
│   ├── config.yaml
│   ├── seed-service-keys.sh
│   └── jwks/
├── token-manager/
│   ├── README.md
│   └── config.yaml
├── kratos/
├── keto/
├── oathkeeper/
│   ├── config.yaml
│   └── rules.yaml
├── traefik/
└── postgres/
    └── init/
```

每个 Step 完成后先解释实际修改、请求流程、端口和数据存储，确认后再提交。
Commit 必须包含阶段和步骤，例如：

```text
feat(auth): stage 5 step 0 copy keto baseline
feat(auth): stage 5 step 1 deploy talos api token service
```
