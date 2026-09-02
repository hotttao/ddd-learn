# Higress：API、流量与 AI Gateway

## 目标

通过最小实验理解 Higress 如何组合 Envoy、Istio 能力、API Gateway、Wasm Plugin 和 AI Gateway 能力。

## 步骤

1. 创建 `deployments/gateway/008_higress`，部署 Higress 并接入现有 `xhs_service` backend。
2. 创建 HTTP Route，验证 Gateway 到 `xhs_service` 的基本转发和配置入口。
3. 选择 JWT、Rate Limit 或 CORS 之一作为 Plugin，观察 Plugin 对请求的影响。
4. 部署 v1/v2 backend，使用权重或 Header `x-canary=true` 实现灰度路由。
5. 将当前 `/v1/xhs` 接入 Higress，确认认证、Internal JWT 和 Keto `403` 行为保持一致。
6. 只阅读并记录 Wasm Plugin 的扩展边界，不在本模块开发自定义 Wasm Plugin。
7. 使用 mock LLM endpoint 演示模型路由、Token 限流、Fallback 和 API Key 管理；不连接真实外部模型。
8. 对比 Higress 与 Istio 的关系、与 APISIX/Kong 的 API Management 差异，记录适用场景。

## 完成标准

能够说明 Higress 为什么同时具备 Traffic Gateway、API Gateway 和 AI Gateway 属性，以及这些能力分别位于哪一层。
