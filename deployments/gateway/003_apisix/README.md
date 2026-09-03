# gateway/003：APISIX 当前实验

本 README 按步骤累计记录本实验的全部实现，后续步骤只追加内容，不删除之前步骤。

## 第 1 步：复制认证基线

本实验首先从 `deployments/auth/003_keto` 原样复制为
`deployments/gateway/003_apisix`，保留 Kratos、Oathkeeper、Keto、PostgreSQL、
Mailpit、JWKS、用户初始化和 `xhs_service` 配置。

复制基线已经单独提交，APISIX 实验使用独立的 Compose 项目和数据卷，不与 auth/003
共享运行数据。

## 第 2 步：替换为 APISIX 并创建 Route/Upstream

删除复制来的 Traefik 配置，增加 APISIX 和 etcd：

```text
APISIX host :8080 → container :9080
APISIX Admin :9180
etcd :2379
```

APISIX 使用 traditional 模式，从 etcd 读取动态配置。`apisix-seed` 通过 Admin API
幂等写入：

```text
Upstream 1：roundrobin
  ├── xhs_service:8082
  └── xhs_service_2:8082

Route 1：xhs-api
  ├── URI：/v1/xhs/*
  ├── Host：192.168.2.41
  └── Methods：GET、POST、PUT
```

Upstream 对 `/health` 执行主动健康检查，配置保存于 etcd。验证结果：

```text
正确 Host + GET → 401，已转发到 xhs_service
DELETE          → 404，不满足方法条件
错误 Host        → 404，不满足 Host 条件
```

## 第 3 步：为 Route 增加限流 Plugin

本步骤通过 Admin API 在现有 `/v1/xhs/*` Route 上配置 `limit-count` Plugin。
它按客户端 `remote_addr` 统计请求，60 秒内允许 3 次，超过配额后由 APISIX 返回
`429`。Upstream 和两个 `xhs_service` 实例保持不变。

## 当前拓扑

```text
APISIX host :8080 → container :9080 → xhs-api Upstream → xhs_service / xhs_service_2 :8082
APISIX Admin :9180 ────────────────────────> APISIX 配置
etcd :2379 ──────────────────────────────> Route/Upstream/Plugin/Consumer 存储
Dashboard :8081 ──────────────────────────> 查看 etcd 中的 APISIX 配置
```

认证链路仍沿用 auth/003：Kratos、Oathkeeper 和 Keto 不由 APISIX 重复实现。
`xhs_service` 继续通过 Oathkeeper 签发的 Internal JWT 和 Keto 完成业务认证鉴权。

## 补充：部署 APISIX Dashboard

Dashboard 使用宿主机 `8081`，读取 etcd 中的 APISIX 配置：

```text
Browser :8081 → APISIX Dashboard → etcd :2379
```

访问：

```text
http://192.168.2.41:8081
用户名：admin
密码：admin
```

登录后可以查看 Route `xhs-api`、Upstream `1` 以及 Route 上的 `limit-count` Plugin。
Dashboard 只负责配置查看和管理，不进入 `8080` 的业务请求链路。Dashboard 登录密码
与 APISIX Admin API 的 `X-API-KEY` 是两套不同凭证。

## 启动

```shell
docker compose -f deployments/gateway/003_apisix/docker-compose.yml up -d
```

检查 Compose 配置：

```shell
docker compose -f deployments/gateway/003_apisix/docker-compose.yml config --quiet
```

检查 APISIX Admin API 和动态对象：

```shell
curl -H 'X-API-KEY: edd1c657-da07-4c75-bf47-9b6f4a4e8c12' \
  http://192.168.2.41:9180/apisix/admin/routes
```

启动时 `apisix-seed` 会幂等写入 Upstream `1` 和带有 `limit-count` Plugin 的 Route
`1`。

验证 Plugin：

```shell
for i in $(seq 1 5); do
  curl -i http://192.168.2.41:8080/v1/xhs/organizations/G/crawl/contents
done
```

前三次请求通过 Plugin，继续到 `xhs_service`，因尚未接入认证 Plugin 而返回 `401`；
第四次开始由 APISIX 直接返回 `429`。响应头中的 `X-RateLimit-Limit`、
`X-RateLimit-Remaining` 和 `X-RateLimit-Reset` 用于观察配额。

执行顺序是：

```text
Route 匹配
  -> limit-count Plugin
  -> 未超限：选择 Upstream -> xhs_service -> 401
  -> 已超限：APISIX 直接返回 429，不访问 Upstream
```

## 第 4 步：接入 Oathkeeper External Auth

在 Route 上增加 APISIX `forward-auth` Plugin：

```text
Client
  → APISIX /v1/xhs
  → Oathkeeper http://oathkeeper:4456/decisions
  → 认证通过：返回 Authorization: Bearer <Internal JWT>
  → APISIX 转发给 xhs_service
```

