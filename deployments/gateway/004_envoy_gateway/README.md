# gateway/004：Envoy Gateway k3s 实验

本目录只保存当前 k3s 实验的配置，不复制之前 Docker Gateway 的部署目录。
所有服务使用 `ddd-learn` namespace；Gateway API 的 CRD 和 `GatewayClass` 仍属于集群级资源。

## 第 1 步：部署 Ory 认证基础设施

### 当前资源

```text
ddd-learn namespace
  ├── ddd-learn-postgres：CNPG PostgreSQL，独立数据卷
  ├── mailpit：SMTP 1025、Web 8025
  ├── keto：Read 4466、Write 4467
  ├── kratos：Public 4433、Admin 4434、Courier
  └── oathkeeper：Proxy 4455、Decision API 4456
```

PostgreSQL 由 `postgres/cluster.yaml` 创建，CNPG 自动生成以下集群内 Service：

```text
ddd-learn-postgres-rw.ddd-learn.svc.cluster.local:5432
```

Kratos 使用数据库 `ory`，Keto 使用同一个 PostgreSQL 实例中的独立数据库 `keto`。
两者不共享表，也不复用 auth、Traefik 或 APISIX 实验的数据卷。

### Values 文件

- `values/kratos.yaml`：Kratos Public/Admin、浏览器回跳地址、identity schema、SMTP 和
  PostgreSQL DSN；
- `values/keto.yaml`：Keto Read/Write、PostgreSQL DSN，以及 OPL namespace 文件挂载；
- `values/oathkeeper.yaml`：Cookie Session、Internal JWT issuer/JWKS、Access Rule 和
  Decision API；
- `keto/namespaces-configmap.yaml`：本实验自己的 Keto OPL namespace。

三个 Chart 都通过 `fullnameOverride` 固定 Service 名称，并使用 Kubernetes Service DNS，
不再使用 Docker Compose 服务名或宿主机 IP。

### 部署命令

```shell
kubectl apply -f deployments/gateway/004_envoy_gateway/postgres/cluster.yaml
kubectl apply -f deployments/gateway/004_envoy_gateway/keto/namespaces-configmap.yaml
kubectl apply -f deployments/gateway/004_envoy_gateway/mailpit.yaml

kubectl -n ddd-learn wait --for=condition=Ready \
  cluster/ddd-learn-postgres --timeout=600s

helm upgrade --install keto deployments/gateway/helm/keto \
  --namespace ddd-learn \
  --values deployments/gateway/004_envoy_gateway/values/keto.yaml

helm upgrade --install kratos deployments/gateway/helm/kratos \
  --namespace ddd-learn \
  --values deployments/gateway/004_envoy_gateway/values/kratos.yaml \
  --set-file kratos.identitySchemas.default=deployments/auth/003_keto/kratos/identity.schema.json

helm upgrade --install oathkeeper deployments/gateway/helm/oathkeeper \
  --namespace ddd-learn \
  --values deployments/gateway/004_envoy_gateway/values/oathkeeper.yaml \
  --set-file oathkeeper.mutatorIdTokenJWKs=deployments/auth/002_internal_jwt/jwks/id_token.jwks.json
```

`--set-file` 只在安装时把 schema 和 JWKS 写入 Kubernetes ConfigMap/Secret，运行中的
服务不依赖这些本地文件路径。示例密钥仍是教学用途，生产环境应改用 Secret 管理。

### 当前验证结果

```shell
kubectl -n ddd-learn get pods,svc
helm list --namespace ddd-learn
```

结果：PostgreSQL、Mailpit、Keto、Kratos、Kratos Courier、Oathkeeper 均为 Running/Ready，
三个 Helm Release 状态为 `deployed`。

## 第 2 步：部署 xhs_service

### 当前配置

- Chart：`deployments/gateway/helm/xhs`；
- Values：`values/xhs.yaml`；
- 镜像：`xhs_service:0.0.1`；
- Service：`xhs-service:80`，转发到 Pod 的 `8082`；
- Keto 地址：`http://keto-read:80`；
- Oathkeeper JWKS 地址：`http://oathkeeper-api:4456/.well-known/jwks.json`；
- 健康检查：`GET /health`。

### 构建并导入镜像

当前 k3s 节点使用 containerd，Docker 构建出的镜像需要手动导入：

