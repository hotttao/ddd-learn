# K3s 可观测性实验

本目录对应 `docs/human/opentelemetry/003_k3s_observability_lgtm_ebpf.md`。

当前环境：

```text
K3s: v1.36.2+k3s1
节点: debian11 / 192.168.2.41
业务 namespace: ddd-learn-proxyless
观测 namespace: observability
```

## 第 1 步：创建基础资源

执行：

```bash
kubectl apply -f deployments/opentelemetry/003_k3s_observability/base/resources.yaml
```

创建了 `observability` Namespace、两个 Alloy ServiceAccount，以及 Node Agent 使用的只读 ClusterRole 和 ClusterRoleBinding。

`observability` 使用 `istio.io/dataplane-mode: none`，表示观测组件暂不加入 Istio Ambient。当前没有创建 NetworkPolicy，避免在后续组件端口确定前阻断通信。

## 第 2 步：部署 Prometheus 基础监控栈

### 添加仓库和检查 Chart

```bash
helm repo add prometheus-community https://prometheus-community.github.io/helm-charts
helm repo update
helm show values prometheus-community/kube-prometheus-stack --version 91.4.1
```

当前固定版本：

```text
Chart: kube-prometheus-stack 91.4.1
Prometheus: v3.14.0
Alertmanager: v0.34.0
```

### 安装

配置文件：[prometheus-values.yaml](./prometheus-values.yaml)

```bash
helm upgrade --install prometheus \
  prometheus-community/kube-prometheus-stack \
  --version 91.4.1 \
  --namespace observability \
  --values deployments/opentelemetry/003_k3s_observability/prometheus-values.yaml \
  --timeout 10m \
  --wait
```

本步骤关闭 Chart 内置 Grafana，Grafana 在后续步骤单独部署。

### 创建的组件

| 组件 | 资源 | 端口 | 功能 |
|---|---|---:|---|
| Prometheus Operator | Deployment | 443 | 管理 Prometheus 相关资源 |
| Prometheus | StatefulSet | 9090 | 抓取、存储和查询 Metrics |
| Alertmanager | StatefulSet | 9093 | 接收和处理告警 |
| kube-state-metrics | Deployment | 8080 | 暴露 Kubernetes 对象状态 |
| node-exporter | DaemonSet | 9100 | 暴露节点操作系统指标 |

实际 Service：

```text
prometheus-kube-prometheus-prometheus:9090
prometheus-kube-prometheus-alertmanager:9093
prometheus-kube-prometheus-operator:443
prometheus-kube-state-metrics:8080
prometheus-prometheus-node-exporter:9100
```

Prometheus 使用一个 `10Gi` 的 `local-path` PVC，指标保留 `7d`。这是单节点学习环境配置。

### 验收命令

```bash
kubectl get pods -n observability
kubectl get svc -n observability
kubectl get prometheus,alertmanager -n observability
kubectl get pvc -n observability
kubectl get servicemonitor -n observability
```

访问 Prometheus：

```bash
kubectl port-forward -n observability \
  svc/prometheus-kube-prometheus-prometheus 9090:9090
```

浏览器访问 `http://127.0.0.1:9090/targets`，并查询：

```promql
up
```

## 什么是监控 CRD

CRD 是 CustomResourceDefinition，即 Kubernetes 自定义资源定义。它可以为 Kubernetes API 增加新的资源类型。

安装 Prometheus Operator 后，集群除了认识 Pod、Service、Deployment 等内置对象，还认识：

```text
Prometheus
Alertmanager
ServiceMonitor
PodMonitor
PrometheusRule
Probe
ScrapeConfig
```

这些对象不是普通 ConfigMap，也不是 Prometheus 配置文件，而是 Kubernetes API 中的新对象类型。

例如 `ServiceMonitor`：

```yaml
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  name: my-service
spec:
  selector:
    matchLabels:
      app: my-service
  endpoints:
    - port: metrics
```

处理流程：

