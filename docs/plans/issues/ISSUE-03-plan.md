# ISSUE-03 实现计划：Preview 快照语义与 Promote 非合并

> 只读调研结论 · 2026-08-31  
> 关联 issue：[ISSUE-03-preview-snapshot-semantics.md](./ISSUE-03-preview-snapshot-semantics.md)

## 背景摘要（现状）

| 维度 | 代码/文档现状 | 与用户心智的差距 |
|------|----------------|------------------|
| **数据 fork** | 子 version 在 orchestrator `branching` 阶段对 offshoot + celld（D1/KV/R2/Queue）做 branch；D1 在父 LTX 的 `fork_txid` 处切断（`celld` / `docs/plans/D1-BRANCH-RPC.md`） | 用户常以为 preview =「当前 prod 代码 + 实时 prod 数据」 |
| **Promote** | `orch.Orchestrator.Promote`：drain 旧 prod → `branch.Promote`（offshoot，失败仅 warn，见 ISSUE-01）→ CAS `prod_version_id` → 激活路由；**不**合并 prod 在 fork 之后的写入到子 bucket | 用户常以为 promote = Git merge / 把 prod 期间新数据并入 preview |
| **API** | `GET /v1/projects/{p}/versions/{v}` 直接序列化 `registry.Version`，已有 `parent_version_id`、`ready_at`；**无** `fork_txid` / `forked_at` 持久化 | issue 要求可观测性：至少文档化已有字段 |
| **DESIGN** | §8.3 与 AD-8 一致（子 version branch）；§8.1 **P3** 仍写「KV/R2/Queue 子 version 空起步」→ **与 AD-8 矛盾** | 内部顶层设计自相矛盾 |
| **site** | `concepts/preview.md`、`promote.md`、`build/data.md` 有 fork/指针叙述，**缺少**「时间线 + 与 Git 分支对比 + fork 后 prod 写入不合并」的集中说明 | 验收要求专门一节 |
| **Dashboard** | `version-detail-view.tsx` 展示 `parent_version_id` 与 “Preview branch” badge；`ad7-banner.tsx` 只讲 Workflow/Cron 不 branch | 缺少 promote 非 merge / 快照时刻的短文案 |

**非目标（issue 已声明）：** rebase/merge 实现、改 promote 合并语义、D1 compact、二层 branch。

---

## 1. 范围与风险

### 1.1 本期范围（ISSUE-03）

1. **用户文档（`site/`）**：新增或扩展一节，讲清数据时间线、promote 语义、与 Git 分支差异。
2. **`DESIGN.md`**：修正 §8.1 P3，与 §8.3 / `docs/decisions.md` AD-8 对齐。
3. **API 可观测性**：优先 **文档化** `parent_version_id`；评估并可选实现低成本 `forked_at`；**defer** `fork_txid` 与 S3 `base.json` 探测。
4. **Dashboard**：在 **version 详情**（`VersionDetailView`）增加一处与 site 文案一致的英文说明（仓库无 i18n；「中/英一致」= 语义与 site 英文一致，中文可放在本 plan / 内部 ADR，不强制 site 双语）。
5. **测试**：文档类改动 + 可选契约/e2e 证明「fork 后 prod 写入在 promote 后不可见」。

### 1.2 风险

| 风险 | 说明 | 缓解 |
|------|------|------|
| 文案仍像「实时 prod」 | `build/data.md` L13「promote → that version's data becomes production」易被读成合并 | 在同一节显式写 **cutover 指针**，并举例 fork 后 prod 订单不进 preview |
| 新增 API 字段承诺过重 | `fork_txid` 仅在 celld owner 内决定，cellp sqlite **未存** | 一期不暴露 `fork_txid`；用 `parent_version_id` + 文档中的 `fork_txid` 概念 |
| `forked_at` 精度 | 若用 `ready_at` 近似，晚于真实 branch 完成时刻 | 若加字段，应在 `StatusBranching` 结束写入，而非复用 `ready_at` |
| 与 ISSUE-01 交叉 | promote saga 行为变更属 ISSUE-01 | ISSUE-03 只改叙事与可观测性，不依赖 offshoot 硬门禁 |
| StoragePage 误导 | R2 tooltip「inherit parent via branch」未强调 **快照** | 可选微调 tooltip，非验收硬性项（issue 点名的是 version 详情） |

### 1.3 与 ISSUE-04 的边界（一期）

ISSUE-04（多 ready version 时 Cron N 倍触发）与 ISSUE-03（数据快照 / promote 非 merge）**产品叙事相关但实现分离**。

