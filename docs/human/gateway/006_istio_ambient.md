# Istio Ambient：Service Mesh

## 目标

使用 Istio Ambient 学习东西流量治理、workload identity、mTLS、AuthorizationPolicy 和 Traffic Management，不把 Istio 仅当作入口 Gateway。

## 步骤

1. 在 Cilium 或普通 CNI 的 Kubernetes 集群安装 Istio Ambient，将现有 `xhs_service` 作为 backend，并准备 frontend、other 测试 workload。
2. 将 namespace 加入 ambient mesh，验证 frontend → `xhs_service` 仍然可用，业务代码不增加 TLS 配置。
3. 查看 workload identity，确认服务通过 Service Account/工作负载身份而不是 Pod IP 被识别。
4. 验证 ztunnel 建立服务间 mTLS，并观察证书和身份由 Istio 管理。
5. 配置 AuthorizationPolicy，只允许 frontend 访问 `xhs_service`，验证 other 被拒绝。
6. 为需要 HTTP 路由、Retry 或超时的场景增加 waypoint，明确 ztunnel 的 L4 能力和 waypoint 的 L7 能力。
7. 部署 backend-v1/backend-v2，使用 VirtualService 或 Gateway API 做 90/10 灰度，再验证超时或 Retry。
8. 将 Istio Ingress Gateway 与当前 `/v1/xhs` 入口组合，确认入口认证和服务间 mTLS 是两层不同职责。

## 第七步拆解：灰度发布与故障处理

第七步拆分为以下三个小步骤，每一步都使用声明式 Kubernetes/Istio 资源，并在完成后单独验证。

### 7.1 准备两个后端版本

1. 将现有 Helm 管理的 `xhs-service` Deployment 标记为 `version: v1`。
2. 使用相同的 `xhs_service:0.0.1` 镜像创建 `backend-v2` Deployment，并标记为 `version: v2`。
3. 让两个 Deployment 使用相同的 Service 选择器和 ServiceAccount，确保它们属于同一个逻辑后端服务。
4. 检查两个版本的 Pod 均通过 `/health` 就绪检查，并确认 `xhs-service` 的 EndpointSlice 同时包含 v1、v2 的 Pod。

本小步骤只准备版本和服务端点，不改变流量比例；完成后停止，确认两个版本的工作负载已正常运行。

### 7.2 配置 90/10 灰度

1. 创建 `DestinationRule`，以 `version: v1` 和 `version: v2` 标签定义两个 subset。
2. 创建 `VirtualService`，将 `xhs-service` 的请求按 90%/10% 分发到 v1/v2。
3. 检查 `DestinationRule`、`VirtualService` 的状态和 waypoint 的路由配置，确认规则被接受并绑定到正确的服务。
4. 从现有 frontend workload 连续请求 `xhs-service`，结合 waypoint/Envoy 的路由统计验证两个 subset 都能收到流量。

本小步骤只验证流量分配，不引入超时或 Retry 参数。

### 7.3 增加超时与 Retry 并验证

1. 在 `VirtualService` 上增加请求总超时时间、单次尝试超时时间、最大重试次数和可重试的网络错误类型。
2. 使用 Istio 的测试性故障注入制造延迟或临时失败，不修改 `xhs_service` 业务代码。
3. 验证请求超时后按配置停止等待，连接失败或指定的上游错误会按配置重试，非可重试错误不会无限重试。
4. 通过 waypoint 日志、路由统计和客户端响应，记录超时、重试前后的现象。
5. 清理仅用于演示故障注入的配置，保留灰度、超时和 Retry 的正式配置。

## 第八步拆解：入口 Gateway 与服务间 mTLS

1. 确认当前 Envoy Gateway 的 `public-gateway` 只负责外部 HTTP 入口，不承担服务间 mTLS。
2. 将 `/v1/xhs` 的入口 Route 指向 `xhs-service`，确认入口请求经过 Gateway 后再进入 ambient mesh。
3. 验证 Gateway 到 xhs waypoint/ztunnel、以及 mesh 内服务到服务之间分别由对应的 Istio 数据面建立 mTLS。
4. 对比入口认证、Gateway 路由、waypoint 的 L7 策略和 ztunnel 的 L4/mTLS 职责，记录一次完整请求链路。

## 完成标准

能够解释 Cilium 负责网络和 NetworkPolicy，Istio 负责服务身份、mTLS、服务级授权和东西流量治理；理解 Ambient 中 ztunnel 与 waypoint 的分工。
