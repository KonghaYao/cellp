# 09 Promote, Archive and Migration

> 状态：Draft；AD-5 saga、AD-9 archive/wake 为现行事实；Q1/Q2/Q3/Q6/Q8/Q9/Q12 已作为 migration 固定输入，新 AD 未批准。

## 目的
在不破坏现有 deploy/promote/archive 语义下引入 `deploy_ready`、qualification、cold 与 endpoint cutover。
## 范围
SURGE §16、§21；feature flag、legacy adoption、controller 互斥、promote gate、rollback reserve。
## 非目标
不合并 branch 数据、不创建 promote bucket、不改变 archived 为自动 wake、不批准 schema。
## 术语
`deploy_ready` 表示 artifact、数据和 bindings 可启动；qualification replica 验证 deploy；legacy replica 是现有 route 投影；rollback reserve 是 old prod 有界保温；dual controller 必须互斥。
## 输入/输出
输入：deploy完成、policy、ready endpoints、prod CAS。输出：Version `deploy_ready`/现行 `ready`、route snapshot cutover、old prod drain/reserve、补偿结果。
## 接口/数据模型
**Draft**：deploy prepare→`deploy_ready`→qualification/migration/health→现行 `ready`→按 policy warm/cold。promote 在 CAS 前要求 `ready endpoints>=1`，可选无副作用 prewarm，失败逆序补偿；路由变更与 snapshot revision 同事务发布。preview idle 默认本地 5m/生产 15m；第一阶段 prod `min>=1`。
## 状态/不变量
`deploy_ready` 不重定义现有 `ready`；prod Host 在失败时仍指旧 Version；CAS 前新 prod ready；promote 不改 bucket/AD-8；archive 仍显式产品状态；E1/E2 旧/新 reconciler 同时至多一个有 Start/assignment 写权；resident/unknown background 不得归零；live WebSocket 不因普通缩容被断开。
## 错误/降级
prewarm 失败不切流；snapshot 发布失败补偿/保持旧 prod及 LKG；migration 中断可重入；无法安全 downgrade 时停止并要求人工，而非删状态；spike 失败缩小范围。
## 依赖和并行边界
依赖 01 schema、05 snapshot、08 Cron、14 gates。WP-ORCH 独占 orch promote/archive/deploy；schema/API/wiring 各自 owner。
## 未来实现 WP
`WP-ORCH` saga；`WP-REG` migration；`WP-WIRE` flag/互斥；不能并行改同一 orch 文件。
## 验证
unit：saga补偿/状态映射；contract：legacy projection；component：qualification/cutover failure；e2e：TP-E8、现有 TP-V4/V4b/V15 回归；stress：promote burst；chaos：cutover时 controller/Gateway失效。
## 证据产物
`docs/evidence/surge/e4/promote/<run-id>/` 与 `.../rollback/`：CAS、snapshot revision、endpoint/drain 时间线。
## 阻塞 spike
SP-E4 drain；prewarm endpoint 必须证明无副作用；`deploy_ready` migration 与旧客户端兼容；controller 互斥及 snapshot 原子发布须 E1/E2 证据。
## 回滚/兼容注意事项
关闭新 scale→每个现行 ready Version 单 legacy replica→发布单 endpoint→drain额外 replica→停 elastic→启 legacy；不可颠倒。保留 additive `deploy_ready` 数据，M1/M2 全量串行回归。
