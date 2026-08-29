# Linux 实施计划

## 1. 当前状态

已完成：

- [x] 根 Go workspace、Hertz 基础设施和 `xhs_service` 骨架。
- [x] `idl/xhs_service/crawl.proto` 及 Hertz model/client/handler/router/OpenAPI 生成。
- [x] Kratos、Keto、Oathkeeper、Talos 以 Git submodule 放入 `third_party/ory/`。
- [x] 公共 JWT/JWKS 验证器初版，覆盖签名、issuer、audience、有效期和未知 `kid` 刷新。

待完成：

- [ ] 移除旧的自建 `auth_service` 路线及相关 IDL/生成产物。
- [ ] 完成 Oathkeeper Internal JWT 中间件接入和主体归一化。
- [ ] 实现 `xhs_service` 的 Keto adapter、业务用例和仓库。
- [ ] 添加 Kratos、Talos、Keto、Oathkeeper 配置与容器编排。
- [ ] 完成 Alice、Bob、CLI 和故障场景的端到端测试。

业务数据不落库。`xhs_service` 使用进程内 mock repository，所有验收重点放在认证、Keto 鉴权、拒绝和依赖故障语义。

## 2. 生成与基线

```bash
go version
hz --version
protoc --version
docker version
docker compose version

go work sync
make hz-model
make hz-client
make -C xhs_service hz-update
make -C xhs_service swagger-gen
```

禁止手工修改 `hertz_gen/**`、生成 router 和 DAL query。业务 Handler 只填写函数体。

## 3. 清理服务边界

1. 从 `go.work`、根生成列表和部署配置中移除 `auth_service`。
2. 删除 `idl/auth_service` 和 `hertz_gen/auth_service`。
3. 删除自建 `auth_service` 模块。
4. 重新生成并构建，确保仓库只包含一个业务服务 `xhs_service`。

Kratos、Talos、Keto 的协议直接使用固定版本的官方 HTTP/OpenAPI，不复制为自有 Hertz IDL。

## 4. 内部 JWT

在 `hertz_infra/internaljwt` 保留无业务含义的公共验证能力：

- 只接受配置白名单中的非对称算法。
- 验证签名、issuer、audience、`exp`、`iat`、`nbf` 和 `jti`。
- JWKS 本地缓存；未知 `kid` 强制刷新一次；刷新失败继续使用未过期缓存。
- 拒绝 `alg=none`、HMAC 降级、私钥或对称 JWK。

在 `xhs_service/biz/middleware/internal_jwt.go`：

- 保护 `/v1/` 业务 API，不影响 health、metrics、pprof。
- 删除不受信的身份 Header。
- 人类请求使用 `sub`；机器请求使用受信 `service_actor`。
- 把归一化 Principal 写入 `context.Context`。

测试至少覆盖正常 Token、过期、错误 audience、错误 issuer、错误签名、未知 `kid` 刷新和算法降级。

## 5. Keto 授权

在 domain/application 边界定义最小接口：

```go
type PermissionChecker interface {
    Check(ctx context.Context, subject Subject, resource Resource, relation Relation) (bool, error)
}
```

`biz/shared/client/keto` 使用与 submodule 锁定版本匹配的 Keto OpenAPI 客户端：

- 调用 Keto Check API。
- 设置 HTTP timeout、有限重试、tracing 和熔断。
- `allowed=false` 映射为业务拒绝。
- 网络错误和 5xx 映射为 dependency unavailable。
- Keto 故障一律 fail-closed。

## 6. xhs_service 业务实现

```text
Handler
  → Command
  → crawl Service
  → PermissionChecker (Keto)
  → Repository
```

| 用例 | Keto relation |
| --- | --- |
| StartCrawlTask | `start_crawl_task` |
| ListCrawlContents | `view_crawl_content` |
| GetKeywords | `view_crawl_content` |
| UpdateKeywords | `update_keywords` |

先鉴权，再读取或修改业务数据。写入用例在 Keto 拒绝或不可用时不得进入事务写路径。

## 7. Ory 与 Talos 配置

部署目录：`deployments/ory/`。

### Kratos

- 配置 identity schema、自助登录流程和 Session Cookie。
- 创建 Alice、Bob，记录稳定 identity ID。

### Talos

- 只在内部网络运行 Admin API。
- 配置数据库、HMAC secret、issuer 和 JWT signing JWKS。
- 为 `service:xhs-cli` 创建长期 API Key。
- 派生 JWT 默认和最大 TTL 配为 5 分钟。
- 私钥和 API Key 明文不进入仓库。

### Keto

- 定义 Organization 的 member、admin、automation 关系与业务权限。
- 写入 Alice、Bob 和 `service:xhs-cli` 的关系元组。

### Oathkeeper

- 浏览器规则：`cookie_session` 调 Kratos `/sessions/whoami`。
- CLI 规则：`jwt` 验证 Talos derived JWT 和 Talos JWKS。
- 两条规则最终都由 `id_token` mutator 签发 5 分钟 Internal JWT。
- CLI 规则只从 Talos 内建 `act` claim 复制 `service_actor`。
- 转发时覆盖外部 `Authorization`。

## 8. Docker 与联调

根 `compose.yaml` 至少包含：

- PostgreSQL
- Kratos migration + Kratos
- Keto migration + Keto
- Talos migration + Talos Admin
- Oathkeeper
- xhs_service

不要在 compose 或 Git 中写死签名私钥、Talos API Key 或 HMAC secret；开发环境通过未跟踪的 `.env` 或 Docker secret 注入。

## 9. 测试与验收

```bash
go test ./hertz_infra/...
go test ./xhs_service/...
cd xhs_service && go run ../harness/tools/archcheck
git diff --check
```

端到端顺序：

1. Alice 登录并修改关键词，返回 200。
2. Bob 查看内容，返回 200。
3. Bob 修改关键词，返回 403。
4. CLI 用 Talos API Key 派生 JWT，再启动抓取任务，返回 200。
5. 吊销 Talos Key后再次派生，必须失败。
6. 停止 Keto 后执行写操作，返回 503 且数据不变。
7. 过期或错误 audience 的 JWT 返回 401。

## 10. 完成定义

- 只开发 `xhs_service`，不自建认证门面。
- 人类身份由 Kratos 管理，机器凭证由 Talos 管理。
- 所有业务入口经过 Oathkeeper，业务服务只接受短期 Internal JWT。
- 资源权限由 Keto 实时计算并 fail-closed。
- Ory/Talos DTO 不进入 Domain、Policy 或 Service。
- Alice、Bob、CLI、Talos 吊销和 Keto 故障均有自动化证据。
