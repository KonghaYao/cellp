# ISSUE-04 实现计划：多 ready version 时 Cron 重复触发（一期）

**对应 issue：** [ISSUE-04-cron-multi-ready.md](./ISSUE-04-cron-multi-ready.md)  
**状态：** 计划（未实施）  
**调研日期：** 2026-08-31

---

## 1. 范围与风险

### 1.1 问题复述

- **AD-1：** 每个 `ready` version 独立 `celld` 子进程（`cellp/internal/runtime/manager.go` `Start` / `StartOnPort`）。
- **celld 行为：** `celld deploy` 将 `triggers.crons` 写入 fleet manifest；节点 boot / adoption 后 `spawn_cron_arm` 对**该 version 的 fleet** 武装调度（`celld/crates/celld/main.rs`，`Generation::cron_cell()` 见 `generation.rs`）。同一项目内 N 个 ready version ⇒ N 套独立 cron 调度 ⇒ 相同表达式 N 倍 `scheduled` 副作用。
- **与产品预期冲突：** Preview 应可测 HTTP/数据分支，但不应与 prod 并行跑生产级定时任务（webhook、清队列、计费批处理等）。
- **Promote 未停旧 prod 进程：** `Promote` 仅 `SetRouteActive(false)` 旧 prod，**不** `Stop` celld（`orchestrator.go`）。旧 prod 若仍为 `ready`，其 cron 会继续触发，加剧「多实例」问题。

### 1.2 一期目标（对齐 issue 验收）

| 条目 | 一期 |
|------|------|
| 书面策略 | `docs/decisions.md` 新 AD（建议 **AD-11**） |
| 运行时缓解 | **推荐：仅 `prod_version_id` 对应 version 在 celld 内实际武装 cron** |
| e2e | 两 ready + cron wrangler，断言仅 prod 产生 tick 副作用 |
| 用户文档 | `site/docs` 说明 preview / prod 与 cron |
| 非目标 | Workflow branch、分布式 cron 选举（二期） |

### 1.3 与 DESIGN 的关系

- DESIGN §8.7 仍将「Cron 平台级只让 prod 跑 / 多 version 选举」列为**本期不做**；本 issue 是一期**最小架构缓解**，实施后应：
  - 在 `decisions.md` 写清 AD-11；
  - 将 DESIGN §8.7 该行改为「已由 AD-11 覆盖（仅 prod arm）；选举仍二期」。

### 1.4 技术结论（调研）

| 路径 | 可行性 | 说明 |
|------|--------|------|
| `Start` 时仅传 `CELLD_VARS` / Worker env | **不足** | celld 无「只读展示 cron、不 arm」的现成 env；`spawn_cron_arm` 由 manifest 是否含 `crons` 决定 |
| 非 prod 部署时从 wrangler **剥离** `triggers.crons` 再 `celld deploy` | **可行（cellp 单仓）** | manifest 无 `crons` ⇒ 无 reserved cron cell ⇒ `spawn_cron_arm` 直接 return |
| promote 后 reconcile | **必须** | 新 prod 若以 preview 部署过（已 strip），promote 后需 **带 crons 再 deploy + Restart**；旧 prod 需 **strip 再 deploy + Restart** |
| celld 新增 `CELLD_CRON_ARM=0` | **可选增强** | 当前代码库**无**该变量；可作为 submodule 跟进，**非一期阻塞** |
| 「preview 默认不 arm，除非 env 标志」 | **二期或 celld 依赖** | 若要在**不 strip** 的前提下 opt-in preview cron，需要 celld 支持跳过 arm；一期用 strip + `CELLP_PREVIEW_CRON` 仅作文档占位 **defer** |

### 1.5 风险

| 风险 | 缓解 |
|------|------|
| 首包部署时 `prod_version_id` 尚为 NULL，却先 `deploy` 后 `SetProdVersionCAS` | `CronShouldArm` 规则：`prod == nil` **或** `prod == versionID` ⇒ arm（首版与 issue 一致） |
| `GET …/bindings` 仍读 artifact wrangler | **保持**：展示「声明的 cron」；与「是否 arm」分离；文档说明 preview 列表可见但不调度 |
| promote 漏 reconcile | promote saga 末尾固定两步 reconcile；单测 + e2e |
| e2e 依赖 `* * * * *` 等待时间长 | 用 celld `examples/cron` + KV 副作用；等待 65–90s；或 Go 集成测 mock deploy 参数 |
| 改 `celld/` submodule | 一期**不强制**；若后续加 `CELLD_CRON_ARM`，再 bump submodule |

