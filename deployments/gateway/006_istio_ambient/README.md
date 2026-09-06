# gateway/006：Istio Ambient 基础配置

本目录是 Istio Ambient 实验的基础服务配置，来源于 `gateway/004_envoy_gateway` 的当前服务架构。
本次只保留运行实验所需的基础服务，不包含其他 Gateway 实验的入口控制器和入口资源；Istio 的安装、
网格标签、Waypoint、入口路由、AuthorizationPolicy 和流量治理会在后续步骤单独加入。

## 当前基础服务

| 配置 | 作用 |
| --- | --- |
| `postgres/cluster.yaml` | 在 `ddd-learn` 中创建 CNPG PostgreSQL，提供 `ory` 和 `keto` 数据库 |
| `values/keto.yaml` | 部署 Keto 及其 namespace 模型，使用同一 PostgreSQL |
| `values/kratos.yaml` | 部署 Kratos Public/Admin/Courier，使用同一 PostgreSQL |
| `values/oathkeeper.yaml` | 部署 Oathkeeper API 和内部 JWT/JWKS 配置；关闭 Proxy |
| `values/xhs.yaml` | 部署 `xhs_service`，保留内部 JWT 和 Keto 客户端配置 |
| `keto/` | 提供组织与角色权限模型 |
| `seed/ory-seed.yaml` | 初始化 Alice、Bob 及组织 G 的教学数据 |
| `base/ui.yaml` | 部署静态 UI |
| `base/mailpit.yaml` | 部署开发环境邮件服务 |

## Manifest 目录

按资源用途组织当前实验的声明文件，Helm values、Keto 模型、PostgreSQL 和初始化脚本继续
使用各自的目录：

| 子目录 | 内容 |
| --- | --- |
| `base/` | Ambient namespace 标签、测试 workload、UI 和 Mailpit |
| `ingress/` | Istio Gateway、HTTPRoute 和 xhs Waypoint |
| `security/` | Istio MeshConfig extension provider 与 AuthorizationPolicy |
| `traffic/` | xhs v1/v2 后端、DestinationRule 和 VirtualService |
| `seed/` | Alice、Bob、组织和关系的初始化 Job |
| `keto/` | Keto namespace/model |
| `postgres/` | CNPG PostgreSQL Cluster |
| `values/` | Keto、Kratos、Oathkeeper、xhs 的 Helm values |

## 与 004 的关系

这些文件沿用 004 的 namespace、Service 名称、镜像、端口和数据库连接，目的是让后续实验在同一
服务拓扑上学习东西向流量。它们不是对 004 README 的累积复制；本 README 只记录 006 当前提交的
基础内容。

本基础配置提交不包含 Istio 安装资源，也没有给 namespace 加 `istio.io/dataplane-mode=ambient` 标签。
Istio 安装命令在实验步骤中单独执行；在加入 namespace 之前，不会改变现有工作负载的流量路径。

## 将 namespace 加入 Ambient

```shell
kubectl apply -f deployments/gateway/006_istio_ambient/base/ambient-namespace.yaml
```

`ambient-namespace.yaml` 使用 namespace 级标签启用 Ambient。标签会影响 `ddd-learn` 中所有工作负载，
不只是 xhs_service；包括 PostgreSQL、Keto、Kratos、Oathkeeper、UI 和 Mailpit。

如果 namespace 中已经存在 Pod，需要让这些 Pod 重新创建，使它们经过 Istio CNI 完成网络重定向。
本实验使用以下精确资源列表：

```shell
kubectl rollout restart -n ddd-learn \
  deployment/ambient-frontend \
  deployment/ambient-other \
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

## 验证 workload mTLS

Ambient 的 mTLS 由节点上的 ztunnel 负责，业务容器不需要挂载证书，也不需要把请求改成
`https://`。请求过程是：

```text
frontend 容器 --HTTP--> frontend 所在节点的 ztunnel
                           --HBONE/mTLS--> xhs-service 所在节点的 ztunnel
                                             --HTTP--> xhs_service 容器
```

