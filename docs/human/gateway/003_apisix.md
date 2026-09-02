# APISIX：API Gateway

## 目标

使用 APISIX 学习 `Route → Plugin → Upstream`，并理解 `Service`、`Consumer` 与 API Management 的关系。

## 步骤

1. 创建 `deployments/gateway/003_apisix`，启动 APISIX、配置存储和 `xhs_service` backend。
2. 创建 `/v1/xhs` Route 与 Upstream，验证路径、方法、Host 匹配和负载均衡。
3. 通过 APISIX Admin API 配置一个 rate-limit 或 CORS Plugin，观察插件执行顺序和响应头。
4. 配置 JWT 或外部认证 Plugin；先验证独立实验，再决定如何与当前 Oathkeeper 链路组合，不能同时让两个组件重复签发身份。
5. 创建 `alice` Consumer 和 Credential，验证 Alice 与 unknown consumer 的请求差异。
6. 将 `xhs_service` 接入 APISIX，验证 Alice/Bob 的 Keto `200/403` 结果不被 Gateway 错误改写。
7. 使用 Consul Discovery 配置动态 Upstream，比较 APISIX 与 Traefik Consul Catalog 的服务发现模型。
8. 记录 Route、Plugin、Upstream、Consumer 的职责边界，并完成失败请求和配置变更验证。

## 完成标准

能够解释 APISIX 为什么不只是“把请求转发到服务”，以及 Consumer/API Key/Plugin 解决了什么 API 管理问题。