---

## 2. 推荐决策（写入 `docs/decisions.md` 草案）

**AD-11（建议标题）：Cron 仅 prod version 武装调度**

- **规则：** 对项目 `P`，仅当 `versions.id == projects.prod_version_id`（或项目尚无 prod 且该 version 为首个将设为 prod 的 ready 版）时，cellp 对 celld 的 deploy 保留 `triggers.crons`；其余 ready preview 在 deploy 时**不**把 crons 写入 manifest（临时从用于 deploy 的 wrangler 视图去掉 `triggers.crons`，**不修改** artifact 原件）。
- **Promote：** `CAS_prod` 成功后，对 `old_prod`（若仍 ready）执行 disarm reconcile；对 `new_prod` 执行 arm reconcile（全量 wrangler deploy + `runtime.Restart`）。
- **Archive / Wake：** archived 无进程 ⇒ 无 tick；wake 时按当前是否 prod 走同一 `CronShouldArm` deploy 策略。
- **bindings API：** 继续反映 wrangler **声明**；不新增「运行中」字段亦可，文档写清即可（可选二期：`cron_scheduled: bool`）。
- **明确 defer：** `CELLP_PREVIEW_CRON=1` 让 preview 与 prod 同时 arm（需 celld 或接受双倍触发）；跨节点 cron 选举；按 route 活跃而非 prod 指针 arm。

---

## 3. 代码改动 vs defer 边界

### 3.1 一期必须（代码）

| 模块 | 内容 |
|------|------|
| `cellp/internal/orch/` | `CronShouldArm(project, versionID)`；deploy 前选择 strip/保留 crons |
| `cellp/internal/runtime/` | `Deploy` 增加 `armCron bool` 或独立 `PrepareWranglerForDeploy(dir, armCron)`（写临时目录或内存 overlay，避免改 artifact） |
| `cellp/internal/orch/orchestrator.go` | `runDeploy` 调用 deploy 时传入 arm 决策；`Promote` 末尾 `reconcileCron(project, oldProd, newProd)` |
| `cellp/internal/orch/archive.go` | `Wake` 后若需保证策略一致，可复用 reconcile（wake 路径已是 Start，依赖 S3 内 manifest；若 archived 前已 strip 则 OK） |
| `docs/decisions.md` | AD-11 正文 |
| `DESIGN.md` | §8.7 与 §8.2 Cron 行交叉引用 AD-11 |
| `site/docs/` | 见 §5 |
| `e2e/` | 新脚本或扩展，见 §4 |

### 3.2 一期仅文档 / 决策（可零 celld 变更）

- 书面 AD、site 用户说明、e2e 门禁。
- **不**改 `web/` 除非要在 Storage 徽章加 tooltip「preview 不调度」（issue 未强制；可标为可选小改）。

### 3.3 明确 defer（非一期）

- celld `CELLD_CRON_ARM` / disarm API / operator `celld cron pause`。
- 分布式 cron 选举、按 `routes.active` 而非 `prod_version_id` arm。
- `CELLP_PREVIEW_CRON=1` 行为实现（依赖 celld 或接受双触发）。
- OpenAPI `cron_armed` 字段（可选增强）。
- Promote 时 `Stop` 非 prod ready celld（与 AD-9 封存策略相关，**非本 issue**）。

---

## 4. 具体文件与函数改动清单

### 4.1 新增

