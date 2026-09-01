# 003_keto：Keto 权限管理

本目录是 Keto 模块的独立部署目录。它从
`deployments/002_internal_jwt` 复制基础配置作为起点，之后独立维护 Kratos、
Traefik、PostgreSQL、Courier、Mailpit、Oathkeeper、JWKS 和 Keto。

003 使用一个 PostgreSQL 容器，但创建 `ory` 和 `keto` 两个逻辑数据库；两者
使用不同的数据库用户，不读取或修改 001、002 的运行数据。复制配置表示继承
上一步的部署基线，不表示两个 Compose 项目运行时共享容器或配置文件。

## 当前范围

Keto 服务、数据库、迁移和 Alice/Bob Relation Tuple 已完成。第 4 步把
`xhs_service` 切换到真实 Keto 校验：

```yaml
KETO_ENABLED: "true"
KETO_READ_URL: http://keto:4466
```

`xhs_service` 不再使用 MockPermissionChecker。Keto 不可用或返回异常时采用
fail-closed：业务接口返回 `503`，不会绕过权限检查。

## 服务拓扑

```text
Browser
  │ Kratos Session Cookie
  ▼
Traefik host :8080
  │ ForwardAuth / Decision API
  ▼
Oathkeeper host :4456 / container :4456
  │ Internal JWT
  ▼
xhs_service :8082
  │ 调用 Keto Read API 检查业务 Permission
  ▼
Keto host :4466 / container :4466
```

各组件职责如下：

| 组件 | 本模块职责 | 不负责的内容 |
| --- | --- | --- |
| Kratos | 验证用户 Session，提供 `identity.id` | 签发 Internal JWT、业务权限判断 |
| Oathkeeper | 认证外部凭证，签发 Internal JWT | 业务权限和业务数据 |
| Traefik | 接收请求并调用 Oathkeeper Decision API | 业务权限判断 |
| `xhs_service` | 验证 Internal JWT，调用 Keto 判断业务权限 | 真实抓取实现 |
| Keto | 根据 OPL 计算业务权限 | 用户登录和 JWT 签发 |
| PostgreSQL `ory` 数据库 | 保存 Kratos Identity 和 Session | Keto Relation Tuple |
| PostgreSQL `keto` 数据库 | 保存 Keto Relation Tuple 和内部数据 | Kratos Identity 和 Session |

## Keto API

| 地址 | 用途 | 调用方 |
| --- | --- | --- |
| 容器内 `http://keto:4466` | Read API，检查权限和读取关系 | `xhs_service`、管理服务 |
| 容器内 `http://keto:4467` | Write API，写入或删除 Relation Tuple | 初始化脚本、可信管理服务 |

Keto 不经过 Traefik，也不供浏览器直接访问。宿主机映射端口为 `4466` 和 `4467`，
仅用于本地教学调试；
生产环境应限制 4467，只允许可信管理平面访问。

## OPL 配置

`keto/keto.yml` 通过以下配置加载权限模型：

```yaml
namespaces:
  location: file:///etc/config/keto/namespaces.ts
```

`namespaces.ts` 定义 `User`、`Organization`、`members`、`admins` 以及三个
Permission。OPL 是静态模型，Relation Tuple 是运行时数据；两者分别由配置文件
和数据库管理。

## 启动和迁移

在仓库根目录执行：

```shell
docker compose -f deployments/003_keto/docker-compose.yml up -d
```

Compose 的启动顺序是：

```text
postgres 健康
  -> keto-migrate 完成
  -> keto 启动
```

PostgreSQL 的初始化脚本只在 `auth-003-postgres` volume 第一次创建时执行，负责
创建 `keto` 数据库和 `keto` 用户。Kratos 仍按 002 的方式执行自己的 migration
和用户初始化，使用同一个 PostgreSQL 实例中的 `ory` 数据库。

查看 Keto 服务日志：

```shell
docker compose -f deployments/003_keto/docker-compose.yml logs -f keto
```

验证 Read API：

```shell
curl http://192.168.2.41:4466/health/ready
```

验证 OPL 已加载：

```shell
curl http://192.168.2.41:4466/namespaces
```

此时应该能够看到 `User` 和 `Organization`。随后执行 `keto-seed`，写入并验证
Alice 和 Bob 的业务关系。

## 关系初始化

第 3 步由 `keto-seed` 完成关系初始化。它不会读取或信任
`metadata_admin.role`，而是按邮箱从 Kratos Admin API 查询真实的 `identity.id`：

```http
GET http://kratos:4434/admin/identities?credentials_identifier=alice%40example.com
GET http://kratos:4434/admin/identities?credentials_identifier=bob%40example.com
```

首先写入用户角色：

```text
PUT http://keto:4467/admin/relation-tuples
Organization:G#admins@User:<alice-identity-id>

PUT http://keto:4467/admin/relation-tuples
Organization:G#members@User:<bob-identity-id>
```

然后分别写入组织权限和角色权限。组织 G 拥有三个操作，因此 `entitled_*` 会
覆盖 `members` 和 `admins` 两个角色集合：

```text
Organization:G#entitled_start_crawl@Organization:G#members
Organization:G#entitled_start_crawl@Organization:G#admins
Organization:G#entitled_view_content@Organization:G#members
Organization:G#entitled_view_content@Organization:G#admins
Organization:G#entitled_modify_keywords@Organization:G#members
Organization:G#entitled_modify_keywords@Organization:G#admins
```

角色层只把修改关键词授予管理员：

```text
Organization:G#granted_start_crawl@Organization:G#members
Organization:G#granted_start_crawl@Organization:G#admins
Organization:G#granted_view_content@Organization:G#members
Organization:G#granted_view_content@Organization:G#admins
Organization:G#granted_modify_keywords@Organization:G#admins
```

`keto/namespaces.ts` 使用 AND 计算最终权限：

```typescript
this.related.entitled_start_crawl.includes(ctx.subject) &&
this.related.granted_start_crawl.includes(ctx.subject)
```

因此，角色权限只能是组织权限的子集：

| 用户 | `start_crawl` | `view_content` | `modify_keywords` |
| --- | ---: | ---: | ---: |
| Alice (`admins`) | 允许 | 允许 | 允许 |
| Bob (`members`) | 允许 | 允许 | 拒绝 |

如果组织 W 只拥有查看权限，则只写 `entitled_view_content`。即使 W 的某个角色
存在 `granted_start_crawl`，因为缺少 `entitled_start_crawl`，最终仍然是
`false && true = false`。

手动执行关系初始化：

```shell
docker compose -f deployments/003_keto/docker-compose.yml up keto-seed
```

`keto-seed` 是一次性 Job，重复执行不会创建重复关系。容器内和宿主机调试端口
都是 `4467`。Write API 只应由初始化脚本或可信管理服务调用，浏览器不应
直接访问。

## 目录结构

```text
deployments/
├── 002_internal_jwt/             # 003 的复制来源
└── 003_keto/                     # 从 002 复制后独立演进
    ├── README.md
    ├── docker-compose.yml
    ├── jwks/
    ├── kratos/
    ├── oathkeeper/
    ├── traefik/
    └── keto/
        ├── keto.yml
        ├── namespaces.ts
        └── seed-relations.sh
```

`id_token.jwks.json` 是开发环境私钥，只挂载给 Oathkeeper。业务服务只通过
`/.well-known/jwks.json` 获取公钥。生产环境必须通过 Secret、Vault 或其他
密钥管理系统提供私钥，不能把私钥提交到代码仓库。
