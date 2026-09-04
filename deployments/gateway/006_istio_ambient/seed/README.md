# Ory 教学数据初始化

`ory-seed.yaml` 为当前 `ddd-learn` namespace 中已经部署的 Kratos 和 Keto 写入开发数据。
它不是数据库迁移：Ory Helm Chart 负责创建数据库表，下面的 Job 负责调用 Ory Admin API
写入身份和关系数据。

## 初始化内容

| 服务 | 初始化内容 |
| --- | --- |
| Kratos | 创建或更新 `alice@example.com` 和 `bob@example.com` 两个密码身份 |
| Kratos | 将两个演示邮箱导入为已验证地址，方便直接登录测试；新注册用户仍需邮件验证 |
| Kratos | 在 `metadata_admin` 写入 `organization_id=G` 和 `role=admin/member` |
| Keto | Alice 写入 `Organization:G#admins`，Bob 写入 `Organization:G#members` |
| Keto | 写入组织权限上限和角色授予关系：成员可启动抓取、查看内容；管理员额外可修改关键词 |
| Oathkeeper | 没有业务数据；规则由 `values/oathkeeper.yaml` 配置并由 Helm 管理 |

`G` 是 Keto 中的组织对象标识，不会额外创建一条组织表记录。组织和权限关系都由
Keto relation tuple 表示，OPL namespace 定义在 `keto/namespaces.ts`。

## 执行

先完成第 1 步中的 Kratos、Keto Helm 部署，再执行：

```shell
kubectl apply -f deployments/gateway/006_istio_ambient/ory-seed.yaml
kubectl -n ddd-learn wait --for=condition=complete job/kratos-seed --timeout=180s
kubectl -n ddd-learn wait --for=condition=complete job/keto-seed --timeout=180s
```

查看执行过程：

```shell
kubectl -n ddd-learn logs job/kratos-seed
kubectl -n ddd-learn logs job/keto-seed
kubectl -n ddd-learn get secret,configmap,job -l app.kubernetes.io/name
```

两个 Job 都是幂等的：Kratos 按邮箱查找并更新身份，Keto 使用 PUT 写入关系。
如果要再次执行 Job，需要先删除 Job 对象；删除 Job 不会删除 Kratos 或 Keto 数据库中的数据：

```shell
kubectl -n ddd-learn delete job kratos-seed keto-seed
kubectl apply -f deployments/gateway/006_istio_ambient/ory-seed.yaml
```

清单中的密码是教学环境示例，生产环境应改成外部 Secret 或受保护的管理流程。