| 文件 | 函数 / 类型 | 职责 |
|------|-------------|------|
| `cellp/internal/orch/cron_policy.go` | `CronShouldArm(proj *registry.Project, versionID string) bool` | `prod == nil \|\| *prod == versionID` |
| 同上 | `ReconcileCronAfterProdChange(ctx, o *Orchestrator, project, oldProd, newProd string) error` | 对 old/new 分别 `deploy+restart` |
| `cellp/internal/runtime/wrangler_cron.go`（名可调整） | `DeployBundle(ctx, project, version, bundleDir string, includeCrons bool) error` | `includeCrons==false` 时复制 bundle 到临时目录并删除 `triggers.crons` 后调用 `celld deploy` |
| `cellp/internal/orch/cron_policy_test.go` | 表驱动 | 首版 / preview / prod / promote 前后 |
| `cellp/internal/runtime/wrangler_cron_test.go` | 单测 | strip 后 JSON 无 crons、原 artifact 不变 |
| `e2e/scripts/v12-cron-prod-only.sh` | — | 见 §5.2 |
| `e2e/scripts/MANIFEST` | 登记 `v12-cron-prod-only.sh` | |
| `docs/evidence/` | e2e 日志/json | gitignore 产物 |

### 4.2 修改

| 文件 | 改动点 |
|------|--------|
| `cellp/internal/runtime/manager.go` | `Deploy(...)` 签名扩展或委托 `DeployBundle`；**不在** `StartOnPort` 加 cron 逻辑（除非未来 celld env） |
| `cellp/internal/orch/orchestrator.go` | `runDeploy`：deploy 前 `GetProject` + `CronShouldArm`；`Promote`：`activate_prod_route` 之后调用 `ReconcileCronAfterProdChange` |
| `docs/decisions.md` | 新增 §「AD-11 — Cron 仅 prod arm」 |
| `DESIGN.md` | §8.2 Cron 行、§8.7 列表 |
| `site/docs/bindings/cron.md` | 「When it runs」：仅 prod ready 调度；preview 见声明不执行 |
| `site/docs/concepts/preview.md` | 一小节 Cron 与 promote |
| `site/docs/concepts/bindings.md` | Cron 行补充「调度仅 prod」 |
| `site/docs/how-it-works.md` | 若已有 Cron 一句，对齐 AD-11 |
| `e2e/README.md` | TP-V12 一行 |
| `docs/test-plan.md` | 可选新增 TP-V12 勾选块（与 issue 验收对齐） |

### 4.3 不必改（一期）

- `cellp/internal/runtime/bindings.go` `ParseBindings` — 继续读 artifact wrangler。
- `web/` — 除非加 tooltip（可选）。
- `celld/` submodule — 除非走 defer 的 env 方案。

### 4.4 celld 行为引用（实现时对照）

- Deploy 读 crons：`celld/crates/celld/deploy.rs` `read_crons` / manifest `crons`。
- Arm：`main.rs` `spawn_cron_arm` → `arm_cron_schedule`。
- 无 crons：`generation.rs` `cron_cell()` → `None` ⇒ 不 arm。

---

## 5. 测试 / e2e 步骤

### 5.1 Go 单元 / 集成

```bash
cd cellp && go test ./internal/orch/... ./internal/runtime/... -count=1
```

覆盖：

- `CronShouldArm`：无 prod、有 prod 且当前为 preview、当前为 prod。
- `strip` 后 `celld deploy` 调用参数（可用 `CELLP_E2E_INJECT` 模式或 fake exec 若已有模式）。
- `ReconcileCronAfterProdChange`：mock runtime 记录 `includeCrons` 与 `Restart` 次数。

### 5.2 e2e `v12-cron-prod-only.sh`（建议 TP-V12）

**前置：** `./dev/scripts/health.sh`；`celld` 在 PATH。

**步骤：**

1. `ensure_project` + `cleanup_e2e_versions`（沿用 `e2e/scripts/lib.sh`）。
2. 基于 `celld/examples/cron`（`triggers.crons: ["* * * * *"]`）stage 两个 version：`V_PROD`、`V_PREVIEW`。
3. 部署 `V_PROD` → `poll ready`；`POST promote` 或确认其为 `prod_version_id`（首版自动 prod）。
4. 部署 `V_PREVIEW`（parent 可选）→ `poll ready`；确认 `prod_version_id == V_PROD`。
5. **断言 bindings：** 两者 `GET …/bindings` 的 `crons` 均含 `* * * * *`（声明仍在）。
6. **副作用：** 为 cron worker 增加可观测写入（若 example 仅 `console.log`，可临时在 e2e stage 的 `index.js` 里 `env` 写 KV，或解析 `celld` 日志 `/tmp/celld-{project}-{version}.log` 中 `cron` 行计数）。推荐：**stage 时 patch** `scheduled` 写 KV key `cron:tick`（需 wrangler `kv_namespaces` 最小绑定，或沿用 `dev/examples/commerce` 的 CACHE 模式简化）。
7. 等待 **≥90s**（1 分钟 tick + 余量）。
8. **断言：** prod version 的 KV（或日志）tick 次数 **≥1**；preview **0**（或显著少于 prod）。
9. **Promote 回归（可选同脚本第二段）：** promote `V_PREVIEW` → 等待 → 仅新 prod 继续累加 tick；旧 `V_PROD` preview 化后停止累加（需 reconcile 生效）。

