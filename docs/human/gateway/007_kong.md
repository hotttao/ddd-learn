# Kong：API Gateway / API Management

## 目标

用最小实验对比 Kong 与 APISIX 的 API Gateway 对象模型，重点理解 Service、Route、Upstream、Consumer 和 Plugin。

## 步骤

1. 创建 `deployments/gateway/007_kong`，启动 Kong OSS、配置数据库和 `xhs_service` backend。
2. 创建 Kong Service 和 Route，验证 `/v1/xhs` 的路径与方法匹配。
3. 创建 Upstream/Target 或两个 Service 实例，验证负载均衡和健康状态。
4. 添加 Rate Limiting 或 CORS Plugin，确认 Plugin 可以绑定到 Route、Service 或 Consumer。
5. 创建 Alice Consumer 和 Credential，验证认证请求与 anonymous 请求的差异。
6. 将 Kong 放到当前 Oathkeeper 前面或后面做一次组合实验，明确只能保留一个 Internal JWT 签发职责。
7. 访问 `xhs_service` 的三个接口，确认 Keto 仍是业务授权来源，Kong 不替代组织/角色权限判断。
8. 对比 Kong 与 APISIX 的对象模型、插件模型和 API Management 侧重点，补充选型结论。

## 完成标准

能够解释 Kong Service 与 Route 的区别、Consumer 为什么存在、Plugin 解决什么问题，以及 Kong 与 Traefik 的定位差异。
