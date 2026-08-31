# ISSUE-03 实现审查

**审查日期：** 2026-08-31  
**依据：** [ISSUE-03-preview-snapshot-semantics.md](./ISSUE-03-preview-snapshot-semantics.md)、[ISSUE-03-plan.md](./ISSUE-03-plan.md)  
**代码范围：** 当前工作区相对 `origin/main` 的未提交改动（`git status` 含 `??` 新文件）

## 结论摘要

| 维度 | 判定 |
|------|------|
| ISSUE-03 验收（文档 + 可观测性 + Dashboard） | **基本满足**，可合并叙事向交付 |
| 阻塞合并 | **无**（就 ISSUE-03 条目而言） |
| 范围混杂 | 同一 diff 含 **ISSUE-01**（promote offshoot 硬门禁、deploy fail-closed）与 **ISSUE-04 / AD-11**（cron 仅 prod arm）；合并 PR 时需写清 scope 或拆分 |

---

## 验收标准逐条

### 1. `site/` 用户文档：preview 时间线、promote 语义、与 Git 差异

**状态：满足**

| 交付 | 证据 |
|------|------|
| 集中说明 | `site/docs/concepts/preview.md`：`## Data snapshot timeline`、`## Not like Git` |
| promote 非 merge | `site/docs/concepts/promote.md`：`## What promote does not do` |
| 交叉修订 | `site/docs/build/data.md`（promote 指针 + 时间线例子）、`site/docs/how-it-works.md`、`site/docs/guides/ci.md` |
| API 字段叙事 | `site/docs/reference/api.md`：`### GET /versions/{v} fields (snapshot semantics)` |

`preview.md` 中 **Cron on preview** 与 AD-11 一致，和快照叙事不冲突。

**建议（非阻塞）：** `npm run docs:build` 走一遍站内链（`#data-snapshot-timeline`、`#what-promote-does-not-do`）。

---

### 2. `DESIGN.md` 修正与 AD-8 矛盾的 P3

**状态：满足**

- §8.1 **P3** 已改为子 version `celld * branch` + Workflow/Cron/脚本不 branch。
- §8.3 区分根 version 空起步 vs 子 version branch。
- `docs/decisions.md` AD-8 下已加 **产品不变量（ISSUE-03）** bullet，与 site 同源。

---

### 3. API：`GET …/versions/{vid}`（或 bindings）说明 parent 快照

**状态：满足（文档化路径）；OpenAPI 有小缺口**

| 项 | 状态 |
|----|------|
| 运行时 JSON | `registry.Version.ParentVersionID` → `parent_version_id`（既有字段，未改 schema） |
| 用户向 API 文档 | `site/docs/reference/api.md` 已说明 fork 切断、`fork_txid` 概念但不暴露 JSON |
| `fork_txid` / `forked_at` | 按 plan **defer**，合理 |
| `data_parent_version_id` | 未做（plan 建议不做），合理 |
| OpenAPI | `components/schemas/Version` **仍无** `parent_version_id` 属性与描述；仅 promote 502 有增补 |

**建议（非阻塞）：** 在 `cellp/api/openapi.yaml` 的 `Version` schema 补上 `parent_version_id`（及可选 `ready_at`），与 `reference/api.md` 对齐。

**测试：** `cellp/internal/api/server_test.go` → `TestGetVersion_JSONIncludesParentVersionID` 覆盖 GET 序列化。

---

### 4. Dashboard version 详情短文案（issue：中/英与 site 一致）

**状态：满足 plan 解读；严格读 issue 的「中文」未在 UI 体现**

| 项 | 证据 |
|----|------|
| 文案位置 | `web/src/components/version-detail-view.tsx`，`parent_version_id` 非空时 `data-testid="preview-snapshot-notice"` |
| 语义 | 与 site 英文一致：fork 时刻快照、promote 不 merge prod 后续写入 |
| 中文 | Dashboard 无 i18n；**仅英文**。plan §1.1 已说明「中/英一致 = 语义与 site 英文一致」 |

**建议：** 若产品坚持 issue 字面「中/英」，需在 site 加中文页或 Dashboard i18n——**非本期 plan 范围**。

**测试：** `web/e2e/dashboard.spec.ts` — `preview branch shows snapshot notice (ISSUE-03)`；fixture `mock-api-server.mjs` 中 `v2.parent_version_id = "v1"` 已具备。

---

### 5. 不要求实现 rebase/merge

**状态：满足**

