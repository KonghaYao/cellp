# SURGE E0 Start Pack

> **状态：E0 COMPLETE（contract/audit/plan）— SP 未运行 — AD 未正式批准**  
> **E0 证据：** `docs/evidence/surge/e0/2026-09-05-e0-01/`（verdict=`PASS`，仅限 E0 范围）  
> **架构边界：** Proposed AD-15 文本审查已通过；**正式 AD approval 仍待**用户/架构 owner 基于当次证据独立执行；SP-E1..E6 仍为 `NOT_RUN`；产品代码与 schema migration 未授权。  
> **依据：** [Proposed AD-15](./SURGE-PROPOSED-AD.md) · [Adversarial Review](./SURGE-PROPOSED-AD-REVIEW.md) · [Design Index](./SURGE-DESIGN-INDEX.md) · [Adoption Gates](./14-adoption-gates-and-rollback.md)

## 1. 当前 Gate 与下一合法动作

| 类别 | 状态 | 说明 |
|---|---|---|
| Proposed AD 文本起草 | 已完成 | 状态保持 `PROPOSED — REVIEWED, NOT EFFECTIVE`。 |
| Proposed AD 文本对抗审查 | 已完成 | C1、H1–H6、M1–M6 共 13/13 finding `CLOSED`；最终文本 verdict 为 `APPROVE-AS-PROPOSED`。 |
| 正式 AD approval | 待用户/架构 owner 批准 | 文本 verdict 不能替代正式 approval，也不能提前写入 `DESIGN.md` 或 `docs/decisions.md`。 |
| E0 contract freeze / consumer audit | **已完成** | 见 `docs/evidence/surge/e0/2026-09-05-e0-01/`（`contract-freeze.md`、`consumer-audit.md`）。 |
| singleton guard / HTTP+mTLS / Route Snapshot / cold request / background manifest | 待执行 | 先冻结可测试契约；实现与运行证据属于后续 gate。 |
| SP-E1..E6 | **计划已审阅，待授权执行** | 可执行合同见 E0 `sp-plan-review.md`；**无实验结果**。 |
| 产品实现与 schema migration | 禁止 | 只有 E0 verdict 为 `PASS`、正式 AD approval 完成且对应 phase gate/owner/rollback 齐备后，才能进入相应 WP。 |

**当前 blocker：** 正式 AD approval 尚未发生；SP 运行证据为空；legacy 性能基线未采集。  
**下一合法动作：** 架构 owner 审阅 `docs/evidence/surge/e0/2026-09-05-e0-01/verdict.md` 并独立批准/拒绝 Proposed AD-15；**仅当 E0 verdict=PASS 且正式批准完成** 后，从 `WP-CONTRACT` → `WP-REG` 开始 E1 实现（仍禁止未授权的 migration/SP 实验）。

## 2. E0 目标与非目标

### 2.1 目标

1. 将 Proposed AD 的逻辑语义收敛为版本化、可测试、单 owner 的 contract freeze 记录，不改变 serving 行为。
2. 完成 `deploy_ready` 对现有 API、CLI、CI、Dashboard 和生命周期操作的 consumer audit，逐项给出兼容策略与 owner。
3. 冻结 single-active controller、HTTP+mTLS Node Agent、Route Snapshot、Activator 安全预算机制与 background manifest 的接口和 fail-closed 行为。
4. 将 SP-E1..E6 转成可重复执行的实验合同：环境、输入、预登记阈值、PASS/FAIL、证据目录和 scope reduction 均明确。
5. 形成 E0 `PASS` / `FAIL` / `INFRA_FAIL` / `SCOPE_REDUCED` verdict 包，供正式 AD approval 独立审阅。

### 2.2 非目标

- 不修改 `cellp/`、`celld/`、`web/`、`site/`、`e2e/`、`stress/`、`dev/` 或任何产品代码。
- 不执行 SQLite/schema migration，不生成或应用迁移文件。
- 不启动 dev 栈、celld fleet、Node Agent、Gateway 或任何 E0/SP 实验。
- 不修改冻结 D1 import/branch RPC，不引入 PostgreSQL、Redis、外部 broker 或云存储绕过现行 guarantees。
- 不修改 `DESIGN.md`、`docs/decisions.md`、`docs/test-plan.md`，不写正式 AD，不把 TP/SP 纳入现行门禁。
- 不校准或臆定 timeout、body、queue、concurrency、pressure、takeover 等数值；数值必须来自后续预登记实验。

