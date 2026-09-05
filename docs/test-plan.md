# cellp 功能验收计划（开发完成门禁）

> **完成定义（M2）：** 本文件所有 `TP-*` → `[x]`，且 `./e2e/scripts/run-all.sh` exit 0。  
> **中间门禁（M1 · Dashboard 开工）：** TP-V0a + TP-API-* + **TP-VE-ALL** 全绿。  
> **设计：** [DESIGN.md](../DESIGN.md) · **计划：** [plans/](./plans/) · **审查：** [plans/REVIEW.md](./plans/REVIEW.md)

## 里程碑

| ID | 名称 | 包含 | 解锁 |
|----|------|------|------|
| **M1** | 后端门禁 | TP-V0a · TP-API-* · TP-VE-* | Phase 4 Dashboard |
| **M2** | 功能完成 | 全部 TP-*（含 UI · V1–V7） | test-plan-phase2 压测 |
| **M3** | 生产压测 | [test-plan-phase2.md](./test-plan-phase2.md) | 生产数据面 sign-off |

**注意：** M2 可在 offshoot **local** tier 达成（AD-4）；RustFS offshoot prod 另需 **TP-V0b**。

---

## 规则

- 每项记录：**日期 · 版本 · 命令 · 输出摘要 · exit code**
- Registry：**SQLite** `cellp-registry.sqlite`（WAL）
- Gateway：**cellpd 内置**；prod **Host**（AD-12，`e2e/lib-ingress.sh`）必测
- Git/CI：外部边界；V7 脚本模拟即可

---

## A. 存储门禁（Phase 0）

### [x] TP-V0a — RustFS × celld 条件写

| 命令 | `e2e/scripts/v0a-celld-diagnose.sh` |
| 通过 | exit 0；输出含 `ok bucket conditional write` |
| 证据 | `docs/evidence/v0a-*.log` |

### [x] TP-V0b — offshoot branch × RustFS S3 全序列

| 命令 | `e2e/scripts/v0d-offshoot-attach.sh` → `e2e/scripts/v0b-offshoot-rustfs.sh` |
| 通过 | 全序列 exit 0；并行 fork 无 CAS 冲突 |
| 证据 | [v0b-pass-report.md](./evidence/v0b-pass-report.md)（2026-08-29 PASS） |
| 阻塞 | **prod offshoot 使用 RustFS**（AD-4 prod tier） |

### [x] TP-V0b-L — 大库物化 fork

| 命令 | `e2e/scripts/v0b-l-large-fork.sh`（`stress/phase6/offshoot-branch-scale.sh` `OB_SUITE=v0bl`） |
| 通过 | ≥100MB 库 fork+export exit 0；证据 `docs/evidence/offshoot-branch-scale-report.md` |

### [x] TP-V0c — RustFS 多节点条件写（可选）

| 跳过 | 单一 VIP → `docs/evidence/v0c-skip.md` |

### [x] TP-V0d — offshoot attach 探针

| 命令 | `e2e/scripts/v0d-offshoot-attach.sh` |
| 通过 | exit 0 |

---

## B. 集成能力（Phase 2 · 依赖 AD-1 多 upstream）

### [x] TP-V1 — offshoot export → celld D1 seed

| 命令 | `e2e/scripts/v1-d1-seed.sh` |
| 通过 | seed 后 Worker 返回 **fixture 计数**（如 `count:42`） |
| 契约 | [D1-IMPORT-RPC.md](./plans/D1-IMPORT-RPC.md) · 证据 `docs/evidence/d1-import-scale-report.md` |

### [x] TP-V2 — Quiesce + checkpoint 一致性 fork

| 命令 | `e2e/scripts/v2-quiesce-fork.sh` |
| 通过 | parent 写入停止后 fork；子 version 不含 fork 后写入 |

### [x] TP-V3 — Gateway 双 version 路由（不同 artifact）

| 命令 | `e2e/scripts/v3-dual-route.sh` |
| 通过 | versionA/B 的 `curl` body **不同**（不同 deploy）；同时 200 |
| 架构 | 每 version 独立 upstream 端口（[REVIEW AD-1](./plans/REVIEW.md)） |

### [x] TP-V4 — Promote 原子 cutover

