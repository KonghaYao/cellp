# SURGE Design Index

> 状态：Draft 设计包；方案 A 与 Q1–Q12 已闭合，Proposed AD-15 已起草并通过文本对抗审查，E0 启动包已就绪，但 AD 尚未正式批准或生效；E0/SP 尚未执行，产品开发未授权。现行 AD、冻结契约和门禁优先。

## 阅读顺序

1. `SURGE-DECISION-BRIEF.md`：已确认决策、风险和授权边界。
2. `SURGE-E0-START-PACK.md`：只差用户明确指令即可执行的 E0 contract freeze、consumer audit 与 SP 计划；当前仍未执行。
3. `SURGE-PROPOSED-AD.md` 与 `SURGE-PROPOSED-AD-REVIEW.md`：待正式批准的架构决定草案及 13/13 finding closure。
4. `SURGE-DESIGN-OPTIONS.md`：方案 A 的拆分理由。
5. `00-shared-contracts-and-ownership.md`：共享术语、逻辑接口和 owner。
6. `01-serving-state-registry.md` 至 `10-security-observability-operations.md`：组件设计，可按 DAG 并行消费。
7. `11-api-config-and-public-docs.md`、`12-test-concurrency-and-evidence.md`：公开 surface 与验证编排。
8. `13-resilience-chaos-runbook.md`、`14-adoption-gates-and-rollback.md`：故障与阶段收口。

## 设计文档清单

| ID | 文档 | 唯一职责 |
|---|---|---|
| 00 | `00-shared-contracts-and-ownership.md` | 共享逻辑契约、错误枚举、owner |
| 01 | `01-serving-state-registry.md` | Version/Serving facts、CAS、revision、snapshot source |
| 02 | `02-runtime-backend-node-agent.md` | transport-neutral lifecycle、HTTP+mTLS Agent |
| 03 | `03-scheduler-capacity-pressure.md` | assignment、capacity、pressure、floor |
| 04 | `04-activator-cold-start.md` | cold request 分类、有界等待、singleflight |
| 05 | `05-gateway-endpoint-serving.md` | endpoint snapshot、LB、LKG、drain |
| 06 | `06-autoscaler-policy.md` | desired、min/max、idle、scale guards |
| 07 | `07-celld-fleet-consistency.md` | multi-node celld guarantees 与 SP |
| 08 | `08-background-workloads.md` | resident detection、singleton guard |
| 09 | `09-promote-archive-migration.md` | promote/archive 兼容、cutover、migration |
| 10 | `10-security-observability-operations.md` | mTLS、审计、OTLP、redaction |
| 11 | `11-api-config-and-public-docs.md` | OpenAPI/config/Dashboard/site future surface |
| 12 | `12-test-concurrency-and-evidence.md` | 测试分片、隔离、证据 |
| 13 | `13-resilience-chaos-runbook.md` | 故障矩阵、chaos、runbook |
| 14 | `14-adoption-gates-and-rollback.md` | E0–E5、授权、rollback |

## 文档依赖 DAG

```text
00 shared contracts
├─01 registry ─┬─03 scheduler ─┬─02 runtime/agent ──07 celld fleet
│              │               └─06 autoscaler
│              ├─04 activator ──05 gateway
│              ├─08 background
│              └─09 promote/migration
├─10 security/observability/ops (cross-cutting)
├─11 API/config/docs (consumes 01..10 public surfaces)
├─12 tests/evidence (consumes every contract)
└─13 resilience/runbook ──14 adoption/rollback
Q1..Q12 confirmed ─► Proposed AD/E0; SP-E1..E6 ─► corresponding phase/E4
```

## SURGE §0–§29 全量映射

| 章节 | 主设计 | 次设计 |
|---|---|---|
| §0–§4 | 00 | 14 |
| §5 | 00 | 01–10 |
| §6–§7 | 01 | 00,11 |
| §8 | 06 | 08 |
| §9 | 04 | 05 |
| §10 | 05 | 04,10 |
| §11 | 06 | 03 |
| §12 | 03 | 01,02 |
| §13 | 02 | 10,13 |
| §14 | 07 | 02,08 |
| §15 | 08 | 07 |
| §16 | 09 | 05,08 |
| §17 | 13 | 01–10 |
| §18–§19 | 10 | 13 |
| §20 | 11 | 00,01 |
| §21 | 09 | 14 |
| §22 | 14 | 全部 |
| §23–§24 | 12 | 对应组件 |
| §25 | 13 | 10,14 |
| §26 | 00 | 14 |
| §27 | SURGE-DECISION-BRIEF | 00–14 |
| §28–§29 | 11,14 | SURGE-DECISION-BRIEF |

