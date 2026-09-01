## 目标

加入 Ory Oathkeeper，为内部服务生成 Internal JWT。先复制
`deployments/001_auth` 作为本模块的部署基线，再在独立目录中增加 Oathkeeper
和鉴权配置。使用现有的 `xhs_service` 作为受保护的业务服务；业务接口只返回
Mock 数据，不实现真实抓取。

## 步骤

1. 从 `deployments/001_auth` 复制基础配置，建立独立部署目录，明确 Oathkeeper、Traefik、Kratos 与 `xhs_service` 的边界，并固定 Mock 接口契约。
2. 准备 Internal JWT 的 JWKS 密钥。
3. 配置 Oathkeeper 使用 Kratos Session，并通过 `id_token` Mutator 签发 Internal JWT。
4. 将 Traefik 接入 Oathkeeper Decision API。
5. 让 `xhs_service` 验证 Internal JWT，并为三个业务能力返回 Mock 数据。
6. 验证三个接口的成功、未认证、无效 JWT 和权限失败请求。
7. 补充 Oathkeeper、Traefik、Kratos、`xhs_service` 之间的完整请求文档。

本模块使用的业务接口：

- `POST /v1/xhs/organizations/:organization_id/crawl/tasks`：启动抓取任务；
- `GET /v1/xhs/organizations/:organization_id/crawl/contents`：查看抓取内容；
- `PUT /v1/xhs/organizations/:organization_id/crawl/keywords`：修改抓取关键词。