## 3. E0 启动前置检查表

`[x]` 只表示启动材料存在，不表示 E0 执行完成。

| 检查项 | 当前 | 启动/退出要求 | 依据 |
|---|---|---|---|
| Proposed AD 已起草且明确未生效 | [x] | E0 全程保持 Proposed 状态 | Proposed AD §9 |
| 文本对抗审查 critical/high 已关闭 | [x] | 保持 13/13 `CLOSED` 可追踪；新 critical/high 必须重开 gate | Review §3–§5 |
| Q1–Q12 已闭合 | [x] | 不在 E0 静默改写；变更须回到 decision seam | Decision Brief |
| WP owner 与依赖 DAG 已登记 | [x] | E0 为每项 freeze/audit 指定唯一 owner；共享 contract 先行 | Design Index |
| rollback 顺序已有 Draft | [x] | E0 必须审阅 per-Version enrollment、单 legacy replica 收敛、snapshot、drain、controller handoff 与 M1/M2 计划 | Adoption §rollback |
| feature flag 名称与默认关闭 | [x] | 冻结 `CELLP_ELASTIC_RUNTIME=0` 为默认；不得以直接停 controller 代替安全收敛 | Proposed AD §5 |
| D1 frozen RPC 零变更 | [x] | contract diff 必须证明没有修改 import/branch wire contract | Proposed AD §2.1/§8 |
| legacy baseline 方法与目标环境 | [ ] | 预登记现行单 celld RSS、start p50/p95、纯 RustFS restore p95 与 Gateway inflight/latency/rejection 的采集方法；本轮不采集 | SURGE E0 / Design Index |
| 正式 architecture approver 已接收 E0 输出清单 | [ ] | E0 开始时登记 approver；仅其独立 approval 可使 AD 生效 | Proposed AD §3/§8 |
| E0 `run-id`、证据根与审阅者已登记 | [ ] | 使用 `docs/evidence/surge/e0/<run-id>/`，禁止覆盖旧 run | Test/Evidence design |
| SP 环境、硬件、版本与阈值已预登记 | [ ] | 每个 SP 执行前完成；不得看结果后改 PASS 阈值 | 本文 §6 |

## 4. Contract Freeze 清单

E0 的每项输出至少记录：`contract_id`、revision、normative semantics、consumer、owner、compatibility、failure mode、rollback、open numeric parameters、reviewer。所有项目当前均为 `PENDING`。

| ID | 冻结范围 | 必须冻结的最小内容 | E0 接受条件 |
|---|---|---|---|
| CF-STATE | Version 状态机 | 保留 legacy `ready`；additive `deploy_ready`；qualification 在 health、route verify、snapshot 发布后才进入 `ready`；仅 enrolled elastic preview 可 `ready → deploy_ready+cold`；`archived` 与 cold 分离 | 转换表、CAS/generation、失败路径、API enum/migration 和 flag=0 行为无歧义 |
| CF-CONSUMER | `deploy_ready` consumer audit | CLI、CI polling、OpenAPI、Dashboard、promote、destroy、branch、archive/wake 的读取、写入、终态和错误处理 | 每个 consumer 有兼容结论、唯一 owner、测试断言和未处理风险；critical/high 为 0 |
| CF-BRANCH | branch-source eligibility | `ready`、`deploy_ready`、`archived` 仅在 immutable fork metadata、bucket、LTX/overlay anchor、binding identity 完整时可作父版；cold branch 直接读 RustFS，不启动 celld | 缺任一证明 fail-closed；D1 frozen RPC/branch format diff 为零 |
| CF-SINGLETON | single-active controller guard | 与权威 SQLite 同宿主的进程生命周期排他 guard；获取失败/运行中失去 guard 时停止写循环并拒绝新 Start/Drain/Stop/assignment；双 active 至少一方 fail-closed；无自动 standby promotion | 启动、失效、暂停进程、人工接管、审计/告警与 operator checklist 可测试；普通 TTL lease 不冒充 fencing |
| CF-MTLS | HTTP+mTLS Node Agent | 本机/远程同一 transport；内部 listener；node identity 与 project/version/action scope；generation、lease/expiry、nonce/replay；仅 secret reference；CA/SAN、签发、轮换、吊销、过期、mixed-version | auth/replay/mismatch 全部 fail-closed；无本机 bypass；不记录 token、private key、credential 或完整连接串 |
| CF-SNAPSHOT | Route Snapshot | 同事务递增 revision；一致性读构建；schema/content 校验；进程内 immutable atomic swap；首次无快照 fail-closed；LKG 不越过 endpoint `valid_until`；draining 不接新请求 | bootstrap、rollback、revision regression、writer loss、全部 endpoint 过期、promote 收敛均有确定结果；E1/E2 拓扑限定同宿主同权威 SQLite |
| CF-ACTIVATOR | cold request / Activator budgets | 仅 `deploy_ready+cold`；GET/HEAD/符合上限的小 body 可有界等待；大 body/stream/WebSocket 快速 `503 + Retry-After`；可能已送达 upstream 的请求不重放；per-Version/global count、bytes、deadline 硬上限 | 字段、分类、reason、取消释放、singleflight/CAS 幂等和配置校验冻结；具体数值标记 `EVIDENCE_PENDING_SP_E6`，不得伪造默认值 |
| CF-BACKGROUND | workload manifest | versioned static manifest/bindings；`none/resident_required/unknown`；Queue、Workflow、alarm、已武装 Cron；非 prod 未 arm Cron 例外；解析失败/未知 fail-closed；无证明时 `min>=1,max=1` | 每类 capability、schema 兼容、unknown、secret redaction、policy guard、Cron arm/disarm owner 和 future evolution 明确 |
| CF-ROLLBACK | enrollment / rollback | per-Version opt-in；flag 默认关闭；先停 activation/scale，再恢复单 legacy endpoint、发布 snapshot、drain extra replicas、移交 controller；失败停在人工安全模式 | rehearsal 输入/步骤/观测/中止点/恢复点完整；不得静默 archive，不删除 additive 表，不绕过 M1/M2 |

