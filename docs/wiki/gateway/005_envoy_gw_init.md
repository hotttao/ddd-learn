# Envoy Gateway 初始化资源与启动流程

本文记录当前实验使用本地 Helm Chart 初始化 Envoy Gateway 时创建的资源，以及这些资源之间的关系。
当前安装目标是 `ddd-learn` namespace。Gateway API 的 CRD 属于集群级资源，Envoy Gateway
扩展 CRD 也属于集群级资源。

## 一、初始化资源总览

```text
Helm Release: eg
        │
        ├── ServiceAccount/envoy-gateway
        ├── ConfigMap/envoy-gateway-config
        ├── Deployment/envoy-gateway
        ├── Service/envoy-gateway
        ├── Role / RoleBinding
        ├── ClusterRole / ClusterRoleBinding
        ├── ServiceAccount/eg-gateway-helm-certgen
        ├── Job/eg-gateway-helm-certgen
        ├── Secret/envoy-gateway
        └── MutatingWebhookConfiguration
```

这些资源可以分成四组：

| 资源组 | 主要资源 | 作用 |
| --- | --- | --- |
| 控制器运行资源 | Deployment、Pod、Service | 运行 Envoy Gateway Controller |
| 控制器配置 | ConfigMap | 提供 `EnvoyGateway` 启动配置 |
| 控制器权限 | ServiceAccount、Role、ClusterRole 及 Binding | 授权 Controller 访问 Kubernetes API |
| 安装安全资源 | certgen Job、Secret、WebhookConfiguration | 生成证书并启用 Admission Webhook |

本次使用的 Helm 命令是：

```shell
helm upgrade --install eg deployments/gateway/helm/envoy_gateway/gateway-helm \
  --namespace ddd-learn \
  --values deployments/gateway/helm/envoy_gateway/gateway-helm/values.tmpl.yaml \
  --values deployments/gateway/004_envoy_gateway/values/envoy-gateway.yaml \
  --wait --timeout 600s
```

## 二、ServiceAccount：Pod 的身份

Controller Deployment 使用：

```yaml
spec:
  template:
    spec:
      serviceAccountName: envoy-gateway
```

`ServiceAccount/envoy-gateway` 不是权限列表，而是 Controller Pod 调用 Kubernetes API 时使用的身份。
Pod 启动后，Kubernetes 会向 Pod 提供该身份对应的 Token，Envoy Gateway 使用 Token 请求 API Server。

certgen Job 使用另一个身份：

```yaml
serviceAccountName: eg-gateway-helm-certgen
```

这样可以将“持续运行的 Controller 权限”和“一次性证书生成任务权限”分开。

## 三、RBAC：Kubernetes 如何控制 Controller 权限

权限关系如下：

```text
Deployment/envoy-gateway
        ↓ serviceAccountName
ServiceAccount/envoy-gateway
        ↓ 被 Binding 引用
RoleBinding / ClusterRoleBinding
        ↓ 授权
Role / ClusterRole
        ↓
Kubernetes API Server RBAC Authorizer
```

Controller 访问 API Server 时，RBAC Authorizer 会根据以下信息判断是否允许：

```text
请求身份 + API Group + Resource + Namespace + Verb
```

例如一条规则：

```yaml
- apiGroups: ["gateway.networking.k8s.io"]
  resources: ["gateways", "httproutes"]
  verbs: ["get", "list", "watch"]
```

表示 `envoy-gateway` 身份可以读取和监听 Gateway、HTTPRoute，但不代表它可以执行规则中没有声明的
删除或更新操作。

当前 Chart 同时创建两种 Binding：

```text
ClusterRoleBinding
└── ClusterRole/eg-gateway-helm-envoy-gateway-role
    └── 集群级资源和跨 namespace 资源权限

RoleBinding/ddd-learn
└── Role/eg-gateway-helm-infra-manager
    └── ddd-learn namespace 内数据面资源权限
```

`ClusterRoleBinding` 的存在不代表 Controller 可以无限制操作所有资源，具体允许什么仍由
`ClusterRole.rules` 决定。没有匹配规则时，API Server 返回 `403 Forbidden`。

## 四、envoy-gateway-config：Controller 的启动配置

Helm 将 values 中的 `config.envoyGateway` 渲染为：

```text
ConfigMap/envoy-gateway-config
└── envoy-gateway.yaml
```