## Phase / test / spike 全量映射

| ID | Owner | 层级 | 前置 | 证据路径（Draft） |
|---|---|---|---|---|
| E0 | WP-ADOPT | design/lab | Q1..Q12 已确认、baseline、AD draft/review、SP 计划 | `docs/evidence/surge/e0/<run-id>/` |
| E1 | WP-REG | unit/contract/component/e2e | E0、新 AD 生效、单 active writer | `docs/evidence/surge/e1/<run-id>/` |
| E2 | WP-GW | unit/component/e2e/stress | E1、SP-E6 | `docs/evidence/surge/e2/<run-id>/` |
| E3 | WP-RT | contract/e2e/chaos | E1、HTTP+mTLS 安全契约 | `docs/evidence/surge/e3/<run-id>/` |
| E4 | WP-CELLD | component/e2e/stress/chaos | 对应 SP-E1..E6 PASS | `docs/evidence/surge/e4/<run-id>/` |
| E5 | WP-OPS | e2e/stress/chaos/docs | E1..E4 | `docs/evidence/surge/e5/<run-id>/` |
| TP-E1,E5,E6,E7 | WP-REG | unit/contract/component/e2e | E1 schema/generation | `.../tp-e/{id}/<run-id>/` |
| TP-E2,E3 | WP-GW | unit/component/e2e | Activator contract | `.../tp-e/{id}/<run-id>/` |
| TP-E4 | WP-RT | component/e2e | drain contract | `.../tp-e/tp-e4/<run-id>/` |
| TP-E8 | WP-ORCH | component/e2e | readiness/CAS | `.../tp-e/tp-e8/<run-id>/` |
| TP-E9,E10 | WP-BG | contract/e2e | SP-E3、static guard、singleton | `.../tp-e/{id}/<run-id>/` |
| TP-EP1 | WP-GW | stress | warm baseline | `.../tp-ep/tp-ep1/<run-id>/` |
| TP-EP2,EP6 | WP-GW | stress | SP-E6、budgets | `.../tp-ep/{id}/<run-id>/` |
| TP-EP3,EP7 | WP-SCALE | stress | E4/SP-E1 | `.../tp-ep/{id}/<run-id>/` |
| TP-EP4,EP8 | WP-SCALE | component/stress | drain/pressure floor | `.../tp-ep/{id}/<run-id>/` |
| TP-EP5 | WP-REG | stress | global budget | `.../tp-ep/tp-ep5/<run-id>/` |
| TP-EC1,EC4 | WP-CELLD | chaos | SP-E2/SP-E5 | `.../tp-ec/{id}/<run-id>/` |
| TP-EC2,EC5 | WP-RT | chaos | E3 leases/mTLS | `.../tp-ec/{id}/<run-id>/` |
| TP-EC3 | WP-REG | chaos | revision/snapshot LKG | `.../tp-ec/tp-ec3/<run-id>/` |
| TP-EC6 | WP-GW | stress/chaos | cold budgets | `.../tp-ec/tp-ec6/<run-id>/` |
| TP-ES1,ES2 | WP-SEC | contract/e2e | Agent HTTP+mTLS/listener | `.../tp-es/{id}/<run-id>/` |
| TP-ES3,ES4 | WP-SEC | unit/e2e | redaction/Host | `.../tp-es/{id}/<run-id>/` |
| TP-ES5 | WP-GW | unit/stress | body budgets | `.../tp-es/tp-es5/<run-id>/` |
| SP-E1 | WP-CELLD | lab/stress | dual celld fixture | `docs/evidence/surge/sp-e1/<run-id>/` |
| SP-E2 | WP-CELLD | lab/chaos | SP-E1 | `docs/evidence/surge/sp-e2/<run-id>/` |
| SP-E3 | WP-CELLD | lab/contract | SP-E1；每类 background uniqueness | `docs/evidence/surge/sp-e3/<run-id>/` |
| SP-E4 | WP-RT | lab/chaos | shutdown/WS fixture | `docs/evidence/surge/sp-e4/<run-id>/` |
| SP-E5 | WP-CELLD | lab/chaos | V0a + non-skipped V0c | `docs/evidence/surge/sp-e5/<run-id>/` |
| SP-E6 | WP-GW | lab/stress | restore matrix | `docs/evidence/surge/sp-e6/<run-id>/` |

