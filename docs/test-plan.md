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
- Gateway：**cellpd 内置**；prod 路径 `/{project}/` 必测
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
| 通过 | `/{project}/` → 新 prod body；旧 prod 显式 version 路径 **404 或 410**；无双写窗口 > **2s** |

### [x] TP-V5 — Orchestrator 失败补偿

| 命令 | `e2e/scripts/v5-saga-compensate.sh` |
| 通过 | 注入 deploy 失败 → `failed`；Registry 无泄漏 route；offshoot branch 已 GC |

### [x] TP-V6 — Schema migration × fork 顺序

| 命令 | `e2e/scripts/v6-migrate-order.sh` |
| 通过 | fork → deploy → migrate → health → `ready`；坏 migration → `failed` |

### [x] TP-V7 — 外部 CI 模拟端到端

| 命令 | `e2e/scripts/v7-external-ci.sh` |
| 通过 | upload → POST → ready → preview → promote → **`/{project}/` 200** |

### [x] TP-V7-D — Promote 后数据一致

| 通过 | prod URL body 与 offshoot export checksum / fixture 一致 |

---

## C. 端口级 E2E（Phase 3 · M1 门禁）

### [x] TP-VE-1 — 健康检查

| 检查 | `:8790/v1/health` · `:8787/health` · `:8792/__celld/health` → 200 |

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

| 命令 | `cd web && npm run test:e2e` |
| 通过 | exit 0 |

### [x] TP-UI-6 — 无直连运行时

| 命令 | `rg ':8792|offshoot' web/` |
| 通过 | 无匹配 |

---

## G. Dev 栈

### [x] TP-DEV-1 — cellpd 替换 mock

| 通过 | `dev/scripts/up-native.sh` + `health.sh` exit 0 |

### [x] TP-DEV-2 — 证据目录

| 通过 | `docs/evidence/` 存在且可写 |

---

## VE vs V1–V7 分工

| 层 | 用途 | 权威脚本 |
|----|------|----------|
| **TP-VE-*** | 端口烟雾 · M1 门禁 | `ve-*.sh` |
| **TP-V1–V7** | 集成深度 · 数据/路由/saga | `v1-*.sh` … `v7-*.sh` |

`run-all.sh` 先跑 VE，再跑 V*（见 phase-3 manifest）。

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

## 汇总

**M2 完成 = 上表全部 `[x]` → [test-plan-phase2.md](./test-plan-phase2.md)**

---

*test-plan v3 · 2026-08-29 · 含 D1 import/branch 验收*