```text
kubectl apply ServiceMonitor
        -> Kubernetes API Server 保存对象
        -> Prometheus Operator Watch 到对象变化
        -> Operator 生成 Prometheus scrape 配置
        -> Prometheus 定期请求 Service 的 metrics 端口
```

因此：

- CRD 定义集群支持什么新类型；
- `ServiceMonitor` 是这个类型的一个实例；
- Prometheus Operator 负责读取实例并生成配置；
- Prometheus 负责真正发起 HTTP scrape。

当前安装的 CRD：

```bash
kubectl get crd | rg 'monitoring.coreos.com'
```

| CRD | 作用 |
|---|---|
| `servicemonitors.monitoring.coreos.com` | 根据 Service 发现 Metrics 端点 |
| `podmonitors.monitoring.coreos.com` | 根据 Pod 发现 Metrics 端点 |
| `prometheuses.monitoring.coreos.com` | 声明 Prometheus 实例 |
| `alertmanagers.monitoring.coreos.com` | 声明 Alertmanager 实例 |
| `prometheusrules.monitoring.coreos.com` | 声明告警规则和 recording rule |
| `probes.monitoring.coreos.com` | 配置探针监控 |
| `scrapeconfigs.monitoring.coreos.com` | 扩展 Prometheus 抓取配置 |

第 2 步已提交：

```text
feat(opentelemetry): stage 7 step 2 deploy prometheus stack
```

## 第 3 步：接入 Ory 和 Istio Metrics

### 应用采集声明

```bash
kubectl apply -f deployments/opentelemetry/003_k3s_observability/servicemonitors/ory-istio.yaml
```

本步骤创建 3 个 `ServiceMonitor` 和 1 个 `PodMonitor`，对象都位于
`observability` namespace，但通过 `namespaceSelector` 选择
`ddd-learn-proxyless` 中的目标：

| 对象 | 发现方式 | 目标 | 端口/路径 | 采集内容 |
|---|---|---|---|---|
| `ServiceMonitor/kratos` | Service 标签 | `kratos-admin` | Service 端口 `http` / `/metrics` | Kratos 指标；admin 端口同时提供管理 API 和 metrics |
| `ServiceMonitor/keto` | Service 标签 | `keto-metrics` | `http-metrics` / `/metrics` | Keto 指标 |
| `ServiceMonitor/oathkeeper` | Service 标签 | `oathkeeper-metrics` | Service 端口 `http` / `/metrics` | Oathkeeper 指标；进程实际监听 9000 |
| `PodMonitor/istio-ingress` | Pod 标签 | Istio Ingress Pod | 容器端口 `metrics`（15020）/ `/stats/prometheus` | Envoy 数据面指标 |

这里的 `ServiceMonitor` 不会把请求代理到 Ory 的业务端口，也不会创建新的业务
Service。Prometheus Operator 根据它生成 scrape target，Prometheus 再从目标 Pod
发起 HTTP 请求。`PodMonitor` 直接从 Pod 发现地址，是因为当前
`istio-ingress-istio` Service 没有暴露 Envoy 的 15020 端口。

### 为什么没有接入 xhs_grpc 和 social_grpc

当前两个业务 Pod 没有暴露 Prometheus `/metrics` endpoint，也没有对应的 metrics
端口或 ServiceMonitor。因此本步骤不修改业务代码；它们的应用指标需要后续在
`serverhertz` 中启用并暴露独立 metrics 端口后，再增加 `ServiceMonitor`。

### 验收

```bash
kubectl get servicemonitor,podmonitor -n observability
kubectl describe servicemonitor -n observability kratos keto oathkeeper
kubectl describe podmonitor -n observability istio-ingress
```

Prometheus Service 使用 NodePort `30090`，浏览器打开
`http://192.168.2.41:30090/targets`，应能看到 Kratos、Keto、Oathkeeper
和 Istio Ingress 的采集目标；PromQL 可先查询：

```promql
up{namespace="ddd-learn-proxyless"}
```

本步骤提交：

```text
feat(opentelemetry): stage 7 step 3 scrape ory and application metrics
```
```
