# Linux 实施计划

## 1. 当前迁移点

已完成：

- [x] 旧版 `cmd/`、`internal/` 混合实现已可恢复地归档到 `legacy/`。
- [x] 补齐根 workspace、基础 IDL、`hertz_gen` 和 `hertz_infra`。
- [x] 使用 `harness/tools/new_server` 创建独立的 `auth_service`（8081）。
- [x] 使用 `harness/tools/new_server` 创建独立的 `xhs_service`（8082）。
- [x] 创建 `idl/auth_service/authorization.proto`。
- [x] 创建 `idl/xhs_service/crawl.proto`。
- [x] 两个服务的 health 骨架可以在 Windows 编译。

尚未完成：

- [ ] 正式业务 IDL 的 Hertz model、client、handler 和 router 生成。
- [ ] 内部 JWT 中间件与公共 JWKS 加载。
- [ ] Kratos、Keto OpenAPI adapter。
- [ ] Service Token 管理与 introspection。
- [ ] `auth_service`、`xhs_service` 业务实现及测试。
- [ ] Oathkeeper rules、Ory 容器编排和端到端测试。
- [ ] 最终 Docker 镜像构建。

迁移到 Linux 后不要再次运行 `new_server`，否则会因为目标目录已存在而失败。应从 IDL 生成开始。

## 2. Linux 前置检查

```bash
go version
hz --version
protoc --version
make --version
docker version
docker compose version
```

确认 Go 版本满足 `go.work`，并安装：

- `protoc-gen-go`
- `protoc-gen-openapi`
- Hertz `hz`
- 架构检查工具所需依赖

然后执行：

```bash
go work sync
go test ./harness/tools/new_server
```

## 3. Phase 1：重新生成 IDL 产物

严格按照仓库规范执行：

```bash
make hz-model
make hz-client

make -C auth_service hz-update
make -C xhs_service hz-update

make -C auth_service swagger-gen
make -C xhs_service swagger-gen
```

注意：

- Linux 上 `hz --use media_agent/hertz_gen/model` 可以正常工作。
- 生成后检查 Handler import 指向 `media_agent/hertz_gen/model/...`。
- 不手工修改 `hertz_gen/**`、`router_gen.go` 和生成 router。
- `hz update` 可能重建 Handler 骨架；业务实现应通过薄 Handler 调用 Service。

验收：

```bash
go build ./auth_service/...
go build ./xhs_service/...
```

## 4. Phase 2：内部 JWT 基础设施

在两个服务分别实现 `biz/middleware/internal_jwt.go`：

1. 从 `Authorization: Bearer` 提取 JWT。
2. 根据 `kid` 从公共 JWKS 选择公钥。
3. 验证允许的算法、签名、`iss`、`aud=internal-api`、`exp`、`nbf`。
4. 将 `Principal` 写入 `context.Context`。
5. 禁止信任客户端直接提交的 `X-User-ID`、`X-Role` 等身份 Header。

公共代码只共享无业务含义的解析能力；每个服务仍需显式配置 issuer、audience 和 JWKS。

单元测试至少覆盖：

- 正常 Token
- 过期 Token
- 错误 audience
- 错误 issuer
- 未知 `kid` 刷新
- `alg=none` 或算法降级

## 5. Phase 3：auth_service

### 5.1 Port 接口

在 domain/application 边界定义最小接口，不暴露 Ory DTO：

```go
type SessionResolver interface {
    Resolve(ctx context.Context, cookie, sessionToken string) (Principal, error)
}

type PermissionChecker interface {
    Check(ctx context.Context, subject Subject, resource Resource, relation Relation) (bool, error)
}
```

### 5.2 Adapter

- Kratos adapter：调用 `/sessions/whoami`，映射 identity/session/expiry。
- Keto adapter：调用 relation check API，映射 `allowed`。
- 为 HTTP timeout、错误类型、重试和日志设置边界。
- 4xx 转换为认证/拒绝错误；网络错误和 5xx 转换为 dependency unavailable。