| 命令 | `e2e/scripts/v4-promote-cutover.sh` |
| 通过 | prod **Host** → 新 prod body；旧 prod preview Host 仍可 200（独立 binding）；无双写窗口 > **2s** |

### [x] TP-V4b — Promote offshoot 硬门禁

| 命令 | `e2e/scripts/v4b-promote-offshoot-fail.sh` |
| 通过 | 注入 offshoot promote 失败 → **非 200** + `offshoot_promote_failed`；`prod_version_id` 与 prod URL body **不变** |

### [x] TP-V5 — Orchestrator 失败补偿

| 命令 | `e2e/scripts/v5-saga-compensate.sh` |
| 通过 | 注入 deploy 失败 → `failed`；Registry 无泄漏 route；offshoot branch 已 GC |

### [x] TP-V5B — Deploy 默认 fail-closed（D1 branch）

| 命令 | `e2e/scripts/v5b-deploy-d1-branch-fail.sh` |
| 通过 | `CELLP_E2E_INJECT_D1_BRANCH_FAIL=1` 下子 version D1 branch 失败 → `failed`；preview 非 200。默认 cellpd **不**设 `CELLP_LENIENT_DEPLOY` |

### [x] TP-V6 — Schema migration × fork 顺序

| 命令 | `e2e/scripts/v6-migrate-order.sh` |
| 通过 | fork → deploy → migrate → health → `ready`；坏 migration → `failed` |

### [x] TP-V7 — 外部 CI 模拟端到端

| 命令 | `e2e/scripts/v7-external-ci.sh` |
| 通过 | upload → POST → ready → preview Host 200 → promote → **prod Host 200** |

### [x] TP-V7-D — Promote 后数据一致

| 通过 | prod URL body 与 offshoot export checksum / fixture 一致 |

---

## C. 端口级 E2E（Phase 3 · M1 门禁）

### [x] TP-VE-1 — 健康检查

| 检查 | `:8790/v1/health` · `:8787/health` · `:8792/.well-known/celld/health` → 200 |

### [x] TP-VE-2 — CD 闭环

| 命令 | `e2e/scripts/ve-cd-loop.sh` |

### [x] TP-VE-3 — Promote 路由

| 命令 | `e2e/scripts/ve-promote.sh` |

### [x] TP-VE-4 — 失败补偿

| 命令 | `e2e/scripts/ve-fail-compensate.sh` |

### [x] TP-VE-5 — Destroy 生命周期

| 命令 | `e2e/scripts/ve-destroy.sh` |
| 通过 | DELETE → `draining` → `destroyed`；Gateway 404 ≤ **120s** |

### [x] TP-VE-ALL — 聚合

| 命令 | `./e2e/scripts/run-all.sh` |
| 通过 | exit 0 · **阻塞 Dashboard（M1）** |

---

## D. API 契约（Phase 1）

### [x] TP-API-1 — Project CRUD

| 通过 | POST/GET `/v1/projects` → 201/200；JSON 含 `id` |

### [x] TP-API-2 — Version 生命周期

| 通过 | POST → **202** + `poll_url`；GET status 字段齐全 |

### [x] TP-API-3 — 鉴权

| 通过 | 错 token → **401**；DEPLOY 调 promote → **403**；ADMIN 调 POST versions → **403** |

### [x] TP-API-4 — SQLite 持久化

| 命令 | `sqlite3 $REGISTRY_DB "PRAGMA journal_mode;"` → `wal` |
| 通过 | 重启 cellpd 后 rows 仍在 |

### [x] TP-API-5 — DELETE / draining

| 通过 | 见 TP-VE-5 |

### [x] TP-API-6 — 非法状态转换

| 通过 | promote on `pending` → **409** |

### [x] TP-API-7 — OpenAPI 契约

| 路径 | `cellp/api/openapi.yaml` |
| 通过 | 与实现一致；`make -C cellp openapi-check` exit 0 |

---

## E. 安全（Phase 1–2）

### [x] TP-SEC-1 — Artifact SSRF

| 通过 | POST body 含 `artifact_uri: http://169.254.169.254/` → 忽略；仅用 `s3://cellp-artifacts/{project}/{version}/` |

### [x] TP-SEC-2 — Token 分离

