# Istio Proxyless：Kitex xDS 客户端治理

## 目标

在独立的 Istio Sidecar 实验 namespace 中增加一条 Kitex gRPC 调用链，理解 RPC 框架如何直接消费 Istio
下发的 xDS 配置：

```text
ui_example / curl
      │ HTTP/JSON
      ▼
Istio Ingress
  ├── Oathkeeper ext_authz
  └── gRPC-JSON Transcoder
      │ gRPC
      ▼
Social Kitex gRPC Server
      │ Kitex gRPC + Proxyless xDS Outbound
      ▼
XHS Kitex gRPC Server
```

完成后应能观察到：

1. 新建的 `xhs_grpc` 使用 Kitex 和 gRPC Transport 提供内容查询 RPC；现有 `xhs_service` 保持 Hertz 实现。
2. `social_service` 提供聚合查询 RPC，内部通过 Kitex Client 调用 `xhs_grpc`。
3. Social 的 Kitex Client 直接从 Istio 获取 XHS 的路由和 Endpoint 配置。
4. XHS 故障由 Social 的 Outbound 熔断器统计并触发熔断。
5. `ui_example` 提供 Social 内容查询页面，可以观察正常调用、故障和熔断响应。

## 实验边界

### Kitex gRPC 不等于 grpc-go

本实验所说的“gRPC Server”是：

```text
Kitex Server + Protobuf IDL + gRPC Transport
```

服务端仍由 Kitex 创建和调度，不使用 `google.golang.org/grpc.Server`。协议必须显式使用 gRPC，
不能使用 Kitex 默认的 TTHeader，否则 `kitex-contrib/xds` 的 HTTP Route 映射无法按 gRPC 方法名工作。

### Gateway 转发 gRPC 不等于提供 HTTP/JSON 接口

Istio Gateway 可以通过 `GRPCRoute` 或 HTTP/2 路由把外部 gRPC 请求转发给 XHS，但普通路由不会
根据 `google.api.http` 注解自动完成 JSON/HTTP 到 gRPC Message 的转换：

```text
grpcurl --HTTP/2--> Gateway --gRPC--> XHS       可以
browser fetch JSON --> Gateway --> gRPC XHS    
```

若要让浏览器直接调用，需要额外启用 gRPC-Web 或 Envoy gRPC-JSON Transcoder。本实验选择在
Istio Ingress Envoy 上显式配置 gRPC-JSON Transcoder：Gateway 使用 Protobuf descriptor 将
HTTP/JSON 请求转换为 gRPC，再转发给 Kitex Server。

### Proxyless 的验证主体是 Social Outbound

当前 `kitex-contrib/xds` 的主要能力在客户端：

```text
Social Kitex Client
  -> xds.Init()
  -> xdssuite.NewClientOption()
  -> RDS 选择路由
  -> CDS/EDS 发现 XHS Endpoint
  -> Kitex Circuit Breaker 执行熔断
```

XHS 使用 Kitex gRPC Server 只代表它可以接收 Kitex gRPC 请求，不代表它具备与 gRPC-Go
`xds.NewGRPCServer()` 相同的 LDS、RDS、RBAC 等完整 Inbound 能力。

### Sidecar 与 Proxyless 的实验边界

本实验使用独立的 `ddd-learn-sidecar` namespace，并通过 `istio-injection=enabled` 使用传统
Sidecar 数据面。Ory、Ingress 后端和 XHS Server 可以使用 Envoy Sidecar；第八步为 Social
启用 Proxyless xDS 时，单独对 Social Pod 设置 `sidecar.istio.io/inject: "false"`。这样只有
Social Outbound 由 Kitex 执行 xDS 路由和熔断，不会再由 Envoy Sidecar重复执行。

## 步骤

每个步骤只完成一个可独立验证的功能点。每一步完成后先解释实现和验证结果，确认无误后再单独
提交；提交信息必须包含 `gateway/009`、阶段和步骤编号。

### 第一步：建立实验基线并验证版本兼容性

1. 从 `deployments/gateway/006_istio_ambient` 复制本实验仍需要的 Ory、PostgreSQL、
   Ingress 和基础 Helm values 到 `deployments/gateway/009_istio_proxyless`。