优先使用与部署版本匹配的 Ory 官方 Go SDK；若官方 SDK 粒度过大，则固定 Ory OpenAPI 版本生成客户端。禁止手写完整 Ory IDL 镜像。

### 5.3 Service Token

先修改 `idl/auth_service/authorization.proto`，增加：

- 创建/吊销 Service Token 的管理接口。
- 给 Oathkeeper 使用的 introspection 接口。
- Token 明文只在创建时返回一次，数据库只保存哈希、client_id、状态、过期时间。

修改 IDL 后重新执行 Phase 1 的生成命令。

## 6. Phase 4：xhs_service

实现 DDD 分层：

```text
Handler
  → Command
  → crawl Service
  → Authorization Port / auth_service Hertz Client
  → Repository
```

每个用例在写入或读取数据前映射权限：

| 用例 | Keto relation |
| --- | --- |
| StartCrawlTask | `start_crawl_task` |
| ListCrawlContents | `view_crawl_content` |
| GetKeywords | `view_crawl_content` |
| UpdateKeywords | `update_keywords` |

`xhs_service/biz/shared/client` 使用生成的 `hertz_gen/auth_service` typed client，并通过 `hertz_infra/clienthertz.ClientSuite` 安装 timeout、retry、tracing 和 circuit breaker。

内部调用转发原始 Internal JWT。`auth_service` 从 JWT 重新提取主体，不能接受 xhs_service 提交的 subject。

## 7. Phase 5：Ory 配置

部署目录：`deployments/ory/`。

### Kratos

- 配置 identity schema 和自助登录流程。
- 创建 Alice、Bob，并记录稳定 identity ID。
- Session Cookie 仅供客户端与入口认证使用。

### Keto

- 定义 Organization 权限关系。
- 写入组织 G 的 member/admin 元组。
- 为三种业务 relation 添加检查测试。

### Oathkeeper

- 浏览器规则：`cookie_session` → Kratos `/sessions/whoami`。
- CLI 规则：Bearer Service Token → introspection。
- 认证成功：`id_token` mutator 签发 5 分钟 Internal JWT。
- 转发时覆盖外部 `Authorization`，不能同时保留客户端伪造的内部 JWT。

私有 JWKS 只挂载给 Oathkeeper；公共 JWKS 挂载或发布给两个服务。

## 8. Phase 6：Docker 与联调

先构建独立镜像：

```bash
docker build -f auth_service/Dockerfile -t ddd-learn/auth-service:dev .
docker build -f xhs_service/Dockerfile -t ddd-learn/xhs-service:dev .
```

再补充根 `compose.yaml`，至少包含：

- PostgreSQL
- Kratos migration + Kratos
- Keto migration + Keto
- Oathkeeper
- auth_service
- xhs_service

不要在 compose 中写死签名私钥或 Service Token。

## 9. Phase 7：测试与验收

单元测试：

```bash
go test ./auth_service/...
go test ./xhs_service/...
go test ./hertz_infra/...
```

架构检查：

```bash
cd auth_service && go run ../harness/tools/archcheck
cd ../xhs_service && go run ../harness/tools/archcheck
```

端到端顺序：

1. Alice 登录后修改关键词“技术”，返回 200。
2. Bob 登录后查看抓取内容，返回 200。
3. Bob 尝试修改关键词“Agent”，返回 403。
4. 使用 CLI Service Token 启动抓取任务，按 Keto 中服务主体权限返回结果。
5. 停止 Keto，再执行写操作，返回 503 且数据不变。
6. 使用过期或错误 audience 的 JWT，返回 401。

最终检查：

```bash
gofmt -w auth_service xhs_service
go test ./auth_service/... ./xhs_service/...
git diff --check
```

## 10. 完成定义

- 两个服务均可独立构建、测试和制作镜像。
- 业务 API 全部来自 IDL 生成，不手写路由。
- Kratos/Keto DTO 不进入 Domain、Policy、Service。
- Gateway 请求不会让每个业务服务同步调用 Kratos。
- 内部 JWT 字段、验证规则和公共密钥分发方式有测试。
- Alice/Bob、Service Token、Keto 故障三个核心路径均有端到端证据。
