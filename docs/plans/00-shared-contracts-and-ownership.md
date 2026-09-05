# 00 Shared Contracts and Ownership

> 状态：Draft；方案 A 与 Q1–Q12 已确认为本设计输入，但新 AD 尚未批准，E0/SP 尚未执行；现行 AD 与冻结契约优先，当前不授权产品开发。

## 目的
建立所有弹性组件唯一共享词汇、逻辑契约和产品文件 owner，防止并发实现重复定义。
## 范围
Version/Serving、Policy/Desire/Node/Replica/Endpoint/Snapshot、generation、错误枚举、ownership 登记；对应 SURGE §0–§7、§26–§29。
## 非目标
不批准 SURGE，不实现 schema/API/RPC，不修改 D1 frozen contracts，不启动产品开发。
## 术语
**现行事实**：Project+Version、独立 bucket、RustFS、`ready`/`archived`。**已确认设计输入**：保留 `ready` 现有语义；新增逻辑状态 `deploy_ready` 表示 artifact、数据和 bindings 已准备且可启动；Serving Fleet=`0..N` replica，cold/waking/warm 为 serving 派生视图，assignment 是 placement 意图。具体字段名仍由 E0/API contract freeze 确定。
## 输入/输出
输入：现行 AD、SURGE Draft、discovery、decision-001..017。输出：供 01–14 消费的逻辑 schema、接口版本与 owner 表。
## 接口/数据模型
**Draft**：键均含 `(project_id,version_id)`；`ServingPolicy{revision,min,max,priority,background_mode}`；`ServingDesire{desired,generation,reason}`；`RuntimeReplica{replica_id,node_id,generation,state,valid_until}`；`Endpoint{replica_id,address,state}`；`RouteSnapshot{revision,bindings,endpoint_sets,policy_revision}`。跨组件命令：`EnsureCapacity`、`StartReplica`、`DrainReplica`、`StopReplica`；错误仅低基数枚举。Node Agent 逻辑命令保持 transport-neutral，本机和远程传输统一 HTTP+mTLS。
## 状态/不变量
跨 Version bucket 禁止共享写；watch 可删；archived 不隐式激活；`ready` 不得被重定义为“可启动”。Autoscaler 唯一写 desired，Scheduler 唯一写 assignment，Agent 只执行；E1/E2 仅单 active `cellpd` 持有 Scheduler/Reconciler 写权；旧 generation 无效。Gateway 热路径不查 SQLite，以 revision 轮询构建并原子替换不可变 snapshot；仅接受严格递增且校验通过的 revision。非幂等请求一旦可能被 upstream 接收，不自动重放。
## 错误/降级
未知字段和 background capability fail-closed；snapshot 校验失败保留 last-known-good，首次无有效 snapshot 时 fail-closed；控制面不可达时 warm 可继续、cold 有界 `503`；不得暴露 secret 或内部地址。
## 依赖和并行边界
本篇与 `SURGE-DESIGN-INDEX.md` 是共享契约 owner。01 可细化存储但不得改语义；02/04/05 只消费契约。接口变更先由 WP-CONTRACT 串行更新，再并发传播。
## 未来实现 WP
`WP-CONTRACT` 唯一写新共享 contract/types；`WP-WIRE` 最终组装；权威文档由 `WP-ADOPT` 在新 AD 获批且用户授权后独占。
## 验证
- unit：枚举、generation、派生状态。
- contract：JSON/RPC golden、向后兼容。
- component：Store→snapshot、desired→assignment mock。
- e2e：flag=0 保持现状；Host/version/bucket 隔离。
- stress：schema/snapshot 大规模构建无热路径阻塞。
## 证据产物
`docs/evidence/surge/contracts/<run-id>/`：golden diff、兼容报告、版本矩阵（未来实现时；本轮不生成产品证据）。
## 阻塞 spike
SP-E1..E6；Node Agent 证书生命周期、multi-node celld、冷启动预算和后台 workload 唯一执行仍须证据闭合。Q1–Q12 不再是未决项，但只有新 AD 批准后才成为产品规范。
## 回滚/兼容注意事项
flag=0 使用 legacy Route/AD-1；不改变 `ready`/`archived` 现行 API；新增字段先 additive/只读；D1 RPC 零变更。回滚前先收敛每个现行 ready Version 为单 legacy replica。