| 项 | ISSUE-03（本期） | ISSUE-04（另 issue） |
|----|------------------|----------------------|
| **文档** | preview 数据在 fork 时刻固定父状态；promote 不合并 prod 后续写入 | preview 与 **Cron 副作用**（仅 prod arm / env 标志等） |
| **代码** | 无 celld cron 调度改动 | orchestrator `Start` / wrangler `triggers.crons` reconcile |
| **site** | `concepts/preview.md`、`promote.md`、`build/data.md`；Cron 仅 **交叉链接** 到 `bindings/cron.md` | `site/docs/bindings/cron.md` + `decisions.md` 决策条文 |
| **defer** | 不在 ISSUE-03 实现「仅 prod arm cron」 | ISSUE-04 若一期只能 **决策 + 文档**，则 **代码改动 defer 到 ISSUE-04 二期**；ISSUE-03 文档可加一句：「多个 ready version 各自可能触发 cron，见 ISSUE-04 / cron 文档」 |

---

## 2. 具体文件与函数改动清单

### 2.1 文档：`site/`（验收：用户文档一节）

**推荐结构（二选一，优先 A）：**

- **A.** 在 `site/docs/concepts/preview.md` 增加 `## Data snapshot timeline`（及 `## Not like Git`），并更新 `site/docs/concepts/promote.md` 增加 `## What promote does not do`。
- **B.** 新建 `site/docs/concepts/data-snapshot.md`，在 VitePress sidebar（`site/docs/.vitepress/config.ts`）Concepts 下挂链；`preview.md` / `promote.md` / `build/data.md` 互链。

**必须写清的不变量（建议正文要点）：**

1. 创建子 version 时，`parent_version_id` 指向 **数据父**；branch 在 deploy 流水线中完成，父 celld bucket 在 **`fork_txid` 时刻**的状态对子只读可见（之后父上的写入子 **看不见**）。
2. **Production** 仍是独立 version/bucket；fork 之后 prod 上的写入 **不会** 在 promote 时合并进子 version。
3. **Promote** = 将 `prod_version_id` 与网关 prod 路由切到 **该 ready version 已有 bucket**（saga + CAS），不是 SQL/KV merge，不是 rebase。
4. **与 Git**：Git branch 可反复 merge/rebase；cellp 子 version 是 **一次性快照 + 独立写集**，promote 是 **换 prod 指针**，无自动把「另一条线上的提交」合并进来。
5. 典型 PR：parent = **staging seed**，不是 live prod（与现有 `versions.md` / TP-SEC-3 一致）。

**同步修订（避免矛盾）：**

| 文件 | 改动 |
|------|------|
| `site/docs/build/data.md` | 「Promote and rollback」小节补充非 merge + 时间线例子 |
| `site/docs/how-it-works.md` | 「Promote, without magic」下加 1 段指针语义 |
| `site/docs/reference/api.md` | `GET …/versions/{v}` 字段表：`parent_version_id` = 数据 fork 父版本 |
| `site/docs/guides/ci.md` | PR 流水线一句：preview 不含 promote 之后 prod 的数据 |

### 2.2 文档：`DESIGN.md`（验收：修正 P3）

| 位置 | 改动 |
|------|------|
| §8.1 原则表 **P3** 行（约 L451） | 将「子 version KV/R2/Queue 空起步」改为与 AD-8 一致：**子 version 经 `celld * branch` CoW；仅 Workflow/Cron/Worker 脚本不 branch** |
| §8.3 标题「为何空起步」 | 可改为「Version 数据面」或保留标题但在表前一句：**根 version** 空起步；**子 version** 见下表 branch |
| 可选 | §6 CD 用户故事下 Promote 一行加注「不切 merge 子与 prod 增量」 |

**不修改冻结契约：** `docs/plans/D1-*-RPC.md`。

### 2.3 文档：`docs/decisions.md`（可选，非 issue 硬性）

- 在 AD-8 或 Bindings 摘要加 **产品不变量**  bullet：preview = fork 时刻父快照；promote = prod 指针切换。便于内部与 site 同源。

### 2.4 API：`cellp/`（验收：GET version 或 bindings）

**现状：** `handleGetVersion`（`cellp/internal/api/server.go`）`writeJSON(w, http.StatusOK, v)`；`Version.ParentVersionID` 已 JSON 为 `parent_version_id`。

**推荐分期：**

