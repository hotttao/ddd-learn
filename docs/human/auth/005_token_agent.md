# Agent API Token、Gateway 与 Internal JWT

## 目标场景

用户 Alice 创建一个 AI Agent。Agent 需要访问内部 MCP、控制面和业务服务。创建
Agent 时，系统为 Agent 申请长期 API Token，Agent 后续携带 API Token 请求
agent_gateway，而不是使用 Alice 的 Kratos Session。

```text
Agent
  │ Authorization: Bearer <agent-api-token>
  ▼
agent_gateway
  │ Talos 验证 API Token
  │ 查询 Agent 与用户的委托关系
  │ 签发 Internal JWT
  ▼
MCP / Control Plane / Business Service
  │
  ▼
Keto
```

## 一、认证路由是否合理

合理。agent_gateway 可以作为外部 Agent 请求和内部服务之间的信任边界：

```text
Agent
  ↓ 长期 API Token
agent_gateway
  ↓ Talos verify
  ↓ Agent Registry / Keto
  ↓ 短期 Internal JWT
内部服务
  ↓
Keto 业务授权
```

它解决：

| 问题 | 解决方式 |
| --- | --- |
| Agent 不应持有用户 Session | Agent 使用独立 API Token |
| 长期 Token 不应在服务间传播 | Gateway 入口处转换 |
| 下游服务不应理解 Talos Token | 下游只验证统一 Internal JWT |
| Agent 需要代表用户执行任务 | JWT 同时保存用户和 Agent |
| Agent 泄露后需要单独撤销 | Talos 以 Agent 为主体管理 Token |
| 用户权限变化后需要生效 | Keto 按当前关系校验 |

agent_gateway 必须删除客户端提交的 X-Actor-ID、X-User-ID、X-Scopes 和
X-Organization-ID，再写入经过验证的身份结果。内部服务只信任签名 JWT。

## 二、Internal JWT 应该以谁为身份

### 2.1 用户创建的 Agent

不推荐只生成：

```text
sub = User:alice
```

这样下游无法区分 Alice 浏览器和 Alice 的 Agent。

推荐使用委托身份：

```text
sub     = User:<alice-identity-id>
act.sub = Agent:<agent-id>
```

含义是“当前业务操作代表 Alice，但实际发起请求的是 Agent”。这与 OAuth2
Token Exchange 的 actor/delegation 语义一致，即使本实验不使用 Hydra，也可以
采用相同的表达。

### 2.2 完全自主 Agent

如果 Agent 不代表任何用户，而是系统自有的自动化 Agent：

```text
sub = Agent:<agent-id>
不设置 act
```

| Agent 类型 | sub | act |
| --- | --- | --- |
| 用户创建的 Agent | User:id | Agent:id |
| 系统自主 Agent | Agent:id | 不设置 |
| 用户浏览器直接请求 | User:id | 不设置 |

## 三、agent_gateway 如何转换 API Token

### 3.1 API Token 是不透明凭证

不能从 API Token 字符串中截取 agent_id 后直接签发 JWT。必须调用 Talos：

```http
POST http://talos:4420/v2alpha1/admin/apiKeys:verify
Content-Type: application/json

{
  "credential": "<agent-api-token>"
}
```

实际 URL 和请求结构以当前 Talos 版本 OpenAPI 为准。

Talos 验证结果至少需要包含：

```json
{
  "is_valid": true,
  "key_id": "key-agent-17",
  "actor_id": "Agent:agent-17",
  "scopes": ["mcp.read", "xhs.read"],
  "metadata": {
    "organization_id": "G"
  }
}
```

Talos 负责 Token 的完整性、状态、过期时间、撤销状态、actor_id 和 Scope。
Talos 不负责判断 Agent 是否仍属于 Alice，也不负责业务权限。

### 3.2 查询 Agent 委托关系

agent_gateway 得到 Talos 结果后继续查询：

