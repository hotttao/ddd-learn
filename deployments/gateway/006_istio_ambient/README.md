# gateway/006：Istio Ambient 基础配置

本目录是 Istio Ambient 实验的基础服务配置，来源于 `gateway/004_envoy_gateway` 的当前服务架构。
本次只保留运行实验所需的基础服务，不复制 Envoy Gateway 的 Gateway、HTTPRoute、ExtAuth、限流和
mTLS 配置；Istio 的安装、网格标签、Waypoint、AuthorizationPolicy 和流量治理会在后续步骤单独加入。

## 当前基础服务

| 配置 | 作用 |
| --- | --- |
| `postgres/cluster.yaml` | 在 `ddd-learn` 中创建 CNPG PostgreSQL，提供 `ory` 和 `keto` 数据库 |
| `values/keto.yaml` | 部署 Keto 及其 namespace 模型，使用同一 PostgreSQL |
| `values/kratos.yaml` | 部署 Kratos Public/Admin/Courier，使用同一 PostgreSQL |
| `values/oathkeeper.yaml` | 部署 Oathkeeper API 和内部 JWT/JWKS 配置；关闭 Proxy |
| `values/xhs.yaml` | 部署 `xhs_service`，保留内部 JWT 和 Keto 客户端配置 |
| `keto/` | 提供组织与角色权限模型 |
| `ory-seed.yaml` | 初始化 Alice、Bob 及组织 G 的教学数据 |
| `ui.yaml` | 部署静态 UI |
| `mailpit.yaml` | 部署开发环境邮件服务 |

## 与 004 的关系

这些文件沿用 004 的 namespace、Service 名称、镜像、端口和数据库连接，目的是让后续实验在同一
服务拓扑上学习东西向流量。它们不是对 004 README 的累积复制；本 README 只记录 006 当前提交的
基础内容。

本提交尚未安装 Istio，也没有给 namespace 加 `istio.io/dataplane-mode=ambient` 标签，因此不会改变
现有工作负载的流量路径。
