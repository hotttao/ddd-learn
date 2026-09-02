# Traefik：单机 Docker

## 目标

基于当前 auth/003 已有的 Traefik 文件配置，理解 `Provider → Router → Middleware → Service`，并补足单机 Docker 场景下的 Dashboard、负载均衡、故障摘除和流量治理实验。Docker Provider 和 labels 是可选对比项，不是本模块的硬性要求。

## 当前已完成

当前架构已经完成了 Traefik 的基础接入：

- 使用 `deployments/auth/003_keto/traefik/traefik.yml` 加载动态文件配置；
- 使用 `dynamic.yml` 配置 `/kratos` 和 `/v1/xhs` 路由；
- 通过 `oathkeeper-forward-auth` 接入 Oathkeeper Decision API；
- 将业务请求转发到 `xhs_service`，认证和 Keto 权限结果保持有效。

因此本模块不再重复迁移 auth 入口，后续步骤直接在当前架构上补充实验能力。

## 步骤

1. 复核当前 `traefik.yml`、`dynamic.yml` 和 Compose，画出现有 Router、Middleware、Service 的配置关系。
2. 在当前配置方式上开启 Dashboard，仅允许局域网访问，观察静态配置和动态配置的生效结果。
3. 使用现有 `xhs_service` 作为 backend，使用文件配置定义 `/v1/xhs` PathPrefix 路由；如有需要，再额外做一次 Docker labels Provider 对比实验。
4. 启动两个 `xhs_service` 实例，验证 Traefik 负载均衡；停止一个实例，确认配置变化或健康检查如何摘除实例。
5. 为测试路由增加 timeout 或 rate limit，制造慢请求和高频请求，记录响应、Access Log 和 Dashboard 状态。
6. 在不改变 `/kratos`、`/v1/xhs` 认证边界的前提下，验证未认证、Alice、Bob 三组请求。
7. 补充 Provider、Router、Middleware、Service 的请求链路文档，并说明文件配置与 labels 的适用差异。

## 完成标准

能够说明 Traefik Provider 从哪里读取配置，Router 如何匹配，Middleware 在哪里执行，Service 如何选择后端；并证明 Gateway 变化没有改变业务权限。能够根据场景选择文件配置或 Docker Provider，而不是把 labels 当成唯一方式。
