# SURGE Design Options

> 状态：设计拆分选择已闭合；采用方案 A。本文不构成架构决策，新 AD 尚未批准，E0/SP 尚未执行，产品开发未启动。

## 事实分级

- **现行事实**：AD-1/AD-9/AD-10/AD-12/AD-14、SQLite 控制面、RustFS 唯一持久层、celld guarantees、D1 frozen contracts 及 M1/M2 回归继续有效。
- **已确认设计输入**：方案 A、`docs/plans/` 落点、Q1–Q12、失败缩小范围、未来条件性开发门槛。
- **Draft**：Version/Serving 解耦、Registry schema、Activator、endpoint set、Autoscaler 和 E0–E5 rollout；只有新 AD 批准后才能约束实现。
- **unknown**：SP-E1..E6、multi-node/fleet、后台唯一执行、graceful shutdown、冷启动预算及未确认数值阈值。

## 方案 A：共享契约/集成索引 + 组件边界 + 单一写 owner（已采用）

15 篇组件设计覆盖共享契约、Registry、Runtime/Agent、Scheduler、Activator、Gateway、Autoscaler、celld fleet、background、promote/migration、security/observability/operations、API/docs、test orchestration、resilience、adoption。`SURGE-DESIGN-INDEX.md` 是唯一集成索引，`00-shared-contracts-and-ownership.md` 是共享逻辑契约 owner。

| 维度 | 结论 |
|---|---|
| 耦合 | 最低；跨组件只通过登记的 schema/API/snapshot/RPC 接缝 |
| 并发 | WP-CONTRACT 冻结后，Gateway、runtime、policy、ops、test 可按 DAG 并发 |
| 冲突 | Registry schema、OpenAPI、serve wiring、共享配置、权威文档分别由唯一 owner 修改 |
| 顺序 | E0 契约与 spike → E1 本机声明式 0/1 → E2 preview 激活；E3 remote node；SP 闭合后 E4；最后 E5 |
| 测试 | 每 shard 独立 project/version/bucket/port/SQLite/watch/evidence；最终 M1/M2 仍串行 |
| 回滚 | flag=0 前先收敛现行 ready Version 为单 legacy replica，再发布单 endpoint并 drain额外副本 |
| 成本 | 前期接口治理成本较高；共享接口变更必须由契约 owner 串行传播 |

## 方案 B：按 E0..E5 阶段捆绑（拒绝作为主拆分）

阶段内同时包含 Registry、Gateway、runtime、测试与运维，会使 `sqlite.go`、serve wiring、Gateway、OpenAPI 在多个阶段重复争用。E0–E5 适合作为 adoption gates，不适合作为并发开发边界。

## 方案 C：按 SURGE §0..§29 机械切片（拒绝）

章节顺序不是实现依赖顺序；§7/§9/§11/§12 会重复定义 desired/replica/snapshot，形成表面并行、实际共享类型和产品文件冲突，也缺少统一 migration/rollback owner。

## 已确认的并发治理

1. WP-CONTRACT 先冻结术语、逻辑契约、错误枚举和 owner 表。
2. 组件 WP 只写 `SURGE-DESIGN-INDEX.md` 登记的独占路径，不自行修改共享 schema。
3. WP-WIRE 最后串行接线；WP-ADOPT 独占权威 AD/test-plan 文件，且只能在新 AD 获批后执行。
4. 单元/契约/组件测试可并发；e2e 仅独立栈间并发；stress/chaos 独占硬件或串行。
5. `./e2e/scripts/run-all.sh` 保持现行串行 M1/M2 回归，不由新分片替代。

## 仍不可越过的决策缝

Q1–Q12 已闭合，并已固化到通过文本对抗审查的 Proposed AD-15；它仍不是现行产品规范。后续必须完成 E0 和相关 SP，并独立正式批准该 AD；失败能力正式缩小范围。只有新 AD 批准生效、phase gate 通过、ownership/rollback 证据齐备后，才可启动对应 WP。任何路径均不得弱化 RustFS/celld guarantees、跨 Version bucket 隔离、D1 frozen contracts、安全脱敏或 M1/M2。
