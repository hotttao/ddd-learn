# Gateway 学习基线

## 目标

基于已经完成的 auth 架构，分别接入不同 Gateway，理解它们在南北流量、服务发现、流量治理、认证透传和东西流量中的职责差异。网关可以使用文件配置、容器标签、控制面 API 或 Kubernetes 资源，具体方式以当前网关和实验目标为准。

本目录只负责 Gateway 实验。Kratos、Oathkeeper、Keto 和 `xhs_service` 的业务语义不重新实现。

## 统一业务契约

所有 Gateway 实验都使用当前 `xhs_service` 的三个接口：

- `POST /v1/xhs/organizations/G/crawl/tasks`：启动抓取任务；
- `GET /v1/xhs/organizations/G/crawl/contents`：查看抓取内容；
- `PUT /v1/xhs/organizations/G/crawl/keywords`：修改抓取关键词。

认证和鉴权结果保持不变：Alice 可以执行三个操作，Bob 只能启动抓取和查看内容。

## 统一观察点

每个 Gateway 至少验证以下内容：

1. 请求能否根据 Host、Path 或 Gateway API Route 到达 `xhs_service`；
2. 认证请求是否仍然经过 Kratos/Oathkeeper；
3. Internal JWT 是否仍能被 `xhs_service` 验证；
4. Keto 返回的 `403` 是否能原样表达为业务拒绝；
5. 后端实例变化、请求失败时 Gateway 如何更新或处理；
6. Gateway 配置属于数据面、控制面还是业务服务。

## 部署目录约定

每个需要独立运行环境的实验使用独立目录，避免不同 Gateway 的配置互相覆盖：

```text
deployments/gateway/<step>_<gateway>/
```

实验可以复制当前 auth/keto 部署中的必要配置，并优先复用已经验证的 Kratos、Oathkeeper、Keto、`xhs_service` 和 UI 契约。只有确实需要隔离状态时才新建数据库或 Volume。每个步骤完成后先解释和验证，再单独提交。

## 学习顺序

1. `001_traefik_docker.md`：单机 Docker 的动态发现和基础流量入口；
2. `002_traefik_consul.md`：多机 Docker 的服务发现；
3. `003_apisix.md`：API Gateway 的 Route、Plugin、Consumer；
4. `004_envoy_gateway.md`：Kubernetes Gateway API 和 Envoy 数据面；
5. `005_cilium_gateway.md`：CNI、Gateway API、NetworkPolicy、Hubble；
6. `006_istio_ambient.md`：服务间身份、mTLS 和 L7 流量治理；
7. `007_kong.md`：另一种 API Gateway/API Management 模型；
8. `008_higress.md`：Envoy、Wasm、API Gateway 和 AI Gateway 的组合。
