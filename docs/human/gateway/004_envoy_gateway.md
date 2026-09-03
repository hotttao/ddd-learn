# Envoy Gateway：Kubernetes Gateway API

## 目标

在当前服务器已有的 k3s 集群中使用 Envoy Gateway，把当前认证和业务服务接入
Kubernetes Gateway API。实验不创建 kind 集群，也不复制之前 Docker Gateway 实验的部署内容。

本实验所有资源统一放在 `ddd-learn` namespace，包括独立 PostgreSQL、Keto、Kratos、
Oathkeeper、`xhs_service`、UI、Gateway API 资源和 Envoy Gateway 控制面/数据面。
PostgreSQL 使用本实验自己的实例和数据卷，不复用之前 auth、APISIX 或 Traefik 实验的
数据库和 Volume。

Gateway API 的 CRD 和 `GatewayClass` 是 Kubernetes 集群级资源，不能放入 namespace；
它们作为 `ddd-learn` 中 Gateway/HTTPRoute 的集群级类型和控制器入口保留在集群范围内。

已有的 `deployments/gateway/helm` 是本实验使用的 Ory Helm Chart 来源；本实验只在
`deployments/gateway/004_envoy_gateway` 保存当前 k3s 环境的 values、应用 Chart 和
Gateway API 资源。

## 实施步骤

### 第 1 步：定制并部署 Ory 认证基础设施

1. 创建并检查 `ddd-learn` namespace、StorageClass、Ingress/LoadBalancer 能力，以及
   `deployments/gateway/helm/{keto,kratos,oathkeeper}` Chart 的可配置项。
2. 为当前实验创建独立的 values 文件，适配现有架构：
   - PostgreSQL 使用新建的实例、数据库和数据卷；
   - Kratos 使用本实验的 PostgreSQL、现有 identity schema 和浏览器 UI 地址；
   - Oathkeeper 使用当前 Internal JWT 的 issuer、JWKS、Rule、Authenticator 和
     `id_token` Mutator；
   - Keto 使用当前组织/角色/权限模型和 PostgreSQL；
   - 服务之间使用 Kubernetes Service DNS，不再使用 Docker Compose 服务名和宿主机 IP。
3. 准备 `ddd-learn` 中的 PostgreSQL、Secret、ConfigMap 和初始化 Job/数据，使用 Helm
   安装或升级 Keto、Kratos、Oathkeeper；所有服务必须部署到 `ddd-learn`。
4. 验证三个 Ory 服务的 Pod、Service、健康检查、配置加载和相互访问；确认不依赖
   APISIX 实验的 Docker 容器或数据卷。

### 第 2 步：准备并部署 xhs_service Helm Chart

1. 完善 `deployments/gateway/helm/xhs`，使其能够部署当前 `xhs_service` 镜像、
   Deployment、Service、配置和 JWT/JWKS/Keto 环境变量。
2. 让 `xhs_service` 使用 `ddd-learn` namespace 内的 Kubernetes Service DNS 访问 Keto
   和 Oathkeeper JWKS，
   不暴露业务 Pod 的宿主机端口。
3. 部署 `xhs_service`，确认 Service、EndpointSlice、健康检查和三个业务接口正常。
4. 使用已经完成的认证流程验证 Alice/Bob 的业务权限结果仍然是 Keto 决定的
   `200/403`，不引入 Gateway 配置。

### 第 3 步：安装 Envoy Gateway

1. 在当前 k3s 集群的 `ddd-learn` namespace 安装 Envoy Gateway Controller，确认其 Deployment、Service 和
   CRD/Gateway API 资源状态正常。
2. 创建并确认 `GatewayClass`，理解它如何把 Gateway 资源交给 Envoy Gateway Controller。
3. 创建 `Gateway` 和 HTTP Listener，确定当前 k3s 的访问方式、监听端口和外部地址。

### 第 4 步：使用 Gateway API 接入当前服务

1. 创建 `HTTPRoute`，将 `/v1/xhs/*` 转发到 `xhs_service` Service。
2. 创建 UI 和 Kratos 公共 API 的路由，使浏览器通过同一个入口访问主页面和认证流程。
3. 验证 `Gateway`、`HTTPRoute`、Service、EndpointSlice 到 Envoy 数据面的完整链路。
4. 验证未登录请求、Alice 请求和 Bob 请求，确认 Envoy Gateway 不替代 Oathkeeper 的
   Session 认证、Internal JWT，也不替代 `xhs_service` 对 Keto 的业务授权。

### 第 5 步：完成一个 Gateway API 扩展实验

选择一个与当前版本兼容且边界清晰的能力进行验证，例如 Retry、限流或外部认证。
记录该能力是 Gateway API 核心字段、Envoy Gateway Policy，还是实现相关扩展；不同时
引入多个策略，避免混淆资源职责。

### 第 6 步：验证动态更新

删除一个 backend Pod、修改 Service 或调整 HTTPRoute，观察 Kubernetes 控制面、Envoy
Gateway Controller 和 Envoy 数据面如何更新，并确认业务请求能够恢复或按预期变化。

## 明确跳过的内容

- 不创建 kind 集群；
- 不复制之前的 `deployments/gateway/001_traefik_docker` 或 `003_apisix`；
- 不在本实验中引入 Consul Discovery；k3s 使用 Service 和 EndpointSlice 做服务发现；
- 不复用其他实验的 PostgreSQL、数据库或数据卷；
- 不把本实验服务拆到多个 namespace；
- 不深入实现 xDS 和 Envoy filter chain。

## 完成标准

能够说明 `GatewayClass`、`Gateway`、`HTTPRoute`、Envoy Gateway 控制面、Envoy 数据面、
Kubernetes Service 和 EndpointSlice 的关系，并能解释认证与业务授权为什么仍分别由
Oathkeeper 和 `xhs_service`/Keto 负责。
