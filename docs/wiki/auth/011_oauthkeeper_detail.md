---
weight: 11
title: "11 Ory Oathkeeper：认证、授权、Token 转换与 Gateway 复用"
date: 2026-09-01T10:00:00+08:00
lastmod: 2026-09-01T10:00:00+08:00
draft: false
author: "宋涛"
authorLink: "https://hotttao.github.io/"
description: "通过具体场景理解 Ory Oathkeeper 的认证器、授权器、Token Mutator、错误处理、规则管理和多 Gateway 复用"
tags: ["auth", "ory", "oathkeeper", "gateway", "jwt"]
categories: ["microservice"]
toc:
  auto: false
---

Ory Oathkeeper 位于 Gateway 和身份服务之间，负责把“外部请求凭证”转换成
“内部服务可以验证的身份上下文”。它不是完整的 API Gateway，不负责服务发现、
负载均衡、限流或业务数据。

```text
Client
  │ Cookie / Bearer Token
  ▼
Gateway
  │ /decisions
  ▼
Oathkeeper
  │ 认证 -> 授权 -> Token Mutator
  ▼
Business Service
```

本项目使用 Oathkeeper Decision API 模式：Traefik 负责业务请求转发，Oathkeeper
负责鉴权决策和身份转换。

## 1. 认证器：请求中的凭证是否有效

认证器（Authenticator）回答：

> 当前请求携带的凭证能否证明调用者是谁？

它把 Cookie、JWT 或 OAuth Token 转换成 Oathkeeper 内部的认证结果：

```text
credential
  -> Authenticator
  -> Subject + Extra
```

### 场景：浏览器携带 Kratos Session

浏览器请求业务接口：

```http
GET /v1/xhs/organizations/G/crawl/contents
Cookie: ory_kratos_session=...
```

Oathkeeper 的 `cookie_session` Authenticator 调用：

```http
GET http://kratos:4433/sessions/whoami
Cookie: ory_kratos_session=...
```

Kratos 返回有效 Session 后，Oathkeeper 得到：

```text
Subject = identity.id
Extra   = Session 的其他可信信息
```

它解决的问题是：业务服务不需要理解 Kratos Cookie，也不需要每次调用
Kratos `/sessions/whoami`。业务服务只接收后续生成的内部身份凭证。

### 其他认证场景

| Authenticator | 凭证 | 适用场景 |
| --- | --- | --- |
| `cookie_session` | Kratos Session Cookie | 浏览器访问业务 API |
| `jwt` | 带签名 JWT | 客户端已经持有 JWT |
| `oauth2_introspection` | 不透明 OAuth Token | 需要向 Hydra 等服务查询 Token |
| `oauth2_client_credentials` | Client Credentials | 服务或自动化任务调用 |
| `anonymous` | 无凭证 | 明确允许匿名的公共接口 |

Authenticator 只负责证明身份，不代表这个身份已经有权访问某个文档、组织或
抓取任务。

## 2. 授权器：这个身份是否允许当前请求

授权器（Authorizer）回答：

> 已经认证的 Subject，是否允许访问当前请求代表的资源？

典型流水线是：

```text
Session -> Subject=Alice
             │
             ▼
Authorizer
             │
       allow / deny
```

### `allow`：只要求认证成功

```yaml
authorizer:
  handler: allow
```

它只表示：

```text
认证成功 -> 允许继续
```

当前 `002_internal_jwt` 使用这个模式，把组织和资源权限交给
`xhs_service` 以及后续的 Keto 模块判断。它适合先建立统一身份传递链路，
也适合所有已登录用户都可以访问的内部 API。

### Keto 或 Remote Authorizer：需要资源权限时

例如：

```text
PUT /v1/xhs/organizations/G/crawl/keywords
Subject = Alice
```

业务权限问题是：

```text
Alice 是否是 organization:G 的 editor？
```

这不是 `cookie_session` 能回答的问题。可以让 Oathkeeper 使用 Keto Authorizer，
或者调用一个 Permission Adapter：

```text
Oathkeeper
  -> Permission Adapter
       -> Keto Check
            subject=Alice
            object=organization:G
            relation=editor
```

授权器解决的是“身份有效但权限不足”的问题，失败结果通常是 `403 Forbidden`。
它与认证失败的 `401 Unauthorized` 必须区分。

## 3. Token Mutator：把可信身份转换给下游服务

