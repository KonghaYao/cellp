# ISSUE-02 实现计划：Deploy 默认 fail-closed

**依据：** [ISSUE-02-deploy-fail-closed-branch.md](./ISSUE-02-deploy-fail-closed-branch.md)  
**状态：** 已实现（见文末「实施记录」）  
**范围：** `cellp` orchestrator + 文档/e2e；**不改** celld RPC 契约

---

## 1. 现状摘要

| 步骤 | 位置 | 失败时行为（默认） |
|------|------|-------------------|
| offshoot `checkpoint` / `fork` / `export` | `branchStep` → `orchestrator.go:307-318` | **warn 并继续**（仅 `CELLP_STRICT_OFFSHOOT_FORK=1` 时 abort） |
| `D1Branch` / `D1Execute`（import seed） | `runDeploy` → `237-254` | **warn 并继续**（同上） |
| KV / R2 / Queue branch | `runBindingBranches` → `459-487` | **已 fail-closed**（直接 `return err`） |

`processOne` 在 `runDeploy` 返回 error 时已：`StatusFailed` + `FailJob` + `compensateDeploy`（`90-96`），preview 不应注册 route。假就绪来自 **D1/offshoot 错误被吞掉后仍走到 `StatusReady`**。

`dev/scripts/up.sh` / `up-native.sh` **未**设置 `CELLP_STRICT_OFFSHOOT_FORK`；e2e 证据里曾手动开 strict（`docs/evidence/d1-branch-e2e-report.md`）。`stress/scripts/chaos-offshoot-fail.sh` 靠重启 cellpd + `STRICT=1` 测 fork 失败。

---

## 2. 范围与风险

### 2.1 在范围内

- 将「数据面步骤失败仍 ready」改为 **默认 abort**，与 issue 验收一致。
- 新增显式 opt-out：`CELLP_LENIENT_DEPLOY=1`（本地调试），文档化。
- 文档：`dev/`、`docs/test-plan.md`、`site/` 各一句默认严格。
- 测试：单元（env 语义）+ 集成/e2e（至少一条「branch/数据步骤失败 → `failed`」）。

### 2.2 非目标（issue 已列）

- celld RPC / CLI 契约变更。
- branch 自动重试。
- ISSUE-01 promote offshoot gate、ISSUE-03 preview 语义（除非实现时顺带读一眼无冲突即可）。

### 2.3 风险

| 风险 | 缓解 |
|------|------|
| 本地开发者 offshoot/celld 偶发失败以前能「凑合 ready」 | `dev/.env.example` 注释 `CELLP_LENIENT_DEPLOY=1`；不在 `up.sh` 默认开启 lenient |
| `CELLP_STRICT_OFFSHOOT_FORK` 已有用户/脚本依赖 | 保留只读兼容：未设 `LENIENT` 时 strict 变量可为 no-op；文档标记 **deprecated**，行为与默认一致 |
| `D1Branch` 在 `celld` 不在 PATH 时 `return nil`（`manager.go:362-364`） | 非本 issue 行为；e2e 已 `require_celld_cli`；计划内不扩大 scope，可在 plan 备注 follow-up |
| e2e `run-all.sh` 全绿依赖真实栈 | 新用例应像 `v5-saga-compensate.sh` 一样可重复；D1 branch 失败用例优先 **Go 单测 + 可选** 轻量 e2e |

### 2.4 与 ISSUE-04 的边界（一期）

[ISSUE-04](./ISSUE-04-cron-multi-ready.md) 解决 **多 ready version 时 Cron 重复触发**，涉及 `Start` 后 wrangler triggers / prod-only arm 等，与 fail-closed **正交**。

