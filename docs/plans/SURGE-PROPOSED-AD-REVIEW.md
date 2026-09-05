# SURGE Proposed AD 对抗审查

> **审查对象：** [Proposed AD-15 — Elastic Serving Fleet 与安全 Scale-to-Zero](./SURGE-PROPOSED-AD.md)  
> **初审裁决：** `APPROVE-WITH-CHANGES`  
> **修订复核：** `APPROVE-AS-PROPOSED`  
> **状态边界：** 文本对抗审查通过；AD 仍为 Proposed、未正式批准、未生效、不授权开发  
> **日期：** 2026-09-05

## 1. Scope and Method

本审查针对 Proposed AD 与现行 AD-1..14、冻结 D1 RPC、Version 生命周期、branch/archive/wake、promote/Cron、Gateway、celld/RustFS guarantees 和现行 M1/M2 门禁之间的接缝进行对抗检查。审查读取了相关权威文档和既有实现以验证兼容风险，但未修改产品代码、`DESIGN.md`、`docs/decisions.md`、`docs/test-plan.md`、冻结契约或站点。

独立审查线程：`01a06f77-d41d-7412-82bc-8fee0cf8e0a2`。复核在同一线程完成，以保留 finding 上下文和修订闭环。

`APPROVE-AS-PROPOSED` 只表示当前 Proposed 文本可以进入后续正式审批准备；它不等于 AD-15 已获正式批准或写入 `docs/decisions.md`，也不表示 E0/SP、schema migration、产品开发或上线已获授权。

## 2. Initial Verdict

**初审：`APPROVE-WITH-CHANGES`。** 方向与已闭合的 Q1–Q12 基本一致，但必须先关闭状态兼容、生命周期排序、controller fencing 和证据边界问题。

| Severity | Count | Finding IDs |
|---|---:|---|
| CRITICAL | 1 | C1 |
| HIGH | 6 | H1–H6 |
| MEDIUM | 6 | M1–M6 |
| LOW / informational | 4 | L1–L4；不纳入阻塞 closure 计数 |

## 3. Finding Closure

以下行号为 2026-09-05 修订复核时的位置；章节和稳定语义锚点是长期引用依据。

