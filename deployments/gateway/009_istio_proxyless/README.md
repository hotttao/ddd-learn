# Gateway 009：Istio Proxyless

本实验从 `deployments/gateway/006_istio_ambient` 选择性继承服务配置，但运行在独立的
`ddd-learn-proxyless` namespace。该 namespace 注册到 Istio Ambient：应用内的 Kitex
Proxyless Client 负责客户端 L7 治理，节点 ztunnel 负责透明 mTLS、工作负载身份和 L4 授权。
本实验不为 XHS 绑定 Waypoint，避免与 Proxyless 重复执行客户端 L7 策略。

## 第一步：XHS 视角的 xDS 兼容性探针

`compatibility/` 不是业务服务，也不监听端口。它是运行一次后退出的诊断 Pod，使用
`kitex-contrib/xds` 生成的 Kitex ADS Client 连接 `istiod.istio-system.svc:15010`。

探针模拟一个只访问 XHS 的 Proxyless outbound 客户端：

```text
NDS：订阅完整 NameTable，只输出 xhs-service 条目
CDS：精确订阅 xhs-service 的基础 Cluster 和 v1/v2 subset
EDS：只订阅 CDS 返回的 XHS endpoint
```

NDS 的 NameTable 是一个整体资源，不能按单个服务订阅；CDS、EDS 支持通过
`ResourceNames` 精确订阅。该探针不再使用 CDS 的空名称通配请求，因此不会把整个集群的
服务 Cluster 下载到本地。LDS/RDS 属于服务端 inbound 或 Envoy 路由场景，本次 outbound
探针不请求它们。

### 继承的部署基线

| 目录 | 内容 |
| --- | --- |
| `base/` | Ambient namespace、Mailpit 和 UI |
| `ingress/` | Istio Ingress、HTTPRoute 和 gRPC Transcoder |
| `postgres/` | CloudNativePG Cluster |
| `keto/`、`seed/`、`values/` | Ory、数据库初始化和 Helm values |
| `security/` | Oathkeeper ext_authz 和 Istio Mesh 配置 |

没有复制 006 的 `traffic/`，因为其中的 v1/v2 灰度、Waypoint Retry 和旧故障注入不属于
本实验基线。

### 探针配置

目标服务通过 `probe.yaml` 配置：

```yaml
TARGET_SERVICE: xhs-grpc-service.ddd-learn-proxyless.svc.cluster.local
TARGET_PORT: "80"
TARGET_SUBSETS: v1,v2
```

探针使用 15010 明文 xDS 且不挂载 ServiceAccount Token，仅用于单集群兼容性实验。生产
Proxyless 客户端应使用 15012、工作负载身份和 TLS。

### 构建、导入和运行

```bash
cd deployments/gateway/009_istio_proxyless/compatibility
GOWORK=off CGO_ENABLED=0 GOOS=linux \
  go build -o .build/kitex-xds-compatibility .
docker build --no-cache -t ddd-learn-kitex-xds-compatibility:0.0.1 .
docker save -o /tmp/ddd-learn-kitex-xds-compatibility-0.0.1.tar \
  ddd-learn-kitex-xds-compatibility:0.0.1
sudo k3s ctr -n k8s.io images import \
  /tmp/ddd-learn-kitex-xds-compatibility-0.0.1.tar
kubectl apply -f probe.yaml
kubectl logs -n ddd-learn-proxyless pod/kitex-xds-compatibility
```

### 判断结果

成功时日志会分别输出：

```text
NDS target service=xhs-grpc-service.ddd-learn-proxyless.svc.cluster.local ips=[...]
CDS resource names=[outbound|80||xhs-grpc-service..., outbound|80|v1|xhs-grpc-service..., ...]
EDS resource names=[outbound|80||xhs-grpc-service..., ...]
compatibility gate passed: [...]
```

这一步只验证 Kitex xDS 客户端能否连接 Istio 并按 XHS 目标订阅、解析 outbound 基础资源；
Router、Resolver 和 Circuit Breaker 的运行时行为在后续步骤验证。

## 第四步：部署独立的 xhs_grpc

本步骤保留原有 `xhs_service/xhs-service`，新增独立的 `xhs_grpc/xhs-grpc-service`，避免
Hertz HTTP 服务和 Kitex gRPC 服务混淆。新服务使用两个 Pod，容器监听 `8090`，Service
对内暴露 `80`，端口名称为 `grpc`。

构建镜像并导入 k3s：

```bash
cd xhs_grpc
make image
docker save -o /tmp/xhs_grpc-0.0.1.tar xhs_grpc:0.0.1
sudo k3s ctr -n k8s.io images import /tmp/xhs_grpc-0.0.1.tar
```

部署：

```bash
helm upgrade --install xhs-grpc deployments/gateway/helm/xhs \
  --namespace ddd-learn-proxyless \
  --values deployments/gateway/009_istio_proxyless/values/xhs-grpc.yaml
```

验证资源和 gRPC Probe：

```bash
kubectl -n ddd-learn-proxyless get pods -l 'app.kubernetes.io/instance=xhs-grpc'
kubectl -n ddd-learn-proxyless get service xhs-grpc-service
kubectl -n ddd-learn-proxyless get endpointslice \
  -l kubernetes.io/service-name=xhs-grpc-service -o wide
kubectl -n ddd-learn-proxyless describe pod -l 'app.kubernetes.io/instance=xhs-grpc'
```

