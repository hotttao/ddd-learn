# Cilium Gateway API

## 目标

在 Kubernetes 中同时观察 Cilium 的 CNI、网络策略、Hubble 和 Gateway API，区分网络层能力与入口 Gateway 能力。

## 步骤

1. 创建独立 kind 集群，安装 Cilium，确认 Cilium 成为 CNI。
2. 使用 `xhs_service` 作为 backend，并准备 frontend、other 两个测试 workload，验证它们到 `xhs_service` 的基础连通性。
3. 使用 Hubble CLI 或 UI 观察一条 frontend → `xhs_service` 流量，记录身份、方向和 verdict。
4. 添加 NetworkPolicy，只允许 frontend 访问 `xhs_service`，验证 other 被阻断，并在 Hubble 中观察 `DROPPED`。
5. 安装并启用 Cilium Gateway API，创建 GatewayClass、Gateway 和 HTTPRoute，验证外部请求进入 `xhs_service`。
6. 将 `/v1/xhs` 路由接入测试服务，确认入口路由与 Cilium NetworkPolicy 是两个不同层次的判断。
7. 删除 Policy 并修改 Gateway Route，分别验证网络策略恢复和入口路由变化。
8. 与 Envoy Gateway 对比是否需要独立 Gateway 数据面，以及 Cilium eBPF 在整个链路中的位置。

## 完成标准

能够区分：CNI 负责 Pod 网络如何建立，NetworkPolicy 负责网络连接是否允许，Gateway API 负责入口 HTTP 路由，Hubble 负责观察网络行为。