Oathkeeper 的 Rule 必须填写完整 URL 模板，但 Host 使用 `<.*>` 通配，因此实际只
约束 `/v1/` 路径，不绑定某个网关 IP 或宿主机端口。APISIX 仍会传递它生成的
`X-Forwarded-*` Header，客户端不能直接决定这些 Header 的可信值。

APISIX 只负责调用外部认证服务和复制 `Authorization` 响应头，不签发 JWT；JWT
仍由 Oathkeeper 的 `id_token` Mutator 生成。没有 Kratos Session Cookie 的请求由
Oathkeeper 拒绝，不会进入 `xhs_service`。

这避免了 APISIX `jwt-auth` 和 Oathkeeper 同时签发两种身份。后续如果改用 APISIX
原生 JWT，需要使用独立测试 Route 和 Consumer，不能直接叠加到当前业务 Route。

## 配置边界

- APISIX 动态配置保存于 etcd，不使用 Compose labels；
- APISIX 启动配置使用可写挂载，因为启动过程会回写本地节点信息；
- `9180` 是管理面，不能作为业务入口；
- `8080` 是宿主机数据面入口，容器内对应 APISIX `9080`；
- etcd 的 `2379` 仅为教学调试暴露，生产环境应限制为 APISIX 管理网络。

## 补充：接入 UI 主页面

复用前面已经构建的 `ui_example:0.0.1` 镜像，并由 APISIX 增加三个独立对象：

```text
Upstream 2：ui:80
Route 2：/* → Upstream 2
Route 3：/kratos/* → kratos:4433（去掉 /kratos 前缀）
```

Route 2 是浏览器主页面的兜底路由，优先级低于 `/kratos/*` 和 `/v1/xhs/*`，所以静态
页面、Kratos 公共 API、业务 API 分别进入各自的后端。UI 与业务接口使用同一个
`192.168.2.41:8080` 来源，前端请求 `/v1/xhs/*` 不需要再配置 Vite 开发代理。

访问主页面：

```text
http://192.168.2.41:8080/
```

`/login`、`/registration` 等前端路由由 Nginx 回退到 `index.html`；登录流程调用
同源的 `/kratos/self-service/*`，业务请求仍经过 `/v1/xhs/*` 的 Oathkeeper 认证。

启动或重新写入路由：

```shell
docker compose -f deployments/gateway/003_apisix/docker-compose.yml up -d ui apisix-seed
```

## 第 5 步：创建 Consumer 和 Credential

本步骤创建一个独立的 Consumer 测试路由，不改变现有业务 API 的 Oathkeeper 认证：

```text
Consumer：alice
Credential：key-auth / alice-api-key
Route 4：/consumer-demo/* → UI Upstream 2
```

APISIX 中的关系是：Consumer 表示调用方身份，Credential 保存该调用方使用的认证
凭证，Route 上的 `key-auth` Plugin 负责读取请求中的 `apikey` Header，并根据 Key
找到对应 Consumer。没有 Key 或 Key 不存在时，请求在 APISIX 层直接返回 `401`。

验证：

```shell
# 不带 Key：401
curl -i http://192.168.2.41:8080/consumer-demo/

# Alice 的 Key：200，并返回 UI 页面
curl -i -H 'apikey: alice-api-key' \
  http://192.168.2.41:8080/consumer-demo/

# unknown Key：401
curl -i -H 'apikey: unknown-key' \
  http://192.168.2.41:8080/consumer-demo/
```

这里的 API Key 只用于演示 APISIX Consumer 认证，不代表 Kratos 用户身份，也不参与
Keto 权限判断。生产环境应使用 Secret 管理 Credential，避免把示例 Key 写入仓库。

## 第 6 步：验证 xhs_service 的业务权限结果

业务 API 继续使用既有链路：

```text
浏览器携带 Kratos Session Cookie
  → APISIX forward-auth
  → Oathkeeper cookie_session + id_token
  → APISIX 转发 Authorization: Bearer <Internal JWT>
  → xhs_service JWT 中间件
  → xhs_service 调用 Keto 判断组织权限
```

APISIX 只负责路由、外部认证和 JWT Header 转发，不参与组织权限判断。当前组织 G
的结果为：

| 请求 | Alice | Bob |
| --- | --- | --- |
| `GET /v1/xhs/organizations/G/crawl/contents` | `200` | `200` |
| `POST /v1/xhs/organizations/G/crawl/tasks` | `200` | `200` |
| `PUT /v1/xhs/organizations/G/crawl/keywords` | `200` | `403` |

因此 Bob 的 `403` 是 `xhs_service` 根据 Keto 返回的业务授权结果，不是 APISIX
因为路由、Consumer 或外部认证失败而产生的错误。
