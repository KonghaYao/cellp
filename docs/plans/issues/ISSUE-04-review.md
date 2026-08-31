# ISSUE-04 实现审查

**审查日期：** 2026-08-31  
**依据：** [ISSUE-04-cron-multi-ready.md](./ISSUE-04-cron-multi-ready.md)、[ISSUE-04-plan.md](./ISSUE-04-plan.md)、工作区未提交改动（`cron_policy` / `wrangler_cron` / `orchestrator` / AD-11 文档 / `v17-cron-prod-only.sh` 等）

**结论摘要：** 一期方案（AD-11：deploy 时 strip 非 prod 的 `triggers.crons` + promote 后 reconcile）**主体已实现**，与计划一致。验收上文档与 e2e 脚本已就位，但 **test-plan TP-V17 仍为未勾选**、**缺少 promote 场景 cron 翻转 e2e**、**Reconcile 失败与 saga 语义**需产品/工程确认。合并前建议至少跑通 `v17-cron-prod-only.sh` 或全量 `run-all.sh`。

---

## 1. 验收标准（issue 原文逐条）

| # | 标准 | 状态 | 证据 |
|---|------|------|------|
| 1 | 书面决策写入 `docs/decisions.md`（一期：仅 prod arm） | **满足** | `docs/decisions.md` §16 **AD-11**；`DESIGN.md` §8.7 已交叉引用 |
| 2 | 仅 prod：orchestrator 在 deploy / promote reconcile 传 arm 决策（非 Start env 主路径） | **满足** | `CronShouldArm`（`cellp/internal/orch/cron_policy.go`）；`runDeploy` → `runtime.Deploy(..., armCron)`（`orchestrator.go`）；`PrepareDeployBundle` strip（`wrangler_cron.go`）；`Promote` 末尾 `ReconcileCronAfterProdChange` |
| 3 | e2e/脚本：两 ready version，断言 cron 仅 prod 触发（或 preview 不触发） | **基本满足** | `e2e/scripts/v17-cron-prod-only.sh`：prod + preview 均 ready、bindings 均有 `* * * * *`，90s 内仅 prod celld 日志 `e2e-cron-tick` 增长；已列入 `e2e/scripts/MANIFEST` |
| 4 | `site/docs` 说明 preview 与 cron | **满足** | `site/docs/bindings/cron.md` §3、`site/docs/concepts/preview.md`「Cron on preview」、`site/docs/concepts/bindings.md` Cron 行 |

**计划内额外交付（非 issue 四条，但 plan §2 有）：**

| 项 | 状态 |
|----|------|
| 单测 `CronShouldArm` | 有（`cron_policy_test.go`） |
| 单测 strip / 不污染 artifact | 有（`wrangler_cron_test.go`） |
| 单测 `ReconcileCronAfterProdChange`（mock Deploy/Restart） | **无**（plan §5.1 曾列） |
| promote 后 old prod disarm + new prod arm 的 e2e | **无**（仅静态两 ready 场景） |
| `docs/test-plan.md` TP-V17 勾选 | **仍为 `[ ]`**（脚本与描述已写） |
| `ISSUE-04-plan.md` 状态「未实施」 | **未更新**（文档滞后） |

---

## 2. 实现核对（相对 plan）

### 2.1 核心路径

- **Deploy：** `Manager.Deploy(..., includeCrons)` 在 `includeCrons==false` 时临时目录拷贝并 `stripCronsFromWranglerJSON`，不修改 artifact — 与 AD-11 一致。
- **首版 / `prod_version_id` 为空：** `CronShouldArm` 返回 `true`，与 plan 风险表「deploy 在 CAS 前」一致；`runDeploy` 在 ready 后 `SetProdVersionCAS` 仅当 `prod == nil`。
- **Bindings API：** 仍解析 artifact wrangler（未改 `bindings.go`），与「声明可见、调度仅 prod」一致。
- **Promote：** CAS + 激活 route 之后 `ReconcileCronAfterProdChange`，对 old/new ready 版本分别 `Deploy` + `Restart`，`arm` 由**更新后的** `GetProject` + `CronShouldArm` 决定 — 逻辑正确。

### 2.2 已知缺口 / 偏差

1. **Wake（`archive.go`）** 仅 `Start`，不 redeploy。plan 允许「archived 前 manifest 已 strip 则 OK」。若历史上某 preview 曾在 AD-11 前 deploy（manifest 含 crons），wake 后可能再次 arm — **迁移/存量**问题，非本 PR 独有；可选在 wake 对 ready 非 archived 路径加一次 `Deploy(CronShouldArm)`（defer 或 follow-up）。