| 通过 | 见 TP-API-3 |

### [x] TP-SEC-3 — 禁止 fork prod 数据（PR preview）

| 通过 | `parent_version_id=prod` + PR ref → **422** 或 scrubbed seed（D1 ≠ prod 快照） |

### [x] TP-SEC-4 — CD env 不可覆盖平台键

| 通过 | payload 含 `CELLP_*` / `CELLD_REGISTRY` → 拒绝或 strip |

### [x] TP-SEC-5 — celld 不对外暴露

| 通过 | 非 loopback 访问 `:8792` → refused；Gateway `:8787` → 200 |

---

## F. Dashboard（Phase 4 · M1 后）

### [x] TP-UI-1 — Project 列表 `/`

### [x] TP-UI-2 — Version 列表

### [x] TP-UI-3 — 详情 + Promote + Destroy

### [x] TP-UI-4 — 仅消费 API

### [x] TP-UI-5 — Playwright smoke

| 命令 | `cd web && pnpm run test:e2e` |
| 通过 | exit 0 |

### [x] TP-UI-6 — 无直连运行时

| 命令 | `rg ':8792|offshoot' web/` |
| 通过 | 无匹配 |

### [x] TP-UI-7 — Storage hub 徽章（Bindings）

| 检查 | `/projects/{id}/storage` 可见 d1 / kv / queue / workflow / r2 / cron |
| 对齐 | [phase-7-t4-dashboard.md](./plans/phase-7-t4-dashboard.md) Playwright |
| 通过 | `storage-bindings.spec.ts` · `cd web && CI=1 pnpm --filter cellp-dashboard test:e2e`（2026-08-30） |

### [x] TP-UI-8 — KV browser

| 检查 | version KV 页可见 key；PUT 后 list 出现 |
| 对齐 | T4 Playwright · 后端契约 TP-V9 |
| 通过 | `kv.spec.ts` · 2026-08-30 Playwright 绿 |

### [x] TP-UI-9 — Queue 控制台

| 检查 | queues 见 `tasks`；peek 渲染；purge 无确认不得静默清空 |
| 对齐 | T4 Playwright · 后端契约 TP-V10 |
| 通过 | `queues-workflows.spec.ts` · 2026-08-30 Playwright 绿 |

### [x] TP-UI-10 — Workflow 只读

| 检查 | workflows 实例列表；**无** Pause / Resume / Restart |
| 对齐 | T4 Playwright · 后端契约 TP-V11 |
| 通过 | `queues-workflows.spec.ts` · 2026-08-30 Playwright 绿 |

### [x] TP-UI-11 — AD-8 branch 横幅（KV/Queue 继承）

| 检查 | 子 version 横幅可见；KV 继承父 key（如 `greeting`）；Queue peek 继承父 backlog；兄弟 version 写入仍隔离 |
| 对齐 | T4 Playwright · `kv.spec.ts` · `queues-workflows.spec.ts` |
| 通过 | 2026-08-30 Playwright 绿 |

### [x] TP-UI-12 — R2/Cron 无独立浏览器

| 检查 | hub 上 R2/Cron **不是**链到 `/r2` 或 `/cron`；直接 goto 404 或回 hub |
| 对齐 | T4 Playwright |
| 通过 | `storage-bindings.spec.ts` · 2026-08-30 Playwright 绿 |

---

## G. Dev 栈

### [x] TP-DEV-1 — cellpd 替换 mock

| 通过 | `dev/scripts/up-native.sh` + `health.sh` exit 0 |

### [x] TP-DEV-2 — 证据目录

| 通过 | `docs/evidence/` 存在且可写 |

---

## VE vs V1–V7 / V9–V11 分工

| 层 | 用途 | 权威脚本 |
|----|------|----------|
| **TP-VE-*** | 端口烟雾 · M1 门禁 | `ve-*.sh` · `health-all.sh` |
| **TP-V1–V7** | 集成深度 · 数据/路由/saga | `v1-*.sh` … `v7-*.sh` |
| **TP-V9–V11** | Bindings · KV / Queue / Workflow+Cron | `v9-kv.sh` · `v10-queue.sh` · `v11-workflow-cron.sh` |
| **TP-UI-7..12** | Dashboard Bindings（FE） | Playwright · 见 §F · 2026-08-30 绿 |