| 归属 | 内容 |
|------|------|
| **ISSUE-02 代码** | `deployFailClosed` 语义；offshoot / D1 seed&branch 失败 abort；lenient env；相关测试与文档 |
| **ISSUE-04 defer（代码）** | `docs/decisions.md` 草案、orchestrator 传 env 禁用 preview cron、双 ready cron 断言 e2e、`site` cron 说明 |
| **同仓并行注意** | 若两 issue 同改 `orchestrator.go` `runDeploy` 中 `Start` 之后段落，合并时把 cron 逻辑放在独立函数，避免与 D1/binding 块纠缠 |

**本计划不实现 ISSUE-04**；仅在 `docs/decisions.md` 加交叉引用（可选，非 ISSUE-02 验收项）。

---

## 3. 具体文件与函数改动清单

### 3.1 核心逻辑（`cellp/internal/orch/`）

| 文件 | 改动 |
|------|------|
| `orchestrator.go` | 将 `strictOffshoot()` 重命名/替换为 `deployFailClosed() bool`：**默认 `true`**；`CELLP_LENIENT_DEPLOY=1` → `false`。可选：`CELLP_STRICT_OFFSHOOT_FORK=1` 在 lenient 未开启时仍强制 strict（兼容旧脚本，与默认等价）。 |
| `orchestrator.go` | `branchStep`：`if deployFailClosed()` 则 `return fmt.Errorf("offshoot %s: %w", ...)`，**删除** lenient 下的 `return nil`。 |
| `orchestrator.go` | `D1Branch` / `D1Execute` 错误处理：统一用 `deployFailClosed()`，失败时 `return fmt.Errorf("d1 branch|seed: %w", err)`，不再 `log.Printf` warn 后继续。 |
| `orchestrator.go` | `runBindingBranches`：**无行为变更**（已 strict）；在注释中写明与 `deployFailClosed` 一致，避免日后误加 lenient 分支。 |

建议将 `deployFailClosed()` 放在单独小文件 `deploy_policy.go`（同 package），便于单测且不膨胀 `orchestrator.go`。

**函数签名（建议）：**

```go
// deployFailClosed reports whether offshoot/D1 errors abort deploy (default true).
func deployFailClosed() bool
```

**环境变量语义（建议）：**

| 变量 | 效果 |
|------|------|
| （默认） | fail-closed |
| `CELLP_LENIENT_DEPLOY=1` | warn 继续（与今日默认相当） |
| `CELLP_STRICT_OFFSHOOT_FORK=1` | deprecated；与默认 fail-closed 相同，lenient 优先时以 `LENIENT` 为准并 `log` 一次冲突说明 |

### 3.2 测试注入（可选但推荐，满足「branch 失败 → failed」）

| 文件 | 改动 |
|------|------|
| `cellp/internal/runtime/manager.go` | 新增 `CELLP_E2E_INJECT_D1_BRANCH_FAIL=1`：在 `D1Branch` 入口返回固定 error（与现有 `CELLP_E2E_INJECT_DEPLOY_FAIL` 模式一致，`173` 行附近）。 |
| `deploy_policy_test.go`（新） | 表驱动：`LENIENT` / `STRICT` / 默认组合 → `deployFailClosed()` 期望值。 |
| `orchestrator_test.go` 或新 `orchestrator_fail_closed_test.go` | 用 **fake `runtime.Manager`** 不现实（具体类型）；更可行：**仅测 policy** + **e2e**；或抽 `runtime` 接口（**过大**，不推荐一期）。 |

### 3.3 文档与 dev 环境

| 文件 | 改动 |
|------|------|
| `dev/.env.example` | 注释块：`# CELLP_LENIENT_DEPLOY=1  # 本地调试：offshoot/D1 失败仍尝试 ready（默认关闭）` |
| `dev/README.md` | 环境变量表增加 `CELLP_LENIENT_DEPLOY`；说明默认 deploy 严格、与 `CELLP_STRICT_OFFSHOOT_FORK` 废弃关系 |
| `dev/AGENTS.md` | 可选一行：改 orchestrator 后跑 `go test ./internal/orch/...` |
| `docs/test-plan.md` | 新增或扩展 TP 行：默认 fail-closed；引用新脚本/单测名 |
| `site/docs/guides/environment-variables.md`（或 `site/docs/how-it-works.md` §Parent versions） | 一句：子 version 的 D1/KV branch 失败会使 version `failed`，除非 cellpd 设 `CELLP_LENIENT_DEPLOY=1` |
| `docs/decisions.md` | 可选短句链到 orchestrator 默认严格（非 AD 编号也可，issue 验收「一句说明」可落在 test-plan + site） |

