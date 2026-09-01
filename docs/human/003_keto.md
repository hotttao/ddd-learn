## 目标



```md
组织 G 有两名用户：

Alice：管理员
Bob：普通成员

普通成员可以：
- 启动抓取任务
- 查看抓取内容

管理员还可以：
- 修改抓取关键词
```

接入 ory keto 实现上述权限管理模型。完善 xhs_service 服务，对请求进行权限校验。

## 执行步骤

1. 定义 Keto 的 Namespace、Relation 和 Permission，并固定用户与组织的标识格式。
2. 创建 `deployments/003_keto` 独立部署，增加 Keto、Keto 数据库和迁移流程。
3. 初始化 Alice、Bob 与组织 G 的 Keto Relation Tuple。
4. 将 `xhs_service` 的业务接口接入 Keto 权限校验。
5. 增加当前用户所属组织接口，UI 登录后自动选择组织，不把组织和权限写入 Internal JWT。
6. 使用 Alice、Bob 验证权限矩阵，并补充请求流程文档。

## 第 1 步：权限模型

本步骤固定以下模型，后续步骤不得自行引入另一套角色或权限名称：

| 类型 | 定义 |
| --- | --- |
| User Namespace | `User`，主体 ID 使用 Kratos 的 `identity.id` |
| Organization Namespace | `Organization`，组织 G 的 object ID 为 `G` |
| 管理员关系 | `Organization#admins@User:<identity-id>` |
| 普通成员关系 | `Organization#members@User:<identity-id>` |
| 启动抓取权限 | `start_crawl`，管理员和普通成员都拥有 |
| 查看抓取内容权限 | `view_content`，管理员和普通成员都拥有 |
| 修改抓取关键词权限 | `modify_keywords`，仅管理员拥有 |

Relation 表示用户与组织之间的事实关系，Permission 由 OPL 根据 Relation 计算得出。
UI 通过 `xhs_service` 获取当前用户的组织和角色，不直接访问 Keto；业务接口仍然
必须独立检查对应 Permission。