- 无合并/rebase 实现；`v17-promote-no-merge.sh` 从行为上证明 promote 换指针。

---

## ISSUE-03 语义 e2e（plan 强烈推荐）

| 项 | 状态 |
|----|------|
| `e2e/scripts/v17-promote-no-merge.sh` | 已实现：fork 后 prod-only 行 promote 子 version 后不可见 |
| `e2e/scripts/MANIFEST` | 已列入（在 `v17-cron-prod-only.sh` 之后） |
| `docs/test-plan.md` | **未**新增对应 TP 行（仅有 TP-V17 cron）；门禁文档与脚本编号易混淆 |

**建议（非阻塞）：**

1. 在 `docs/test-plan.md` 增加 **TP-V17b**（或重命名）指向 `v17-promote-no-merge.sh`，避免与 **TP-V17 Cron（AD-11）** 共用 `v17` 前缀。
2. 两个脚本均名 `v17-*`，长期维护成本高，可考虑 `v18-promote-no-merge.sh`。

---

## 同 diff 内非 ISSUE-03 改动（审查备注）

便于合并评审，**不属于 ISSUE-03 验收条目**，但与 `run-all.sh` 强相关：

| 主题 | 主要文件 |
|------|----------|
| ISSUE-01 promote offshoot 硬门禁 | `orch/orchestrator.go` `ErrOffshootPromote`、`api/server.go` 502、`branch/manager.go` 注入、`v4b-promote-offshoot-fail.sh` |
| Deploy fail-closed | `deploy_policy.go`、`CELLP_LENIENT_DEPLOY`、`v5b-deploy-d1-branch-fail.sh` |
| AD-11 cron 仅 prod | `cron_policy.go`、`runtime/wrangler_cron.go`、`v17-cron-prod-only.sh` |

**潜在行为风险（建议项，非 ISSUE-03 阻塞）：** `Promote` 在 `CAS_prod` 与路由激活成功后，若 `ReconcileCronAfterProdChange` 失败会 **return err** 且日志写 `warn`——客户端可能见 5xx，但 `prod_version_id` 已切换。属 AD-11 saga 完整性，宜在 ISSUE-04 PR 中单测/文档说明是否需补偿。

---

## 阻塞问题

**无**——就 ISSUE-03 五条验收而言，文档、DESIGN、GET `parent_version_id` 文档化、Dashboard notice、e2e 语义证明均已到位。

若 PR **仅声称 ISSUE-03**，建议拆分或至少在描述中列出捆绑的 ISSUE-01/04，避免审查范围漂移。

---

## 建议（优先级）

1. **OpenAPI `Version`** 补 `parent_version_id` 描述（低成本，闭合 API 验收「文档化」）。
2. **`test-plan.md`** 为 `v17-promote-no-merge.sh` 登记独立 TP，解决双 `v17` 命名。
3. 合并前跑验证（见下）；全栈 e2e 对 **v4b / v5b / v17-*** 依赖 `dev` 栈 + offshoot + celld。
4. 可选：`site/docs:build` 纳入 CI（若尚未）。

---

## 建议执行的验证命令

| 目的 | 命令 |
|------|------|
| ISSUE-03 文档 | `cd site && npm run docs:build` |
| API 单测（含 parent_version_id） | `cd cellp && go test ./...` |
| Dashboard 文案 | `cd web && npm run test:e2e`（mock API，含 snapshot notice） |
| **全栈回归（含本 diff 新增脚本）** | `./dev/scripts/up.sh && ./dev/scripts/health.sh && ./e2e/scripts/run-all.sh` |

**点名脚本（ISSUE-03 语义 + 本 diff 关键路径）：**

```bash
./e2e/scripts/v17-promote-no-merge.sh   # ISSUE-03 promote 非 merge
./e2e/scripts/v4b-promote-offshoot-fail.sh   # ISSUE-01（若同 PR）
./e2e/scripts/v5b-deploy-d1-branch-fail.sh   # deploy fail-closed（若同 PR）
./e2e/scripts/v17-cron-prod-only.sh    # AD-11（若同 PR；耗时长 ~90s）
```

**不必为 ISSUE-03 单独再跑** D1 branch 族，除非改动 orchestrator branch 路径；当前 diff 动了 `runDeploy`/`Promote`，**推荐至少 `run-all.sh` 或上表四条**。

---

## 审查方法

只读：`git diff`、issue/plan、关键 `site/` / `web/` / `cellp/internal/api` / `e2e` 文件；未在本机执行全栈 e2e。
