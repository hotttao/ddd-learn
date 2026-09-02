# Envoy Gateway：Kubernetes Gateway API

## 目标

在 kind 集群中使用 Envoy Gateway，把 Kubernetes Gateway API 资源转换为 Envoy 数据面配置。

## 步骤

1. 创建独立 kind 集群和 `deployments/gateway/004_envoy_gateway`，安装 Envoy Gateway。
2. 将现有 `xhs_service` 部署为 backend Service；需要做灰度时再部署两个可区分版本的 `xhs_service` 实例，确认 Kubernetes Service 和 EndpointSlice 正常工作。
3. 创建 `GatewayClass`、`Gateway` 和 `HTTPRoute`，让 `/v1/xhs` 到达 backend。
4. 使用 `GRPCRoute` 或另一个 HTTPRoute 做一次协议/路径匹配实验，明确 Route 与 Gateway 的边界。
5. 配置 backend 权重实现 90/10 流量切分，再改成 50/50，观察版本流量。
6. 选择一个 Policy 实验：Retry、Rate Limit 或 JWT；记录 Gateway API 核心资源和扩展 Policy 的区别。
7. 将认证结果以 Header 或外部认证方式透传到业务服务，确认不会绕过当前 `xhs_service` JWT 和 Keto 校验。
8. 删除 backend Pod 或修改 Service，验证 Kubernetes 控制面如何更新 Envoy 数据面。

## 完成标准

能够说明 `GatewayClass`、`Gateway`、`HTTPRoute`、Envoy Gateway 控制面和 Envoy 数据面的关系；不要求学习 xDS 和 Envoy filter chain 细节。