`run-all.sh` 先跑 VE，再跑 V*（MANIFEST：v7 之后为 v9–v11）。

---

## H. D1 数据面（celld + cellp orchestrator · 2026-08-29）

> 决策摘要：[decisions.md](./decisions.md) §7

### [x] TP-D1-IMP — 二进制 `celld d1 import`（100 MB scale）

| 命令 | `stress/phase6/d1-import-scale.sh` |
| 通过 | import + execute + G3 restore；无 `.dump.sql` |
| 契约 | [D1-IMPORT-RPC.md](./plans/D1-IMPORT-RPC.md) |
| 证据 | `docs/evidence/d1-import-scale-report.md` |

### [x] TP-D1-BRANCH — 子 version `celld d1 branch`

| 命令 | `e2e/scripts/v1-d1-branch.sh` · `stress/phase6/d1-branch-scale.sh` |
| 通过 | B3 行数继承 · B4 父隔离 · B5 kill+wipe restore · B6 子前缀 ≪ 父 snapshot |
| 契约 | [D1-BRANCH-RPC.md](./plans/D1-BRANCH-RPC.md) |
| 证据 | `docs/evidence/d1-branch-e2e-report.md` · `d1-branch-scale-report.md` |

### [x] TP-D1-BRANCH-MULTI — 100 MB × 多 sibling 分支（手动）

| 命令 | `e2e/scripts/v1-d1-branch-multi-100mb.sh` |
| 通过 | 父 100 行；每分支 101；sibling 交叉可见性 0；S3 节省 ~74% |
| 注意 | **不在** `run-all.sh` 默认路径（耗时） |
| 证据 | `docs/evidence/d1-branch-multi-100mb.json` |

---

## I. Bindings（Phase 7 · celld 0.4.0）

> KV / Queue / Workflow / Cron 经 **cellpd `:8790`**。 **E2E 全绿：** `docs/evidence/m2-run-all-20260830-190100.log`（`run-all.sh` · 2026-08-30）。Dashboard 全绿不能代替本表。对齐 [VALIDATION.md V9–V11](../VALIDATION.md) · [phase-7-t5-e2e.md](./plans/phase-7-t5-e2e.md)。

### [x] TP-V9 — celld KV operator（经 cellpd）

| 命令 | `e2e/scripts/v9-kv.sh` |
| 通过 | bindings 含 kv；put/get 200；子 version GET 同 key **404**；父值不变 |
| 证据 | `docs/evidence/v9-kv-e2e.log` · `v9-kv-e2e.json` |
| 对齐 | VALIDATION **V9** |
| 不做 | bulk · inherit（AD-7） |

### [x] TP-V10 — celld Queue operator

| 命令 | `e2e/scripts/v10-queue.sh` |
| 通过 | bindings 含 queue；info/peek 200；purge 无 force → **400** |
| 证据 | `docs/evidence/v10-queue-e2e.log` · `v10-queue-e2e.json` |
| 对齐 | VALIDATION **V10** |
| 缺口 | `celld/examples` 无 queue → 使用 `dev/examples/queue`（producer-only；consumer 不能 `export fetch`） |
| 不做 | pull consumer · 跨 version 共享 queue |

### [x] TP-V11 — Workflow 只读 + Cron 清单

| 命令 | `e2e/scripts/v11-workflow-cron.sh` |
| 通过 | bindings 含 workflow 与 cron；`GET …/workflows/{name}/instances` 不 500 |
| 证据 | `docs/evidence/v11-workflow-cron-e2e.log` · `v11-workflow-cron-e2e.json` |
| 对齐 | VALIDATION **V11** |
| 不做 | workflow 控制 · R2 对象浏览器 · Cron 平台调度 |

### [x] TP-V12 — KV branch（AD-8）

| 命令 | `e2e/scripts/v12-kv-branch.sh` |
| 通过 | 父 put → 子 get 同值；子 put 后父不变 |
| 证据 | `docs/evidence/v12-kv-branch-e2e.log` |

### [x] TP-V13 — R2 branch（AD-8）

| 命令 | `e2e/scripts/v13-r2-branch.sh` |
| 通过 | 子 GET 父对象；子 overwrite 后父不变 |
| 证据 | `docs/evidence/v13-r2-branch-e2e.log` |

