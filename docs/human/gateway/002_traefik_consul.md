# Traefik + Consul：多机 Docker

## 目标

把单机 Docker Provider 替换成 Consul Catalog，理解多台 Docker 主机之间的服务注册、健康检查和 Gateway 服务发现。

## 步骤

1. 创建 `deployments/gateway/002_traefik_consul`，启动 Consul Server、Traefik 和可注册的 `xhs_service` backend。
2. 约定服务注册名称、端口、健康检查 URL 和 Traefik tags，验证服务出现在 Consul Catalog。
3. 配置 Traefik Consul Catalog Provider，把服务 tags 转换成 Router、Middleware 和 Service。
4. 将 backend 放到两个模拟主机或两个独立注册实例，验证 Traefik 从 Catalog 获取实例而不是读取 Docker Socket。
5. 停止或标记一个实例 unhealthy，观察 Consul 和 Traefik 的状态变化及流量结果。
6. 让 `xhs_service` 通过 Consul DNS 直连另一个内部服务，比较东西流量直连和经过 Gateway 的差异。
7. 仅把需要统一认证、限流的内部 API 暴露给 Traefik，验证不会强制所有服务间调用绕行 Gateway。
8. 补充服务注册、健康检查、路由生成和故障摘除的完整流程文档。

## 完成标准

能够解释为什么多机 Docker 不能只依赖单机 Docker Provider，以及 Traefik、Consul、服务本身分别负责什么。
