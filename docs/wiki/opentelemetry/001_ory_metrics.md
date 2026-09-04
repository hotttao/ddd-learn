# Ory 组件的 Metrics 暴露与采集

本文以当前项目的 Kubernetes 架构为例，说明 Keto、Kratos 和 Oathkeeper 如何开启 Prometheus Metrics，以及 Prometheus 如何发现并采集多个 Pod 的指标。

本文讨论的是 Metrics，不是分布式 Trace。Metrics 用于观察请求数量、错误数量、延迟和资源状态；Trace 还需要 OpenTelemetry Collector，以及 Jaeger、Tempo 等 Trace 后端。

## 本文要解决的问题

本文针对以下几个问题展开：

1. 当前架构中的 Keto、Kratos、Oathkeeper 启动了哪些端口？每个端口分别负责什么？
2. Ory 服务在代码层面是如何收集 Metrics 的？业务请求和 `/metrics/prometheus` 之间是什么关系？
3. Kratos 只在 Admin 端口提供 `/admin/metrics/prometheus`，为什么能够收集到 Public 端口的请求指标？
4. Keto 和 Oathkeeper 是否采用相同的机制？它们的 Read、Write、Proxy、API 与 Metrics 端口之间如何关联？
5. 当服务扩展为多个 Pod 后，Prometheus 如何保证采集到每个 Pod 的 Metrics？

## 一、当前组件结构

当前 Ory 服务都部署在 `ddd-learn` namespace 中：

```text
ddd-learn
├── keto
├── kratos
└── oathkeeper
```

三个组件都提供 Prometheus 格式的指标，但指标所在的 HTTP 端口并不完全相同：

| 组件 | Metrics 路径 | Kubernetes Service | 说明 |
| --- | --- | --- | --- |
| Keto | `/metrics/prometheus` | `keto-metrics:80` | 独立 Metrics Service，容器端口为 `4468` |
| Kratos | `/admin/metrics/prometheus` | `kratos-admin:80` | 复用 Admin Service，容器 Admin 端口为 `4434` |
| Oathkeeper | `/metrics/prometheus` | `oathkeeper-metrics:80` | 独立 Metrics Service |

### 当前 Pod 的监听端口

Service 的 `80` 是 Kubernetes Service 端口，下面的端口是 Ory 进程实际监听的容器端口：

| Pod | 监听端口 | 用途 |
| --- | --- | --- |
| Kratos | `4433` | Public API：登录、注册、Recovery、Settings 等自助服务流程 |
| Kratos | `4434` | Admin API，以及 `/admin/metrics/prometheus` |
| Kratos Courier | `4434` | Courier 的 Metrics HTTP 端口，Service 端口为 `80`，路径为 `/metrics/prometheus` |
| Keto | `4466` | Read API：关系权限检查 |
| Keto | `4467` | Write API：关系元组写入和管理 |
| Keto | `4468` | Metrics API：`/metrics/prometheus` |
| Oathkeeper | `4455` | Proxy API；当前关闭了 Kubernetes proxy Service |
| Oathkeeper | `4456` | API：规则管理、JWKS 等管理接口 |
| Oathkeeper | `9000` | Metrics API：`/metrics/prometheus` |

Mailpit 的 `1025` 是 SMTP Service 端口，不是 Kratos Courier 的监听端口；Courier 通过 SMTP 连接 URI 访问它：

```text
Kratos Courier ──SMTP──> mailpit:1025
```

例如当前 Keto 的端口映射是：

```text
keto-read:80     → Pod:4466
keto-write:80    → Pod:4467
keto-metrics:80  → Pod:4468
```

当前 Kratos 的端口映射是：

```text
kratos-public:80 → Pod:4433
kratos-admin:80  → Pod:4434
```

当前 Oathkeeper 的端口映射是：

```text
oathkeeper-api:4456     → Pod:4456
oathkeeper-metrics:80   → Pod:9000
oathkeeper-proxy        → 未创建 Service，Pod 内仍监听 4455
```

其中 `oathkeeper-proxy` 当前没有 Kubernetes Service，因此 Envoy Gateway 或其他组件不能通过该 Service 访问 `4455`；`oathkeeper-api` 和 `oathkeeper-metrics` 仍然可在集群内部访问。

## 二、Keto

Keto 的 Metrics 使用独立端口。当前配置中，Keto 进程监听 `4468`，Helm Chart 再创建一个内部 Service，将 Service 端口 `80` 转发到该端口。

### HTTP 框架与多端口