| ID | Severity | 初审问题 | 修订关闭依据 | Closure |
|---|---|---|---|---|
| C1 | CRITICAL | elastic 父 Version 从 `ready` 缩到 `deploy_ready+cold` 后，现行父状态校验可能拒绝 D1/KV/R2/Queue branch，与 AD-8 冲突。 | Proposed AD §2.1（复核时 46 行）：明确 `ready`、`deploy_ready`、`archived` 的 branch-source eligibility；要求 Registry 证明 fork 元数据、bucket、LTX/overlay anchor 与 binding identity，cold branch 直接读 RustFS，证明缺失 fail-closed，且不修改冻结 D1 RPC。 | CLOSED |
| H1 | HIGH | AD-9 自动 reaper 与 elastic preview 的 5m/15m idle-to-cold 双轨没有优先级，可能混淆 cold 与 archived。 | §2.1 模式表及其后约束（48–57 行）：elastic enrollment 排除 AD-9 自动 reaper，cold 不自动转 archived，operator 仍可显式 archive。 | CLOSED |
| H2 | HIGH | `POST wake` 与 `deploy_ready+cold` HTTP Activator 的边界不明确，可能破坏 TP-V15/OpenAPI 语义。 | §2.1 模式表（50–57 行）：`POST wake` 只接受 archived；cold 由 Activator 处理，wake 不是 cold Activator 的别名。 | CLOSED |
| H3 | HIGH | promote 未闭合 AD-5 compensate、`offshoot_promote`、prod CAS、snapshot revision、Gateway 收敛与 AD-11 Cron reconcile 顺序。 | §2.10（175–183 行）：冻结七步顺序；CAS、route 投影和 revision 同事务；Gateway 维持 LKG 到新 revision；旧 Cron owner 解除后才可 arm 新 owner；失败不得宣称完成。 | CLOSED |
| H4 | HIGH | 单 active `cellpd` 只有运维假设，没有机器可验证的双 active fail-closed 契约。 | §2.2（77–79 行）：要求进程生命周期 singleton guard；失去 guard 即拒绝 Scheduler/Reconciler 写循环和 Start/Drain/Stop；误双 active 至少一方 fail-closed；E0/E1 必须冻结并验证。 | CLOSED |
| H5 | HIGH | deploy qualification 与现行“poll `ready` 后 preview 200”终态未钉死，可能短暂暴露未完成 endpoint。 | §2.1（39、42–44 行）：`ready` 必须有已进入有效 snapshot 的 endpoint；qualification 完成 health、route verify、snapshot 后才进入对外 CD 终态，失败走现有失败路径。 | CLOSED |
| H6 | HIGH | promote 对 elastic `deploy_ready`-only/cold 等状态的非法态处理不明确。 | §2.10 validate（177 行）：目标必须为 `ready` 且有效 endpoint 已在 snapshot；`deploy_ready`-only、cold、archived、failed 均 fail-closed，保持 TP-API-6。 | CLOSED |
| M1 | MEDIUM | Activator 的“小 body”和内存上限可能被误读为已冻结数值。 | §2.6（134 行）：明确不冻结 body、queue、deadline、`Retry-After` 数值，交由 SP-E6/E2 在目标硬件校准并受全局硬上限约束。 | CLOSED |
| M2 | MEDIUM | endpoint lease 续租 owner 及 active writer 失联时 LKG 的截止行为未定义。 | §2.5（116 行）：只有持 singleton guard 的 active writer 可续租；LKG 不得越过 `valid_until`；全部 endpoint 过期后有界 fail-closed。 | CLOSED |
| M3 | MEDIUM | rollback kill switch 名称与 adoption 设计没有对齐。 | §5（224 行）：统一为 `CELLP_ELASTIC_RUNTIME=0`，并规定只有完成安全收敛后才能切换。 | CLOSED |
| M4 | MEDIUM | AD-9 `previous_prod` rollback protection 与 elastic old-prod idle/drain 的关系未定义。 | §2.10（185 行）：旧 prod 至少保留一个 ready replica 60 分钟或更长 `rollback_keep`，保护期内不受 preview idle scale-to-zero。 | CLOSED |
| M5 | MEDIUM | 非 prod bundle 仅声明但未 arm 的 Cron 是否强制 `resident_required` 存在歧义。 | §2.8（153 行）：依 AD-11 未 arm 的非 prod Cron 不单独触发 resident；其他 resident/unknown workload 仍 fail-closed。 | CLOSED |
| M6 | MEDIUM | E1/E2 SQLite polling 的 Gateway/controller 拓扑假设未明确。 | §2.5（120 行）：默认 Gateway 与唯一 active controller 同宿主读取同一权威 SQLite；排除共享文件系统复制、异步 SQLite 副本和跨节点挂载。 | CLOSED |

**Closure 汇总：13/13 CLOSED；0 OPEN；0 PARTIALLY CLOSED。**

## 4. Final Verdict

**修订复核：`APPROVE-AS-PROPOSED`。** C1、H1–H6、M1–M6 均在 Proposed AD 层获得规范性关闭依据，未发现仍阻塞文本进入正式审批准备的 critical/high finding。

该裁决不改变以下事实：

- Proposed AD-15 仍为 `PROPOSED — NOT EFFECTIVE`；
- `docs/decisions.md` 与 `DESIGN.md` 仍是现行权威，尚未加入 AD-15；
- E0/SP 尚未执行，TP-E/EP/EC/ES 与 SP-E1..E6 尚未纳入现行 `docs/test-plan.md`；
- 产品代码、schema migration、dev 栈、产品测试和上线仍未授权；
- 正式批准仍须满足 Proposed AD §8 的 Preconditions，并通过独立批准动作。

## 5. Residual Gate Risks

这些不是未关闭的文本 finding，而是必须由后续 gate 提供证据的实施前置：

1. E0 冻结 `deploy_ready` API/schema、legacy consumer audit、singleton guard 和人工接管协议。
2. E0/SP 证明 Node Agent HTTP+mTLS identity、轮换、吊销、replay protection 和 mixed-version 行为。
3. SP-E1..E5 证明 celld/RustFS owner takeover、epoch/fencing、partition、durability 和 background uniqueness；失败必须 `no-go` 或 `scope-reduced`。
4. SP-E6/E2 校准 cold request、queue/body/deadline、capacity 和 pressure 数值，不得由 Proposed 文本伪造。
5. 正式批准和每个 E1–E5 phase gate 必须分别发生，不得以本审查 verdict 越级。