```shell
docker build -f xhs_service/Dockerfile -t xhs_service:0.0.1 .
docker save -o /tmp/xhs_service-0.0.1.tar xhs_service:0.0.1
sudo k3s ctr images import /tmp/xhs_service-0.0.1.tar
```

### Helm 部署

```shell
helm upgrade --install xhs deployments/gateway/helm/xhs \
  --namespace ddd-learn \
  --values deployments/gateway/004_envoy_gateway/values/xhs.yaml \
  --wait --timeout 180s
```

### 当前验证

```shell
kubectl -n ddd-learn get deploy,pod,svc,endpointslice \
  -l app.kubernetes.io/instance=xhs -o wide
kubectl -n ddd-learn wait --for=condition=Available \
  deployment/xhs-service --timeout=120s
```

当前结果：

```text
xhs-service Pod       Running 1/1
xhs-service Service   ClusterIP:80
EndpointSlice          指向 Pod:8082
GET /health            200 {"status":"ok"}
```

## 第 3 步第 1 个功能点：安装 Envoy Gateway Controller

当前 k3s 已经提供 Gateway API CRD，因此使用本地 `gateway-helm` Chart 时关闭 Chart
内置 CRD，避免重复管理集群级资源。控制器本身安装到 `ddd-learn` namespace。

### 使用的 Values

`values/envoy-gateway.yaml`：

- `crds.enabled: false`：不重复安装当前集群已有的 Gateway API CRD；
- `namespaceOverride: ddd-learn`：将控制器资源放到本实验 namespace；
- `global.images.envoyGateway.image`：使用国内镜像地址和 `v1.6.1` 版本；
- `service.type: ClusterIP`：控制器 Service 只作为集群内部控制面入口。

### 安装命令

```shell
helm upgrade --install eg deployments/gateway/helm/envoy_gateway/gateway-helm \
  --namespace ddd-learn \
  --values deployments/gateway/helm/envoy_gateway/gateway-helm/values.tmpl.yaml \
  --values deployments/gateway/004_envoy_gateway/values/envoy-gateway.yaml \
  --wait --timeout 600s
```

第一次安装时 certgen Job 拉取控制器镜像耗时较长，第一次 Helm 等待超时；镜像拉取完成后
重新执行相同命令，Release `eg` revision 2 成功部署。

### 当前验证

```shell
kubectl -n ddd-learn get deploy,pod,svc \
  -l app.kubernetes.io/instance=eg -o wide
helm list --namespace ddd-learn
```

当前结果：

```text
envoy-gateway Deployment  1/1 Available
envoy-gateway Pod         1/1 Running
eg Helm Release           deployed
```

### 本功能点创建的资源及作用

本次 Helm Release 创建的是 Envoy Gateway 控制面资源，不是业务流量入口。主要资源如下：

| 资源 | 名称 | 作用 |
| --- | --- | --- |
| Deployment | `envoy-gateway` | 运行 Envoy Gateway Controller，监听 Gateway API 并生成数据面资源 |
| Pod | `envoy-gateway-*` | Controller 的实际运行实例 |
| Service | `envoy-gateway` | 暴露 Controller 的内部管理、Webhook、Metrics 等端口 |
| ConfigMap | `envoy-gateway-config` | 保存 Envoy Gateway Controller 配置，例如 GatewayClass controllerName 和 Kubernetes Provider |
| ServiceAccount | `envoy-gateway` | Controller 访问 Kubernetes API 的身份 |
| ClusterRole/ClusterRoleBinding | `eg-gateway-helm-envoy-gateway-*` | 授权 Controller 读取和更新集群级、namespace 级 Gateway API 与工作负载资源 |
| Role/RoleBinding | `eg-gateway-helm-infra-manager` | 授权 Controller 管理 `ddd-learn` 中的数据面基础资源 |
| Role/RoleBinding | `eg-gateway-helm-leader-election-*` | 支持 Controller 多副本时进行 Leader Election |
| ServiceAccount | `eg-gateway-helm-certgen` | certgen Job 生成或更新控制器所需的证书 |
| Job | `eg-gateway-helm-certgen` | 安装阶段生成 Webhook 和控制器使用的 TLS Secret |
| Secret | `envoy`、`envoy-gateway` 等 | 保存 certgen 生成的 TLS 证书和密钥 |
| MutatingWebhookConfiguration | `envoy-gateway-topology-injector.ddd-learn` | 为启用拓扑注入的数据面 Pod 提供 Kubernetes Admission Webhook |