从测试 Pod 发起实际请求：

```shell
kubectl exec -n ddd-learn \
  deploy/ambient-frontend -- \
  wget -qO- -T 5 http://xhs-service/health
```

预期业务响应仍然是：

```json
{"status":"ok"}
```

查看 ztunnel 为 workload 持有的证书摘要：

```shell
istioctl ztunnel-config certificates -i istio-system
```

重点检查 `frontend` 和 `xhs-service` 对应的身份：

| 字段 | 预期 | 含义 |
| --- | --- | --- |
| `identity` | `spiffe://cluster.local/ns/ddd-learn/sa/frontend`、`.../xhs-service` | 证书绑定的 workload 身份 |
| `Leaf` | `Available` | 当前 workload 的短期叶子证书已经可用 |
| `Root` | `Available` | ztunnel 已取得验证对端证书的根证书 |
| 有效期 | 约 24 小时 | Istio 会自动轮换 workload 证书 |

再查看实际连接：

```shell
istioctl ztunnel-config connections \
  -i istio-system \
  --workload-namespace ddd-learn
```

服务间连接的 `PROTOCOL` 应显示为 `HBONE`。HBONE 是 Ambient 使用的隧道协议；其连接由双方
ztunnel 使用 Istio workload 证书完成身份认证和加密。业务服务看到的仍然是普通 HTTP，因此
本步骤没有修改 `xhs_service` 或 frontend 的代码和 TLS 配置。

本步骤验证的是“身份凭证存在并用于 Ambient 隧道”。它还没有限制谁可以访问谁；访问控制将在
下一步通过 `AuthorizationPolicy` 验证。

## 使用 AuthorizationPolicy 限制 xhs_service

本步骤使用 `AuthorizationPolicy` 只允许 `frontend` 访问 xhs workload：

```shell
kubectl apply -f deployments/gateway/006_istio_ambient/security/authorization-policy-xhs.yaml
kubectl get authorizationpolicy -n ddd-learn xhs-allow-frontend -o yaml
```

策略的 `selector` 匹配 xhs Helm release 实际生成的 Pod 标签：

```yaml
app.kubernetes.io/name: xhs
app.kubernetes.io/instance: xhs
```

`rules.from.source.principals` 允许的来源是：

```text
cluster.local/ns/ddd-learn/sa/frontend
```

当一个 workload 被至少一个 `AuthorizationPolicy` 选中后，没有匹配 allow 规则的请求默认拒绝。
因此 `other` 会被拒绝，而 `frontend` 可以访问。策略状态应包含：

```text
type: ZtunnelAccepted
status: "True"
reason: Accepted
```

分别从两个测试调用方验证：

```shell
kubectl exec -n ddd-learn deploy/ambient-frontend -- \
  wget -qO- -T 5 http://xhs-service/health

kubectl exec -n ddd-learn deploy/ambient-other -- \
  wget -qO- -T 5 http://xhs-service/health
```

预期结果：

| 调用方 | 结果 | 原因 |
| --- | --- | --- |
| `frontend` | 返回 `{"status":"ok"}` | mTLS 对端 principal 匹配 allow 规则 |
| `other` | 请求被拒绝，可能表现为连接重置或 HTTP 403 | principal 不在允许列表中 |

本策略只用于演示 Ambient 身份授权，因此当前 Oathkeeper 或其他未加入 allow 列表的调用方访问
xhs 也会被拒绝。生产策略需要把实际合法调用方逐一加入规则，或者为实验流量
设计独立的测试 workload。

## 通过 manifest 部署 Waypoint

本步骤不使用 `istioctl waypoint apply`，而是把 Waypoint 的期望状态保存为
`xhs-waypoint.yaml`。该文件只声明 Gateway API 对象：

```shell
kubectl apply -f deployments/gateway/006_istio_ambient/ingress/xhs-waypoint.yaml
```

`GatewayClass/istio-waypoint` controller 观察到这个对象后，会自动生成以下派生资源：

