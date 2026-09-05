# 01 Serving State Registry

> 状态：Draft；Q1、Q6、Q9、Q12 已确认为设计输入，但不构成已批准 migration；现行 SQLite/AD-1 继续有效。

## 目的
定义 Version 与 Serving 解耦的控制面事实、CAS 与快照投影。
## 范围
SURGE §6–§7；policy/desire/node/replica 逻辑表，Store 接口，legacy route 投影。
## 非目标
不运行 celld、不决定 autoscale、不处理请求、不修改冻结 D1 RPC。
## 术语
Version 是部署生命周期；逻辑状态 `deploy_ready` 表示 artifact、数据和 bindings 已准备且可启动；现有 `ready` 语义保持不变；Serving state 是 replica 汇总派生值；generation 是命令 fencing；snapshot revision 是 Gateway 读模型版本。
## 输入/输出
输入：API/policy、部署状态、Agent observations。输出：一致的 desired、assignments、派生 serving summary、RouteSnapshot source。
## 接口/数据模型
**Draft**：为 Version 增加 additive 的 `deploy_ready` 逻辑事实（最终列名/API 名由 E0 contract freeze 确定）；新增 `serving_policies`、`serving_desires`、`runtime_nodes`、`runtime_replicas`；事务方法 `CompareAndSetDesired`、`ClaimAssignment`、`RecordObservation`、`BuildSnapshotAfter(revision)`。现有 `versions/routes` 保持兼容投影。所有路由写入与 snapshot revision 递增在同一事务提交。
## 状态/不变量
`deploy_ready` 不等价于 active replica 或现有 `ready`；cold/waking/warm 由 serving facts 派生，不手工写。同 replica ID 不复用；generation 单调；一 assignment 同时至多一个有效 node；route 只含 ready/non-draining endpoint。E1/E2 仅单 active `cellpd` 写 Scheduler/Reconciler 状态；SQLite revision 单调；Gateway 只接受严格递增且校验通过的完整 snapshot。
## 错误/降级
CAS 冲突重读；busy 有界退避；损坏/未知状态拒绝发布；控制面故障保留 LKG snapshot；首次没有有效 snapshot 时 fail-closed；过期 node 不立即删除审计记录。
## 依赖和并行边界
依赖 00。WP-REG 独占 `registry/store.go`,`registry/sqlite.go`；Scheduler/Gateway 不直写表。与 02/04/06 在 Store contract 冻结后并行。
## 未来实现 WP
`WP-REG`：migration、Store、snapshot builder；`WP-WIRE` 仅注入。不得由 WP-GW/WP-SCHED 修改 schema。
## 验证
unit：`deploy_ready`/serving 派生、CAS；contract：migration up/down、Store fake；component：重启对账、revision/LKG；e2e：TP-E1/E5/E6/E7；stress：TP-EP5 与 SQLite contention。
## 证据产物
`docs/evidence/surge/e1/registry/<run-id>/`：schema、事务冲突、revision、重启、进程/bucket 清单。
## 阻塞 spike
E1 必须证明单 active writer、revision 原子提交、重启重建和 legacy 投影；V12 多 active 选主/分发另立设计，不进入 E1/E2。
## 回滚/兼容注意事项
先将现行 ready Version 收敛为单 legacy replica并发布单 endpoint，再停 elastic；migration 不删除旧 routes；旧客户端继续看到现有 Version 枚举和 `ready` 语义；新字段保持 additive。