2. 不复制 006 的历史 README 内容，为 009 新建只记录本实验操作的 README。
3. 使用独立的 `ddd-learn-sidecar` namespace，入口地址和现有 NodePort 保持不变。
4. 记录当前 Istio、Kitex 和 `kitex-contrib/xds` 版本。
5. 使用最小 Kitex Client 验证能否连接当前 istiod、完成 ADS 握手并收到 LDS/RDS/CDS/EDS；
   不在兼容性未确认前重构业务服务。

验证：现有认证链路和 XHS 服务保持可用；最小客户端能够收到 xDS Resource，或者明确记录具体的
兼容性错误和缺失能力。

提交边界：只提交实验基线、版本记录和兼容性验证代码，不修改 `xhs_service`。

### 第二步：定义 XHS gRPC 契约并生成 Kitex 代码

1. 复用 `idl/xhs_service` 下已有 Protobuf 消息，增加按组织和关键词查询内容的 RPC 契约。
2. 请求至少包含 `organization_id` 和 `keyword`；响应保留内容 ID、标题、来源关键词和平台标识。
3. 使用 Kitex Protobuf 代码生成器生成 Client、Server 和数据模型代码。
4. 固定生成命令，确保生成结果可以重复构建。
5. 本步骤只定义契约和生成代码，不切换 XHS 运行入口。

验证：生成代码可编译；通过契约测试确认关键词、组织和内容字段映射正确。

提交边界：只提交 IDL、生成配置和生成代码。

### 第三步：将 XHS Transport 重构为 Kitex gRPC Server

1. 将 `xhs_grpc` 作为独立服务目录；现有 `xhs_service` 的 domain、service、repository、Keto Client 和配置加载逻辑保持不变。
2. 使用 Kitex Handler 适配已有应用服务，不把业务逻辑重新写进 RPC Handler。
3. 使用 gRPC Transport 启动 Kitex Server，并注册第二步生成的 XHS Service。
4. 将 Internal JWT 的读取和 Principal 构造迁移到 Kitex Server Middleware。
5. 将 HTTP Header 中的身份传播约定改为对应的 gRPC Metadata 约定。
6. 增加 gRPC Health Checking，供 Kubernetes readiness/liveness probe 使用。
7. 保留 `xhs_service` 的 Hertz 路由入口；`xhs_grpc` 作为独立服务，不修改旧服务的生成路由。

验证：使用不经过 xDS 的固定地址 Kitex Client 直接调用 XHS；正常身份可以查询内容，缺少或错误
Internal JWT 被拒绝，Keto 授权结果保持不变。

提交边界：只提交 XHS 的 Kitex Server 重构，不加入 Social Service 和 xDS Client。

### 第四步：将 XHS gRPC Server 接入 Kubernetes

1. 更新 XHS 镜像构建和 Helm values，使容器暴露 gRPC 端口。
2. 更新 `Service/xhs-service`，为 gRPC 端口设置稳定名称和 `appProtocol`。
3. 将 HTTP 健康检查替换为 Kubernetes gRPC Probe。
4. 部署至少两个 XHS Pod，确认 EndpointSlice 包含两个 gRPC Endpoint。
5. 不创建 Waypoint；XHS Server 由 namespace 的 Sidecar 注入策略管理。
6. 暂不修改外部 XHS HTTPRoute；Gateway 转码在下一步单独实现。

验证：Pod Ready、Service 和 EndpointSlice 正确；集群内固定地址 Kitex Client 能通过
`xhs-service` 调用 RPC；Pod 不带 Ambient redirection，Sidecar 注入状态符合本步骤约定。

提交边界：只提交 XHS 的镜像和 Kubernetes 接入配置。

### 第五步：在 Gateway 配置 gRPC-JSON Transcoder

1. 从 XHS Protobuf IDL 生成包含依赖的 File Descriptor Set。
2. 将 descriptor 以可重复生成的形式提供给 Istio Ingress Envoy。
3. 使用声明式 `EnvoyFilter` 在 Ingress HTTP Filter Chain 中加入
   `envoy.filters.http.grpc_json_transcoder`，并明确它和 ext_authz、router 的顺序。
4. 配置 `/v1/xhs` HTTPRoute，使原有 HTTP/JSON 请求在 Gateway 转换后发往 XHS gRPC 端口。
5. 保留 Oathkeeper 的 XHS HTTP 认证规则；Oathkeeper 注入的 Internal JWT Header 转换为 XHS
   能读取的 gRPC Metadata。
6. 另建 `GRPCRoute` 或使用 gRPC Listener 验证 `grpcurl` 可以不经过 JSON 转码直接调用 XHS。