ConfigMap 文件的内容是一个 `gateway.envoyproxy.io/v1alpha1`、Kind 为 `EnvoyGateway` 的配置对象。
它用于描述 Controller 如何工作，例如：

```yaml
apiVersion: gateway.envoyproxy.io/v1alpha1
kind: EnvoyGateway
spec:
  gateway:
    controllerName: gateway.envoyproxy.io/gatewayclass-controller
  provider:
    type: Kubernetes
```

Controller Deployment 通过启动参数读取它：

```yaml
args:
  - server
  - --config-path=/config/envoy-gateway.yaml
```

并将 ConfigMap 挂载到容器：

```yaml
volumeMounts:
  - name: envoy-gateway-config
    mountPath: /config
    readOnly: true
```

因此它的流程是：

```text
Helm values
    ↓
ConfigMap/envoy-gateway-config
    ↓ 挂载
Controller:/config/envoy-gateway.yaml
    ↓ --config-path
Envoy Gateway Controller 启动
```

## 五、certgen Job：生成 TLS 证书

`eg-gateway-helm-certgen` 是 Helm 的 `pre-install`、`pre-upgrade` Hook：

```yaml
annotations:
  helm.sh/hook: pre-install, pre-upgrade
```

它使用 Envoy Gateway 镜像执行：

```text
envoy-gateway certgen
```

创建流程如下：

```text
Helm install / upgrade
        ↓
创建 ServiceAccount/eg-gateway-helm-certgen
        ↓
创建 certgen Role、ClusterRole 和 Binding
        ↓
创建 Job/eg-gateway-helm-certgen
        ↓
Job Pod 使用 certgen ServiceAccount 调用 Kubernetes API
        ↓
生成或更新 TLS Secret
        ↓
Job 完成
        ↓
Helm 继续创建或更新 Controller Deployment
```

这个 Job 不是长期运行的服务。它只负责安装或升级时的证书准备，完成后 Pod 退出。

## 六、Secret 如何挂载到 Controller

Controller Deployment 将 `Secret/envoy-gateway` 挂载为名为 `certs` 的 Volume：

```yaml
volumes:
  - name: certs
    secret:
      secretName: envoy-gateway

volumeMounts:
  - name: certs
    mountPath: /certs
    readOnly: true
```

关系如下：

```text
Secret/envoy-gateway
        ↓ Secret Volume
Controller Pod:/certs
        ↓
Webhook TLS 或 Controller 内部安全通信
```

Secret 中保存证书和私钥，Kubernetes 以文件形式挂载到容器。应用读取 `/certs` 下的证书文件，
而不是直接读取 Kubernetes API。

当前集群中还可以看到 `envoy`、`envoy-rate-limit`、`envoy-oidc-hmac` 等 Secret。它们分别服务于
Envoy 数据面或特定扩展能力，不能简单理解为 Controller 的全部证书；当前 Controller Deployment
明确挂载的是 `envoy-gateway`。

## 七、MutatingWebhookConfiguration：Kubernetes 如何调用 Webhook

对应的 Helm 模板是：

```text
deployments/gateway/helm/envoy_gateway/gateway-helm/templates/envoy-proxy-topology-injector-webhook.yaml
```

相关模板还有：

```text
templates/certgen.yaml
templates/certgen-rbac.yaml
templates/envoy-gateway-deployment.yaml
templates/envoy-gateway-service.yaml
```

功能开关位于：

```yaml
topologyInjector:
  enabled: true
```

当前 Chart 创建：

```text
MutatingWebhookConfiguration/
  envoy-gateway-topology-injector.ddd-learn
```

它告诉 Kubernetes Admission 机制：当符合规则的 Pod Binding 创建请求出现时，将请求发送到：

```text
Service/envoy-gateway:9443
        ↓
Pod/envoy-gateway:9443
        ↓
/inject-pod-topology
```

Webhook 配置中的关键关系：

```yaml
clientConfig:
  service:
    name: envoy-gateway
    namespace: ddd-learn
    port: 9443
    path: /inject-pod-topology
```

TLS 证书由 certgen Job 准备，Controller Pod 从 `/certs` 读取。Kubernetes API Server 在调用 Webhook
时会使用该配置中的 CA 信息验证 Webhook 服务身份。

当前 Webhook 的规则只匹配：

```text
operations: CREATE
resources: pods/binding
```

它不是业务请求代理，也不会处理浏览器访问的 HTTP 流量；它属于 Kubernetes API Admission 阶段。

