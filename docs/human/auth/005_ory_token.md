# 005：基于 Ory Talos 的 API Token 管理

## 目标

在 auth/003_keto 的基础上，增加类似 LLM API Key 的长期 API Token：

1. 用户登录管理页面后申请 API Token。
2. CLI 工具和内部服务直接携带 API Token 请求接口。
3. 用户可以查看、撤销和轮换自己创建的 Token。
4. 管理员可以管理组织、用户和内部服务的 Token。
5. Token 支持 Scope、过期时间、主体和组织上下文。
6. Token 泄露后可以单独撤销，不影响用户登录 Session。

本实验不是 OAuth2 用户登录实验。Hydra 不作为核心组件。Hydra 适合
Authorization Code、Consent、OAuth2 Client 和 OAuth2 Access Token；本实验需要
的是用户主动创建并长期管理的 API Key/PAT。

本实验不修改 deployments/auth/003_keto，而是复制它创建独立的
deployments/auth/005_ory_token。

Kratos 和 Keto 使用 PostgreSQL；Talos OSS 按当前版本限制使用独立 SQLite：

~~~text
PostgreSQL
├── ory  → Kratos
└── keto → Keto

SQLite Volume
└── Talos
~~~

## 一、组件职责

```text
Kratos          用户身份和登录 Session
Token Manager   面向用户和管理员的 Token 管理接口
Talos           API Key 签发、校验、派生、轮换和撤销
Oathkeeper      API Token 认证和下游身份传递
Keto            用户、组织、服务和 Agent 的业务权限
xhs_service     业务接口和 Keto 权限检查
Traefik         对外入口和 Oathkeeper Decision API 转发
```

完整链路：

```text
用户登录 Kratos
      ↓
Token Manager 确认 identity.id
      ↓
Token Manager 调用 Talos 创建 API Key
      ↓
Secret 只返回一次
      ↓
CLI / 内部服务携带 API Key
      ↓
Oathkeeper 验证 Talos Credential
      ↓
Keto 检查主体业务权限
      ↓
xhs_service
```

Kratos 证明“用户是谁”，Talos 管理“凭证是否有效”，Keto 判断“主体能做什么”。

## 二、API Token 与 OAuth Token 的区别

| 项目 | 本实验 API Token | OAuth2 Access Token |
| --- | --- | --- |
| 使用方式 | 用户创建后长期保存 | 通过 OAuth2 授权流程获取 |
| 典型调用方 | CLI、脚本、内部服务、Agent | 第三方应用、浏览器授权客户端 |
| 是否需要 Consent | 通常不需要 | 通常需要 |
| 生命周期 | 创建、轮换、撤销 | OAuth2 Grant 和 Token 流程管理 |
| 本实验组件 | Ory Talos | Ory Hydra |

用户的 Kratos Session 只用于申请或管理 Token。API 请求阶段不需要再次执行浏览器
登录。

## 三、Token 主体

用户 Token 绑定到 User:<kratos-identity-id>。内部服务使用独立的机器主体：

```text
Service:social-service
Service:xhs-service
Agent:agent-17
```

如果 Alice 创建专属 Agent，需要保存两个事实：

```text
实际调用者是谁？  Agent:agent-17
Agent 属于谁？     User:<alice-identity-id>
```

Talos 证明 Agent Token 属于 Agent:agent-17，Keto 证明 Agent 当前属于 Alice。
不能把 Agent Token 直接伪装成 Alice，否则无法区分浏览器操作和 Agent 操作。

## 四、部署拓扑和端口

```text
Browser
  │ Kratos Session
  ▼
Traefik :8080
  ├── Kratos :4433
  ├── Token Manager :8090
  └── Oathkeeper :4456
          │
          └── Talos :4420

xhs_service ──> Keto :4466
Token Manager ──> Talos Admin API
```

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
| 8080 | Traefik | 统一外部入口 |
| 8025 | Mailpit | 教学邮件查看 |

Talos Public API 和 Admin API 使用同一个 HTTP 监听端口，通过不同的 API 路径区分。
健康检查使用 talos:4420/health/alive 和 /health/ready；Prometheus 指标使用独立
的 4422 端口。
Talos Admin API 没有内置认证，不能直接暴露给浏览器。只有 Token Manager 可以
访问 Admin API，并且 Token Manager 必须验证当前用户的 Kratos Session 和 Keto 权限。

## 五、用户申请和使用 Token

用户登录后请求：

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

Token Manager 验证 Session，取得 identity.id，使用 Keto 判断创建权限，然后以
User:<identity-id> 作为 actor_id 调用 Talos。完整 Secret 只返回一次，不能写入
日志或业务数据库。

CLI 或服务直接调用：

```http
GET http://192.168.2.41:8080/v1/xhs/content
Authorization: Bearer <api-token>
```

API Token 是不透明凭证，不能从字符串中解析 actor_id。认证组件必须调用 Talos
校验接口，使用返回的 actor_id、Scope 和 metadata。

## 六、API Token 请求链路

