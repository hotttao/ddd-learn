# 权限管理后台

## 目标

管理后台支持：

- 查看、修改组织权限；
- 查看用户角色和最终权限；
- 修改用户角色；
- 管理系统超级管理员。

本阶段只支持“给用户分配角色”，不支持对单个用户逐项授权或拒绝。

## 权限数据放在哪里

| 数据 | 存放位置 | 示例 |
| --- | --- | --- |
| 服务支持的全部权限 | `xhs_service` 权限目录 | `start_crawl`、`view_content`、`modify_keywords` |
| 组织已启用的权限 | Keto `entitled_*` Relation | `entitled_view_content` |
| 角色拥有的权限 | Keto `granted_*` Relation | `granted_modify_keywords` |
| 用户所属角色 | Keto `members`、`admins` Relation | `Organization:G#admins@User:<id>` |

Keto 不能列出 OPL 中定义的全部 Permission，所以 `xhs_service` 必须维护权限目录。
浏览器只访问 `xhs_service`，不能直接访问 Keto。

## 前置：检查系统超级管理员

所有管理接口首先执行同一个检查：

```http
POST http://keto:4466/relation-tuples/check/openapi
Content-Type: application/json

{
  "namespace": "System",
  "object": "global",
  "relation": "manage_authorization",
  "subject_id": "User:<operator-id>"
}
```

| Keto 返回 | `xhs_service` 处理 |
| --- | --- |
| `{"allowed": true}` | 继续执行管理操作 |
| `{"allowed": false}` | 返回 `403` |
| Keto 请求失败 | 返回 `503` |

超级管理员使用独立关系，不加入每个组织的 `admins`：

```text
System:global#super_admins@User:<identity-id>
```

## 流程一：查看组织权限

管理页面请求：

```http
GET /v1/xhs/admin/organizations/G/permissions
```

| 步骤 | 查询 | 返回 | 下一步 |
| ---: | --- | --- | --- |
| 1 | 检查当前操作者的 `manage_authorization` | `allowed` | 允许后查询组织关系 |
| 2 | `GET /relation-tuples?namespace=Organization&object=G` | 组织 G 的全部 Tuple | 提取 `entitled_*` |
| 3 | 读取 `xhs_service` 权限目录 | 全部三个权限 | 与 `entitled_*` 对比 |
| 4 | 无 | 每项权限的 `entitled: true/false` | 返回管理页面 |

返回示例：

```json
{
  "organization_id": "G",
  "permissions": {
    "start_crawl": true,
    "view_content": true,
    "modify_keywords": false
  }
}
```

## 流程二：查看用户权限

管理页面请求：

```http
GET /v1/xhs/admin/organizations/G/users/<identity-id>/permissions
```

| 步骤 | 查询 | 返回 | 下一步 |
| ---: | --- | --- | --- |
| 1 | 检查当前操作者的 `manage_authorization` | `allowed` | 允许后查询用户角色 |
| 2 | `GET /relation-tuples?namespace=Organization&object=G&subject_id=User:<id>` | 用户的 `members` 或 `admins` Tuple | 得到角色 |
| 3 | 读取 `xhs_service` 权限目录 | 全部三个权限 | 生成 Batch Check 请求 |
| 4 | `POST /relation-tuples/batch/check` | 三个 Permission 的 `allowed` | 合并角色和结果 |
| 5 | 无 | 用户角色和最终权限 | 返回管理页面 |

返回示例：

```json
{
  "organization_id": "G",
  "user_id": "<identity-id>",
  "roles": ["members"],
  "permissions": {
    "start_crawl": true,
    "view_content": true,
    "modify_keywords": false
  }
}
```

这里返回的是最终权限：

```text
最终权限 = 组织已启用权限 ∩ 角色权限
```

## 流程三：修改组织权限

管理页面提交完整的目标权限集合：

```http
PUT /v1/xhs/admin/organizations/G/permissions

{"permissions":["start_crawl","view_content"]}
```

| 步骤 | 操作 | 结果 | 下一步 |
| ---: | --- | --- | --- |
| 1 | 检查 `manage_authorization` | `allowed` | 允许后读取当前组织权限 |
| 2 | 查询组织 G 的 `entitled_*` | 当前权限 | 与目标权限比较 |
| 3 | `PATCH http://keto:4467/admin/relation-tuples` | 写入新增和删除的 Tuple | 重新查询组织权限 |
| 4 | 执行“查看组织权限”流程 | 最新权限 | 返回管理页面 |

管理页面提交权限名称，不能直接提交 Keto Relation Tuple。

## 流程四：修改用户角色

管理页面提交完整的目标角色集合：

```http
PUT /v1/xhs/admin/organizations/G/users/<identity-id>/roles

{"roles":["admins"]}
```

| 步骤 | 操作 | 结果 | 下一步 |
| ---: | --- | --- | --- |
| 1 | 检查 `manage_authorization` | `allowed` | 允许后读取当前角色 |
| 2 | 查询用户的 `members`、`admins` Tuple | 当前角色 | 与目标角色比较 |
| 3 | 调用 Keto Write API 删除旧角色、增加新角色 | 角色更新完成 | 重新计算用户权限 |
| 4 | 执行“查看用户权限”流程 | 最新角色和最终权限 | 返回管理页面 |

## 普通 UI 获取自己的权限

业务页面不需要管理数据，只查询当前用户的最终权限：

```http
GET /v1/xhs/me/permissions?organization_id=G
```

处理过程：

```text
Internal JWT 取得 identity.id
→ 按权限目录生成 Keto Batch Check
→ 返回三个 Permission 的 allowed
→ UI 根据结果显示页面和按钮
```

UI 隐藏按钮不等于完成鉴权。用户调用具体业务接口时，`xhs_service` 仍然必须再次
检查对应的 Keto Permission。

## 执行步骤

1. 增加 `System` Namespace 和超级管理员关系。
2. 在 `xhs_service` 中定义权限目录。
3. 实现查看组织权限接口。
4. 实现查看用户权限接口。
5. 实现修改组织权限接口。
6. 实现修改用户角色接口。
7. 实现当前用户权限快照接口。
8. 开发管理页面并验证完整流程。
