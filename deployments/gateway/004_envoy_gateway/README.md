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