`xhs-grpc-service` 不绑定 Waypoint，Pod 也不注入 Envoy Sidecar；namespace 的 Ambient 标签
使其网络流量由节点 ztunnel 接管。后续 Social 启用 Proxyless xDS 后，Kitex Client 是客户端
outbound L7 的唯一执行者，ztunnel 只执行 L4 安全与转发。

Kitex Server 保留默认协议探测器，由它根据 HTTP/2 preface 选择 gRPC 的 nphttp2
处理路径；不要直接用 `WithTransHandlerFactory(nphttp2.NewSvrTransHandlerFactory())`
替换默认探测器。当前版本在该显式配置下只会建立 TCP 连接，标准 gRPC 握手无法完成，最终表现为
Kubernetes gRPC Probe 超时。Protobuf unary RPC 同时启用
`WithCompatibleMiddlewareForUnary()`，使 Internal JWT endpoint middleware 可以处理 Kitex
生成代码中的 `StreamingUnary` 方法。

同一个 Kitex Server 注册了 `CrawlService`、`OrganizationService` 和标准
`grpc.health.v1.Health`。`OrganizationService` 从 Internal JWT Principal 取得用户 ID，再通过
`KETO_READ_URL=http://keto-read:80` 查询 `Organization` relation tuples；它不会把 Alice/Bob
的组织写死在 gRPC 服务中。

`CrawlService` 在每个业务 RPC 内使用同一个 Keto Read Client 校验组织权限：

| RPC | Keto relation |
| --- | --- |
| `StartCrawlTask` | `start_crawl` |
| `ListCrawlContents`、`GetKeywords` | `view_content` |
| `UpdateKeywords` | `modify_keywords` |

校验主体来自 Internal JWT 的 `sub`，并转换成 Keto subject `User:<sub>`；namespace 是
`Organization`，object 是请求中的 `organization_id`。权限不足返回 gRPC
`PermissionDenied`，Transcoder 将其映射为 HTTP 403；Keto 暂时不可用返回 gRPC
`Unavailable`，避免把基础设施错误误报成无权限。

## 第五步：生成 XHS gRPC-JSON Transcoder 配置

先根据 XHS Protobuf IDL 生成 descriptor：

```bash
cd xhs_grpc
make descriptor
```

输出文件为：

```text
deployments/gateway/009_istio_proxyless/transcoder/xhs.pb
```

再将 descriptor 内联到只作用于 Istio Ingress Gateway 的 EnvoyFilter：

```bash
make transcoder-filter
```

生成文件为：

```text
deployments/gateway/009_istio_proxyless/ingress/xhs-grpc-transcoder.yaml
```

该 EnvoyFilter 使用 `workloadSelector` 匹配
`gateway.networking.k8s.io/gateway-name: istio-ingress`，并在 HTTP Connection Manager 的
router filter 之前插入 `envoy.filters.http.grpc_json_transcoder`。descriptor 通过
`proto_descriptor_bin` 内联，当前只启用 `CrawlService` 和 `OrganizationService`。

Transcoder 必须设置：

```yaml
match_incoming_request_route: true
```

这样 Envoy 根据转码前的 `/v1/xhs/...` 选择 `HTTPRoute/istio-xhs-service`。如果不设置，转码器
把 path 改成 `/<package>.<service>/<method>` 后会重新匹配路由，并落入 `/` 的 UI 兜底路由，
表面现象是请求被发往 `ui-example` 并返回 `503 connection termination`。

应用该声明式配置后，可通过下面的请求确认 Transcoder 和路由已经生效：

```bash
kubectl apply -f deployments/gateway/009_istio_proxyless/ingress/xhs-grpc-transcoder.yaml
curl -b /tmp/alice-proxyless-cookies.txt \
  http://192.168.2.41:30425/v1/xhs/me/organizations
```

Alice 返回组织 `G` 和角色 `admins`；Bob 返回组织 `G` 和角色 `members`。

## 第五步：将 `/v1/xhs` 路由切换到 gRPC Service

`ingress/istio-ingress-routes.yaml` 中的 `HTTPRoute/istio-xhs-service` 仍然匹配
`/v1/xhs`，但后端已经切换为：

```yaml
backendRefs:
  - name: xhs-grpc-service
    port: 80
```

入口请求的处理顺序是：

```text
HTTP/JSON /v1/xhs/*
  → HTTPRoute 匹配
  → Oathkeeper ext_authz
  → grpc_json_transcoder 根据 xhs.pb 转成 gRPC
  → xhs-grpc-service:80
  → xhs_grpc Pod:8090
```

旧的 `xhs-service` HTTP 后端不参与这条新链路；Oathkeeper 的认证规则仍按 `/v1/xhs` 匹配，
因此本步骤没有改变认证入口。

### 联调结果

当前请求链为：

```text
Kratos Session Cookie
  -> Istio Ingress
  -> Oathkeeper ext_authz 签发 Internal JWT
  -> gRPC-JSON Transcoder
  -> xhs-grpc-service
  -> XHS JWT Middleware
  -> CrawlService / OrganizationService
  -> Keto Read API
```

实际验证结果：

| 身份与操作 | HTTP 状态 |
| --- | --- |
| Alice 查询组织、查看内容、启动任务、修改关键词 | `200` |
| Bob 查询组织、查看内容、启动任务 | `200` |
| Bob 修改关键词 | `403` |
| 未携带 Kratos Session 请求 XHS API | `401` |

两个 `xhs-grpc-service` Pod 均为 Ready，说明 Kubernetes 原生 gRPC Probe 可以直接调用各
Pod 的 `grpc.health.v1.Health/Check`；Ambient 不需要将 Probe 改写给 Sidecar。
