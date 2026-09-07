# Gateway 009：Istio Proxyless

本目录从 `deployments/gateway/006_istio_ambient` 选择性继承当前可运行架构。本实验只记录
Proxyless 实验新增或改变的内容，不复制 006 的历史操作说明。

## 第一步：实验基线与兼容性门禁

### 继承的基线

009 从 006 原样复制以下目录中的基础资源：

| 目录 | 继承内容 |
| --- | --- |
| `base` | Ambient namespace、测试 workload、Mailpit 和 UI |
| `ingress` | Istio Ingress、现有 HTTPRoute 和当前 XHS Waypoint |
| `postgres` | CloudNativePG Cluster |
| `keto` | Keto namespace 配置 |
| `security` | Oathkeeper ext_authz、XHS 授权策略和 Istio mesh config |
| `seed` | Ory 初始化 Job |
| `values` | Keto、Kratos、Oathkeeper 和 XHS Helm values |

没有复制 006 的 `traffic` 目录，因为 backend-v2、90/10 灰度和旧的故障注入不是 009 的
Proxyless 基线。当前集群中已经存在的 006 流量实验资源暂不在本步骤删除，后续切换 XHS gRPC
部署时再按明确清单清理。

### 当前版本

| 组件 | 版本 |
| --- | --- |
| k3s / Kubernetes Server | `v1.36.2+k3s1` |
| kubectl Client | `v1.34.9` |
| Istio | `1.31.0` |
| `kitex-contrib/xds` | `v0.4.2` |
| `github.com/cloudwego/kitex` | `v0.11.3`，由 xDS v0.4.2 约束 |

`kitex-contrib/xds` 的公开说明只记录测试过 Istio 1.13.3，因此不能仅凭版本号假设它与当前
Istio 兼容。

### 探针检查什么

`compatibility` 不是业务服务，也不监听端口。它使用 `kitex-contrib/xds` 自带的 Kitex ADS
Client 连接 `istiod.istio-system.svc:15010`：

```text
请求 NDS、LDS、CDS
        │
        ├── 从 LDS 提取 RouteConfiguration 名称，再请求 RDS
        └── 从 CDS 提取 EDS ServiceName，再请求 EDS
        │
        ▼
使用 kitex-contrib/xds 的 UnmarshalNDS/LDS/RDS/CDS/EDS 解析
        │
        ▼
逐类发送 ACK
```

RDS 和 EDS 是按需资源。对它们发送空名称的通配请求时，当前 istiod 不返回响应，所以探针必须
先从 LDS/CDS 建立依赖关系，再精确订阅。

探针使用 15010 明文 xDS 端口且不挂载 ServiceAccount Token。这只用于当前单集群兼容性实验；
生产接入应使用 15012、ServiceAccount Token 和 TLS 验证。

### 构建和运行

探针是独立 Go Module，构建时关闭仓库根 `go.work`：

```bash
cd deployments/gateway/009_istio_proxyless/compatibility
GOWORK=off GOCACHE=/tmp/ddd-learn-kitex-xds-gocache go mod tidy
GOWORK=off GOCACHE=/tmp/ddd-learn-kitex-xds-gocache \
  CGO_ENABLED=0 GOOS=linux \
  go build -o .build/kitex-xds-compatibility .
docker build -t ddd-learn-kitex-xds-compatibility:0.0.1 .
docker save -o /tmp/ddd-learn-kitex-xds-compatibility-0.0.1.tar \
  ddd-learn-kitex-xds-compatibility:0.0.1
sudo k3s ctr images import /tmp/ddd-learn-kitex-xds-compatibility-0.0.1.tar
kubectl apply -f deployments/gateway/009_istio_proxyless/compatibility/probe.yaml
kubectl logs -n ddd-learn pod/kitex-xds-compatibility
```

若执行环境不能访问 k3s containerd，可以把静态二进制临时复制到已有测试 Pod 的 `/tmp`，只复用
Pod 网络完成同一检查；该方式不作为正式部署流程。

### 实际结果

2026-09-07 在当前集群得到：

```text
compatible type=NameTable resources=1
compatible type=Listener resources=17
compatible type=Cluster resources=47
requesting on-demand type=RouteConfiguration resources=13
requesting on-demand type=ClusterLoadAssignment resources=46
compatible type=RouteConfiguration resources=13
compatible type=ClusterLoadAssignment resources=46
compatibility gate passed
```

同时检查 istiod 日志，ADS 连接、NDS/LDS/CDS Push 均成功，没有出现 NACK 或资源解析错误。

结论：当前 Istio 1.31.0 仍能向 `kitex-contrib/xds v0.4.2` 的 Kitex ADS Client 下发该库能够
解析的 NDS/LDS/RDS/CDS/EDS，第一步兼容性门禁通过。该结果只证明基础资源传输和解析兼容；
具体 Router、Resolver 和 Circuit Breaker 的运行时行为仍将在后续步骤分别验证。

- `base/`：Ambient namespace、UI、Mailpit 和连通性测试 workload；
- `ingress/`：Istio Ingress、HTTPRoute 和当前 XHS Waypoint；
- `postgres/`、`keto/`、`seed/`、`values/`：Ory、数据库、初始化数据和服务 values；
- `security/`：Oathkeeper ext_authz、XHS 授权和 Istio Mesh 配置。

没有复制 006 的 `traffic/`：v1/v2 灰度、Waypoint Retry 和故障注入属于上一个实验，不是
Proxyless 基线。

### 当前版本

| 组件 | 版本 |
| --- | --- |
| Kubernetes Server | `v1.36.2+k3s1` |
| Istio | `1.31.0` |
| Go | `1.26.3` |
| Kitex | `0.11.3`（由 `kitex-contrib/xds` v0.4.2 依赖） |
| kitex-contrib/xds | `v0.4.2`，提交 `c9100af` |

`kitex-contrib/xds` 的公开说明只验证到较旧的 Istio 版本，因此不能直接开始改造业务服务。
`compatibility/` 使用该项目自带的 Kitex ADS Client 连接当前 istiod，并用当前库分别解析：

- Istio NameTable（NDS）；
- Listener（LDS）；
- RouteConfiguration（RDS）；
- Cluster（CDS）；
- ClusterLoadAssignment（EDS）。

### 构建与运行兼容性探针

```bash
cd deployments/gateway/009_istio_proxyless/compatibility
go mod tidy
mkdir -p .build
CGO_ENABLED=0 GOOS=linux go build -o .build/kitex-xds-compatibility .
docker build -t ddd-learn-kitex-xds-compatibility:0.0.1 .
docker save ddd-learn-kitex-xds-compatibility:0.0.1 \
  | sudo k3s ctr images import -
kubectl apply -f probe.yaml
kubectl logs -n ddd-learn pod/kitex-xds-compatibility
```

探针只有在 ADS 建连成功、五类响应均能由 `kitex-contrib/xds` 解码时才返回成功。失败日志会明确
停留在连接、接收或哪一种 Resource 解码阶段。