| 方案 | 工作量 | 建议 |
|------|--------|------|
| **仅文档** | 低 | **一期默认**：`site/docs/reference/api.md` + OpenAPI 注释（若有）说明 `parent_version_id` |
| **`forked_at`** | 中：sqlite migration + orchestrator 写一次 | **可选一期**：`versions.forked_at TEXT`；在 `orchestrator.go` 完成 offshoot fork + celld binding branch 步骤后 `store.SetVersionForkedAt`；`GET version` 返回 |
| **`data_parent_version_id`** | 低但冗余 | **不做**（与 `parent_version_id` 重复） |
| **`fork_txid`** | 高：解析 celld 输出或读 S3 `base.json` | **defer**；仅在文档中解释概念 |
| **bindings 响应** | 中 | **defer**；除非产品坚持 bindings 为入口，否则 version GET 足够 |

**若实现 `forked_at`，涉及：**

- `cellp/internal/registry/sqlite.go`：`migrate()` 加列；`GetVersion`/`scanVersion` 读取
- `cellp/internal/registry/store.go`：`Version.ForkedAt *time.Time`，`SetForkedAt` 或扩展现有 update
- `cellp/internal/orch/orchestrator.go`：在 `StatusBranching` 成功路径末尾（fork + 计划内 branch 调用前或后，需与「fork_txid 固定时刻」对齐）写入 UTC 时间
- `cellp/internal/api/server_test.go`：创建子 version 集成测断言字段存在（若 e2e 覆盖可减弱单测）

### 2.5 Dashboard：`web/`（验收：version 详情短文案）

| 文件 | 改动 |
|------|------|
| `web/src/components/version-detail-view.tsx` | 当 `version.parent_version_id` 非空时，在 metadata 区或 header 下增加 `role="note"` 的短段落（与 site 英文一致），要点：数据来自父 version **在创建/branch 时**的快照；**Promote 不会**把当前 prod 在 fork 之后的写入合并进来 |
| `web/src/lib/cellp-api.ts` | 若 API 增加 `forked_at`，扩展 `Version` 接口并在 UI 可选展示「Data snapshotted at …」 |
| `web/e2e/mock-api-server.mjs` | fixture 子 version 保持 `parent_version_id`；若测文案则加 `data-testid="preview-snapshot-notice"` |
| `web/e2e/*.spec.ts` | 新增或扩展：打开带子 parent 的 version 页，断言 notice 文案存在 |

**非本期（issue 调研提示中的 StoragePage）：** `StoragePage.tsx` / `ad7-banner.tsx` 可仅在 R2 tooltip 去掉「实时继承」歧义；**验收硬性位置是 version 详情**。

### 2.6 不改动的代码（明确边界）

- `orch.Promote` 合并逻辑：**不实现**
- `celld` submodule：**不实现** rebase/merge
- `docs/plans/D1-BRANCH-RPC.md`：**不修改**冻结字段集

---

## 3. 测试 / e2e 步骤

### 3.1 门禁（实现后常规）

```bash
cd site && npm run docs:build          # 站点链接与 build
cd cellp && go test ./...              # 若改了 API/sqlite
./dev/scripts/up.sh && ./dev/scripts/health.sh
./e2e/scripts/run-all.sh               # 回归；若新增脚本则加入 e2e/scripts/MANIFEST
cd web && npm run test:e2e             # 若改了 Dashboard 文案测
```

### 3.2 建议新增 e2e（语义证明，非 issue 强制但强烈推荐）

**脚本名建议：** `e2e/scripts/v16-promote-no-merge.sh`（或并入现有 d1 e2e 族）

**场景（对齐 issue 事故模型）：**

1. 部署 root `P`，seed D1 行 `before-fork`，promote `P` 为 prod。
2. `POST` 子 version `C`，`parent_version_id: P`（或 staging 父），poll `ready`。
3. 经 gateway 向 **prod** 写入 `after-fork-prod-only`（Worker 或 `celld d1 execute` / 现有 e2e helper）。
4. 向 **preview `C`** 写入 `child-only`。
5. `POST …/C/promote`。
6. 断言 prod URL 可读 `child-only`，**不可读** `after-fork-prod-only`（或 count/ checksum 证明）。

**依赖：** 与 `e2e/scripts/v1-d1-branch.sh`、`v4-promote-cutover.sh`、`dev/examples/d1-seed` 同模式；需 Worker 或 SQL 探针。

**证据：** 日志写入 `docs/evidence/`（可选 `v16-promote-no-merge.log`）。

### 3.3 API 契约

