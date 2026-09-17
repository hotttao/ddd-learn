# K3s 可观测性 LGTM、Alloy 与 eBPF

本实验基于 `docs/wiki/opentelemetry/003_k3s_observability_lgtm_ebpf.md`，在当前 K3s、Istio Ambient、Ory、`xhs_grpc`、`social_grpc` 和 `ui_example` 架构上，逐步建立：

```text
Metrics  -> Prometheus
Logs     -> Alloy Node Agent -> Alloy Gateway -> Loki
Traces   -> Alloy Gateway -> Tempo
Profiles -> Alloy eBPF -> Pyroscope
```

观测组件部署到 `observability` namespace，业务仍位于 `ddd-learn-proxyless`。本实验不修改现有认证、Keto、Istio Gateway 和业务功能；除明确需要外不使用 `istioctl` 创建资源。

## 实施约定

每一步都是独立功能点，完成后必须：

1. 先部署并验证；
2. 说明创建的 Kubernetes 资源；
3. 说明组件实际监听的端口和 Service；
4. 说明组件功能、数据入口、数据出口；
5. 等待确认后单独提交；
6. commit 信息包含 `stage 7 step N`。

安装前固定 Chart 版本，并用 `helm show values` 检查当前 Chart 字段，不使用 `latest`。

## 阶段一：基础环境

### 步骤 0：确认 K3s 可观测性基线

检查：

```bash
kubectl get nodes
kubectl get storageclass
kubectl get pods -n ddd-learn-proxyless
```

确认节点 CPU、内存、Linux 内核、默认 StorageClass，以及：

```text
/var/log/pods
/var/log/containers
/var/lib/rancher/k3s/agent/containerd/
```

本步骤不部署组件。

提交：

```text
docs(opentelemetry): stage 7 step 0 define k3s observability baseline
```

### 步骤 1：创建 observability 基础资源

创建：

- `observability` Namespace；
- 基础 ServiceAccount；
- 后续组件需要的 RBAC；
- 必要的 NetworkPolicy。

说明 Namespace、ServiceAccount、RBAC 的作用，并解释为什么初期不让观测组件加入 Istio Ambient。

提交：

```text
feat(opentelemetry): stage 7 step 1 create observability namespace
```

## 阶段二：Metrics

### 步骤 2：部署 Prometheus Operator 和基础指标

部署 `kube-prometheus-stack`，初期关闭其中的 Grafana。组件包括：

- Prometheus Operator；
- Prometheus；
- Alertmanager；
- kube-state-metrics；
- node-exporter；
- ServiceMonitor、PodMonitor CRD。

典型端口：

| 组件 | 端口 | 作用 |
|---|---:|---|
| Prometheus | 9090 | PromQL 查询和指标存储 |
| Alertmanager | 9093 | 告警接收和分发 |
| kube-state-metrics | 8080 | Kubernetes 对象状态指标 |
| node-exporter | 9100 | 节点 CPU、内存、磁盘、网络指标 |
| Operator webhook | 443 | 监控 CRD 校验和管理 |

验证 Prometheus Targets 为 `UP`，并查询 `up`、节点指标和 Pod 重启指标。完成后记录实际资源、Service、端口和 PVC。

提交：

```text
feat(opentelemetry): stage 7 step 2 deploy prometheus stack
```

### 步骤 3：接入 Ory、Istio 和业务 Metrics

逐个确认并接入：

- Kratos；
- Keto；
- Oathkeeper；
- Istio Ingress；
- `xhs_grpc`；
- `social_grpc`。

记录每个服务的 metrics 进程、监听端口、Service、ServiceMonitor 以及是否通过 admin 端口提供指标。如果业务服务尚未提供 `/metrics`，只记录缺口，不扩展业务实现。

提交：

```text
feat(opentelemetry): stage 7 step 3 scrape ory and application metrics
```

## 阶段三：后端

### 步骤 4：部署 Loki

部署单体 Loki 和 PVC。典型端口：

| 端口 | 作用 |
|---:|---|
| 3100 | HTTP API、日志写入和查询 |
| 9096 | 内部 gRPC，按实际 Chart 确认 |

说明 Loki 的 labels 和日志正文区别，以及为什么 Token、Cookie、动态 ID 不能作为高基数 label。本步骤只部署后端，不接入采集。

提交：

```text
feat(opentelemetry): stage 7 step 4 deploy loki backend
```

### 步骤 5：部署 Tempo

部署单体 Tempo 和 PVC。典型端口：

| 端口 | 作用 |
|---:|---|
| 3200 | HTTP 查询 API |
| 4317 | OTLP gRPC 接收 |
| 4318 | OTLP HTTP 接收 |
| 9411 | Zipkin 接收，可选 |

说明应用、Alloy 和 Tempo 的职责边界，以及 Tempo 为什么不会主动采集 Trace。

提交：

```text
feat(opentelemetry): stage 7 step 5 deploy tempo backend
```

### 步骤 6：部署 Pyroscope

部署单体 Pyroscope 和 PVC。典型端口：

| 端口 | 作用 |
|---:|---|
| 4040 | HTTP API、查询和 Profile 接收 |

说明 Profile 与 Trace、Metrics 的区别，以及为什么本步骤完成后还不会有业务 Profile。

提交：

```text
feat(opentelemetry): stage 7 step 6 deploy pyroscope backend
```

## 阶段四：Grafana 和 Alloy

### 步骤 7：部署 Grafana 和数据源

