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

## 第六步：部署 Social gRPC Service

本步骤只把 Social 的 Mock Provider 部署到当前 Ambient namespace，不接入 Gateway、JWT、xDS、
故障注入或熔断。Social 使用与 `xhs_grpc` 相同的 Kitex gRPC Transport，监听容器端口 `8091`，
Service 对内暴露 `80`，端口名称为 `grpc`。

镜像构建和导入：

```bash
cd social_grpc
make image
docker save -o /tmp/ddd-learn-social-grpc-0.0.1.tar social_grpc:0.0.1
sudo k3s ctr -n k8s.io images import /tmp/ddd-learn-social-grpc-0.0.1.tar
```

部署命令：

```bash
helm upgrade --install social-grpc deployments/gateway/helm/xhs \
  --namespace ddd-learn-proxyless \
  --values deployments/gateway/009_istio_proxyless/values/social-grpc.yaml
```

这里复用已有的通用 `helm/xhs` Chart，使用 `fullnameOverride` 将资源命名为
`social-grpc-service`；Chart 的模板负责生成 Deployment、Service 和 ServiceAccount，
Social 的差异只放在 `values/social-grpc.yaml` 中。这样不会复制一份结构相同的 Helm 模板。

验证：

```bash
kubectl -n ddd-learn-proxyless rollout status deployment/social-grpc-service
kubectl -n ddd-learn-proxyless get deployment,service,pod \
  -l app.kubernetes.io/instance=social-grpc -o wide
```

当前验证结果为 1 个 Pod Ready，容器名为 `xhs` 是通用 Chart 的容器名称；Pod 没有
`istio-proxy`，并带有 `ambient.istio.io/redirection=enabled`。后续接入 Social Gateway
路由和真实 XHS Kitex Client 时，再增加对应的 descriptor、HTTPRoute 和 xDS 配置。

## 第七步：接入 Social HTTP/JSON Gateway 路由

本步骤将 Social 的两个 gRPC RPC 暴露为 HTTP/JSON：

```text
GET /v1/social/me/organizations
GET /v1/social/organizations/{organization_id}/contents?keyword=golang
```

生成 descriptor 和 Transcoder：

```bash
cd social_grpc
make descriptor
make transcoder-filter
```

其中 `transcoder/social.pb` 保存 Social service、RPC、message 和 HTTP annotation；
`ingress/social-grpc-transcoder.yaml` 将该 descriptor 内联到只匹配
`istio-ingress` Gateway Pod 的 Envoy HTTP Filter Chain。

应用路由和 Filter：

```bash
kubectl apply -f deployments/gateway/009_istio_proxyless/ingress/istio-ingress-routes.yaml
kubectl apply -f deployments/gateway/009_istio_proxyless/ingress/social-grpc-transcoder.yaml
```

`HTTPRoute/istio-social-service` 将 `/v1/social` 请求转发到
`social-grpc-service:80`；Transcoder 再将 HTTP/JSON 转换成 `SocialService` gRPC 调用。
当前返回的是 Mock Provider 内容，且 Social 尚未加入 Oathkeeper 认证规则，因此该步骤的
验证请求可以直接返回 `200`。认证、Internal JWT 和真实 XHS Client 在后续步骤单独接入。

验证：

```bash
curl 'http://192.168.2.41:30425/v1/social/organizations/G/contents?keyword=golang'
```

预期返回包含 `platform: xhs` 的模拟内容。

## 第八步：为 Social API 启用 Oathkeeper ext_authz

Oathkeeper 的 `accessRules` 已使用 `/v1/<...>` 规则覆盖 Social，但 Istio Gateway 的
`AuthorizationPolicy` 还必须把 `/v1/social` 加入 CUSTOM ext_authz 的路径列表。否则请求
虽然有 Oathkeeper 规则，也不会从 Istio Gateway 发起 ext_authz 请求。

应用策略：

```bash
kubectl apply -f deployments/gateway/009_istio_proxyless/security/authorization-policy-ingress-oathkeeper.yaml
```

未登录请求验证：

```bash
curl -i 'http://192.168.2.41:30425/v1/social/organizations/G/contents?keyword=golang'
```

预期返回 `401`。这证明请求已经进入 Oathkeeper 认证链；Social Handler 的 JWT 解析和
用户组织/Keto 权限校验仍在后续步骤实现。

