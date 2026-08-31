## 目标
使用 ory elements + ory kratos 实现基本的注册/登录/注销/重置密码等功能。

前端直接请求 ory kratos 的 api，不添加任何网关。部署的目录放在 deployments/001_auth

## 步骤

1. 建立 `deployments/001_auth`，明确本模块只包含 Kratos、PostgreSQL 和 Account UI。
2. 准备 Kratos 配置，包括 DSN、Public/Admin API、Session、密码认证和 Identity Schema。
3. 准备 PostgreSQL 与 Kratos 数据库迁移，并验证 Kratos 可以启动。
4. 使用 Ory Elements 创建 Account UI，实现注册和登录。
5. 实现注销和当前 Session 查询。
6. 实现密码恢复与密码修改，并配置教学环境所需的消息投递方式。
7. 验证注册、登录、注销、Session 查询和密码恢复流程，补充本模块文档。
