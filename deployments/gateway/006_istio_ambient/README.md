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

本基础配置提交不包含 Istio 安装资源，也没有给 namespace 加 `istio.io/dataplane-mode=ambient` 标签。
Istio 安装命令在实验步骤中单独执行；在加入 namespace 之前，不会改变现有工作负载的流量路径。

## Istio 部署完成检查

### 0. 本次安装命令

```shell
istioctl install \
  --set profile=ambient \
  --set values.global.platform=k3s \
  --skip-confirmation
```

该命令安装 Istio Ambient profile，包括 Istio CRD、`istiod`、`istio-cni-node` 和 `ztunnel`。
它不会自动给业务 namespace 加 Ambient 标签。

### 1. 查看控制面版本

```shell
istioctl version
```

正常情况下应同时看到 Client version 和 Control plane version。当前实验使用 Istio `1.31.0`。

### 2. 检查核心 Pod 是否 Ready

```shell
kubectl get pods -n istio-system -o wide
```

Ambient profile 的核心 Pod 预期如下：

| Pod | 类型 | 作用 | 完成条件 |
| --- | --- | --- | --- |
| `istiod-*` | Deployment | 控制面，向数据面下发配置并管理身份与证书 | `1/1 Running` |
| `istio-cni-node-*` | DaemonSet | 在每个节点配置 Ambient 流量重定向 | 每个节点一个 Pod，全部 `1/1 Running` |
| `ztunnel-*` | DaemonSet | 在每个节点承载 Ambient L4 流量、mTLS 和服务身份 | 每个节点一个 Pod，全部 `1/1 Running` |

也可以使用 rollout 检查工作负载：

```shell
kubectl -n istio-system rollout status deployment/istiod --timeout=180s
kubectl -n istio-system rollout status daemonset/istio-cni-node --timeout=180s
kubectl -n istio-system rollout status daemonset/ztunnel --timeout=180s
```

### 3. 查看部署了哪些组件

```shell
kubectl get deployment,daemonset,service -n istio-system
kubectl get serviceaccount,configmap,secret -n istio-system
```

这些命令可以看到：

| 组件 | 当前资源 | 作用 |
| --- | --- | --- |
| 控制面 | `Deployment/istiod`、`Service/istiod` | 管理配置、服务发现、工作负载身份和证书签发 |
| CNI | `DaemonSet/istio-cni-node` | 接入节点 CNI，在 Pod 创建时设置流量重定向 |
| Ambient 数据面 | `DaemonSet/ztunnel` | 处理节点上的服务间 L4 流量和 mTLS |
| 控制面访问 | `Service/istiod`、`Service/istiod-revision-tag-default` | 为数据面提供 xDS、CA 和控制面服务发现 |
| 服务账号 | `ServiceAccount/istiod`、`ServiceAccount/istio-cni` 等 | 为控制面和节点组件提供 Kubernetes 身份 |
| 配置与证书 | `ConfigMap`、`Secret` | 保存 Istio 配置、根证书和引导信息 |

`kubectl get all` 只能查看部分命名空间级资源，不能显示 CRD、ClusterRole、Webhook 等集群级资源。

### 4. 查看 Istio CRD 和 Webhook

```shell
kubectl get crd | rg 'istio.io|extensions.istio.io'
kubectl get validatingwebhookconfiguration,mutatingwebhookconfiguration | rg istio
```

CRD 用于让 Kubernetes API Server 识别 `AuthorizationPolicy`、`PeerAuthentication`、
`RequestAuthentication`、`VirtualService` 等 Istio 对象。Webhook 用于在相关资源变更时参与 Kubernetes
的校验或变更流程。

### 5. 检查 Istio 的 RBAC 资源

```shell
kubectl get role,rolebinding -n istio-system
kubectl get clusterrole,clusterrolebinding | rg istio
```

这些资源控制 `istiod`、`istio-cni` 和其他 Istio 组件能够读取或修改哪些 Kubernetes 对象。

本步骤只验证 Istio 控制面已经部署完成，不代表业务已经加入 Ambient。只有给目标 namespace 添加：

```shell
kubectl label namespace <namespace> istio.io/dataplane-mode=ambient
```

并重新创建或更新工作负载后，目标 Pod 的流量才会进入 Ambient 数据面。
