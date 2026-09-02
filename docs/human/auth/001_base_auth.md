## 目标
使用 ory elements + ory kratos 实现基本的注册/登录/注销/重置密码等功能。

Account UI 直接消费 Kratos Public API，不增加自定义认证后端或 BFF。开发环境通过 Vite 的同源代理访问 `/kratos`，再由 Traefik 转发到 Kratos。部署和配置统一放在 `deployments/auth/001_auth`。

本模块的运行组件包括 PostgreSQL、Kratos API、Kratos Courier Worker、Traefik 和 Account UI；Mailpit 只作为教学环境的 SMTP 接收器，不属于生产认证架构。

## 步骤

1. 建立 `deployments/auth/001_auth`，明确本模块的认证服务、数据存储和 Account UI 边界。
2. 准备 Kratos 配置，包括 DSN、Public/Admin API、Session、密码认证和 Identity Schema。
3. 准备 PostgreSQL 与 Kratos 数据库迁移，并验证 Kratos 可以启动。
4. 使用 Ory Elements 创建 Account UI，实现注册和登录。
5. 实现注销和当前 Session 查询。
6. 添加独立 Kratos Courier Worker 和 Mailpit，验证教学环境的异步邮件投递基础设施。
7. 使用 Ory Elements 实现 Recovery Flow，完成邮箱验证码挑战，并通过 Recovery 授权的 Settings Flow 设置新密码。
8. 完善 Settings Flow 入口，允许已登录用户主动修改密码。
9. 验证注册、登录、注销、Session 查询、密码恢复和密码修改流程，补充本模块文档。