```text
CLI / Service
  │ Authorization: Bearer <api-key>
  ▼
Traefik :8080
  │ ForwardAuth / Decision API
  ▼
Oathkeeper
  │ 调用 Talos verify
  ▼
Talos
  │ is_valid、actor_id、scopes、metadata
  ▼
Oathkeeper
  │ 签发短期 Internal JWT
  ▼
xhs_service
  │ 使用 actor_id 和 Scope 请求 Keto
  ▼
Keto
```

认证成功后，Oathkeeper 可以继续使用 id_token Mutator 向下游签发短期
Internal JWT。内部服务只信任 Oathkeeper 的签名 JWT，不信任客户端提交的
X-Actor-ID、X-Scopes 或 X-Organization-ID Header。

## 七、Keto 与 API Token

| 判断内容 | 负责组件 |
| --- | --- |
| Token 是否存在、有效、过期 | Talos |
| Token 是否被撤销 | Talos |
| Token 的 actor_id 和 Scope | Talos |
| User/Service/Agent 的组织关系 | Keto |
| Alice 是否能修改关键词 | Keto |
| social-service 是否能读取内容 | Keto |
| 具体业务操作 | xhs_service |

最终条件：

```text
Talos Token 有效
&& Token Scope 包含 xhs.read
&& actor 属于正确组织
&& Keto 允许 actor 执行目标 Permission
```

Scope 是凭证能力上限，Keto Permission 是当前主体的实际业务权限。Scope 不能
绕过 Keto；Keto 也不能让已经撤销的 Token 重新有效。

## 八、Token 生命周期

```http
GET    /v1/auth/tokens
POST   /v1/auth/tokens
POST   /v1/auth/tokens/{id}:revoke
POST   /v1/auth/tokens/{id}:rotate
```

查询接口只返回名称、Scope、状态、创建时间、最后使用时间和过期时间。撤销一个
Token 不影响 Kratos Session 和其他 Token。轮换生成新 Secret 并撤销旧 Secret，
新 Secret 只返回一次。

用户被 Kratos 禁用时，本实验采用主动批量撤销该用户所有 API Token 的策略。

## 九、逐步执行计划

### Step 0：复制 003 基础

复制 deployments/auth/003_keto 到 deployments/auth/005_ory_token，保持端口和
服务结构不变，单独提交。本步骤不增加 Talos。

### Step 1：部署 Talos

增加 talos、talos-migrate、SQLite 数据卷和配置，验证 Public、Admin、Health 和
Metrics 接口。Talos OSS 不能使用 PostgreSQL；商业版再单独切换。

### Step 2：创建和验证服务 API Token

由手动执行的一次性 talos-seed 创建 Service:social-service 和
Service:xhs-service，验证 Talos 签发和 apiKeys:verify。

本步骤不验证 xhs_service 访问，因为 Oathkeeper 尚未接入 Talos；业务接口访问放在
Step 5。

~~~shell
docker compose -f deployments/auth/005_ory_token/docker-compose.yml run --rm talos-seed
~~~

### Step 3：实现 Token Manager

新增 `token_manager` 服务，提供用户 Token 创建、查询、撤销和轮换接口。服务先通过
Kratos `GET /sessions/whoami` 获取当前用户，再调用 Talos Admin API；Token Manager
只保存自己的元数据和 Talos `key_id`，不保存可恢复的完整 Secret，也不向浏览器暴露
Talos Admin API。

本步骤接口：

- `POST http://192.168.2.41:8090/v1/auth/tokens`
- `GET http://192.168.2.41:8090/v1/auth/tokens`
- `POST http://192.168.2.41:8090/v1/auth/tokens/{id}/revoke`
- `POST http://192.168.2.41:8090/v1/auth/tokens/{id}/rotate`

创建和轮换响应中的 `secret` 只返回一次；当前步骤直接暴露 8090 便于学习，后续再接入
网关和 Oathkeeper。

#### Token 元数据与 Talos API Key 的对应关系

Token Manager 和 Talos 保存的是两层不同的数据：

```json
// Token Manager PostgreSQL：api_tokens 表中的一行
{
  "id": "tm_1740000000000000000",
  "owner_identity_id": "kratos-identity-id-alice",
  "talos_key_id": "01JEXAMPLEKEYID00000000001",
  "name": "alice-cli",
  "scopes": ["xhs.read", "xhs.crawl.start"],
  "status": "KEY_STATUS_ACTIVE",
  "expire_time": "2027-09-28T00:00:00Z"
}
```

其中 `talos_key_id` 是 Token Manager 与 Talos 之间的关联字段。Talos 的 API Key
元数据大致如下：

```json
// Talos GET /v2alpha1/admin/issuedApiKeys/{key_id} 的结果
{
  "key_id": "01JEXAMPLEKEYID00000000001",
  "name": "alice-cli",
  "actor_id": "User:kratos-identity-id-alice",
  "scopes": ["xhs.read", "xhs.crawl.start"],
  "status": "KEY_STATUS_ACTIVE",
  "create_time": "2026-09-28T00:00:00Z",
  "expire_time": "2027-09-28T00:00:00Z"
}
```

创建时 Talos 另外返回一次性 Secret：