### 3.4 e2e / stress

| 文件 | 改动 |
|------|------|
| `e2e/scripts/v5b-deploy-d1-branch-fail.sh`（新，名称可调整） | 父 ready → 子 deploy；启动前 **不**依赖全局 lenient；通过 `CELLP_E2E_INJECT_D1_BRANCH_FAIL=1` 重启 cellpd 或 document「run cellpd with env」— **更稳妥**：脚本内若无法重启 cellpd，则 **仅跑 Go test** 并在 MANIFEST 注释「需 INJECT」；推荐与 `chaos-offshoot-fail.sh` 类似在脚本末尾恢复 cellpd。 |
| `e2e/scripts/MANIFEST` | 在 `v5-saga-compensate.sh` 后插入新脚本（若 e2e 过重则只加 `deploy_policy_test` 进 CI 说明，test-plan 写清） |
| `stress/scripts/chaos-offshoot-fail.sh` | 默认已是 strict 后：**可删除** `CELLP_STRICT_OFFSHOOT_FORK=1` 重启段落，改为注释「依赖默认 fail-closed」；恢复 cellpd 时勿再隐式 lenient |

### 3.5 明确不改

- `web/`：version `failed` 已可通过 API 展示；无 ISSUE-02 强制 UI 改动。
- `celld/`、`binding_branch.go` 计划逻辑（`BindingBranchPlanForVersion` 父非 ready 已 error）。
- `site/` 大段重写。

---

## 4. 实现顺序（建议）

1. `deployFailClosed()` + 单测；替换 `strictOffshoot` 所有调用点。
2. `go test ./...`（`cellp`）。
3. 可选 `CELLP_E2E_INJECT_D1_BRANCH_FAIL`。
4. e2e 脚本 + MANIFEST + 更新 `chaos-offshoot-fail.sh`。
5. 文档三处（dev、test-plan、site）。
6. 全门禁：`./dev/scripts/health.sh` → `cd cellp && go test ./...` → `./e2e/scripts/run-all.sh`。

---

## 5. 测试 / e2e 步骤

### 5.1 单元

```bash
cd cellp && go test ./internal/orch/... -run 'DeployFailClosed|FailClosed' -v
```

覆盖：

- 默认 / unset env → `deployFailClosed() == true`
- `CELLP_LENIENT_DEPLOY=1` → `false`
- `STRICT=1` + 无 lenient → `true`

### 5.2 集成（offshoot fork 失败，已有模式增强）

```bash
# 默认 cellpd（无 LENIENT）
bash stress/scripts/chaos-offshoot-fail.sh
# 期望：status=failed，stale routes=0（脚本已有断言）
```

实现 ISSUE-02 后应 **无需** 脚本内 `CELLP_STRICT_OFFSHOOT_FORK=1`。

### 5.3 集成（D1 branch 失败）

**方案 A（推荐）：** cellpd 带 `CELLP_E2E_INJECT_D1_BRANCH_FAIL=1`，走 `v1-d1-branch.sh` 前半（父 ready）后对子 version POST，poll `failed`，preview ≠ 200。

**方案 B（最小）：** 仅文档 + `deploy_policy_test`；issue 验收「branch 失败」用 offshoot chaos 代表「数据步骤失败」——需在 PR 说明 D1 与 offshoot 共用同一 gate。

### 5.4 回归