部署 Grafana、Service、PVC 和配置 Secret/ConfigMap，配置：

```text
Prometheus
Loki
Tempo
Pyroscope
```

典型端口：

| 端口 | 作用 |
|---:|---|
| 3000 | Grafana Web UI 和 HTTP API |

说明 Grafana 只负责查询和展示，不负责采集；说明四个 datasource 如何通过 Kubernetes Service DNS 访问，以及 Trace、Log、Profile 如何关联。

提交：

```text
feat(opentelemetry): stage 7 step 7 deploy grafana datasources
```

### 步骤 8：部署 Alloy Gateway

以 Deployment 部署 Alloy Gateway，创建 Deployment、ClusterIP Service、ConfigMap、ServiceAccount 和 RBAC。

典型端口：

| 端口 | 作用 |
|---:|---|
| 4317 | OTLP gRPC |
| 4318 | OTLP HTTP |
| 12345 | Alloy HTTP/metrics/UI，按实际配置确认 |

配置链路：

```text
OTLP receiver
  -> memory_limiter
  -> batch
  -> k8sattributes
  -> Loki / Tempo / Pyroscope exporter
```

说明 Gateway 为什么使用 Deployment、为什么不挂载节点日志目录，以及各处理器解决的问题。

提交：

```text
feat(opentelemetry): stage 7 step 8 deploy alloy gateway
```

### 步骤 9：部署 Alloy Node Agent 采集日志

以 DaemonSet 部署，每个节点一个实例。初期实现：

```text
/var/log/pods
  -> Alloy Node Agent
  -> Alloy Gateway
  -> Loki
```

根据实际 K3s 节点挂载 `/var/log/pods`、`/var/log/containers`，并配置 Kubernetes API RBAC。

说明 DaemonSet 和 Deployment 的区别、CRI 日志格式、Kubernetes labels，以及为什么日志正文不应成为高基数索引。

提交：

```text
feat(opentelemetry): stage 7 step 9 deploy alloy node agent logs
```

### 步骤 10：采集节点和 kubelet 指标

增加主机 CPU、内存、磁盘、网络、kubelet stats 和容器资源指标。明确哪些指标由 Prometheus scrape，哪些由 Alloy remote write，避免重复采集。

验证：

```promql
node_cpu_seconds_total
node_memory_MemAvailable_bytes
container_cpu_usage_seconds_total
kube_pod_container_status_restarts_total
```

提交：

```text
feat(opentelemetry): stage 7 step 10 collect node and kubelet metrics
```

### 步骤 11：启用 Pyroscope eBPF Profile

在 Node Agent 中启用 `pyroscope.ebpf` 和 `pyroscope.write`。根据实际 Chart 确认：

- `hostPID: true`；
- `privileged: true`；
- `/proc)、`/sys)、`/sys/fs/bpf`、`/lib/modules` 挂载；
- Pod、Namespace、Node RBAC。

初期只采集 `xhs_grpc` 和 `social_grpc`。说明 eBPF 高权限、hostPID、进程/容器/Pod 关联，以及内核限制。

提交：

```text
feat(opentelemetry): stage 7 step 11 enable alloy ebpf profiling
```

## 阶段五：业务 Trace 和关联

### 步骤 12：给 Social 和 XHS 接入 OTel Trace

为 `social_grpc` 和 `xhs_grpc` 增加最小 Trace 接入，覆盖：

```text
HTTP /v1/social/...
  -> Istio Gateway
  -> Social gRPC
  -> XHS gRPC
```

验证 `traceparent`、Gateway header、Social 到 XHS 的 parent-child Span，以及 Tempo 中的完整调用链。

提交：

```text
feat(opentelemetry): stage 7 step 12 instrument social and xhs traces
```

### 步骤 13：统一四类信号的资源属性

统一：

```text
service.name
service.namespace
service.version
service.instance.id
deployment.environment
k8s.namespace.name
k8s.pod.name
k8s.pod.uid
k8s.node.name
```

说明哪些字段适合索引，哪些只能作为日志字段或 Trace 属性，以及如何从 Trace 跳转到日志和 Profile。

提交：

```text
feat(opentelemetry): stage 7 step 13 correlate observability signals
```

### 步骤 14：完整验收和故障场景

使用当前系统执行：

```text
登录
访问 Social 页面
调用组织接口
查询内容
调用 XHS
触发 Social 故障注入
触发 XHS 熔断
```

验收：

| 信号 | 验收结果 |
|---|---|
| Metrics | Prometheus 查询请求量、错误率、延迟、Pod 重启 |
| Logs | Loki 按 namespace、pod、container 查询业务日志 |
| Traces | Tempo 显示 Gateway、Social、XHS 调用链 |
| Profiles | Pyroscope 显示业务服务 CPU 调用栈 |
| 关联 | Trace、Log、Profile 定位到同一服务实例 |

提交：

```text
docs(opentelemetry): stage 7 step 14 document lgtm ebpf acceptance
```

## 每步必须记录的实际结果

```text
资源名称
资源类型
namespace
Service 名称
ClusterIP
端口和 targetPort
Pod/Deployment/StatefulSet/DaemonSet
PVC
ServiceAccount
RBAC
数据入口
数据出口
验证命令
```

最终需要能够说明一次业务请求产生的 Metrics、Log、Trace、Profile 分别由谁采集、经过哪些 Service、存储在哪里，以及 Grafana 如何查询和关联。
