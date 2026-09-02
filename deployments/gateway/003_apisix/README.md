# gateway/003：APISIX 当前实验

## 第 1 步：创建并启动 APISIX 基线

本步骤基于已提交的 auth/003 基线，将入口替换为 APISIX，并使用 etcd 保存 APISIX
动态配置。本步骤只启动 APISIX、etcd 和现有认证/业务依赖，暂不创建 APISIX
Route、Plugin 或 Consumer。

## 当前拓扑

```text
APISIX host :8080 → container :9080 ──待配置──> xhs_service :8082
APISIX Admin :9180 ────────────────────────> APISIX 配置
etcd :2379 ──────────────────────────────> Route/Upstream/Plugin/Consumer 存储
```

认证链路仍沿用 auth/003：Kratos、Oathkeeper 和 Keto 不由 APISIX 重复实现。
`xhs_service` 继续通过 Oathkeeper 签发的 Internal JWT 和 Keto 完成业务认证鉴权。

## 启动

```shell
docker compose -f deployments/gateway/003_apisix/docker-compose.yml up -d
```

检查 Compose 配置：

```shell
docker compose -f deployments/gateway/003_apisix/docker-compose.yml config --quiet
```

检查 APISIX Admin API：

```shell
curl -H 'X-API-KEY: edd1c657-da07-4c75-bf47-9b6f4a4e8c12' \
  http://192.168.2.41:9180/apisix/admin/routes
```

本步骤尚未创建 Route，因此 `8080` 不承担业务转发；Admin API 应能返回空的
Route 列表或 APISIX 的标准响应。后续步骤通过 Admin API 创建 `/v1/xhs` Route。

## 配置边界

- APISIX 动态配置保存于 etcd，不使用 Compose labels；
- APISIX 启动配置使用可写挂载，因为启动过程会回写本地节点信息；
- `9180` 是管理面，不能作为业务入口；
- `8080` 是宿主机数据面入口，容器内对应 APISIX `9080`；
- etcd 的 `2379` 仅为教学调试暴露，生产环境应限制为 APISIX 管理网络。