```text
Talos
  → actor_id=Agent:agent-17

Agent Registry / Keto
  → Agent:agent-17 owner User:alice

Keto
  → Alice 是否允许 Agent 访问 Organization:G
```

只有以下条件都满足，才能签发 Internal JWT：

```text
Talos API Token 有效
&& Agent:agent-17 状态为 active
&& Agent:agent-17 属于 User:alice
&& Agent 对 Organization:G 存在有效委托
&& API Token Scope 包含当前接口能力
```

### 3.3 Internal JWT Claims

```json
{
  "iss": "http://agent-gateway.internal",
  "sub": "User:<alice-identity-id>",
  "act": {
    "sub": "Agent:agent-17"
  },
  "subject_type": "delegated_agent",
  "auth_source": "talos",
  "credential_id": "key-agent-17",
  "organization_id": "G",
  "scope": ["mcp.read", "xhs.read"],
  "aud": ["mcp-service", "xhs-service"],
  "iat": 1788062400,
  "nbf": 1788062400,
  "exp": 1788062700,
  "jti": "internal-jwt-request-..."
}
```

| Claim | 含义 |
| --- | --- |
| sub | 当前业务操作代表的用户 |
| act.sub | 实际发起请求的 Agent |
| subject_type | user、service、agent 或 delegated_agent |
| auth_source | API Token 通过 Talos 验证 |
| credential_id | Token ID，便于审计 |
| organization_id | Gateway 校验后的组织上下文 |
| scope | API Token 验证得到的能力上限 |
| aud | 允许接收 JWT 的内部服务 |
| jti | 本次 JWT 的唯一编号 |

organization_id 不能信任 Agent 请求中的同名 Header，必须来自 Token 管理记录、
Agent 关系和 Keto 校验结果。

### 3.4 谁签发 Internal JWT

第一阶段建议 agent_gateway 自己签发：

```text
agent_gateway
  ├── Talos verify
  ├── Agent Registry/Keto
  ├── 使用专用私钥签发 JWT
  └── 发布 JWKS
```

签发逻辑应放在独立 issuer package 中，不要散落在代理 Handler 内。未来多个
Gateway 需要共享签发逻辑时，再拆成独立 Token Exchange Service。

Oathkeeper 的 id_token Mutator 可以签发 JWT，但 Talos API Key 不是 Oathkeeper
默认支持的 Kratos Cookie 或 OAuth2 Introspection 凭证。因此需要自定义 Talos
Authenticator/适配层，不能直接把 cookie_session 改成读取 API Token。

## 四、Keto 权限模型

### 4.1 Agent 必须是独立主体

只有下面关系时：

```text
Organization:G#members@User:alice
```

系统无法判断 Agent:agent-17 是否是 Alice 创建并允许使用的 Agent。

应增加 Agent 关系：

```text
Agent:agent-17#owner@User:alice
Agent:agent-17#enabled_for@Organization:G
```

具体 OPL 类型以当前 Keto Namespace 定义为准；核心是 Agent 作为独立主体。

### 4.2 有效权限是多个上限的交集

```text
effective_permission =
    user_permission
  ∩ agent_permission
  ∩ token_scope
  ∩ organization_permission
```

因此 Alice 有权限，不代表 Agent 自动拥有 Alice 的全部权限；Agent 有 Scope，
也不能绕过 Alice 或组织权限。

用户创建的 Agent 应按以下顺序校验：

```text
1. Agent:agent-17 owner User:alice
2. Agent:agent-17 enabled_for Organization:G
3. User:alice 是否拥有目标业务 Permission
4. Agent:agent-17 是否有额外限制
5. Token Scope 是否包含目标操作
```

系统自主 Agent 则直接以 Agent:agent-17 作为 subject，不设置用户委托关系。

## 五、完整请求过程

### 5.1 创建 Agent 和 Token

