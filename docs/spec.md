# DDD Hertz 认证鉴权实验：技术规格

## 1. 目标

本项目用于验证“方案三”：入口统一认证，Gateway/Oathkeeper 将外部凭证转换成短期内部 JWT；内部服务本地验证 JWT，并在执行资源级操作前调用统一鉴权服务。

业务场景：组织 `G` 有两名用户。

- Alice 是管理员，可以启动抓取任务、查看抓取内容、修改抓取关键词。
- Bob 是普通成员，可以启动抓取任务、查看抓取内容，不能修改抓取关键词。

本阶段包含两个独立 Hertz 微服务：

```text
auth_service  # 认证鉴权门面，适配 Kratos 与 Keto
xhs_service   # 小红书抓取业务服务
```

每个服务都由 `harness/tools/new_server` 创建，拥有独立的 Go module、启动入口、配置、Dockerfile 和 DDD 目录。

## 2. 架构边界

```text
Browser / CLI
      │ Session Cookie / Service Token
      ▼
Gateway + Oathkeeper
      ├── 人类会话 ──► Kratos /sessions/whoami
      ├── 服务令牌 ──► Service Token Introspection
      └── 认证成功后签发短期 Internal JWT
                         │
                         ▼
                    xhs_service
                         │ 转发用户 Internal JWT
                         ▼
                    auth_service
                         │
                         └── Keto Check API
```

组件职责：

| 组件 | 只负责什么 | 不负责什么 |
| --- | --- | --- |
| Kratos | 用户、凭证、登录流程和 Session | OAuth Token、业务权限 |
| Oathkeeper | 入口认证、请求准入、内部 JWT 签发 | 保存用户、计算资源权限 |
| Keto | 基于关系元组计算资源级权限 | 登录、Session、签发 JWT |
| auth_service | 将 Ory HTTP API 转换成内部稳定能力 | 代理 Ory 的全部管理接口 |
| xhs_service | 抓取业务与权限执行点（PEP） | 登录和权限规则存储 |

### 2.1 为什么不把完整 Ory HTTP API 转写成 IDL

IDL 是我们拥有的服务接口的唯一源；Ory OpenAPI 是 Ory 接口的唯一源。复制全部 Ory API 会产生第二份协议源，并丢失 Cookie、重定向、查询参数和错误响应等 HTTP 语义。

因此采用防腐层：

```text
内部调用方
  └── Hertz IDL Client
        └── auth_service 稳定能力
              └── Port Interface
                    └── Kratos/Keto OpenAPI Adapter
```

Ory 官方 SDK 或由官方 OpenAPI 生成的客户端只能出现在基础设施适配器中。Domain、Policy、Service 不得依赖 Ory DTO。

Oathkeeper 本身是反向代理，不包装成 IDL 服务。

## 3. 认证信息传递

### 3.1 浏览器请求

1. 浏览器完成 Kratos 登录并持有 Session Cookie。
2. 浏览器携带 Cookie 请求 Gateway。
3. Oathkeeper 使用 `cookie_session` authenticator 调用 Kratos `GET /sessions/whoami`。
4. 验证成功后，Oathkeeper 使用 `id_token` mutator 签发 5 分钟内部 JWT。
5. Gateway 将 `Authorization: Bearer <internal-jwt>` 传给 `xhs_service`。
6. `xhs_service` 本地验证 JWT，不在每次请求中调用 Kratos。

### 3.2 CLI 请求

CLI 可以使用 Service Token：

1. CLI 在 `Authorization: Bearer <service-token>` 中提交长期凭证。
2. Oathkeeper 调用 Service Token introspection 能力；服务端只存令牌哈希。
3. 验证成功后，Oathkeeper 同样换成短期内部 JWT。
4. 下游服务只处理内部 JWT，不需要区分外部凭证格式。

Service Token 管理和 introspection 接口将在 `auth_service` 中实现；它不是 Kratos Session。

### 3.3 内部服务调用

`xhs_service` 调用 `auth_service.CheckPermission` 时转发当前请求的内部 JWT：

```http
Authorization: Bearer <same-internal-jwt>
```

`auth_service` 必须再次验证签名、issuer、audience 和有效期，并从 `sub` 取得主体。主体不能由 `CheckPermission` body 传入，避免调用方伪造 Alice。

本实验使用统一 audience `internal-api`，因此同一个内部 JWT 可以在受信服务间传递。生产环境如果需要更强隔离，应改用 audience-specific token exchange，并用 mTLS/SPIFFE 验证调用服务身份。

### 3.4 内部 JWT 字段

```jsonc
{
  "iss": "oathkeeper",             // 签发方，只接受配置中的固定值
  "sub": "identity:alice",         // 稳定主体；来自 Kratos identity ID 或服务身份
  "aud": ["internal-api"],         // 内部服务共同验证的 audience
  "exp": 1787970300,                // 过期时间，建议签发后 5 分钟
  "iat": 1787970000,                // 签发时间
  "nbf": 1787970000,                // 在此时间前不可使用
  "jti": "01K...",                 // Token 唯一 ID，便于审计和防重放
  "sid": "kratos-session-id",      // 人类登录会话 ID；服务身份可以为空
  "amr": ["session"],              // session 或 service_token
  "client_id": "xhs-cli"           // 机器调用方 ID；浏览器请求可以为空
}
```