```json
// Talos POST /v2alpha1/admin/issuedApiKeys 的响应
{
  "issued_api_key": {
    "key_id": "01JEXAMPLEKEYID00000000001"
  },
  "secret": "ddd_live_<opaque-secret>"
}
```

三者关系是：

| 数据 | 保存位置 | 作用 |
| --- | --- | --- |
| `token_manager.id` | Token Manager PostgreSQL | 面向用户管理的业务 Token ID |
| `talos_key_id` / `key_id` | Token Manager 元数据、Talos 元数据 | 定位 Talos 中的这把 API Key |
| `secret` | 创建或轮换响应中临时出现 | CLI 或服务实际携带的凭证；Token Manager 和 Talos 都不提供再次读取 |

因此，`key_id` 不是 API Token 本身，也不能替代 Secret 调用业务接口。用户实际保存
的是 `secret`；用户在管理页面看到的应该是 Token Manager 的 `id`、名称、Scope、状态
和过期时间。撤销或轮换时，Token Manager 使用保存的 `talos_key_id` 调用 Talos，
不需要也不能重新读取旧 Secret。

### Step 4：接入用户 API Token

让 Alice 和 Bob 通过 Kratos Session 创建自己的 API Token，验证主体、组织、Scope
和生命周期。

当前步骤使用已经登录的 Kratos Session Cookie 调用 Token Manager。浏览器登录后，从
开发者工具复制 `ory_kratos_session` Cookie，再执行：

```shell
KRATOS_SESSION_COOKIE='ory_kratos_session=...' \
  ./token_manager/user-token-example.sh create

KRATOS_SESSION_COOKIE='ory_kratos_session=...' \
  ./token_manager/user-token-example.sh list
```

创建响应中的 `secret` 只展示这一次，应立即交给 CLI 或服务安全保存。响应中的
`token.id` 是后续撤销和轮换使用的 Token Manager ID：

```shell
KRATOS_SESSION_COOKIE='ory_kratos_session=...' \
  ./token_manager/user-token-example.sh rotate tm_...

KRATOS_SESSION_COOKIE='ory_kratos_session=...' \
  ./token_manager/user-token-example.sh revoke tm_...
```

使用 Alice 的 Session 查询时只能得到 Alice 的 Token；换成 Bob 的 Session 后只能得到
Bob 的 Token。Token Manager 通过 `owner_identity_id` 做数据隔离，当前步骤还不把 Scope
解释为 Keto 权限，Scope 与组织权限的绑定放在后续步骤处理。

### Step 5：接入 Oathkeeper

保留两条认证链路：

```text
Kratos Cookie → Oathkeeper → Internal JWT → xhs_service
API Token      → Talos      → Internal JWT → xhs_service
```

本步骤使用 Oathkeeper 内置的 `bearer_token` Authenticator：Oathkeeper 从
`Authorization: Bearer <api-token>` 提取凭证，调用 Token Manager 的内网校验接口；
Token Manager 再调用 Talos `POST /v2alpha1/admin/apiKeys:verify`。Talos 返回有效的
`actor_id` 和 Scope 后，Oathkeeper 的 `id_token` Mutator 统一生成 Internal JWT。
Token Manager 的校验接口不配置 Traefik 路由，浏览器不能直接访问。

实际处理顺序：

```text
CLI/服务
  │ Authorization: Bearer ddd_...
  ▼
Traefik /v1/*
  ▼ ForwardAuth
Oathkeeper /decisions
  ▼ bearer_token Authenticator
Token Manager /internal/auth/token/verify
  ▼ POST /v2alpha1/admin/apiKeys:verify
Talos
  └─ 返回 actor_id、scopes、expire_time
  ▼
Oathkeeper id_token Mutator
  └─ 返回 Authorization: Bearer <internal-jwt>
```

验证命令（命令只检查状态，不打印 API Secret）：

```shell
curl -i http://192.168.2.41:4456/decisions \
  -H 'Authorization: Bearer <talos-api-token>' \
  -H 'X-Forwarded-Method: GET' \
  -H 'X-Forwarded-Uri: /v1/xhs/content' \
  -H 'X-Forwarded-Host: 192.168.2.41:8080'
```

响应为 `200` 且包含 `Authorization` 响应头时，说明 API Token 已被 Talos 验证，并已
转换为下游使用的 Internal JWT。此时 `sub` 使用 Talos 返回的 `actor_id`，而不是把
API Token 原文放入 JWT。

### Step 6：接入 Keto 业务权限

验证 Alice、Bob 和内部服务的权限矩阵：

```text
Alice Token + modify_keywords → 允许
Bob Token + modify_keywords   → 403
Bob Token + view_content      → 允许
social-service + xhs.read     → 按 Keto 结果决定
```

### Step 7：增加派生短期 Token

使用 Talos 派生短期 JWT 或 Macaroon，比较本地验签和实时撤销的差异。

### Step 8：增加管理员管理和审计

增加管理员查询、创建、撤销、轮换接口，记录操作人、Token 主体、Scope、组织、
撤销原因和时间。

## 十、部署目录

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
建议 Commit：

```text
feat(auth): stage 5 step 0 copy keto baseline
feat(auth): stage 5 step 1 deploy talos api token service
```
