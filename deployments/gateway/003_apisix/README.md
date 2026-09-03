# gateway/003：APISIX 当前实验

## 第 2 步：创建 `/v1/xhs` Route 和 Upstream

本步骤基于已提交的 APISIX 基线，通过 Admin API 创建 `/v1/xhs/*` Route 和
`xhs-api` Upstream。Route 只匹配 `192.168.2.41`、`GET/POST/PUT`，Upstream 使用
两个 `xhs_service` 实例进行 round-robin，并通过 `/health` 主动健康检查。

## 当前拓扑

```text
APISIX host :8080 → container :9080 → xhs-api Upstream → xhs_service / xhs_service_2 :8082
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

检查 APISIX Admin API 和动态对象：

```shell
curl -H 'X-API-KEY: edd1c657-da07-4c75-bf47-9b6f4a4e8c12' \
  http://192.168.2.41:9180/apisix/admin/routes
```

启动时 `apisix-seed` 会幂等写入 Upstream `1` 和 Route `1`；配置实际保存于 etcd，
不是写入 APISIX 容器本地文件。

验证匹配规则：

```shell
curl -i http://192.168.2.41:8080/v1/xhs/organizations/G/crawl/contents
curl -i -X DELETE http://192.168.2.41:8080/v1/xhs/organizations/G/crawl/contents
curl -i -H 'Host: other.example.com' \
  http://192.168.2.41:8080/v1/xhs/organizations/G/crawl/contents
```

第一条请求会到达 `xhs_service` 并因缺少 Internal JWT 返回 `401`；后两条不匹配
Route，返回 APISIX 的 `404 Route Not Found`。两个 backend 的健康状态可以通过
停止其中一个容器后查询 Admin API 中的 Upstream 观察。

## 配置边界

- APISIX 动态配置保存于 etcd，不使用 Compose labels；
- APISIX 启动配置使用可写挂载，因为启动过程会回写本地节点信息；
- `9180` 是管理面，不能作为业务入口；
- `8080` 是宿主机数据面入口，容器内对应 APISIX `9080`；
- etcd 的 `2379` 仅为教学调试暴露，生产环境应限制为 APISIX 管理网络。