Mutator（身份转换器）在认证和授权成功后执行：

```text
Subject + Extra
  -> Mutator
  -> 下游请求 Header / Cookie / JWT
```

### 场景：Session 转换为 Internal JWT

当前项目使用 `id_token` Mutator：

```yaml
mutators:
  id_token:
    enabled: true
    config:
      issuer_url: http://oathkeeper:4456/
      jwks_url: file:///etc/oathkeeper/id_token.jwks.json
      ttl: 5m
      claims: |
        {
          "aud": ["internal-api"],
          "principal_type": "user"
        }
```

请求过程：

```text
1. Oathkeeper 使用 cookie_session 验证 Kratos Session
2. 得到 Subject=Kratos identity.id
3. id_token Mutator 使用私钥签发短期 JWT
4. Oathkeeper 返回 Authorization: Bearer <JWT>
5. Traefik 把该 Header 写入发往业务服务的请求
6. xhs_service 使用 Oathkeeper 的 JWKS 公钥验签
```

JWT 的标准字段由 Oathkeeper 补充：

```json
{
  "iss": "http://oathkeeper:4456/",
  "sub": "kratos-identity-id",
  "aud": ["internal-api"],
  "iat": 1787904000,
  "nbf": 1787904000,
  "exp": 1787904300,
  "jti": "token-id"
}
```

私钥只用于 Oathkeeper 签名；下游服务通过：

```http
GET http://oathkeeper:4456/.well-known/jwks.json
```

获取公钥并缓存。这样业务服务不需要共享私钥，也不需要理解 Kratos Session。

### 其他 Mutator

| Mutator | 作用 | 适用场景 |
| --- | --- | --- |
| `id_token` | 签发短期 JWT | 多个内部服务需要本地验签 |
| `header` | 写入可信身份 Header | 旧服务只能读取 Header |
| `cookie` | 写入身份 Cookie | 需要向上游兼容 Cookie 协议 |
| `noop` | 不转换身份 | 公共接口或只需要做鉴权决策 |

明文身份 Header 容易被客户端伪造。除非有严格的网络边界和 Header 清理策略，
否则应优先使用签名 JWT。

## 4. 错误处理：统一不同认证失败的外部表现

Oathkeeper 的错误处理器（Error Handler）负责把流水线失败转换成客户端能理解的
HTTP 响应。

```text
Authenticator / Authorizer / Mutator error
                 ↓
             Error Handler
                 ↓
           JSON / Redirect / WWW-Authenticate
```

### API 请求：返回 JSON

业务 API 通常使用 JSON 错误：

```http
HTTP/1.1 401 Unauthorized
Content-Type: application/json
```

适合浏览器前端通过 Fetch、Native App 和 CLI 调用。客户端可以根据状态码决定
重新登录、刷新凭证或显示无权限信息。

### 浏览器页面：跳转登录

页面访问可以配置 Redirect Handler：

```text
没有有效 Session
  -> Oathkeeper
  -> 303 Login UI?return_to=...
```

但当前 `002_internal_jwt` 的 Decision API 面向业务 API，默认使用 JSON 错误；
登录页面跳转仍由 Account UI 和 Kratos Browser Flow 负责。

错误处理解决的问题是：不同上游认证服务的错误格式不会直接泄漏给每个业务服务，
Gateway 也可以对浏览器请求和 API 请求采用不同响应方式。

## 5. 规则管理：把请求入口策略配置化

Access Rule 把请求特征与安全流水线绑定：

```text
Host + Path + Method
        ↓
      Rule
        ├── Authenticator
        ├── Authorizer
        ├── Mutator
        └── Upstream / Error Handler
```

当前项目的宽匹配 Rule：

```yaml
- id: internal-api-authentication
  match:
    url: http://192.168.2.41:8080/v1/<.*>
    methods: [GET, POST, PUT, PATCH, DELETE]
  authenticators:
    - handler: cookie_session
  authorizer:
    handler: allow
  mutators:
    - handler: id_token
```

它的目的不是描述每一个业务权限，而是规定：

```text
所有 /v1/ API 都必须先完成统一认证，并获得 Internal JWT
```

### 新增服务时是否需要修改 Rule

如果新接口仍然位于同一个 API Host 和 `/v1/` 前缀下，通常不需要修改这条 Rule：

```text
GET  /v1/reports
POST /v1/orders
DELETE /v1/tasks/1
```