Keto 使用 Ory 公共库封装的 `httprouterx`，底层基于 Go 标准库 `net/http`。Keto 为 Read、Write 和 Metrics 分别准备路由，再在同一个进程中启动多个 HTTP server：

```text
Keto 进程
├── RouterPublic / Read server  :4466
├── RouterAdmin  / Write server :4467
└── Metrics server              :4468
```

这不是一个 HTTP server 同时绑定三个端口，而是一个进程中运行三个独立的 server。概念代码如下：

```go
readRouter := httprouterx.NewRouterPublic()
writeRouter := httprouterx.NewRouterAdmin()
metricsRouter := httprouterx.NewRouter()

go http.ListenAndServe(":4466", readRouter)
go http.ListenAndServe(":4467", writeRouter)
go http.ListenAndServe(":4468", metricsRouter)
```

Keto 源码中对应的启动和路由组装逻辑位于 `third_party/ory/keto/internal/driver/daemon.go`，其中可以看到 `NewRouterPublic`、`NewRouterAdmin` 和 Metrics Handler 的注册。

Keto 的服务端口如下：

```text
keto-read:80     → Pod:4466  Read API
keto-write:80    → Pod:4467  Write API
keto-metrics:80  → Pod:4468  Metrics API
```

```yaml
# deployments/gateway/004_envoy_gateway/values/keto.yaml
service:
  metrics:
    enabled: true

serviceMonitor:
  enabled: true
```

两个配置的职责不同：

- `service.metrics.enabled`：创建 `keto-metrics` Service。
- `serviceMonitor.enabled`：在 Prometheus Operator 已安装的情况下，创建 `ServiceMonitor`。

Keto 的 Metrics 请求地址为：

```text
http://keto-metrics.ddd-learn.svc:80/metrics/prometheus
```

Keto Chart 的 ServiceMonitor 还要求 `service.metrics.enabled` 为 `true`，因此只打开 `serviceMonitor.enabled` 不够。

Keto 的代码层流程可以理解为：

```text
Read 请求 ───────┐
                 ├─ Keto HTTP middleware 记录请求指标
Write 请求 ──────┘
                         ↓
                 进程内 Prometheus Registry
                         ↓
GET :4468/metrics/prometheus
                         ↓
                 输出 Registry 中的指标
```

`4468` 不会反向调用 `4466` 或 `4467`，Read/Write 请求在处理时已经更新了同一个进程内的 Registry。

## 三、Kratos

Kratos 将 Metrics 放在 Admin API 下，不单独创建 Metrics Service：

```text
kratos-admin Service
├── Admin API
└── /admin/metrics/prometheus
```

当前配置为：

```yaml
# deployments/gateway/004_envoy_gateway/values/kratos.yaml
serviceMonitor:
  enabled: true
```

Kratos Admin Service 的 Kubernetes 端口为 `80`，实际转发到容器的 `4434` 端口。因此指标地址为：

```text
http://kratos-admin.ddd-learn.svc:80/admin/metrics/prometheus
```

Kratos Helm Chart 生成的 ServiceMonitor 会选择 `kratos-admin`，并配置：

```yaml
spec:
  endpoints:
    - path: /admin/metrics/prometheus
      port: http-admin
```

这里的 `port: http-admin` 是 Service 的命名端口，不是直接填写容器端口数字。

Kratos Courier 还可以单独暴露自己的 Metrics，但它属于 Courier 邮件发送组件，不是 Kratos Admin API 的 Metrics。

Kratos 的代码层流程是：

```text
Public 请求 :4433
        ↓
Kratos Public HTTP middleware
        ↓
更新 Kratos 进程内 Prometheus Registry

Prometheus
        ↓
kratos-admin:80 → Pod:4434
        ↓
GET /admin/metrics/prometheus
        ↓
读取同一个 Registry，输出 Public + Admin 指标
```

因此 `4434` 并不是主动扫描 `4433`。`4433` 在处理请求时已经完成指标记录，`4434` 只是提供 Registry 的读取接口。

### HTTP 框架与多端口

Kratos 同样使用 Ory 公共库的 `httprouterx`，底层是 Go `net/http`。它创建两个独立的路由器和 HTTP server：

```text
Kratos 进程
├── RouterPublic :4433
└── RouterAdmin  :4434
```

Metrics Handler 被注册在 Admin router 的 `/admin/metrics/prometheus` 路径上，而不是创建第三个 Metrics server。概念代码如下：

```go
publicRouter := httprouterx.NewRouterPublic()
adminRouter := httprouterx.NewRouterAdminWithPrefix()

adminRouter.GET(
    httprouterx.AdminPrefix+prometheusx.MetricsPrometheusPath,
    prometheusx.Handler(),
)

go http.ListenAndServe(":4433", publicRouter)
go http.ListenAndServe(":4434", adminRouter)
```

