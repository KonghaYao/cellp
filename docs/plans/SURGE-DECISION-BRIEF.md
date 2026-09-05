# SURGE Decision Brief

> 状态：设计决策已闭合；方案 A、文档落点、Q1–Q12 与三项总体授权已确认。Proposed AD-15 已起草并以 13/13 finding `CLOSED` 通过文本对抗审查，E0 启动包已就绪；AD 仍未正式批准或生效，E0/SP 和产品开发均未启动。

## Confirmed Environment Facts

- `SURGE-SERVERLESS.md` 是 Draft、未生效；现行 AD 和门禁继续优先。
- AD-1 保持每个现行 ready Version 独立 celld 进程与 bucket；AD-9 保持 archived 显式 wake；AD-10、AD-12、AD-14 边界不变。
- SQLite 是当前控制面持久存储；RustFS 是唯一持久层，`CELLD_WATCH` 可丢弃；生产启动仍须 `celld diagnose`。
- celld 现有 ownership/fencing/durability guarantees 与 D1 frozen RPC 不得弱化。
- 当前 `docs/test-plan.md` 不包含 TP-E/EP/EC/ES 或 SP-E1..E6；现有 M1/M2，尤其 `./e2e/scripts/run-all.sh`，保持串行门禁。

## Recommended Outcome

已确认采用方案 A：按共享契约和组件边界形成 15 篇 Draft，以单一文件 owner 和依赖 DAG 支持并发开发，以独立 project/version/bucket/port/SQLite/watch/evidence 支持并发测试。E0–E5 仅作为串行 adoption gates。该确认已用于起草 [Proposed AD-15](./SURGE-PROPOSED-AD.md)，其[文本对抗审查](./SURGE-PROPOSED-AD-REVIEW.md)已收口；[E0 Start Pack](./SURGE-E0-START-PACK.md)也已把 contract freeze、consumer audit、SP-E1..E6 计划和 E1 启动顺序收敛为待执行清单。这不批准 AD、不表示 E0/SP 已执行，也不启动产品实现。

## Decisions Only the User Can Make

以下事项已经逐项闭合，不再是未决问题；Q1–Q5 由用户直接确认，Q6–Q12 由用户授权的 fable subagent 裁决：

| ID | 已确认设计输入 | 直接影响 |
|---|---|---|
| Q1-B | 保留现有 `ready` 语义，新增 `deploy_ready` 表示 artifact、数据和 bindings 已准备且可启动 | Registry、Activator、migration、API |
| Q2-A | preview idle timeout：本地 dev 5 分钟，生产 preview 15 分钟 | Autoscaler、config、docs |
| Q3-A | 第一阶段 prod 禁止 `min_replicas=0`，至少保留 1 个 replica | Policy、validation、migration |
| Q4-A | GET/HEAD/小 body 可有界同步等待；大 body、流式、WebSocket 快速 `503 + Retry-After`；可能已送达 upstream 的非幂等请求不重放 | Activator、Gateway、安全测试 |
| Q5-B | 本机和远程 Node Agent 统一 HTTP+mTLS，逻辑命令保持 transport-neutral | Runtime、security、E3 |
| Q6-A | E1/E2 仅单 active `cellpd` 持有 Scheduler/Reconciler 写权；V12 再设计 HA 选主 | Registry、Scheduler、operations |
| Q7-B | 第一阶段不增加 `min_durability_nodes=2`；serving demand 与 durability health 分离 | Scheduler、celld fleet、policy |
| Q8-A | 普通 scale-down 不主动断开 live WebSocket；紧急 node drain或显式 operator 操作除外 | Gateway、Autoscaler、runbook |
| Q9-A | 静态 workload manifest/bindings 为主要检测源；未知或不能证明 HTTP-only 时 fail-closed 为 `resident_required` | Background、policy、deploy |
| Q10-A | pressure 可回收 pinned 中超过 operator `min_replicas` 的 idle replica，但不得突破 floor或抢占 active work | Scheduler、Autoscaler、runbook |
| Q11-A | celld fleet 未分别证明全局协调、接管和重复语义前，background Version `min>=1,max=1` | celld、Background、SP-E3 |
| Q12-A | E1/E2 使用 SQLite revision polling + Gateway 进程内不可变 snapshot 原子替换；严格递增、校验、LKG，首次无快照 fail-closed | Registry、Gateway、migration |