## 第九步：增加 Social 调试页面

`ui_example` 新增 `/social` 页面，仅对已登录用户显示。页面使用浏览器当前的 Kratos
Session Cookie，请求两个 Social HTTP/JSON 接口：

```text
GET /v1/social/me/organizations
GET /v1/social/organizations/{organization_id}/contents?keyword=...
```

页面负责选择组织、输入关键词和展示结果；权限判断仍由后端完成。构建验证：

```bash
cd ui_example
npm run build
```

本步骤构建通过。未登录时页面显示登录入口，登录后才请求 Social 接口。

## 第十步：Social 服务验证 Internal JWT

Social 复用 `hertz_infra/serverhertz/jwt` 的公共验证器，不在服务内重新实现 JWT 验签。
Kitex Middleware 从 gRPC Metadata 读取：

```text
authorization: Bearer <token>
```

验证签名、issuer、audience、算法和有效期后，将 `Principal` 写入请求 context，业务 Handler
可以通过公共 API 读取用户身份。JWKS 从 Oathkeeper API 的：

```text
http://oathkeeper-api:4456/.well-known/jwks.json
```

读取，并在内存中缓存和定期刷新。当前 Pod 的环境变量由
`values/social-grpc.yaml` 注入；没有 JWT 或 JWT 无效时，Social 进程拒绝请求。

重新构建和部署：

```bash
cd social_grpc
make image
docker save -o /tmp/ddd-learn-social-grpc-0.0.1.tar social_grpc:0.0.1
sudo k3s ctr -n k8s.io images import /tmp/ddd-learn-social-grpc-0.0.1.tar
helm upgrade --install social-grpc deployments/gateway/helm/xhs \
  --namespace ddd-learn-proxyless \
  --values deployments/gateway/009_istio_proxyless/values/social-grpc.yaml
```

验证结果：Social Pod 为 `Running/Ready`，未登录的 Gateway 请求返回 `401`。当前 Handler
仍返回 Mock 组织和 XHS 内容，Keto 组织权限与真实 XHS Client 在后续步骤接入。

## 第十一步：Social 调用真实 XHS Kitex Client

本步骤把 Social 的 `XHSProvider` 从 Mock 实现切换为真实的 XHS gRPC Client：

```text
Gateway/Oathkeeper -> Social gRPC -> XHS gRPC
                                  └-> xhs-grpc-service:80
```

代码位置：

- `social_grpc/provider.go`：使用 XHS 生成的 `crawlservice.Client`，明确指定
  `client.WithHostPorts` 和 `transport.GRPC`，调用 `ListCrawlContents`。
- `social_grpc/main.go`：从 `XHS_GRPC_ADDR` 创建 Provider；默认地址为
  `xhs-grpc-service:80`。
- `deployments/gateway/009_istio_proxyless/values/social-grpc.yaml`：向 Pod 注入
  `XHS_GRPC_ADDR`。
- `social_grpc/Dockerfile`：将 `xhs_grpc/kitex_gen` 和本地模块元数据复制进构建上下文，
  使镜像可以编译生成的 XHS Client。

JWT 不只在 Social 边界校验一次。Social 从入站 Kitex Metadata 取出
`authorization`，再作为出站 Metadata 传给 XHS；XHS 的 JWT Middleware 会在自己的服务边界
再次校验同一个 Internal JWT。这样每个服务都保持独立的信任边界。

构建和导入镜像：

```bash
cd social_grpc
make image
docker save -o /tmp/ddd-learn-social-grpc-0.0.1.tar social_grpc:0.0.1
sudo k3s ctr -n k8s.io images import /tmp/ddd-learn-social-grpc-0.0.1.tar
rm /tmp/ddd-learn-social-grpc-0.0.1.tar
```

由于镜像标签仍为 `social_grpc:0.0.1` 且使用 `IfNotPresent`，导入新镜像后需要重启
Deployment：

```bash
kubectl -n ddd-learn-proxyless rollout restart deployment/social-grpc-service
kubectl -n ddd-learn-proxyless rollout status deployment/social-grpc-service
```

本步骤还将 XHS Provider 的本地地址配置为 `xhs-grpc-service:80`，没有引入 xDS。当前
Social 和 XHS Pod 均为 `Running/Ready`；通过已登录的 Social 页面查询时，响应内容来自
真实 XHS 服务，而不是 Mock Provider。