建议 E0 证据文件（后续执行时创建，不在本轮创建）：

```text
docs/evidence/surge/e0/<run-id>/
├── manifest.json
├── contract-freeze.md
├── consumer-audit.md
├── rollback-review.md
├── sp-plan-review.md
└── verdict.md
```

## 5. API / Consumer Audit 范围

E0 只读盘点现有实现与调用方，再把兼容结论写入证据；不在 audit 中顺手修代码。

| Consumer | 必查问题 | 要求的兼容结论 |
|---|---|---|
| CLI | 哪些命令把 `ready` 当唯一成功/可操作状态；输出 enum、exit code、wait 行为是否穷举 | legacy CLI 在 flag=0 行为不变；新增状态 additive；未知状态不得误报成功 |
| CI polling | deploy pipeline 是否轮询 `ready`，是否把非 `ready` 当失败或无限等待 | qualification 仍以 `ready` 为外部成功终态；`deploy_ready` 不得被旧 CI 当部署完成 |
| OpenAPI | Version status enum、响应 schema、错误 reason、wake/archive/policy surface | 只允许 additive 兼容；旧 `ready` 语义不变；当前不编辑 `cellp/api/openapi.yaml` |
| Dashboard | badge、按钮、polling、cold/degraded/unknown 展示和操作权限 | 不把 cold 显示为 archived；不向 Agent/celld/RustFS 直连；未实现能力不显示为 GA |
| promote | target validation、CAS、route/snapshot revision、Cron reconcile、compensation | 仅当前有效 snapshot 中有未过期 endpoint 的 `ready` 可 promote；`deploy_ready`-only/cold fail-closed |
| destroy | 状态过滤、父子引用、进程/route/watch 清理顺序 | 新状态不绕过 child/parent 与数据保留规则；失败不遗留半删除状态 |
| branch | 父状态校验、immutable metadata/bucket/anchor/binding proof、cold 读取路径 | `deploy_ready`/archived 的扩展只改 cellp eligibility；缺证明拒绝；D1 frozen RPC 零变更 |
| archive / wake | cold 与 archived 分流、reaper enrollment、显式 wake、错误码 | cold 不自动 archived；elastic enrollment 排除 AD-9 reaper；`POST wake` 只接受 archived |

每行审计结果必须包含：代码/契约位置、当前行为、Proposed 行为、compatibility risk、owner、计划测试、结论（`compatible` / `additive-change` / `blocking`）。任何未分配的 `blocking` 项都使 E0 verdict 不能为 `PASS`。

## 6. SP-E1..E6 可执行计划