### [x] TP-V14 — Queue branch（AD-8）

| 命令 | `e2e/scripts/v14-queue-branch.sh` |
| 通过 | 父 enqueue → 子 peek 见快照 |
| 证据 | `docs/evidence/v14-queue-branch-e2e.log` |

### [x] TP-V15 — Version archive / wake（AD-9）

| 命令 | `e2e/scripts/v15-archive.sh` |
| 通过 | 6+ ready 无 429；archive 非 prod → 503 `version_archived`；wake → 200；archive prod → 422 |
| 证据 | `docs/evidence/v15-archive-e2e.log` |

### [x] TP-V16 — Worker env

| 命令 | `e2e/scripts/v16-worker-env.sh` |
| 通过 | POST env → Worker `env.GREETING`；PUT 后预览更新；平台键不可覆盖 |
| 证据 | `docs/evidence/v16-worker-env-e2e.log` |

### [x] TP-V17 — Cron 仅 prod arm（AD-11）

| 命令 | `e2e/scripts/v17-cron-prod-only.sh` |
| 通过 | 两 ready + 同 wrangler crons；bindings 均可见；90s 内仅 prod celld 日志出现 `e2e-cron-tick` |
| 证据 | `docs/evidence/v17-cron-prod-only-e2e.log` |

### [x] TP-V18 — Promote 换指针非 merge（ISSUE-03）

| 命令 | `e2e/scripts/v17-promote-no-merge.sh` |
| 通过 | 子 version fork 后仅在 prod 写入的行；promote 子 version 后 prod D1 **不含**该行（证明未 merge fork 后 prod 写入） |
| 证据 | `docs/evidence/v17-promote-no-merge-e2e.log` |

### [x] TP-UI-15 — Dashboard 监控与巡检

| 检查 | 项目 **Inspect** 页；Version **Runtime inspection**；Deployments fleet 摘要；Platform 项目过滤 + Gateway 5xx 指标 |
| 命令 | `cd web && pnpm run test`（`inspection.test.ts` · `project-inspect.flow.test.tsx`） |
| 文档 | [dashboard.md](../site/docs/get-started/dashboard.md) · operator-journey §4 Walk |
| 通过 | Vitest 17/17 · **2026-08-31** |

### [x] TP-UI-14 — 用户行为闭环（Dashboard 真栈 + 创建项目）

| 检查 | mock：`create-project.spec.ts`；**Vitest**：`src/flows/*.flow.test.ts`（含 `operator-checklist.flow.test.tsx`）；文档 **Operator checklist**；门禁 `web/scripts/verify-user-loop.sh`；真栈：`pnpm run test:e2e:live` |
| 命令 | `cd web && pnpm run test` · `web/scripts/verify-user-loop.sh` · `./dev/scripts/up.sh` 后 `cd web && pnpm run test:e2e:live` |
| 文档 | [operator-journey.md](../site/docs/get-started/operator-journey.md#operator-checklist) · [user-behavior-closed-loop.md](./plans/user-behavior-closed-loop.md) |
| 通过 | mock + Vitest 绿 + checklist/Overview 引导交付；live 栈未起可 skip · **2026-08-31** |

### [x] TP-UI-13 — Settings Worker env

| 检查 | `/projects/:id/settings` 可编辑生产 version env；Save 走 `PUT …/env` |
| 命令 | `cd web && CI=1 pnpm --filter cellp-dashboard test:e2e` |
| 通过 | `dashboard.spec.ts`「settings edits worker env」· 2026-08-30 |

### TP-VE-1（路径修订，非新 ID）

| 检查 | `:8790/v1/health` · `:8787/health` · `:8792/.well-known/celld/health` → 200 |
| 命令 | `e2e/scripts/health-all.sh` · `dev/scripts/health.sh` |
| 证据 | `docs/evidence/v11-health-path.log` |

---

## 汇总

**M2 完成 = 上表全部 `[x]` → [test-plan-phase2.md](./test-plan-phase2.md)**

---

*test-plan v3 · 2026-08-29 · 含 D1 import/branch 验收 · Bindings TP-V9/V10/V11 + TP-UI-7..12*