### Admission 调用链

Kubernetes API Server 收到资源创建或修改请求后，会在对象写入 etcd 之前经过 Admission 阶段：

```text
客户端 / Controller / Scheduler
        ↓
Kubernetes API Server
        ↓
认证 Authentication
        ↓
权限检查 RBAC Authorization
        ↓
Mutating Admission Webhook
        ↓
Validating Admission
        ↓
写入 etcd
```

Mutating Webhook 可以在对象持久化之前返回 JSON Patch，API Server 应用 Patch 后再保存最终对象。

```text
提交原始对象
    ↓
API Server 发送 AdmissionReview
    ↓
Envoy Gateway Webhook 返回 AdmissionResponse
    ↓
API Server 应用 JSONPatch
    ↓
保存修改后的对象
```

### Envoy Gateway 为什么定义这个 Admission Webhook

当前 Webhook 的名称是：

```text
envoy-gateway-topology-injector
```

它的目的不是认证业务用户，也不是拦截浏览器请求，而是为 Envoy Gateway 管理的数据面 Pod
注入拓扑相关信息。拓扑信息可以帮助数据面了解节点、区域等部署位置，在启用拓扑感知能力时
支持更合理的流量就近和负载选择。

可以将它理解为一个 Kubernetes Pod 创建过程中的自动修改器：

```text
Envoy Gateway 创建数据面 Pod
        ↓
Kubernetes Admission Webhook
        ↓
注入拓扑相关配置
        ↓
Pod 按修改后的对象继续创建和调度
```

对于当前实验的基础 HTTP 路由来说，这个 Webhook 不是必须的。它服务于 Envoy Gateway 的拓扑
注入扩展，和 `Gateway`、`HTTPRoute` 的基本路由匹配不是一回事。

### 为什么不由 Controller 直接注入

Envoy Gateway Controller 创建的是 Envoy 数据面的 Deployment，并不是直接创建最终运行的 Pod：

```text
Envoy Gateway Controller
        ↓
Deployment
        ↓
ReplicaSet
        ↓
Pod
        ↓
Scheduler 选择节点
        ↓
Pod Binding
```

Controller 创建 Deployment 时，Pod 的最终节点通常还没有确定：

```text
创建 Deployment 时：
Node   = 未确定
Zone   = 未确定
Region = 未确定
```

而节点拓扑是在 Scheduler 选择节点并创建 `pods/binding` 后才确定的：

```text
Pod
    ↓
Scheduler 选择 Node
    ↓
确定 Node、Zone、Region
    ↓
创建 pods/binding
```

因此，Controller 可以在 Deployment 模板中注入静态配置，但无法在这个阶段可靠地注入依赖最终
节点的拓扑信息。Admission Webhook 介入 `pods/binding`，就是为了在调度结果产生后处理拓扑相关
信息。

两种方式的区别是：

```text
直接注入：
Controller → 创建带拓扑配置的 Deployment → Pod
                              ↑
                    创建时节点可能未知
```

```text
Admission 注入：
Controller → Deployment → Pod → Scheduler → Binding
                                      ↓
                              Topology Webhook
```

使用独立 Webhook 还可以将拓扑逻辑从 Controller 的 Deployment 生成逻辑中分离出来，并统一处理
滚动升级、Pod 故障重建、扩缩容和节点迁移后重新创建的数据面 Pod。它不要求这些 Pod 必须由某一
段业务代码直接创建，只要请求符合 Webhook 的匹配规则即可被处理。

这个 Webhook 是可选扩展，不是基础 HTTPRoute 转发的必要条件。如果不需要拓扑感知能力，可以
关闭：

```yaml
topologyInjector:
  enabled: false
```

## Admission 发生的时机与请求发起者

Admission 发生在 Kubernetes API Server 处理 API 请求的过程中，具体是在认证和 RBAC 权限检查
之后、对象写入 etcd 之前：

```text
客户端或控制器提交请求
        ↓
API Server 接收请求
        ↓
Authentication：确认请求者身份
        ↓
Authorization：检查 RBAC 权限
        ↓
Mutating Admission：调用 MutatingWebhook
        ↓
Validating Admission：执行校验
        ↓
写入 etcd
```

这里有两个不同的“请求”：

1. 原始 Kubernetes API 请求：可能由用户、Scheduler、Controller 或其他 Kubernetes 组件发起；
2. Webhook AdmissionReview 请求：由 Kubernetes API Server 根据 `MutatingWebhookConfiguration`
   自动发起。