JWT 不写入组织角色和动态权限。原因是关系可能在 JWT 有效期内变化；资源权限由 Keto 实时计算。

签名私钥只交给 Oathkeeper。`auth_service`、`xhs_service` 通过配置加载同一份公共 JWKS，本地缓存，并在遇到未知 `kid` 时刷新。验证失败必须拒绝请求。

## 4. 权限模型

概念关系元组：

```text
Organization:G#member@Identity:alice
Organization:G#member@Identity:bob
Organization:G#admin@Identity:alice
```

权限关系：

```text
start_crawl_task  = member OR admin
view_crawl_content = member OR admin
update_keywords   = admin
```

一次 Bob 修改关键词的完整决策：

```text
PUT /v1/organizations/G/crawl/keywords
  → xhs_service 验证 JWT，得到 sub=identity:bob
  → 调 auth_service.CheckPermission(
        namespace="Organization",
        object="G",
        relation="update_keywords")
  → auth_service 从 JWT 得到 identity:bob
  → Keto 检查 Organization:G#update_keywords@Identity:bob
  → Bob 不是 admin，返回 allowed=false
  → xhs_service 返回 403，不修改数据
```

`auth_service` 不根据字符串自行实现角色判断；它只负责参数归一化、调用 Keto、转换结果和 fail-closed。

## 5. IDL 接口

### 5.1 auth_service

IDL：`idl/auth_service/authorization.proto`

| RPC | HTTP | 用途 |
| --- | --- | --- |
| `ResolveSession` | `GET /internal/auth/v1/session` | 把 Kratos Session 归一化为 Principal |
| `CheckPermission` | `POST /internal/auth/v1/permissions/check` | 使用 JWT `sub` 检查 Keto 权限 |

Service Token introspection 将在 Linux 实现阶段补入同一 IDL，然后重新生成代码。

### 5.2 xhs_service

IDL：`idl/xhs_service/crawl.proto`

| RPC | HTTP | 所需 Keto relation |
| --- | --- | --- |
| `StartCrawlTask` | `POST /v1/organizations/:organization_id/crawl/tasks` | `start_crawl_task` |
| `ListCrawlContents` | `GET /v1/organizations/:organization_id/crawl/contents` | `view_crawl_content` |
| `GetKeywords` | `GET /v1/organizations/:organization_id/crawl/keywords` | `view_crawl_content` |
| `UpdateKeywords` | `PUT /v1/organizations/:organization_id/crawl/keywords` | `update_keywords` |

所有请求通过 `Authorization` header 传入内部 JWT。Handler 只负责 DTO/Command 转换；JWT 验证位于 `biz/middleware`，业务授权编排位于 `biz/service` 或 `biz/workflow`。

## 6. 目录约束

```text
auth_service/
├── biz/domain/identity
├── biz/domain/authorization
├── biz/service/identity
├── biz/service/authorization
├── biz/shared/client/ory
└── biz/handler/...

xhs_service/
├── biz/domain/crawl
├── biz/service/crawl
├── biz/shared/client/auth_service
├── biz/middleware
└── biz/handler/...
```

必须遵守 `harness/contributing/architecture.md`：

- 生成的 `hertz_gen/**`、router 和 DAL query 不手改。
- 生成 DTO 只允许 Handler 引用。
- Service 不依赖 Hertz、Ory DTO 或具体 DAL。
- Ory client 初始化和 Hertz 下游 client 装配收口在 `biz/shared/client`。
- 复杂权限决策 fail-closed；Keto 不可用时返回 503，不能放行。

## 7. 非 HTTP 入口

### 定时任务

定时任务使用自己的 Service Token 换取短期内部 JWT，`sub=service:<job-name>`。Worker 调用业务服务时仍经过相同 JWT 验证和 Keto 授权，不伪造用户身份。

### MQ 消费

消息中不保存可能过期的用户 JWT。事件信封只保存 `actor_subject`、`organization_id`、`trace_id` 和业务数据；消费者使用自身服务身份执行，并在处理时重新向 Keto 鉴权。只有受信生产者可以写入 actor 信息，消息通道需要 ACL 和签名或平台级身份保护。

## 8. 验收场景

| 场景 | 预期 |
| --- | --- |
| Alice 修改组织 G 的关键词为“技术” | 200 |
| Bob 查看组织 G 的抓取内容 | 200 |
| Bob 修改组织 G 的关键词为“Agent” | 403 |
| JWT 签名错误、过期或 audience 错误 | 401 |
| 请求 body 尝试提交 Alice 的 subject | 不存在该字段，无法覆盖 JWT `sub` |
| Keto 不可用 | 503，且业务数据不变 |
| 未知组织 | 403 或 404，接口行为保持一致且不泄漏成员信息 |