共同规则：每次运行使用唯一 `run-id`、隔离 project/version/bucket/SQLite/watch/temp/ports；stress/chaos 独占硬件或串行；manifest 预登记硬件、RustFS/celld 版本、fixture、阈值和停止条件；日志与证据必须脱敏。当前六项均为 `NOT RUN`。

| SP | 环境与输入 | PASS 条件 | FAIL 条件 | 证据目录 | 最小 scope reduction |
|---|---|---|---|---|---|
| SP-E1 单 Version 双 celld node | 两个 celld；同 Version bucket；独立 node identity/watch/public+internal listener；相同 immutable deploy/bindings revision；双 endpoint HTTP 与写负载 | revision/bindings 一致，Version 隔离不破坏，双端点均可服务；吞吐、RSS、写延迟达到 run manifest 预登记阈值 | 数据/identity 混用、revision 分叉、确认写异常，或收益/资源超出预登记边界 | `docs/evidence/surge/sp-e1/<run-id>/` | 阻塞 1→N/WP-CELLD；保留单 celld 的 E1/E2 0/1 路径 |
| SP-E2 ownership/takeover | SP-E1 fixture；同一 Durable Object 交替访问；owner kill、进程暂停、node partition、RustFS 短时故障 | 旧 owner 被 epoch/fencing 拒绝；已确认写不丢；takeover/恢复在预登记预算内；不出现双 owner 成功写 | stale owner 可写、确认写丢失/分叉、无限 takeover 或故障时错误确认成功 | `docs/evidence/surge/sp-e2/<run-id>/` | 禁止 remote/multi-node owner takeover 与自动 scale-in；保持单 node owner |
| SP-E3 bindings/background matrix | SP-E1 基线；D1、KV、R2、Queue、Workflow、Cron、alarm、WebSocket 分别建 fixture；每类记录 owner/duplicate detector | 每类独立证明数据语义及全局协调/接管/重复投递契约；未证明项明确保持 resident/singleton | 任一能力出现不可接受重复、丢失、双 arm/双 consumer 或无法观测 | `docs/evidence/surge/sp-e3/<run-id>/` | 只移除失败 capability 的多 replica；该类保持 `min>=1,max=1`；HTTP-only 不被无关失败连带移除 |
| SP-E4 graceful shutdown | active HTTP、long request、WebSocket、Queue/Workflow/alarm；正常 drain、deadline、emergency operator 场景 | 摘流后无新请求；inflight 在契约 deadline 内完成/可审计终止；普通缩容不主动断 WS；后台 handoff 符合已冻结语义 | 新流量进入 draining endpoint、hard-stop 导致未定义丢失、普通缩容断 WS、后台 owner 不可安全交接 | `docs/evidence/surge/sp-e4/<run-id>/` | 禁止失败 workload 的自动 scale-in；必要时 resident/pinned，emergency 仅保留人工路径 |
| SP-E5 RustFS 多节点条件写 | 不同 runtime node 连接生产等价 RustFS endpoint/VIP；`celld diagnose`；V0a 与 non-skipped V0c；网络抖动/超时/重试 | diagnose 通过；V0c 竞争严格一个成功；重试/partition 不破坏 fencing；成功写 durability 符合现行 guarantee | V0c skipped/多成功/零可解释结果、fencing 破坏、错误确认或证据不完整 | `docs/evidence/surge/sp-e5/<run-id>/` | 禁止多节点 serving 与相关 E4；不得以 cellp CAS、外部存储或 best-effort 补洞 |
| SP-E6 冷启动成本 | 目标硬件；已有本机缓存与无 watch 纯 RustFS restore；小/中/大 D1；不同 artifact/assets；1/10/100 并发冷请求 | 数据足以在预登记安全上限内确定 `wake_timeout`、queue count/body bytes、memory、`Retry-After`、capacity/pressure 参数；并发风暴有界 | 无法给出不超内存/资源的有界配置，结果不可重复，或需要无限 queue/deadline 才成功 | `docs/evidence/surge/sp-e6/<run-id>/` | 取消受影响请求类别的同步等待，改为快速 `503`/预热；必要时该类禁 scale-to-zero |

每个 SP 目录至少包含：`manifest.json`、脱敏环境与版本、fixture/拓扑、命令与时间线、raw logs、metrics、结果摘要、异常/重试记录、scope decision。`INFRA_FAIL` 不得伪装为产品 `FAIL`，重跑必须使用新 `run-id` 并链接原结果。

### Scope-reduction 统一规则

