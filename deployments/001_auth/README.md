# 001_auth：基础认证

本目录是基础认证模块的独立部署目录，目标是使用 Ory Kratos 和 Ory Elements
实现用户注册、登录、注销、Session 查询、密码恢复和密码修改。

## 本模块的边界

```text
Browser
   │
   ├── Account UI（Ory Elements）
   │       │
   │       └── Kratos Public API :4433
   │
   └── Kratos Session Cookie

Kratos ──> PostgreSQL
```

本阶段明确不部署以下组件：

- Gateway、反向代理和 Oathkeeper；
- Hydra；
- Keto、OpenFGA 和 OPA；
- 业务服务。

前端直接调用 Kratos Public API。Kratos Admin API 只供可信的运维或后端操作，
不能暴露给浏览器。

## 目录约定

后续步骤将在本目录中逐步增加以下内容：

```text
deployments/001_auth/
├── README.md
├── docker-compose.yml
├── kratos/
│   ├── kratos.yml
│   └── identity.schema.json
└── web/
    └── ...
```

现有的 `deployments/docker-compose` 是认证鉴权综合示例，不属于本模块，暂不
修改或复用其部署编排。