- 若仅文档化：在 `cellp/internal/api/server_test.go` 增加 `TestGetVersion_JSONIncludesParentVersionID`（已有类似 fixture 可扩）。
- 若加 `forked_at`：子 version ready 后 `GET` 断言 `forked_at <= ready_at` 且非空。

### 3.4 文档验收（人工）

- 内部评审：用 issue 中的用户故事走读 site 新一节，确认无「preview = live prod」表述。
- `DESIGN.md` P3 与 `decisions.md` AD-8 无冲突。

---

## 4. 验收勾选 ↔ issue 条目

| Issue 验收项 | 计划交付物 | 验证方式 |
|--------------|------------|----------|
| [ ] `site/` 用户文档：preview 时间线、promote 语义、与 Git 差异 | §2.1 `preview.md` + `promote.md`（或 `data-snapshot.md`）+ `build/data.md` 补丁 | `npm run docs:build`；人工走读 |
| [ ] `DESIGN.md` 修正与 AD-8 矛盾的 P3 | §2.2 P3 行 + §8.3 标题/导语 | diff 对照 `docs/decisions.md` §13 |
| [ ] API：`GET version` 或 bindings 说明 parent 快照；低成本字段评估 | §2.4 至少 API 文档化 `parent_version_id`；`fork_txid` defer；`forked_at` 可选 | `reference/api.md` + 可选 go test / curl fixture |
| [ ] Dashboard version 详情短文案（与 site 一致） | §2.5 `version-detail-view.tsx` + e2e | Playwright `data-testid` |
| [ ] 不要求 rebase/merge | §2.6 明确非目标 | 代码 review 无 merge 实现 |
| （隐含）子 version skip export | 文档中说明子 version 数据来自 branch 非 export 合并 | 引用 orchestrator `d1 branch` / AD-8 |

---

## 5. 建议实施顺序

1. **DESIGN P3 + site 文案**（零运行时风险，立刻消除矛盾叙事）。
2. **`reference/api.md` 字段说明**（满足 API 验收最低线）。
3. **Dashboard notice + Playwright**。
4. **（可选）`forked_at` sqlite + orchestrator**。
5. **（推荐）`v16-promote-no-merge.sh` + MANIFEST**。
6. **ISSUE-04**：仅在 site cron 段加交叉引用，**不**在本 issue 做 cron arm 代码。

---

## 6. 调研索引（便于实现时跳转）

| 主题 | 路径 |
|------|------|
| Version 模型 | `cellp/internal/registry/store.go` `Version`；`sqlite.go` `versions` 表 |
| GET version | `cellp/internal/api/server.go` `handleGetVersion` |
| Branch 流水线 | `cellp/internal/orch/orchestrator.go`（checkpoint/fork/export skip/D1Branch） |
| Promote saga | 同文件 `Promote` L328+ |
| fork_txid 语义 | `docs/plans/D1-BRANCH-RPC.md`；`celld/crates/ltx/src/base.rs` |
| Dashboard 详情 | `web/src/components/version-detail-view.tsx` |
| 现有 site 概念 | `site/docs/concepts/preview.md`、`promote.md`、`build/data.md` |
| 相关 e2e | `v1-d1-branch.sh`、`v2-quiesce-fork.sh`（offshoot 层隔离）、`v4-promote-cutover.sh` |

---

## 实施记录

**2026-08-31**

- **`DESIGN.md`**：§8.1 P3 与 AD-8 对齐；§8.3 区分根/子 version 数据面。
- **`docs/decisions.md`**：AD-8 增加产品不变量 bullet（fork 快照 + promote 非 merge）。
- **`site/`**：`concepts/preview.md`（Data snapshot timeline、Not like Git）、`concepts/promote.md`（What promote does not do）、`build/data.md`、`how-it-works.md`、`guides/ci.md`、`reference/api.md`（`parent_version_id` 字段说明）。
- **`web/`**：`version-detail-view.tsx` 增加 `preview-snapshot-notice`；`dashboard.spec.ts` Playwright 断言。
- **`cellp/`**：`server_test.go` 增加 `TestGetVersion_JSONIncludesParentVersionID`。**未**实现 `fork_txid` / `forked_at`（plan 一期 defer）。
- **`e2e/scripts/v17-promote-no-merge.sh`**：D1 证明 promote 后 prod 为子 bucket，不含 fork 后仅 prod 写入；已写入 MANIFEST。

验证：`cd site && npm run docs:build`；`cd cellp && go test ./...`；`cd web && npm run test:e2e`（需 mock API）；全栈 `v17` 依赖 `./dev/scripts/up.sh` 与 `run-all.sh`。