因此不是 Scheduler 或普通客户端直接调用 `envoy-gateway:9443`，而是：

```text
Scheduler / Controller / kubectl
        ↓ 原始 API 请求
Kubernetes API Server
        ↓ 根据 Webhook rules 判断
envoy-gateway:9443
        ↓ AdmissionResponse
Kubernetes API Server
        ↓
保存最终对象
```

## 调用 Webhook 时 Pod 是否已经 Ready

调用 Admission Webhook 时，目标业务 Pod 通常还没有 Ready，甚至还没有创建完成。

Admission 处理的是“即将写入 Kubernetes 的对象”，不是一个已经运行的 Pod：

```text
提交 Pod 对象
        ↓
Admission Webhook 修改或检查对象
        ↓
API Server 保存 Pod 对象
        ↓
Scheduler 绑定节点
        ↓
kubelet 创建容器
        ↓
启动探针 / 就绪探针
        ↓
Pod Ready
```

所以 Webhook 不会通过等待目标 Pod Ready 来决定是否调用。它只需要修改 AdmissionReview 中
携带的对象或返回允许/拒绝结果。

需要 Ready 的是 Webhook 自己：

```text
API Server
    ↓ 调用
Service/envoy-gateway:9443
    ↓ EndpointSlice 只包含 Ready 的 Controller Pod
Pod/envoy-gateway
```

如果 `envoy-gateway` Controller Pod 尚未 Ready，它通常不会出现在 Service 的可用 Endpoint 中，
API Server 可能无法调用 Webhook。当前配置使用：

```yaml
failurePolicy: Ignore
```

因此 Webhook 自身暂时不可用时，API Server 会忽略该 Webhook 错误，继续处理原始请求。若使用
`failurePolicy: Fail`，Webhook 不可用会导致匹配的 API 请求失败。

这两个 Ready 要区分：

| 对象 | Ready 的意义 |
| --- | --- |
| `envoy-gateway` Controller Pod | Webhook Service 是否有可用后端，API Server 能否调用 Webhook |
| Envoy 数据面 Pod | 数据面容器是否启动并能接收业务流量 |
| 业务后端 Pod | Service 是否将该 Pod 纳入业务流量 Endpoint |

Admission 发生在数据面 Pod Ready 之前；Webhook Controller 自己必须先完成启动，才能稳定处理
Admission 请求。

### API Server 如何找到 Webhook

Webhook 不直接配置 Pod IP，而是配置一个稳定的 Kubernetes Service：

```text
MutatingWebhookConfiguration
        ↓
Service/envoy-gateway:9443
        ↓
Service EndpointSlice
        ↓
Pod/envoy-gateway:9443
        ↓
/inject-pod-topology
```

对应配置为：

```yaml
clientConfig:
  service:
    name: envoy-gateway
    namespace: ddd-learn
    port: 9443
    path: /inject-pod-topology
```

### 什么请求会触发 Webhook

当前模板只匹配：

```yaml
rules:
  - operations: ["CREATE"]
    apiGroups: [""]
    apiVersions: ["v1"]
    resources: ["pods/binding"]
```

因此 API Server 会依次判断：

```text
是否为 CREATE？
    ↓
是否属于 core/v1？
    ↓
是否为 pods/binding 子资源？
    ↓
是否属于 namespaceSelector 选中的 namespace？
    ↓
是 → 调用 Webhook
否 → 跳过 Webhook
```

当前默认 namespaceSelector 只选择 `ddd-learn`。`pods/binding` 与 Pod 被绑定到节点的过程有关，
这套 Webhook 用于拓扑信息注入，不是普通业务 Pod 请求的通用拦截器。

### AdmissionReview 请求与响应

API Server 发送给 Webhook 的请求是 AdmissionReview：

```json
{
  "apiVersion": "admission.k8s.io/v1",
  "kind": "AdmissionReview",
  "request": {
    "uid": "...",
    "operation": "CREATE",
    "resource": {
      "resource": "pods",
      "subResource": "binding"
    },
    "object": {}
  }
}
```

Webhook 返回 AdmissionResponse。若需要修改对象，会返回 JSON Patch：

```json
{
  "allowed": true,
  "patchType": "JSONPatch",
  "patch": "..."
}
```

API Server 应用 Patch 后，才将最终对象保存到 etcd。

