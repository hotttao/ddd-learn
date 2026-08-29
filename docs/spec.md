# DDD Hertz + Ory 认证鉴权实验：技术规格

## 1. 目标

本项目验证一条以 Ory 现成组件为主的认证鉴权链路，不自建认证服务，也不复制 Ory API：

- Kratos 管理人类用户、登录流程和 Session。
- Talos 管理机器 API Key，并从长期 Key 派生短期 JWT。
- Oathkeeper 作为统一入口，验证外部凭证并签发统一内部 JWT。
- Keto 计算组织资源权限。
- `xhs_service` 处理小红书抓取业务，并在资源操作前直接检查 Keto。

业务场景：组织 `G` 有 Alice 和 Bob。

- Alice 是管理员，可以启动抓取任务、查看内容、修改关键词。
- Bob 是普通成员，可以启动抓取任务、查看内容，不能修改关键词。
- CLI 使用 Talos 管理的机器身份调用抓取接口。

本项目只开发一个业务微服务：

```text
xhs_service  # 小红书抓取业务服务和权限执行点（PEP）
```

本实验不创建或依赖业务数据库；抓取任务、关键词和内容接口使用进程内 mock 返回，数据库目录仅保留模板，不参与启动 wiring。

## 2. 架构边界

```text
Browser ── Session Cookie ───────────────┐
                                         ▼
                                    Oathkeeper ──► Kratos /sessions/whoami
                                         │
CLI ── Talos derived JWT ────────────────┤ JWT authenticator
                                         │
                                         └─► 签发 5 分钟 Oathkeeper Internal JWT
                                                        │
                                                        ▼
                                                   xhs_service
                                                        │
                                                        └─► Keto Check API

Talos API Key ── private network / mTLS ──► Talos :derive ──► Talos derived JWT
```

| 组件 | 负责 | 不负责 |
| --- | --- | --- |
| Kratos | 人类用户、凭证、登录流程、Session | OAuth Token、机器 API Key、业务权限 |
| Talos | API Key 生命周期、哈希存储、轮换、吊销、短期派生 JWT | 人类登录、业务资源权限 |
| Oathkeeper | 入口认证、请求准入、内部 JWT 签发 | 保存用户、管理 API Key、计算资源权限 |
| Keto | 基于关系元组计算资源级权限 | 登录、Session、签发 JWT |
| xhs_service | 抓取业务、JWT 本地验证、Keto 权限执行 | 登录、凭证存储、权限规则存储 |

Ory HTTP/OpenAPI 是 Ory 接口的唯一协议源。项目不得把 Kratos、Talos 或 Keto 的完整 API 转写成自有 IDL。

## 3. 人类请求

1. 浏览器完成 Kratos 登录并持有 Session Cookie。
2. 浏览器携带 Cookie 请求 Oathkeeper。
3. Oathkeeper 的 `cookie_session` authenticator 调用 Kratos `GET /sessions/whoami`。
4. Oathkeeper 将 Kratos identity ID 归一化为 `sub=identity:<id>`。
5. `id_token` mutator 签发 5 分钟 Internal JWT，`amr=["session"]`。
6. `xhs_service` 本地验证 Internal JWT，不在每次请求中调用 Kratos。

## 4. 机器请求

### 4.1 长期凭证

Talos 签发和保存机器 API Key：

- Key 明文只在创建或轮换时返回一次。
- Talos 数据库只保存验证所需的哈希和元数据。
- `actor_id` 使用稳定机器主体，例如 `service:xhs-cli`。
- scopes 限制机器凭证可派生的最大权限范围。

Talos Admin API 没有内置认证，必须只在私有网络、mTLS 或受信管理代理后使用，不能直接暴露到公网。

### 4.2 换取短期凭证

CLI 不把长期 API Key 发送给业务 API。调用前先通过受保护网络调用：

```http
POST /v2alpha1/admin/apiKeys:derive
Content-Type: application/json

{
  "credential": "<talos-api-key>",
  "algorithm": "TOKEN_ALGORITHM_JWT",
  "ttl": "5m",
  "scopes": ["crawl"]
}
```

Talos 返回 derived JWT。CLI 用它请求 Oathkeeper：