总体授权同样已闭合：采纳 SURGE 方向并可另行起草新 AD；E0/SP 失败时正式缩小最小受影响范围，不得绕过 guarantees；只有新 AD 正式批准生效、E0/phase gate、ownership 和 rollback 证据齐备后，才条件性允许未来开发。当前不授权产品开发。

## Options and User-visible Consequences

| 决策域 | 已选边界 | 用户可见后果 |
|---|---|---|
| Version 状态 | additive `deploy_ready`，保留 `ready` | API/UI 最终需 additive 迁移；旧 `ready` 客户端语义不被偷换 |
| preview idle | 5m / 15m | 空闲后可归零并承担冷启动；background/WebSocket guard 优先 |
| prod | 第一阶段 `min>=1` | 不提供 prod scale-to-zero，降低首访与后台事件风险 |
| 冷请求 | 分级有界等待 | 小请求可等待；大/流式/WS 客户端需按 `Retry-After` 重试 |
| Agent | 统一 HTTP+mTLS | 本机也承担认证和证书生命周期要求；无认证快捷路径 |
| controller | E1/E2 单 active writer | 第一阶段不承诺 controller HA；standby/API/Gateway 可多实例但不写调度状态 |
| durability | 无额外 node floor | 正确性继续依赖 RustFS/celld guarantees，不把 serving 副本包装成 durability |
| WebSocket | 普通缩容不主动断开 | 长连接可能延迟缩容；紧急 drain 需单独运维契约 |
| background | 静态、unknown fail-closed、未证实时 singleton | 安全优先，后台 Version 资源下限更高，暂不允许多 replica |
| pressure | 可缩到 operator floor | floor 以上容量可回收；floor 后以 admission/load shedding 保护节点 |
| snapshot | SQLite poll + atomic LKG | 第一阶段简单可审计；跨节点低延迟协议留待有证据时设计 |

## Recommended Defaults

只有 Q2 的 5m/15m、Q3 的 prod `min>=1`、Q11 的未证实 background `min>=1,max=1` 是本任务已确认的设计默认/硬边界。cold wait timeout、body/queue budget、concurrency target、stabilization、pressure 阈值等数值仍须 SP-E6 和目标硬件基线校准，不得从原 Draft 直接固化。

## Risks That Need Explicit Acceptance

已接受的治理边界是：自有容量耗尽允许有界 `503`；E1/E2 接受单 active writer 而非提前宣称 HA；feature flag 回滚必须先安全收敛，不能瞬时关闭 controllers；SP-E1..E6 失败会形成 `no-go` 或 `scope-reduced`。仍未知的是 multi-node celld、后台唯一执行、graceful drain、证书生命周期和具体 SLO；这些未知项不能由本 brief 视为 PASS。

## Current Authorization Boundary

- Proposed AD-15 已起草并通过文本对抗审查；`APPROVE-AS-PROPOSED` 不等于正式批准或生效。
- [E0 Start Pack](./SURGE-E0-START-PACK.md) 已就绪，但只是一份待用户授权执行的 contract/audit/SP 计划，不表示 E0 或任何 SP 已开始或通过。
- 新 AD 未正式批准生效前，现行 AD/冻结契约优先。
- 本任务不启动 E0/SP、不修改产品代码、OpenAPI、数据库、`DESIGN.md`、`docs/decisions.md`、`docs/test-plan.md` 或站点。
- 后续实现必须按 `SURGE-DESIGN-INDEX.md` 的 WP/DAG、唯一 owner、gate 和 rollback 顺序执行。
