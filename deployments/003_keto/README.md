# 003_keto：Keto 权限管理

本目录是 Keto 模块的独立部署目录。它从
`deployments/002_internal_jwt` 复制基础配置作为起点，之后独立维护 Kratos、
Traefik、PostgreSQL、Courier、Mailpit、Oathkeeper、JWKS 和 Keto。

003 使用一个 PostgreSQL 容器，但创建 `ory` 和 `keto` 两个逻辑数据库；两者
使用不同的数据库用户，不读取或修改 001、002 的运行数据。复制配置表示继承
上一步的部署基线，不表示两个 Compose 项目运行时共享容器或配置文件。

## 本步骤范围

本步骤只完成 Keto 服务、Keto 数据库和数据库迁移。Alice、Bob 的 Relation Tuple
在第 3 步写入，`xhs_service` 在第 4 步切换到真实 Keto 校验。

当前 xhs_service 仍保持：

```yaml
KETO_ENABLED: "false"
```

因此本步骤启动后，业务接口仍使用 MockPermissionChecker。

## 服务拓扑

```text
Browser
  │ Kratos Session Cookie
  ▼
Traefik :8080
  │ ForwardAuth / Decision API
  ▼
Oathkeeper :4456
  │ Internal JWT
  ▼
xhs_service :8082
  │ 第 4 步起调用 Keto Read API
  ▼
Keto :4466
```

各组件职责如下：

| 组件 | 本模块职责 | 不负责的内容 |
| --- | --- | --- |
| Kratos | 验证用户 Session，提供 `identity.id` | 签发 Internal JWT、业务权限判断 |
| Oathkeeper | 认证外部凭证，签发 Internal JWT | 业务权限和业务数据 |
| Traefik | 接收请求并调用 Oathkeeper Decision API | 业务权限判断 |
| `xhs_service` | 验证 Internal JWT，后续调用 Keto 判断业务权限 | 真实抓取实现 |
| Keto | 根据 OPL 计算业务权限 | 用户登录和 JWT 签发 |
| PostgreSQL `ory` 数据库 | 保存 Kratos Identity 和 Session | Keto Relation Tuple |
| PostgreSQL `keto` 数据库 | 保存 Keto Relation Tuple 和内部数据 | Kratos Identity 和 Session |

## Keto API

| 地址 | 用途 | 调用方 |
| --- | --- | --- |
| `http://keto:4466` | Read API，检查权限和读取关系 | `xhs_service`、管理服务 |
| `http://keto:4467` | Write API，写入或删除 Relation Tuple | 初始化脚本、可信管理服务 |

Keto 不经过 Traefik，也不供浏览器直接访问。宿主机映射端口仅用于本地教学调试；
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

此时应该能够看到 `User` 和 `Organization`。关系数据尚未写入，因此还不能验证
Alice 和 Bob 的业务权限。

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
        └── namespaces.ts
```

`id_token.jwks.json` 是开发环境私钥，只挂载给 Oathkeeper。业务服务只通过
`/.well-known/jwks.json` 获取公钥。生产环境必须通过 Secret、Vault 或其他
密钥管理系统提供私钥，不能把私钥提交到代码仓库。