Kratos 源码中对应的路由和 Metrics 注册逻辑位于 `third_party/ory/kratos/cmd/daemon/serve.go`。Public 和 Admin server 虽然监听不同端口，但共享同一个 Kratos 进程和 Metrics Registry。

## 四、Oathkeeper

Oathkeeper 使用独立的 Metrics Service：

```yaml
# deployments/gateway/004_envoy_gateway/values/oathkeeper.yaml
service:
  metrics:
    enabled: true

serviceMonitor:
  enabled: true
```

采集地址为：

```text
http://oathkeeper-metrics.ddd-learn.svc:80/metrics/prometheus
```

关闭 Oathkeeper proxy Service 不会关闭 Metrics。两者是不同的 Kubernetes Service：

```text
oathkeeper-proxy    → 业务请求代理端口 4455
oathkeeper-api      → 管理 API 端口 4456
oathkeeper-metrics  → Prometheus 指标端口
```

因此当前架构可以关闭 `oathkeeper-proxy`，同时保留 `oathkeeper-metrics` 给监控系统使用。

Oathkeeper 的代码层流程是：

```text
Proxy 请求 :4455 ─┐
                  ├─ Oathkeeper HTTP middleware 记录指标
API 请求   :4456 ─┘
                          ↓
                  进程内 Prometheus Registry
                          ↓
Prometheus → oathkeeper-metrics:80 → Pod:9000
                          ↓
                  GET /metrics/prometheus
```

当前关闭的是 `oathkeeper-proxy` Kubernetes Service，不是 Pod 内部的整个 Proxy server。没有请求进入 Proxy 时，Proxy 相关指标不会增长；API 请求和其他已启用的请求指标仍可被记录。

### HTTP 框架与多端口

Oathkeeper 使用 Ory 公共库的 `httprouterx`，底层基于 Go `net/http`。它在同一个进程内启动 Proxy、API 和 Metrics 三个 HTTP server：

```text
Oathkeeper 进程
├── Proxy router   :4455
├── API router     :4456
└── Metrics router :9000
```

概念代码如下：

```go
proxyRouter := httprouterx.NewRouterPublic()
apiRouter := httprouterx.NewRouterAdmin()
metricsRouter := httprouterx.NewRouter()

go http.ListenAndServe(":4455", proxyRouter)
go http.ListenAndServe(":4456", apiRouter)
go http.ListenAndServe(":9000", metricsRouter)
```

Oathkeeper 源码中对应的 server 启动逻辑位于 `third_party/ory/oathkeeper/cmd/server/server.go`。关闭 `oathkeeper-proxy` Service 只影响 Kubernetes 的访问入口，不会让 Pod 停止监听 `4455`。

## 五、ServiceMonitor 是如何被发现的

`ServiceMonitor` 不是 Prometheus 原生的配置文件，而是 Prometheus Operator 定义的 Kubernetes 自定义资源。

完整流程如下：

```text
安装 Prometheus Operator
        ↓
注册 ServiceMonitor CRD
        ↓
Helm upgrade Ory 服务
        ↓
生成 Keto/Kratos/Oathkeeper ServiceMonitor
        ↓
Prometheus Operator 监听 ServiceMonitor
        ↓
Prometheus 根据 selector 找到对应 Service
        ↓
Prometheus 读取 Service 的 EndpointSlice
        ↓
逐个抓取 Pod 的 Metrics
```

Helm Chart 只有在集群存在以下 API 时，才会渲染 `ServiceMonitor`：

```text
monitoring.coreos.com/v1
```

### Helm 的判断逻辑

这不是应用启动后的运行时判断，而是 Helm 渲染模板时的判断。Ory Helm Chart 的模板使用 Kubernetes 能力集合：

```gotemplate
{{- if and
      (.Values.serviceMonitor.enabled)
      (.Capabilities.APIVersions.Has "monitoring.coreos.com/v1")
}}
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
...
{{- end }}
```

判断过程是：

```text
serviceMonitor.enabled == true？
        │
        ├── 否 → 不渲染 ServiceMonitor
        │
        └── 是
              ↓
集群是否提供 monitoring.coreos.com/v1？
        │
        ├── 否 → 不渲染 ServiceMonitor
        │
        └── 是 → 渲染 ServiceMonitor YAML
```

`.Capabilities.APIVersions.Has` 是 Helm 提供的模板函数。Helm 在渲染 Chart 时读取目标 Kubernetes 集群支持的 API 版本，并将它们放入 `.Capabilities.APIVersions`。Prometheus Operator 安装后会注册 `ServiceMonitor` 对应的 API，因此判断才会成立。