```bash
bash e2e/scripts/v1-d1-branch.sh      # 成功路径仍 ready
bash e2e/scripts/v12-kv-branch.sh     # binding branch 成功路径
bash e2e/scripts/v5-saga-compensate.sh
./e2e/scripts/run-all.sh
```

### 5.5 Lenient 冒烟（opt-out 未破坏）

```bash
CELLP_LENIENT_DEPLOY=1 ./dev/scripts/up.sh   # 或仅 export 后重启 cellpd
# 手动制造 D1 branch 失败（inject）→ 仍可能 ready（与旧行为一致）
```

---

## 6. 验收勾选（对应 ISSUE-02）

| Issue 条目 | 验证方式 |
|------------|----------|
| 默认：任一步失败 → `failed` / 不可 ready；preview 非 200；job failed | `chaos-offshoot-fail.sh` +（可选）`v5b-deploy-d1-branch-fail.sh`；`GET .../versions/{id}` → `failed`；`curl` preview ≠ 200 |
| `CELLP_LENIENT_DEPLOY=1` opt-out + 文档 | `dev/.env.example` + `dev/README.md` + `site` 环境变量页；lenient 冒烟 |
| 更新 `dev/` 默认 env；test-plan / e2e 保持 fail-closed | `up.sh` **不**设 lenient；`run-all.sh` 无 lenient；test-plan 更新 |
| 单元或集成测试覆盖 branch 失败 → failed | `deploy_policy_test.go` + chaos 或 v5b |
| `site/` 或 `test-plan.md` 一句默认严格 | 两处至少各一句 |

---

## 7. 预估工作量

| 项 | 规模 |
|----|------|
| Go policy + orchestrator 接线 | S（<100 行） |
| 单测 | S |
| INJECT + e2e 脚本 | M（cellpd 重启与环境恢复） |
| 文档 | S |
| **合计** | ~0.5–1 人日 |

---

## 8. 参考代码索引

- `cellp/internal/orch/orchestrator.go` — `runDeploy` 121–293，`branchStep` 307–318，`strictOffshoot` 303–305
- `cellp/internal/orch/d1_branch.go` — `D1DeployPlanForVersion`（父非 ready 已 fail）
- `cellp/internal/orch/binding_branch.go` — `runBindingBranches` 调用前计划
- `cellp/internal/runtime/manager.go` — `D1Branch` 361–393
- `e2e/scripts/v5-saga-compensate.sh` — failed + 无泄漏 route
- `stress/scripts/chaos-offshoot-fail.sh` — strict fork 失败补偿

---

## 实施记录

**日期：** 2026-08-31

- 新增 `cellp/internal/orch/deploy_policy.go`：`deployFailClosed()` 默认 `true`；`CELLP_LENIENT_DEPLOY=1` 为 opt-out；`CELLP_STRICT_OFFSHOOT_FORK` 废弃（与默认等价，与 lenient 冲突时 log 一次）。
- `orchestrator.go`：`branchStep`、D1 branch/seed 统一走 `deployFailClosed()`；`runBindingBranches` 注释标明始终 fail-closed。
- `runtime/manager.go`：`CELLP_E2E_INJECT_D1_BRANCH_FAIL=1` 在 `D1Branch` 入口注入失败。
- 单测 `deploy_policy_test.go`；e2e `v5b-deploy-d1-branch-fail.sh` + `MANIFEST`；`chaos-offshoot-fail.sh` 不再依赖 `CELLP_STRICT_OFFSHOOT_FORK`。
- 文档：`dev/.env.example`、`dev/README.md`、`docs/test-plan.md`（TP-V5B）、`site/docs/guides/environment-variables.md`。

**验证：** 本地 `go test ./...` 因代理/网络拉依赖超时未在本环境跑通；请在有模块缓存或网络的环境下执行 `cd cellp && go test ./...` 与 `./e2e/scripts/v5b-deploy-d1-branch-fail.sh`（需 dev 栈）。