1. 失败先定位最小 capability（例如 Queue multi-replica），不得把无关 HTTP-only 0/1 能力一并判死，也不得扩大成 best-effort 上线。
2. reduction 必须写明受影响的 phase/WP/API/config/docs、恢复的现行安全基线、feature flag 与 rollback。
3. celld/RustFS fencing/durability 失败时，一律阻塞相关 multi-node/multi-replica；禁止新增数据库、broker 或存储旁路。
4. cold budget 失败时优先缩小等待类别或禁用 scale-to-zero，不放宽内存、deadline、重放与 secret 边界。
5. 任何 threshold 变更必须创建新 run/revision 并重新审阅，不能为通过已有结果而事后调线。

## 7. E0 Exit 与正式 Approval

E0 只有在以下全部成立时才能记录 verdict=`PASS`：

- CF-STATE 至 CF-ROLLBACK 均有 versioned freeze、唯一 owner 与 reviewer；
- consumer audit 覆盖本文八类 consumer，未分配或未处置的 critical/high blocker 为 0；
- feature flag 默认关闭、legacy behavior、D1 frozen RPC 与 rollback diff 均可复核；
- SP-E1..E6 的环境、输入、预登记阈值、PASS/FAIL、证据和 scope-reduction 已获 review；
- E0 evidence manifest 完整且不含 secret。

E0 的 `PASS` 只说明 approval preconditions 的 contract/audit/plan 部分完成；它不代表 SP 已运行，不代表 Proposed AD 自动生效，也不授权 E2–E5。E0 输出必须提交用户/架构 owner 独立做正式 AD approval；批准动作完成前不得写正式 AD 或开始产品实现。

## 8. WP 启动顺序

E0 verdict=`PASS` **且**正式 AD approval 完成后，才按以下顺序开放产品 WP：

| 顺序 | 可开放范围 | 条件/限制 |
|---|---|---|
| 1 | `WP-CONTRACT` | 先串行落地获批的共享 types/contracts；其他 WP 只消费，不并发改共享定义 |
| 2 | `WP-REG` | `WP-CONTRACT` handoff 后进入 E1 Registry/generation/legacy compatibility；schema migration 仍需该实现任务单独授权与回滚计划 |
| 3 | E1 限定的 `WP-SCHED`、`WP-RT`、`WP-SEC`、`WP-GW`、`WP-OPS` | 仅实现本机 0/1、single-active、local HTTP+mTLS 与 legacy single-endpoint snapshot 所需范围；按 Design Index DAG 和唯一 owner 开放 |

仍 blocked：

- `WP-GW-ACT`、`WP-SCALE`：待 E1 gate，且进入 E2 前需要 SP-E6 对相关 budget 的证据。
- remote-node 范围的 `WP-RT`/`WP-SEC`：待 E3 及 mTLS/partition/mixed-version 证据。
- `WP-CELLD` 与任何 1→N：待对应 SP-E1..E5 结果和 E4 gate；不得修改冻结 D1 RPC。
- `WP-BG` 的多 replica/scale-to-zero：待 SP-E3 分能力解禁；未证明能力保持 resident singleton。
- `WP-ORCH`、`WP-API`、`WP-CONFIG`：待上游 contract/Registry 与对应 phase handoff。
- `WP-WIRE`：最后串行集成；不得提前修改 serve/main wiring。
- `WP-WEB`、`WP-SITE`、`WP-TEST` 与 `WP-ADOPT` 的权威文件写入：待实现与 phase gate；未实现能力不得标 GA，TP/SP 不得提前写入现行 `docs/test-plan.md`。

## 9. 一句“开始开发指令”模板

> **开始 SURGE E0 与条件式 E1：严格按 `docs/plans/SURGE-E0-START-PACK.md` 先执行 contract freeze、只读 consumer audit 和 SP-E1..E6 可执行计划，期间不改产品代码、不做 schema migration、不跑 E0/SP 实验；仅当 E0 verdict=`PASS` 且我/架构 owner 基于当次证据独立完成 Proposed AD-15 正式 approval 后，才按 `SURGE-DESIGN-INDEX.md` 的 WP/DAG 从 E1 的 `WP-CONTRACT`→`WP-REG` 开始产品实现，任一门禁失败立即停止并记录 `no-go` 或 `scope-reduced`。**

这是一条供用户复制的未来授权模板，不是本文作者代用户作出的 approval，也不改变当前禁止开发状态。