需要注意，`monitoring.coreos.com/v1` 是 API Group/Version，不是完整的资源名称。具体资源是：

```text
API Group/Version: monitoring.coreos.com/v1
Kind: ServiceMonitor
```

Keto Chart 还额外要求：

```gotemplate
.Values.service.metrics.enabled == true
```

因为 Keto 必须先创建 `keto-metrics` Service，才有可供 ServiceMonitor 选择和采集的目标。Kratos 和 Oathkeeper 的 Chart 则分别通过它们自己的 Metrics Service 或 Admin Service 提供采集目标。

因此推荐先安装 Prometheus Operator，再执行 Ory 服务的 Helm upgrade：

```bash
helm upgrade keto deployments/gateway/helm/keto \
  -n ddd-learn \
  -f deployments/gateway/004_envoy_gateway/values/keto.yaml

helm upgrade kratos deployments/gateway/helm/kratos \
  -n ddd-learn \
  -f deployments/gateway/004_envoy_gateway/values/kratos.yaml \
  --set-file kratos.identitySchemas.default=deployments/auth/003_keto/kratos/identity.schema.json

helm upgrade oathkeeper deployments/gateway/helm/oathkeeper \
  -n ddd-learn \
  -f deployments/gateway/004_envoy_gateway/values/oathkeeper.yaml \
  --set-file oathkeeper.mutatorIdTokenJWKs=deployments/auth/002_internal_jwt/jwks/id_token.jwks.json
```

## 六、多个 Pod 如何被采集

Prometheus 不会只访问 Service 的 ClusterIP。ServiceMonitor 选择 Service 后，Prometheus Operator 会通过 Kubernetes API 获取该 Service 对应的 EndpointSlice。

假设 Kratos 有三个 Pod：

```text
kratos-admin Service
├── 10.42.1.10:4434
├── 10.42.1.11:4434
└── 10.42.1.12:4434
```

Prometheus 会为三个 Pod 建立三个独立的抓取目标：

```text
10.42.1.10:4434/admin/metrics/prometheus
10.42.1.11:4434/admin/metrics/prometheus
10.42.1.12:4434/admin/metrics/prometheus
```

而不是只请求：

```text
kratos-admin:80/admin/metrics/prometheus
```

如果只请求 ClusterIP，Kubernetes Service 的负载均衡可能每次只转发到某一个 Pod，无法保证所有实例都被采集。

Pod 扩缩容或重启时，EndpointSlice 会同步变化，Prometheus 会自动增加或删除对应的抓取目标。

因此普通 ClusterIP Service 就足够，Metrics Service 不要求必须是 Headless Service。关键条件是：

1. Service 通过 selector 选中目标 Pod。
2. Service 的命名端口与 ServiceMonitor 中的端口名称一致。
3. Pod 上的指标 HTTP 端口和路径可访问。

## 七、ServiceMonitor 的两个 selector

ServiceMonitor 中通常有两层选择关系：

```text
Prometheus
  ↓ serviceMonitorSelector
ServiceMonitor
  ↓ spec.selector
Service
  ↓ Service selector
Pod
```

- Prometheus 的 `serviceMonitorSelector` 决定它采集哪些 ServiceMonitor。
- ServiceMonitor 的 `spec.selector` 决定它选择哪些 Service。
- Service 的 `selector` 决定最终有哪些 Pod 成为采集目标。

如果 ServiceMonitor 已创建，但 Prometheus 页面没有 Target，通常依次检查：

```bash
kubectl -n ddd-learn get servicemonitor
kubectl -n ddd-learn describe servicemonitor keto-metrics
kubectl -n ddd-learn get endpointslice
kubectl -n ddd-learn get pods --show-labels
```

重点确认 ServiceMonitor、Service 和 Pod 的标签是否能够逐层匹配。

## 八、Metrics 与 Trace 的关系

Metrics 只能回答：

```text
请求是否变多了？错误率是多少？延迟是否升高？
```

Trace 才能回答：

```text
一次请求依次经过了 Envoy Gateway、Oathkeeper、Kratos 和 xhs_service 的哪些步骤？
哪一个服务耗时最长？
```

后续要实现 Trace，需要增加：

```text
Ory / Gateway / Backend
        ↓ OTLP
OpenTelemetry Collector
        ↓
Jaeger 或 Tempo
```

Prometheus 负责 Metrics，OpenTelemetry Collector 和 Trace 后端负责 Trace，两条链路可以同时部署，但职责不同。
