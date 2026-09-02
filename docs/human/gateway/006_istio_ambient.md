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

## 完成标准

能够解释 Cilium 负责网络和 NetworkPolicy，Istio 负责服务身份、mTLS、服务级授权和东西流量治理；理解 Ambient 中 ztunnel 与 waypoint 的分工。
