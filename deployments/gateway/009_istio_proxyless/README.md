# Gateway 009：Istio Proxyless

本实验从 `deployments/gateway/006_istio_ambient` 选择性继承可运行基线，后续逐步将 XHS 和
Social 改造成 Kitex Proxyless xDS 服务。

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
| `base/` | Ambient namespace、测试 workload、Mailpit 和 UI |
| `ingress/` | Istio Ingress、HTTPRoute 和 XHS Waypoint |
| `postgres/` | CloudNativePG Cluster |
| `keto/`、`seed/`、`values/` | Ory、数据库初始化和 Helm values |
| `security/` | Oathkeeper ext_authz、XHS 授权和 Istio Mesh 配置 |

没有复制 006 的 `traffic/`，因为其中的 v1/v2 灰度、Waypoint Retry 和旧故障注入不属于
本实验基线。

### 探针配置

目标服务通过 `probe.yaml` 配置：

```yaml
TARGET_SERVICE: xhs-service.ddd-learn.svc.cluster.local
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
kubectl logs -n ddd-learn pod/kitex-xds-compatibility
```

### 判断结果

成功时日志会分别输出：

```text
NDS target service=xhs-service.ddd-learn.svc.cluster.local ips=[...]
CDS resource names=[outbound|80||xhs-service..., outbound|80|v1|xhs-service..., ...]
EDS resource names=[outbound|80||xhs-service..., ...]
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
  --namespace ddd-learn --create-namespace \
  --values deployments/gateway/009_istio_proxyless/values/xhs-grpc.yaml
```

验证资源和 gRPC Probe：

```bash
kubectl -n ddd-learn get pods -l 'app.kubernetes.io/instance=xhs-grpc'
kubectl -n ddd-learn get service xhs-grpc-service
kubectl -n ddd-learn get endpointslice \
  -l kubernetes.io/service-name=xhs-grpc-service -o wide
kubectl -n ddd-learn describe pod -l 'app.kubernetes.io/instance=xhs-grpc'
```

`xhs-grpc-service` 没有绑定 Waypoint，但 Pod 仍处于 `ddd-learn` 的 Ambient namespace 中，
东西向流量继续由 ztunnel 接管。当前步骤不修改旧 XHS 的 HTTPRoute，HTTP/JSON 转码留到
下一步处理。

## 第五步：生成 XHS gRPC-JSON Transcoder descriptor

本步骤只生成 descriptor，不创建 `EnvoyFilter`，也不修改现有 HTTPRoute。
descriptor 是 Protobuf 的 `FileDescriptorSet` 二进制文件，包含 XHS 的 service、RPC、message
以及 `google.api.http` 注解。Envoy 的 gRPC-JSON Transcoder 通过它建立 HTTP/JSON 请求和
gRPC 方法之间的映射。

在 `xhs_grpc` 目录执行：

```bash
cd xhs_grpc
make descriptor
```

生成文件：

```text
deployments/gateway/009_istio_proxyless/transcoder/xhs.pb
```

生成命令使用 `--include_imports`，因此 descriptor 内同时包含 XHS IDL 和
`google/api/annotations.proto` 等依赖；使用 `--include_source_info` 便于调试和定位 IDL
来源。后续 Gateway 配置直接引用该文件，不需要在运行时读取源码或重新执行 `protoc`。