验证：同一个 XHS Kitex Server 同时可以被 `grpcurl` 以 gRPC 调用，也可以通过 Gateway 的
`/v1/xhs` HTTP/JSON 接口调用；Pod 内没有 Hertz HTTP Server。

提交边界：只提交 descriptor 生成方式、Gateway Transcoder 和 XHS 路由配置。

### 第六步：创建 Social Kitex gRPC Service

Social Service 是面向多个社交平台的聚合应用，当前只接入 XHS：

```text
Social Application Service
        │
        └── ContentProvider
                └── XHSProvider -> XHS Kitex Client

未来：
        ContentProvider
          ├── XHSProvider
          ├── WeiboProvider
          └── DouyinProvider
```

1. 新建 Social Protobuf IDL 和 `social_service`，定义两个 RPC，并通过 `google.api.http` 声明
   对外 HTTP 映射：
   - `ListMyOrganizations` -> `GET /v1/social/me/organizations`；
   - `SearchContents` ->
     `GET /v1/social/organizations/{organization_id}/contents?keyword={keyword}`。
2. 定义 Social 自己的统一 `SocialContent` 模型，不直接把 XHS 的 RPC Response 暴露给调用者。
3. 定义 `ContentProvider` 接口和聚合服务；当前只注册 `XHSProvider`。
4. 使用 Kitex gRPC Server 暴露 Social RPC；不在 Social 中启动 Hertz HTTP Server。
5. `XHSProvider` 首先使用固定 XHS 地址，确保本步骤只验证聚合逻辑。
6. Social 从 gRPC Metadata 验证 Oathkeeper 签发的 Internal JWT，并将用户身份继续传给 XHS。
7. 为聚合服务使用 Mock Provider 编写单元测试，覆盖正常结果、空结果和 Provider 返回错误。
8. 将 Social descriptor 合入 Gateway Transcoder，增加 `/v1/social` HTTPRoute 和 Oathkeeper
   认证规则，使浏览器 HTTP/JSON 请求被转换为 Social gRPC 请求。

验证：使用 Kitex Client 直连和通过 Gateway HTTP/JSON 两种方式都能获得 XHS mock 内容，返回值
带 `platform: xhs`。

提交边界：只提交 Social Service、Gateway 路由及其基础部署，不启用 xDS 和熔断。

### 第七步：为 ui_example 增加 Social 调试页面

1. 在 `ui_example` 增加 Social 页面和导航入口。
2. 页面首先请求 `/v1/social/me/organizations`，允许选择当前用户所属组织。
3. 提供关键词输入框，请求
   `/v1/social/organizations/{organization_id}/contents?keyword={keyword}`。
4. 展示统一内容字段，包括标题、来源关键词和 `platform`。
5. 展示 Loading、空结果、认证失败、授权失败、上游超时和服务不可用状态。
6. 增加仅用于实验的故障模式选择器；默认关闭，启用后通过测试 Header 请求 Social，方便后续
   观察 XHS 故障和熔断。

验证：Alice 登录后可以选择组织并查询 XHS 内容；刷新页面不会卡在旧请求状态；正常模式不会携带
故障测试 Header。

提交边界：只提交 UI 页面、API Client 和前端测试，不启用 Proxyless xDS。

### 第八步：为 Social 的 XHS Client 启用 Proxyless xDS

1. 在 Social 启动时调用 `xds.Init()`。
2. 创建 XHS Kitex Client 时使用 gRPC Transport 和 `xdssuite.NewClientOption()`。
3. 目标服务使用 `xhs-service.ddd-learn-sidecar.svc.cluster.local:<grpc-port>`，不写 Pod IP。
4. 通过 Downward API 注入 `POD_NAMESPACE`、`POD_NAME` 和 `INSTANCE_IP`，并声明
   `KITEX_XDS_METAS`、istiod 地址及认证参数。
5. 使用声明式 Istio 配置为 XHS 内容查询方法生成 RDS/CDS/EDS 配置；RDS 方法路径必须来自
   第二步生成代码中的实际 Protobuf package、service 和 method 名称，不能照抄 `SayHello` 示例。
6. 删除 Social 中的固定地址 Resolver，确保 Endpoint 只来自 xDS Resolver。

验证：

1. Social 日志显示 xDS 初始化和资源更新成功。
2. 连续调用 Social API 时，两个 XHS Pod 都能收到请求。
3. 删除并重建一个 XHS Pod 后，Social 不重启也能使用新的 Endpoint。
4. Social Pod 没有 Envoy Sidecar；XHS Server 可以保留 Sidecar，证明 Proxyless Client 能调用传统 Sidecar 工作负载。