### Webhook 的 TLS 验证

API Server 调用 Webhook 使用 HTTPS。`MutatingWebhookConfiguration` 中的 `caBundle` 用于验证
`envoy-gateway:9443` 返回的服务端证书：

```text
certgen
    ├── 生成 CA
    ├── 生成 envoy-gateway 服务端证书和私钥
    ├── 写入 Secret/envoy-gateway
    └── 写入 MutatingWebhookConfiguration.caBundle
```

Controller Deployment 将 Secret 挂载到 `/certs`：

```yaml
volumes:
  - name: certs
    secret:
      secretName: envoy-gateway

volumeMounts:
  - name: certs
    mountPath: /certs
    readOnly: true
```

调用关系为：

```text
API Server
    ↓ HTTPS
envoy-gateway:9443
    ↓ 使用 /certs 中的证书
Topology Injector Webhook
```

### `failurePolicy` 的含义

当前配置是：

```yaml
failurePolicy: Ignore
```

表示 Webhook 超时、连接失败或返回错误时，API Server 忽略 Webhook 错误，继续处理原始请求：

```text
Webhook 正常   → 使用 Webhook 返回的修改结果
Webhook 不可用 → 忽略 Webhook，继续原始请求
```

如果改成 `Fail`，Webhook 不可用时，符合规则的 Kubernetes API 请求也会失败。

### Webhook 与业务数据面边界

Webhook 属于 Kubernetes 控制面：

```text
API Server → envoy-gateway:9443
```

业务请求属于 Envoy 数据面：

```text
客户端 → Envoy Service → xhs-service
```

两条链路没有直接关系。`MutatingWebhookConfiguration` 不处理浏览器请求，也不负责执行
`HTTPRoute` 的业务转发。

## 八、envoy-gateway Service 的作用

`Service/envoy-gateway` 是 Controller 的集群内部入口，当前暴露：

```text
18000  Controller 内部 gRPC
18001  Rate Limit 相关端口
18002  WASM 相关端口
19001  Controller Metrics
9443   Topology Injector Webhook
```

它不是业务流量入口。创建 `Gateway` 后，Envoy Gateway Controller 会另外创建属于 Gateway 数据面的
Envoy Deployment 和 Service，浏览器流量最终进入的是那套数据面 Service。

## 九、envoy-gateway-config 与 EnvoyProxy 的关系

两者职责不同：

```text
ConfigMap/envoy-gateway-config
└── Controller 启动配置：Controller 如何工作

EnvoyProxy CRD 实例
└── 数据面配置：生成的 Envoy Deployment/Service 如何运行
```

通常通过 `GatewayClass.parametersRef` 将 GatewayClass 与 EnvoyProxy 关联：

```yaml
kind: GatewayClass
spec:
  controllerName: gateway.envoyproxy.io/gatewayclass-controller
  parametersRef:
    group: gateway.envoyproxy.io
    kind: EnvoyProxy
    name: envoy-proxy-config
```

当前安装配置中：

- `envoy-gateway-config` 已创建，并已挂载到 Controller；
- `EnvoyProxy` 扩展 CRD 尚未安装，因为本实验暂时使用 `crds.enabled: false`；
- 当前没有 `EnvoyProxy` 实例；
- 后续如果需要定制数据面，需要先安装 Envoy Gateway 扩展 CRD，再创建 `EnvoyProxy`。

## 十、初始化完成后的整体关系

```text
Helm
 │
 ├─ ServiceAccount ──┐
 ├─ Role/ClusterRole  ├─ RBAC ──> Controller 调用 Kubernetes API
 ├─ Binding ──────────┘
 │
 ├─ ConfigMap ──> /config/envoy-gateway.yaml ──> Controller 启动配置
 │
 ├─ certgen Job ──> Secret/envoy-gateway ──> /certs ──> Webhook TLS
 │
 ├─ Service:9443 <─────────────────────────────┘
 │       ↑
 │       └── MutatingWebhookConfiguration
 │
 └─ Deployment/envoy-gateway
          │
          ├─ 监听 Gateway API 资源
          ├─ 监听 Service、EndpointSlice、Secret
          └─ 创建并配置 Envoy 数据面
```

初始化阶段只建立 Controller 的运行环境。真正的 `GatewayClass`、`Gateway`、`HTTPRoute` 和 Envoy
数据面资源，需要在后续业务接入步骤中创建。