上述 TP/SP 仍不是当前 `docs/test-plan.md` 门禁；现有 M1/M2 `run-all.sh` 继续串行。spike FAIL 必须形成 `no-go` 或 `scope-reduced`。

## 未来开发 WP 与唯一产品写入范围

| WP | 唯一产品范围 | 共享文件规则 | 设计 |
|---|---|---|---|
| WP-CONTRACT | 新共享 contract/types 包 | 唯一 owner；其他包只消费 | 00 |
| WP-REG | `cellp/internal/registry/` | `sqlite.go`,`store.go` 独占 | 01 |
| WP-RT | `cellp/internal/runtime/` | RuntimeBackend/Agent 独占 | 02 |
| WP-SCHED | 新 `cellp/internal/elastic/scheduler/` | 不改 Registry schema | 03 |
| WP-GW-ACT | Gateway activator 新文件 | 不改 warm proxy wiring | 04 |
| WP-GW | `cellp/internal/gateway/` warm/snapshot | `gateway.go` 集成仅 WP-WIRE | 05 |
| WP-SCALE | 新 autoscaler/policy 包 | 只经 Store contract 写 desired | 06 |
| WP-CELLD | `celld/` | submodule 单 owner；不得改 D1 frozen RPC | 07 |
| WP-BG | background policy/orch 新文件 | Cron/Queue 共享入口单 owner | 08 |
| WP-ORCH | `cellp/internal/orch/` migration/promote | promote/archive 文件独占 | 09 |
| WP-OPS | metrics/operations/runbook | AD-14 schema 单 owner | 10,13 |
| WP-SEC | Agent HTTP+mTLS middleware/cert lifecycle | security contract 单 owner | 02,10 |
| WP-API | `cellp/internal/api/`, `cellp/api/openapi.yaml` | OpenAPI 唯一 owner | 11 |
| WP-CONFIG | 配置包、`dev/.env.example` | config 共享文件唯一 owner | 11 |
| WP-WEB | `web/` | 不直连 celld/Agent | 11 |
| WP-SITE | `site/` | 产品行为获批并实现后才写 | 11 |
| WP-TEST | 未来 `e2e/surge/`, `stress/elastic/` | MANIFEST/test-plan 仅 gate owner | 12 |
| WP-WIRE | `cellp/internal/serve/`、CLI main wiring | 最后串行集成 | 14 |
| WP-ADOPT | `DESIGN.md`,`docs/decisions.md`,`docs/test-plan.md`,`VALIDATION.md` | 每个权威文件唯一 owner，新 AD 获批后才写 | 14 |

## 并发测试分片与隔离

- 每 shard 使用唯一 `RUN_ID`；project=`surge-<shard>-<run>`，version=`v-surge-<case>-<run>`，bucket 前缀=`surge/<shard>/<run>/`。
- 每 shard 独立 SQLite、watch/temp 目录与不重叠端口块；端口由 `BASE_PORT + shard*100` 分配并在启动前探测，不复用默认 dev 栈。
- evidence=`docs/evidence/surge/<gate-or-spike>/<run-id>/`；原始日志、脱敏 env、版本、硬件、policy、metrics 分离，禁止共享覆盖。
- unit/contract/component 可并发；e2e 仅在独立栈间并发；stress/chaos 独占硬件或串行。
- 最终 M1/M2 `./e2e/scripts/run-all.sh` 在标准栈串行执行；并发分片不能替代它。

## 当前 Gate

设计拆分、Q1–Q12 决策、Proposed AD 起草、文本对抗审查和 E0 启动包均已完成。当前 blocker 是正式 AD approval 未发生，E0 contract freeze/consumer audit 尚未执行，SP-E1..E6 无运行证据。下一合法动作是由用户按 `SURGE-E0-START-PACK.md` 明确授权启动 E0；在 E0 verdict=`PASS` 且用户/架构 owner 完成独立正式 approval 前，不得启动产品开发。approval 后也只能按 phase gate、WP ownership、DAG 和 rollback 证据开放对应 WP。