| 资源 | 作用 | 是否手工维护 |
| --- | --- | --- |
| `Gateway/xhs-waypoint` | Waypoint 的声明式入口，使用 `HBONE:15008` listener | 是，保存在 Git |
| `Deployment/xhs-waypoint` | 运行 Waypoint Envoy | 否，由 controller 生成 |
| `Service/xhs-waypoint` | 为 Waypoint 提供集群地址和 HBONE 端口 | 否，由 controller 生成 |
| `ServiceAccount/xhs-waypoint` | Waypoint Envoy 的工作负载身份 | 否，由 controller 生成 |

Waypoint 只处理 Service 流量，因为 Gateway 使用了：

```yaml
metadata:
  labels:
    istio.io/waypoint-for: service
```

listener 中的 `allowedRoutes.namespaces.from: Same` 是 Gateway API 的 Route 绑定范围控制。
理解它需要区分“路由是否可以挂到 Gateway”和“请求是否有权限访问服务”两个过程。

```yaml
allowedRoutes:
  namespaces:
    from: Same
```

一个 Route 想使用 Gateway，通常需要在自己的 `parentRefs` 中引用 Gateway：

```yaml
spec:
  parentRefs:
    - name: xhs-waypoint
      # 如果 Route 与 Gateway 不在同一个 namespace，通常还需要显式填写 namespace，
      # 并且仍然要通过 Gateway listener 的 allowedRoutes 检查。
```

Gateway listener 按以下顺序判断：

```text
1. Route 声明 parentRefs，表示它想绑定哪个 Gateway
2. Gateway 找到对应 listener
3. listener 检查 Route 所在 namespace 是否被 allowedRoutes 接受
4. 只有检查通过，Route 才能挂载到该 listener 并参与路由配置
```

`from: Same` 的具体结果是：`Gateway/xhs-waypoint` 位于 `ddd-learn`，因此只接受
`ddd-learn` 中的 Route；其他 namespace 即使在 `parentRefs` 中写了 `xhs-waypoint`，也不能绑定。
如果需要跨 namespace，可以使用 `from: All`，或使用 `from: Selector` 配合 namespace selector，
但应同时考虑跨 namespace 的管理边界。

当前实验没有 HTTPRoute 直接绑定这个 Waypoint，xhs 是通过 Service 上的
`istio.io/use-waypoint: xhs-waypoint` 选择 Waypoint。`allowedRoutes` 只是限制谁可以把 Route
挂到 Gateway listener，不决定 frontend 或 other 是否可以访问 xhs；访问权限仍由
`AuthorizationPolicy` 判断。

不要给 `ddd-learn` namespace 添加 `istio.io/use-waypoint`，否则 namespace 内的所有服务都会绑定
该 Waypoint。本实验通过 xhs Helm values 给单个 Service 添加绑定：

```yaml
service:
  labels:
    istio.io/use-waypoint: xhs-waypoint
```

由于 Helm Chart 原先不支持 Service 自定义标签，本实验在
`deployments/gateway/helm/xhs/templates/service.yaml` 中增加了 `service.labels` 渲染逻辑。
使用 Helm 部署时执行：

```shell
helm upgrade --install xhs deployments/gateway/helm/xhs \
  --namespace ddd-learn \
  --values deployments/gateway/006_istio_ambient/values/xhs.yaml
```

绑定后的流量路径是：

```text
frontend 容器
  → frontend 节点 ztunnel
  → xhs-waypoint Envoy（L7）
  → xhs 节点 ztunnel
  → xhs_service 容器
```

因此 AuthorizationPolicy 使用 `targetRefs: Service/xhs-service`，由 xhs 对应的 Waypoint 执行；
它可以继续使用 frontend 的原始 principal。若仍使用只选择 xhs Pod 的 workload selector，策略会
在 ztunnel 层执行，不适合作为本步骤 Waypoint 的 Service 级 L7 策略。

检查 Waypoint 和绑定状态：