```text
1. Alice 通过 Kratos Session 创建 Agent
2. 创建 Agent:agent-17
3. 写入 Agent:agent-17 owner User:alice
4. 写入 Agent:agent-17 enabled_for Organization:G
5. Token Manager 调用 Talos：
   actor_id=Agent:agent-17
   scopes=[mcp.read,xhs.read]
6. 完整 Secret 只返回给 Agent
```

### 5.2 Agent 请求 xhs_service

```text
1. Agent -> agent_gateway
   Authorization: Bearer <agent-api-token>

2. agent_gateway -> Talos
   verify credential

3. Talos -> agent_gateway
   actor_id=Agent:agent-17
   scope=xhs.read

4. agent_gateway -> Agent Registry/Keto
   owner=User:alice
   enabled_for=Organization:G

5. agent_gateway 签发 Internal JWT
   sub=User:alice
   act.sub=Agent:agent-17

6. agent_gateway -> xhs_service
   Authorization: Bearer <internal-jwt>

7. xhs_service -> Keto
   检查用户、Agent、组织和 Permission
```

### 5.3 失败结果

```text
API Token 被撤销
  → Talos is_valid=false
  → 401 Unauthorized

Token 有效，但 Alice 没有组织权限
  → Keto=false
  → 403 Forbidden

Agent 已解绑组织
  → Agent relation=false
  → 403 Forbidden
```

401 表示凭证无效，403 表示身份有效但没有业务权限。

## 六、安全边界

长期 API Token：

- 只在 Agent 初始化时返回一次。
- 不写入日志、Trace、URL 或普通业务数据库。
- 支持过期、轮换、撤销和来源网段限制。
- Agent 删除时同步撤销全部 Token。

Internal JWT：

- 有效期建议 1～5 分钟。
- 使用 agent_gateway 专用私钥签发。
- 通过 JWKS 发布公钥。
- 设置严格 audience。
- 不把原始 API Token 放入 JWT。
- 下游拒绝非 agent_gateway 签发的 JWT。

## 七、实现步骤

### Step 1：定义 Agent 主体和数据

统一使用 Agent:<agent-id>、User:<identity-id>、Organization:<organization-id>，
记录 Agent owner、enabled、organization 和 Token 状态。

### Step 2：补充 Keto Agent Namespace

增加 owner、组织绑定、启用状态和业务权限关系，验证 Alice 创建的 Agent 能否
访问 xhs.read。

### Step 3：为 Agent 创建 Talos API Token

以 actor_id=Agent:agent-17 创建 Token，Scope 只授予 Agent 必需能力，metadata
用于辅助查询和审计，权限事实仍以 Agent Registry 和 Keto 为准。

### Step 4：实现 agent_gateway Token 验证

读取 Bearer、删除伪造 Header、Talos verify、查询 owner、检查 Scope 和组织。

### Step 5：实现 Internal JWT 签发

签发 sub=User:alice、act.sub=Agent:agent-17 的双身份 JWT。

### Step 6：接入 MCP 和控制面服务

每个下游服务只验证 Internal JWT，并按自己的 audience 和 Keto Permission 授权。

### Step 7：验证撤销和权限变化

验证 Token 撤销、Agent 禁用、Agent 组织解绑、Alice 权限删除和 Scope 删除。

## 八、最终建议

你的总体架构合理，但身份设计应采用：

```text
长期 API Token 代表 Agent
Internal JWT：
  sub     = 被代表的用户
  act.sub = 实际调用的 Agent
```

授权使用用户权限、Agent 权限、组织权限和 Token Scope 的交集；审计同时记录用户
和 Agent。不要只把 Internal JWT 的 sub 设置成用户，也不要让 Agent Token 直接
继承用户全部权限。

参考：

- [Ory Talos：API Key 与机器身份凭证](../../wiki/auth/009_ory_talos.md)
- [Ory Talos 官方介绍](https://www.ory.com/blog/ory-launches-ory-talos)
- [Ory Oathkeeper](https://www.ory.com/docs/oathkeeper)
