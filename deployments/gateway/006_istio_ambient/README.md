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

## 将 namespace 加入 Ambient

```shell
kubectl apply -f deployments/gateway/006_istio_ambient/ambient-namespace.yaml
```

`ambient-namespace.yaml` 使用 namespace 级标签启用 Ambient。标签会影响 `ddd-learn` 中所有工作负载，
不只是 xhs_service；包括 PostgreSQL、Keto、Kratos、Oathkeeper、UI、Mailpit 和 Envoy Gateway。

如果 namespace 中已经存在 Pod，需要让这些 Pod 重新创建，使它们经过 Istio CNI 完成网络重定向。
本实验使用以下精确资源列表：

```shell
kubectl rollout restart -n ddd-learn \
  deployment/ambient-frontend \
  deployment/ambient-other \
  deployment/envoy-ddd-learn-public-gateway-daacb6b6 \
  deployment/envoy-gateway \
  deployment/keto \
  deployment/kratos \
  deployment/mailpit \
  deployment/oathkeeper \
  deployment/ui-example \
  deployment/xhs-service \
  statefulset/kratos-courier

# PostgreSQL 由 CNPG Cluster 管理，不直接重启 StatefulSet；需要时只回收实例 Pod，
# CNPG 会使用原 PVC 自动重建它。
kubectl delete pod -n ddd-learn ddd-learn-postgres-1
```

如果只是在 namespace 标签存在之前部署工作负载，则不需要额外重启。删除 PostgreSQL Pod 不会删除
CNPG Cluster 或 PVC，但 CNPG 的实例终止宽限期可能较长，重建期间 Keto、Kratos 等数据库依赖服务
会暂时无法启动。

## Ambient 加入后的验证

检查 namespace 标签和测试调用方：

```shell
kubectl get namespace ddd-learn --show-labels
kubectl get pods -n ddd-learn -l app.kubernetes.io/part-of=istio-ambient-lab -o wide
```

从两个不同身份的测试调用方访问 xhs 健康接口：

```shell
frontend=$(kubectl get pod -n ddd-learn \
  -l app.kubernetes.io/name=ambient-frontend -o jsonpath='{.items[0].metadata.name}')
other=$(kubectl get pod -n ddd-learn \
  -l app.kubernetes.io/name=ambient-other -o jsonpath='{.items[0].metadata.name}')
kubectl exec -n ddd-learn "${frontend}" -- wget -qO- -T 5 http://xhs-service/health
kubectl exec -n ddd-learn "${other}" -- wget -qO- -T 5 http://xhs-service/health
```

两个请求都应返回：

```json
{"status":"ok"}
```

查看 ztunnel 是否识别工作负载，并确认协议为 HBONE：

```shell
istioctl ztunnel-config workloads \
  -i istio-system \
  --workload-namespace ddd-learn
```

输出中的 `frontend`、`other` 和 `xhs-service` 应显示 `PROTOCOL=HBONE`，`WAYPOINT=None`。
这一步只验证 Ambient 接管和基础连通性，还没有配置 AuthorizationPolicy 或 waypoint。

## 查看 workload identity

Ambient 不使用 Pod IP 作为授权身份。Pod IP 只用于当前节点上的流量转发；Istio 使用 Pod 的
Kubernetes ServiceAccount 生成工作负载身份。先查看测试 Pod 绑定的 ServiceAccount：

```shell
kubectl get pod -n ddd-learn \
  -l app.kubernetes.io/part-of=istio-ambient-lab \
  -o custom-columns='POD:.metadata.name,SERVICE_ACCOUNT:.spec.serviceAccountName,NAMESPACE:.metadata.namespace'
```

预期结果中，`ambient-frontend` 使用 `frontend`，`ambient-other` 使用 `other`。两个 Pod 虽然
ServiceAccount 不同，但都位于 `ddd-learn` namespace。

从 ztunnel 读取结构化 workload 配置：

```shell
istioctl ztunnel-config workloads \
  -i istio-system \
  --workload-namespace ddd-learn \
  -o json
```

重点查看每个 workload 的以下字段：

| 字段 | 当前示例 | 作用 |
| --- | --- | --- |
| `serviceAccount` | `frontend`、`other`、`xhs-service` | ztunnel 从 Kubernetes 工作负载得到的服务账号名称 |
| `namespace` | `ddd-learn` | 身份所属的 Kubernetes namespace |
| `trustDomain` | `cluster.local` | Istio 信任域，用于组成 SPIFFE 身份 |
| `protocol` | `HBONE` | 该 workload 已由 Ambient ztunnel 接管 |
| `workloadIps` | `10.42.0.x` | 数据面定位地址，不是授权主体 |

在当前信任域下，`frontend` 的身份可表示为：

```text
spiffe://cluster.local/ns/ddd-learn/sa/frontend
```

`other` 和 `xhs-service` 的身份分别将最后的 ServiceAccount 替换为 `other` 和 `xhs-service`。
后续 `AuthorizationPolicy` 使用的正是这类 workload identity，因此可以按调用方 ServiceAccount
授权，而不依赖会变化的 Pod IP。`automountServiceAccountToken: false` 只是不把 Kubernetes API
访问令牌挂载进业务容器，不会改变 ztunnel 根据 ServiceAccount 建立的 Istio 身份。

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