```shell
kubectl get gateway xhs-waypoint -n ddd-learn
kubectl get deployment,service,serviceaccount -n ddd-learn \
  -l gateway.networking.k8s.io/gateway-name=xhs-waypoint
kubectl get service xhs-service -n ddd-learn \
  -o jsonpath='{.metadata.labels.istio\\.io/use-waypoint}{"\\n"}'
kubectl get authorizationpolicy xhs-allow-frontend -n ddd-learn -o yaml
```

## 第 7.1 步：准备 v1/v2 后端

本步骤只准备两个版本的工作负载，不配置流量比例。现有 Helm release `xhs` 作为
`backend-v1`，通过 `values/xhs.yaml` 增加 `version: v1` Pod 标签；新增的
`backend-v2.yaml` 创建 `backend-v2` Deployment，并使用 `version: v2` 标签。

两个 Deployment 具有相同的 `app.kubernetes.io/name: xhs` 和
`app.kubernetes.io/instance: xhs` 标签，因此都会被 `xhs-service` 选择；它们还复用
`xhs-service` ServiceAccount，保持相同的 workload identity。`version` 标签只用于
下一步的 `DestinationRule` subset，不会改变当前服务的业务代码。

```bash
helm upgrade --install xhs deployments/gateway/helm/xhs \
  --namespace ddd-learn \
  --values deployments/gateway/006_istio_ambient/values/xhs.yaml
kubectl apply -f deployments/gateway/006_istio_ambient/traffic/backend-v2.yaml
kubectl -n ddd-learn rollout status deployment/xhs-service
kubectl -n ddd-learn rollout status deployment/backend-v2
kubectl -n ddd-learn get pods -l app.kubernetes.io/name=xhs \
  -L version -o wide
kubectl -n ddd-learn get endpointslice \
  -l kubernetes.io/service-name=xhs-service -o wide
```

预期可以看到一个 `version=v1` Pod 和一个 `version=v2` Pod，且两者都处于
`Ready` 状态，并同时出现在 `xhs-service` 的 EndpointSlice 中。此时还没有灰度规则，
Kubernetes Service 仍按默认方式选择后端；90/10 分流将在下一小步骤由 Istio 配置。

## 第 7.2 步：使用 Istio 配置 90/10 灰度

本步骤使用两个 Istio 原生流量治理对象：

- `DestinationRule/xhs-service-versions`：根据 Pod 的 `version` 标签定义 `v1`、`v2` subset；
- `VirtualService/xhs-service-canary`：匹配 `xhs-service.ddd-learn.svc.cluster.local`，将请求
  按 90%/10% 发往 v1/v2。

这两个对象作用于 mesh 内的 xhs Service 请求，不修改外部入口的 `HTTPRoute`，也不依赖 Envoy
Gateway。由于 xhs Service 已绑定 `xhs-waypoint`，VirtualService 的 HTTP 路由由该 Waypoint
执行，ztunnel 继续负责两端的 Ambient mTLS 和 L4 转发。

```shell
kubectl apply \
  -f deployments/gateway/006_istio_ambient/traffic/xhs-destination-rule.yaml \
  -f deployments/gateway/006_istio_ambient/traffic/xhs-virtual-service.yaml
kubectl -n ddd-learn get destinationrule xhs-service-versions -o yaml
kubectl -n ddd-learn get virtualservice xhs-service-canary -o yaml
```

从允许访问 xhs 的 frontend workload 连续请求：

```shell
kubectl -n ddd-learn exec deploy/ambient-frontend -- sh -c \
  'i=0; while [ "$i" -lt 100; do wget -qO- -T 2 http://xhs-service/health >/dev/null || exit 1; i=$((i+1)); done; echo requests=100'
kubectl -n ddd-learn exec deploy/xhs-waypoint -- \
  pilot-agent request GET stats | rg 'backend-v2|destination_version.1.16.0'
```

Waypoint 的统计会分别出现 v1 和 v2 的 `destination_workload`，小样本的实际比例会围绕
90/10 随机波动；验证重点是两个 subset 都收到流量，且 v2 的流量明显低于 v1。本步骤不包含
timeout、Retry 或故障注入，它们在 7.3 单独配置和验证。