1. Oathkeeper 的 `jwt` authenticator 使用 Talos JWKS 验证签名、issuer、有效期和 scopes。
2. Oathkeeper 从 Talos 受保护的 `act` claim 取得稳定 `actor_id`。
3. `id_token` mutator 签发统一 Internal JWT，写入 `amr=["service_token"]` 和 `service_actor`。
4. `xhs_service` 仅信任 Oathkeeper Internal JWT；机器主体取 `service_actor`，人类主体取 `sub`。

Talos Key 被吊销后不能再派生新 JWT；已经派生的 JWT 会持续有效到 `exp`，因此派生 TTL 和 Internal JWT TTL 均为 5 分钟。

## 5. Internal JWT

```jsonc
{
  "iss": "oathkeeper",
  "sub": "identity:<kratos-id> or talos-key-id",
  "aud": ["internal-api"],
  "exp": 1787970300,
  "iat": 1787970000,
  "nbf": 1787970000,
  "jti": "01K...",
  "sid": "kratos-session-id",
  "amr": ["session"],
  "service_actor": "service:xhs-cli"
}
```

主体归一化规则：

- `amr` 包含 `session`：主体必须来自 `sub`。
- `amr` 包含 `service_token`：主体必须来自 Oathkeeper 从 Talos `act` 复制的 `service_actor`。
- 请求 body 和 `X-User-ID`、`X-Role`、`X-Subject` 等 Header 永远不能覆盖主体。

JWT 不写入组织角色和动态资源权限。签名私钥只提供给 Oathkeeper；`xhs_service` 通过公共 JWKS 本地验证，并在未知 `kid` 时刷新。

## 6. 权限模型

```text
Organization:G#member@Identity:alice
Organization:G#member@Identity:bob
Organization:G#admin@Identity:alice
Organization:G#automation@Service:xhs-cli
```

```text
start_crawl_task   = member OR admin OR automation
view_crawl_content = member OR admin
update_keywords    = admin
```

Bob 修改关键词的决策：

```text
PUT /v1/organizations/G/crawl/keywords
  → xhs_service 验证 JWT，得到 identity:bob
  → xhs_service 调用 Keto 检查 Organization:G#update_keywords@Identity:bob
  → Keto 返回 allowed=false
  → xhs_service 返回 403，且不修改数据
```

Keto 不可用时必须 fail-closed：返回 503，不能放行业务操作。

## 7. xhs_service API

IDL：`idl/xhs_service/crawl.proto`

| RPC | HTTP | Keto relation |
| --- | --- | --- |
| `StartCrawlTask` | `POST /v1/organizations/:organization_id/crawl/tasks` | `start_crawl_task` |
| `ListCrawlContents` | `GET /v1/organizations/:organization_id/crawl/contents` | `view_crawl_content` |
| `GetKeywords` | `GET /v1/organizations/:organization_id/crawl/keywords` | `view_crawl_content` |
| `UpdateKeywords` | `PUT /v1/organizations/:organization_id/crawl/keywords` | `update_keywords` |

目录边界：

```text
xhs_service/
├── biz/domain/crawl
├── biz/domain/authorization
├── biz/service/crawl
├── biz/shared/client/keto
├── biz/middleware
└── biz/handler/crawl
```

Handler 只做 DTO/Command 转换；JWT 验证位于 `biz/middleware`；授权编排位于 `biz/service/crawl`；Keto OpenAPI adapter 位于 `biz/shared/client/keto`。

## 8. 非 HTTP 入口

定时任务持有自己的 Talos API Key，派生短期 JWT 后走相同 Oathkeeper、JWT 和 Keto 授权链路。

MQ 消息不保存可能过期的 JWT，只保存 `actor_subject`、`organization_id`、`trace_id` 和业务数据。消费者使用自己的 Talos 机器身份处理，并重新向 Keto 鉴权。

## 9. 验收场景

| 场景 | 预期 |
| --- | --- |
| Alice 修改组织 G 的关键词 | 200 |
| Bob 查看组织 G 的内容 | 200 |
| Bob 修改组织 G 的关键词 | 403 |
| CLI 用有效 Talos 派生 JWT 启动任务 | 200 |
| 已吊销 Talos Key 尝试派生新 JWT | 拒绝 |
| JWT 签名、issuer、audience 或有效期错误 | 401 |
| 请求提交伪造主体 Header | 被忽略或删除 |
| Keto 不可用 | 503，业务数据不变 |
| 未知组织 | 403 或 404，行为一致且不泄漏成员信息 |