**证据：** `docs/evidence/v12-cron-prod-only-e2e.log` / `.json`。

**门禁：** 纳入 `e2e/scripts/run-all.sh` 与 `MANIFEST`（与 phase-7 `v11-workflow-cron.sh` 并列）。

### 5.3 手工冒烟

```bash
./dev/scripts/up.sh && ./dev/scripts/health.sh
bash e2e/scripts/v12-cron-prod-only.sh
cd site && npm run docs:build   # 若改 site
```

---

## 6. 验收勾选（对应 ISSUE-04）

| Issue 验收项 | 计划交付物 | 勾选 |
|--------------|------------|------|
| 书面决策写入 `docs/decisions.md` | AD-11 草案定稿 | [ ] |
| 仅 prod：orchestrator Start/deploy/reconcile | `CronShouldArm` + `DeployBundle` + `Promote` reconcile；**非** Start env 主路径 | [ ] |
| e2e：两 ready，cron 仅触发一次（或 preview 不触发） | `v12-cron-prod-only.sh` | [ ] |
| `site/docs` preview 与 cron | `cron.md` + `preview.md` + `bindings.md` | [ ] |
| 非目标：Workflow branch | 不实现 | N/A |
| 非目标：分布式选举 | defer AD-11 附录 | N/A |

---

## 7. 实施顺序建议

1. **决策：** 评审 AD-11（确认「仅 prod」而非「preview opt-in flag」为一期默认）。
2. **runtime strip + 单测** → **orch deploy 接入** → **promote reconcile**。
3. **Go test 全绿** → **v12 e2e** → 更新 `test-plan.md` / `run-all.sh`。
4. **decisions + DESIGN + site** 同步。
5. （可选）开 celld issue：`CELLD_CRON_ARM=0` 作为 strip 的补强，避免「manifest 仍有 crons 仅因未 redeploy」的边缘窗口。

---

## 8. 参考代码索引

| 主题 | 位置 |
|------|------|
| celld 启动 env | `cellp/internal/runtime/manager.go` `StartOnPort`（`CELLD_VAR_*`、`CELLD_VARS_FILE`） |
| deploy | `manager.go` `Deploy` |
| 部署状态机 | `cellp/internal/orch/orchestrator.go` `runDeploy` |
| promote | `orchestrator.go` `Promote` |
| bindings 解析 | `cellp/internal/runtime/bindings.go` |
| 现有 cron e2e（仅清单） | `e2e/scripts/v11-workflow-cron.sh` |
| celld cron arm | `celld/crates/celld/main.rs` `spawn_cron_arm` |

---

## 实施记录

**日期：** 2026-08-31

- **AD-11** 写入 `docs/decisions.md` §16；`DESIGN.md` §8.7 交叉引用。
- **运行时：** `CronShouldArm` + `PrepareDeployBundle`（preview deploy 剥离 `triggers.crons`）；`Manager.Deploy(..., includeCrons)`；`runDeploy` / `Promote` 后 `ReconcileCronAfterProdChange`（redeploy + Restart）。
- **测试：** `cron_policy_test.go`、`wrangler_cron_test.go`；e2e `e2e/scripts/v17-cron-prod-only.sh`（MANIFEST + `docs/test-plan.md` TP-V17）。计划中的 `v12-cron-prod-only` 因 TP-V12 已被 KV branch 占用，脚本命名为 v17。
- **文档：** `site/docs/bindings/cron.md`、`concepts/preview.md`、`concepts/bindings.md`。
- **Defer（未做）：** celld `CELLD_CRON_ARM`、`CELLP_PREVIEW_CRON`、Wake 路径单独 reconcile、promote 可选第二段 e2e。