提交边界：只提交 Proxyless xDS Client 和所需的声明式 Istio/Kubernetes 配置。

### 第九步：制造可控的 XHS 故障

1. 为实验增加测试专用的 XHS 故障模式，通过明确的 gRPC Metadata 或独立 fault Pod 触发。
2. 支持至少两类故障：返回 `Unavailable` 和延迟超过 Social 的调用超时。
3. Social 仅在收到测试 Header 时传递故障标记，普通请求不受影响。
4. 先关闭熔断，记录每次请求确实到达 XHS，并确认 Social 收到真实 RPC 错误或超时。
5. 故障逻辑必须与业务 Handler 隔离，并清楚标记为实验代码。

验证：正常请求持续成功；故障请求稳定产生预期的 gRPC Code 或 DeadlineExceeded；尚未出现本地
快速失败，证明当前只完成了故障源。

提交边界：只提交故障注入与验证入口，不同时启用熔断。

### 第十步：使用 xDS 配置驱动 Social Outbound 熔断

1. 使用声明式 Istio/EnvoyFilter 配置向 Social 订阅的 XHS Cluster 下发
   `OutlierDetection`。
2. 设置适合实验观察的小样本阈值和失败比例，不使用生产参数。
3. 由 `xdssuite.NewClientOption()` 将 CDS `OutlierDetection` 转换为 Kitex
   `circuitbreak.CBConfig`。
4. 先演示实例级熔断：故障实例被避开，健康 XHS Pod 继续响应。
5. 再按需启用 `WithServiceCircuitBreak(true)`，演示整个 XHS Cluster 达到错误率后本地快速失败。
6. 停止故障，验证熔断器经过恢复窗口后重新允许试探请求并恢复调用。

验证时必须区分三种现象：

| 现象 | 说明 |
| --- | --- |
| 请求到达 XHS 后返回错误 | 上游真实故障，还没有被熔断 |
| 请求没有到达 XHS，Social 立即失败 | Social Outbound 熔断器已经打开 |
| 健康 Pod 继续收到请求 | 实例级熔断只摘除了故障 Endpoint |

提交边界：只提交熔断配置、Social Client 选项和验证记录。

### 第十一步：对比 Proxyless Client 与 Envoy Sidecar

1. 记录一次完整请求中 Ingress、Oathkeeper、Social Proxyless Outbound、XHS Sidecar Inbound 和 XHS 的职责。
2. 对比 Kitex Client 熔断与 Envoy Sidecar Outbound 熔断的执行位置，确认 Social 没有重复重试或重复熔断。
3. 说明 `kitex-contrib/xds` 当前未实现的能力，不能仅因为收到了 xDS Resource 就宣称支持。
4. 恢复故障配置，保留可重复执行但默认关闭的测试 Manifest。

验证：README 能通过命令和观测结果证明 xDS Resource 由 Social 消费、熔断由 Social 进程执行、
Social 的 xDS Resource 由应用进程消费，XHS 的入站流量由 Sidecar 承接。

提交边界：只提交最终验证记录和默认关闭的测试配置。

## 完成标准

1. XHS 使用 Kitex gRPC Server 提供按关键词查询内容的 RPC。
2. Gateway 使用显式 gRPC-JSON Transcoder 为 Kitex gRPC Server 提供 HTTP/JSON 接口。
3. Social 使用统一 Provider 模型聚合 XHS 内容，并保留未来增加平台的扩展点。
4. UI 可以选择组织和关键词访问 Social，并显示故障、超时和熔断结果。
5. Social 的 Kitex Client 不依赖 Sidecar，能够通过 xDS 获得 XHS Endpoint 更新。
6. 能从日志和请求现象区分真实 XHS 故障、实例级熔断和服务级熔断。
7. 能解释 Proxyless Outbound、XHS Sidecar Inbound 和入口 Gateway 的职责边界。

## 风险与停止条件

`kitex-contrib/xds` 官方说明主要针对客户端，并且公开文档验证的 Istio 版本较旧。因此第一步是
兼容性门禁。如果当前 Istio 无法向 Kitex Client 下发可用资源，应先记录 ADS/NACK 或资源转换
错误，再决定升级适配依赖或改用 gRPC-Go；不能绕过 xDS Resolver 后仍将实验标记为 Proxyless
成功。