其中 `ClusterRole` 和 `ClusterRoleBinding` 虽然属于集群级资源，但它们只是控制器运行所需的
权限；控制器 Deployment、Service、ConfigMap 和 Job 都位于 `ddd-learn` namespace。

本次没有创建以下资源：

- `GatewayClass`：下一小步创建，由它指定由哪个 Controller 管理 Gateway；
- `Gateway`：后续声明监听地址和端口；
- `HTTPRoute`：后续声明 URL 到 Service 的转发规则；
- Envoy 数据面 Deployment/Service：只有创建并被接受的 `Gateway` 后，Controller 才会生成。

### Envoy Gateway 的 ServiceAccount 与 Kubernetes RBAC

Envoy Gateway Controller 访问 Kubernetes API 时，不使用宿主机用户身份，而使用 Pod 上配置的
ServiceAccount：

```text
Deployment/envoy-gateway
        ↓ serviceAccountName
ServiceAccount/envoy-gateway
        ↓
RoleBinding / ClusterRoleBinding
        ↓
Role / ClusterRole
        ↓
Kubernetes API Server 的 RBAC Authorizer
```

Controller Pod 的身份可以通过以下配置看到：

```yaml
spec:
  template:
    spec:
      serviceAccountName: envoy-gateway
```

Controller 启动后，Kubernetes 会将该 ServiceAccount 的 Token 以投影方式提供给 Pod。Controller
使用这个 Token 调用 Kubernetes API，例如监听 `Gateway`、`HTTPRoute`、`Service`、`EndpointSlice`
和 `Secret`，或者创建 Envoy 数据面 Deployment 和 Service。

权限不是由 ServiceAccount 自己定义的。ServiceAccount 只是身份，真正的权限来自绑定关系：

```text
ServiceAccount/envoy-gateway
    ├── ClusterRoleBinding/eg-gateway-helm-envoy-gateway-rolebinding
    │       └── ClusterRole/eg-gateway-helm-envoy-gateway-role
    │
    └── RoleBinding/eg-gateway-helm-infra-manager
            └── Role/eg-gateway-helm-infra-manager
```

两类绑定的范围不同：

- `ClusterRoleBinding`：授予集群范围的权限，例如读取集群级 `GatewayClass` 和访问多个
  namespace 的资源；
- `RoleBinding`：将一个 Role 的权限限制在 `ddd-learn` namespace，例如管理该 namespace 中
  的 Envoy 数据面 Deployment、Service 和 ConfigMap。

权限规则由 `apiGroups`、`resources` 和 `verbs` 组成：

```yaml
rules:
  - apiGroups: ["gateway.networking.k8s.io"]
    resources: ["gateways", "httproutes"]
    verbs: ["get", "list", "watch"]
```

含义是：使用该 ServiceAccount 的 Controller 可以读取、列出并监听 Gateway API 资源，但不能
自动执行规则中没有声明的操作。Controller 请求 Kubernetes API 时，RBAC Authorizer 会检查：

```text
请求身份 + API Group + Resource + Namespace + Verb
```

只有匹配某条 Role/ClusterRole 规则时才允许访问，否则返回 `403 Forbidden`。例如 Controller
没有 `secrets` 的 `get` 权限时，即使 Pod 能够访问 Kubernetes API，也不能读取 TLS Secret。

可以使用以下命令查看实际授权关系：

```shell
kubectl -n ddd-learn get deployment envoy-gateway \
  -o jsonpath='{.spec.template.spec.serviceAccountName}{"\n"}'
kubectl -n ddd-learn describe serviceaccount envoy-gateway
kubectl get clusterrole eg-gateway-helm-envoy-gateway-role -o yaml
kubectl -n ddd-learn get role eg-gateway-helm-infra-manager -o yaml
kubectl get clusterrolebinding eg-gateway-helm-envoy-gateway-rolebinding -o yaml
kubectl -n ddd-learn get rolebinding eg-gateway-helm-infra-manager -o yaml
```

本功能点只安装控制面，没有创建 `GatewayClass`、`Gateway` 或 `HTTPRoute`。这些资源将在后续
步骤中创建和验证。