预期 `Gateway` 为 `Accepted=True`、`Programmed=True`，AuthorizationPolicy 包含
`WaypointAccepted=True`。最终访问验证仍然使用两个不同的 ServiceAccount：

```shell
kubectl exec -n ddd-learn deploy/ambient-frontend -- \
  wget -qO- -T 5 http://xhs-service/health

kubectl exec -n ddd-learn deploy/ambient-other -- \
  wget -qO- -T 5 http://xhs-service/health
```

预期 frontend 返回 `{"status":"ok"}`，other 返回 `HTTP 403 Forbidden`。

## 8.2 使用 Istio Gateway API 恢复入口路由

本步骤使用 Istio 自己的 Gateway API Controller，不使用 Envoy Gateway。入口资源关系如下：

```text
GatewayClass/istio
    ↓ controllerName: istio.io/gateway-controller
Gateway/istio-ingress
    ↓ 自动生成
Deployment/istio-ingress-istio + Service/istio-ingress-istio
    ↑
Service/istio-ingress-nodeport:30425
    ↓
HTTPRoute/istio-*
```

`istio-ingress-gateway.yaml` 声明 `Gateway/istio-ingress` 和固定 NodePort Service。Istio
Controller 根据 Gateway 创建入口 Envoy Pod；项目自己的 `istio-ingress-nodeport` 只负责将
局域网的 `30425` 端口转发到该 Pod，不负责解析路由。

`istio-ingress-routes.yaml` 声明四个 `HTTPRoute`，它们的 `parentRefs` 都指向
`Gateway/istio-ingress`：

| HTTPRoute | 匹配路径 | 后端 | 说明 |
| --- | --- | --- | --- |
| `istio-ui-example` | `/` | `ui-example:80` | 页面和静态资源 |
| `istio-kratos-public` | `/kratos` | `kratos-public:80` | 转发前移除 `/kratos` |
| `istio-mailpit` | `/mailpit` | `mailpit:8025` | 保留 Mailpit base path |
| `istio-xhs-service` | `/v1/xhs` | `xhs-service:80` | 保留 xhs_service 自己的 API 前缀 |

部署和检查：

```shell
kubectl apply -f deployments/gateway/006_istio_ambient/ingress/istio-ingress-gateway.yaml
kubectl apply -f deployments/gateway/006_istio_ambient/ingress/istio-ingress-routes.yaml
kubectl -n ddd-learn get gateway istio-ingress -o wide
kubectl -n ddd-learn get httproute \
  istio-ui-example istio-kratos-public istio-mailpit istio-xhs-service
kubectl -n ddd-learn get deployment,service \
  -l gateway.networking.k8s.io/gateway-name=istio-ingress -o wide
```

当前页面入口：

```text
http://192.168.2.41:30425/
```

验证结果：页面返回 `200`，`/kratos/health/ready` 返回 `200`，未携带认证信息访问
`/v1/xhs/health` 返回 `401`。响应中的 `server: istio-envoy` 说明请求已经由 Istio Ingress
Gateway 处理；此时集群中不再需要 Envoy Gateway 的 `public-gateway`、`HTTPRoute`、
`EnvoyProxy` 或 `GatewayClass/envoy`。

## 8.3 使用 Istio CUSTOM 调用 Oathkeeper Decision API

本步骤解决浏览器无法查询 Organization ID 的问题。浏览器只有 Kratos Session Cookie，
而 `xhs_service` 只接受 Oathkeeper 签发的 Internal JWT；因此入口 Gateway 需要在请求进入
xhs 之前完成一次外部授权检查，并把授权响应中的 JWT 传给上游。

本实验使用 Oathkeeper 的 **Decision API**，不启用 Oathkeeper Proxy `4455`：