业务服务自身负责验证 JWT，并判断具体权限。只有以下情况才需要增加或调整 Rule：

- 新服务使用不同 Host 或 API 前缀；
- 某个接口需要不同认证器，例如只接受 Bearer JWT；
- 某个接口需要更高认证等级或额外凭证；
- 某个接口需要不同的 Mutator 或错误处理方式。

### Rule 的来源和更新

Rule 可以来自：

```text
本地文件
HTTP(S) 仓库
S3 / GCS / Azure Blob
```

生产环境应把 Rule 放入版本控制，变更经过语法检查、代码 review 和接口回归测试。
Rule 更新后还要确认 Oathkeeper 已重新加载，并检查 `/rules` 或健康检查结果。

## 6. 多种 Gateway 复用：统一 HTTP Decision API

Oathkeeper 的 Decision API 是普通 HTTP 接口，因此不同 Gateway 不需要理解
Oathkeeper 的内部 Go 实现，只需要遵守同一个请求契约：

```text
Gateway
  -> 发送原始 Method、Scheme、Host、URI 和凭证
  -> Oathkeeper /decisions
  <- 200 / 401 / 403
  <- 允许转发的身份 Header
```

### Traefik

```text
Traefik ForwardAuth
  -> http://oathkeeper:4456/decisions
  -> 读取 Oathkeeper 返回的 Authorization Header
  -> 转发到业务服务
```

### Nginx

```text
Nginx auth_request
  -> Oathkeeper /decisions
  -> auth_request_set 复制允许的响应 Header
  -> proxy_pass 到业务服务
```

### Envoy / Istio

```text
Envoy HTTP ext_authz
  -> Oathkeeper /decisions
  -> 200 才继续转发
  -> 将允许的 Header 传给上游
```

### APISIX 和其他 Gateway

APISIX 的 `forward-auth`、Kong 的外部认证插件以及其他支持 HTTP Forward Auth
的 Gateway，也可以复用同一个 Oathkeeper Decision API。

这样解决的问题是：认证规则、Kratos Session 适配、JWT 签发和错误语义只维护一份，
Gateway 只负责自己的路由、TLS、限流和负载均衡。更换 Traefik、Nginx 或 Envoy
时，不需要把 Oathkeeper 的认证逻辑重新实现一遍。

### 复用时的安全边界

无论使用哪种 Gateway，都必须保证：

- Gateway 删除客户端传入的内部身份 Header；
- Gateway 生成或传递可信的原始请求 Header；
- Oathkeeper 的 Decision API 不直接暴露给公网；
- 只有 `200` 的 Decision 结果可以继续访问上游；
- 业务服务仍然验证 Internal JWT，不能只相信请求来自 Gateway。

## 7. 一次完整请求中六个能力如何协作

以浏览器查看抓取内容为例：

```text
GET /v1/xhs/organizations/G/crawl/contents
Cookie: ory_kratos_session=...
        │
        ▼
Traefik
  └── 调用 Oathkeeper /decisions
        │
        ▼
Rule Management
  └── 找到 internal-api-authentication
        │
        ▼
Authenticator
  └── cookie_session -> Kratos /sessions/whoami -> Subject
        │
        ▼
Authorizer
  └── allow，或调用 Keto 判断资源权限
        │
        ▼
Token Mutator
  └── id_token -> Internal JWT
        │
        ▼
Error Handler
  └── 失败返回 JSON 401/403；成功返回 200
        │
        ▼
Traefik -> xhs_service
          Authorization: Bearer <Internal JWT>
```

最终职责边界是：

| 问题 | 负责组件 |
| --- | --- |
| 用户是谁 | Kratos + Oathkeeper Authenticator |
| 是否允许请求进入内部网络 | Oathkeeper Authorizer / Gateway 策略 |
| 内部如何传递可信身份 | Oathkeeper Token Mutator |
| 错误如何返回给客户端 | Oathkeeper Error Handler |
| 哪些 URL 使用哪条安全链路 | Oathkeeper Rule Repository |
| 业务资源是否允许操作 | `xhs_service`、Keto 或其他业务授权服务 |
| 路由、TLS、限流和负载均衡 | Gateway |

Oathkeeper 的价值不是单独“多生成一个 JWT”，而是把这套认证、授权、身份转换
和 Gateway 适配流程做成可以被不同入口复用的独立安全边界。