2. **`versionBundleDir`（`cron_policy.go`）与 `runDeploy` bundle 解析** 两处逻辑近似但不完全一致（例如 reconcile 多认 `wrangler.json`）。reconcile 在 artifact 仅 `wrangler.json` 时更稳，日常 deploy 若只有 `.json` 可能仍走 counter fallback — **建议** 抽公共 `resolveBundleDir` 与 `runDeploy` 对齐（非阻塞，除非生产 bundle 仅 `.json`）。

3. **`site/docs/concepts/promote.md`** saga 列表未写第 6 步 cron reconcile；`cron.md` 已写 promote 行为 — **文档小缺口**。

4. **Dashboard：** `StoragePage` 的 cron tooltip 仍为通用 celld 文案，未点明 preview 不调度（plan 标为可选）。

5. **`DESIGN.md` §8.2** Cron 行「延期」仍写「平台调度器；跨 version 策略」，未像 §8.7 一样写明 AD-11 已覆盖 arm 策略 — 建议补一句避免读者以为未做。

---

## 3. 阻塞问题

### 3.1 Promote 成功路径依赖 reconcile，失败时 saga 语义不清（**高 — 建议合并前定论**）

`Promote` 在 `SetProdVersionCAS` 与 `activate_prod_route` **之后**调用 `ReconcileCronAfterProdChange`；若 reconcile 返回 error，**整段 `Promote` 向 API 返回失败**，但 **prod 指针与 route 已切换**，且 **defer 补偿未覆盖 cron reconcile**。

后果：新 prod 可能仍为 strip 后的 manifest（不 tick），旧 prod（若仍 ready）manifest 仍含 crons（继续 tick）→ **与 AD-11 相反的双跑或零跑**。

- 若产品接受「promote 成功但 cron 延迟一致」：应 reconcile **失败时仍返回 200** 并强告警 + 重试任务，或把 reconcile 纳入可补偿 saga。
- 若产品要求 **promote 与 cron 强一致**：reconcile 失败应触发补偿（回滚 CAS/route）或 reconcile 前置于「对外可见」步骤。

**当前实现偏「硬失败 + 已切 prod」**，建议在 issue 关闭前明确一种语义并改代码或文档。

### 3.2 无阻塞项（celld submodule）

一期不依赖 `CELLD_CRON_ARM`，与 plan defer 一致。

---

## 4. 建议（非阻塞）

| 优先级 | 建议 |
|--------|------|
| P1 | 合并前执行 **`e2e/scripts/v17-cron-prod-only.sh`**（需本地栈 + `celld` PATH）；通过后将 **`docs/test-plan.md` TP-V17** 标为 `[x]` 并附 evidence |
| P1 | 增加 **promote cron 翻转** 脚本或扩展 v17：preview promote 后 old 停 tick、new prod tick（覆盖 reconcile 主价值） |
| P2 | `ReconcileCronAfterProdChange` 表驱动单测（fake Manager 或接口注入） |
| P2 | 统一 bundle 目录解析；`promote.md` 补 cron reconcile 一步 |
| P3 | 更新 `ISSUE-04-plan.md` 状态为已实施；可选 web cron tooltip 引用 AD-11 |

### e2e v17 注意事项

- 依赖 **`${TMPDIR:-/tmp}/celld-${PROJECT}-${vid}.log`**（`manager.go` 使用 `os.TempDir()`，一般与 `TMPDIR` 一致）。
- 断言依赖 worker `console.log` 进入 celld 日志；若 celld 日志策略变更，脚本易脆。
- **~90s** 等待，全量 `run-all.sh` 时长显著增加 — 可接受，但 CI 需预留时间。

---

## 5. 测试建议

| 范围 | 命令 | 说明 |
|------|------|------|
| **最小门禁（本 issue）** | `./dev/scripts/up.sh && ./dev/scripts/health.sh` 后 **`bash e2e/scripts/v17-cron-prod-only.sh`** | 直接覆盖 AD-11 验收 |
| **回归（推荐合并前）** | **`./e2e/scripts/run-all.sh`** | MANIFEST 已含 `v17-cron-prod-only.sh`；含 promote 相关 `v17-promote-no-merge.sh`（D1，非 cron） |
| **Go 单测** | `cd cellp && go test ./internal/orch/ -run TestCronShouldArm ./internal/runtime/ -run 'TestStrip|TestPrepare'` | 审查环境 `go test` 超时未跑通；合并前本地/CI 应绿 |
| **不必为 ISSUE-04 单独跑** | D1 branch scale、web e2e | 除非同时改了 `orchestrator`/`branch` 其他路径 |

---

## 6. 是否可关闭 issue

- **功能口径：** 在确认 **promote + reconcile 失败语义** 并跑绿 **v17** 后，可关闭 ISSUE-04 一期。
- **工程口径：** 建议将 **promote cron e2e** 与 **TP-V17 勾选** 作为关闭 checklist 剩余两项。

---

*审查为只读；未修改产品代码。*