```text
浏览器
  │ Kratos Session Cookie
  ▼
Istio Ingress Gateway
  │ HTTP ext_authz subrequest: oathkeeper-api:4456/decisions/v1/xhs/...
  │
  ├── Oathkeeper cookie_session -> Kratos /sessions/whoami
  ├── Oathkeeper allow authorizer
  └── Oathkeeper id_token mutator -> Authorization: Bearer <Internal JWT>
  │
  └── 允许时把 Authorization Header 传给 xhs_service
      └── xhs_service/serverhertz/jwt 使用 JWKS 校验 JWT
```

这样 Oathkeeper 不会代理每一个业务请求，也不会成为新的业务转发层；它只处理 Gateway
发起的鉴权子请求。普通业务流量仍由 Istio Gateway API 和 Ambient Waypoint 转发。

### 声明式配置

`istio-mesh-config.yaml` 在 Istio `MeshConfig.extensionProviders` 注册名为 `oathkeeper`
的 HTTP 外部授权服务：

| 配置 | 作用 |
| --- | --- |
| `service` + `port: 4456` | 指向 Oathkeeper Decision API，而不是关闭的 Proxy 4455 |
| `pathPrefix: /decisions` | Istio 将原始请求信息转成 Oathkeeper 的 Decision API 请求 |
| `includeRequestHeadersInCheck` | 把 `cookie`、已有 `authorization` 等凭证传给 Oathkeeper |
| `headersToUpstreamOnAllow: authorization` | 鉴权成功后把 Oathkeeper 签发的 Internal JWT 传给 xhs_service |
| `headersToDownstreamOnDeny` | 把登录跳转、CSRF Cookie 和 WWW-Authenticate 等拒绝响应传回浏览器 |
| `failOpen: false` | Oathkeeper 不可用时拒绝请求，避免鉴权失效变成放行 |

`authorization-policy-ingress-oathkeeper.yaml` 使用 `action: CUSTOM`，只匹配
`/v1/xhs` 和 `/v1/xhs/*`，并通过 `targetRefs` 绑定 `Gateway/istio-ingress`：

```yaml
targetRefs:
  - group: gateway.networking.k8s.io
    kind: Gateway
    name: istio-ingress
action: CUSTOM
provider:
  name: oathkeeper
```

这里不能使用普通 workload selector。selector 会让策略被 Ambient ztunnel 识别，而 ztunnel
不支持 `CUSTOM`；绑定到 Gateway 后，策略由 Gateway 的 Envoy 执行 HTTP ext_authz。

入口 Gateway 成功完成鉴权后，访问 xhs Waypoint 还需要通过
`authorization-policy-xhs.yaml` 允许 `istio-ingress-istio` ServiceAccount。这个策略表达的
是“入口 Gateway 可以调用 xhs Service”，不等同于用户授权；用户身份授权仍由 Oathkeeper
和后续 xhs/Keto 业务逻辑负责。

### 部署和检查

```shell
kubectl apply -f deployments/gateway/006_istio_ambient/security/istio-mesh-config.yaml
kubectl apply -f deployments/gateway/006_istio_ambient/security/authorization-policy-ingress-oathkeeper.yaml
kubectl apply -f deployments/gateway/006_istio_ambient/security/authorization-policy-xhs.yaml

kubectl -n ddd-learn get authorizationpolicy \
  istio-ingress-oathkeeper xhs-allow-frontend -o yaml
kubectl -n istio-system get configmap istio \
  -o jsonpath='{.data.mesh}'
```

`istio-ingress-oathkeeper` 应显示 `Accepted=True`，并且状态消息为绑定到
`ddd-learn/istio-ingress`。没有 Session Cookie 的请求应被 Oathkeeper 拒绝：

```shell
curl -i http://192.168.2.41:30425/v1/xhs/me/organizations
```

预期返回 `401`。Oathkeeper 日志中应能看到 Gateway 发起的
`/decisions/v1/xhs/me/organizations` 请求；这可以证明 ext_authz 链路已经生效。
使用 Alice 登录后的浏览器 Cookie 访问同一个 URL 时，Oathkeeper 会验证 Kratos Session、
签发 Internal JWT，Gateway 再将 JWT 传给 xhs_service，最终返回 Alice 所属的组织列表。

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
