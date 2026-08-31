# ISSUE-02 实现审查：Deploy 默认 fail-closed

**审查日期：** 2026-08-31  
**依据：** [ISSUE-02-deploy-fail-closed-branch.md](./ISSUE-02-deploy-fail-closed-branch.md)、[ISSUE-02-plan.md](./ISSUE-02-plan.md)  
**范围：** `deploy_policy.go`、`orchestrator.go`（`branchStep` / D1 / `runBindingBranches` 注释）、`runtime/manager.go`（`CELLP_E2E_INJECT_D1_BRANCH_FAIL`）、`e2e/scripts/v5b-deploy-d1-branch-fail.sh`、`stress/scripts/chaos-offshoot-fail.sh`、`dev/` 与 `docs/test-plan.md`、`site/docs/guides/environment-variables.md`

**说明：** 审查为静态代码 + 文档对照；本环境 `go test` 因拉取模块依赖超时未跑通（与 plan「实施记录」一致）。合并前需在具备模块缓存/网络的机器上执行下文「建议执行的验证」。

---

## 1. 验收标准（逐条）

| # | 验收项 | 结论 | 证据 / 说明 |
|---|--------|------|-------------|
| 1 | 默认：offshoot / D1 import·branch / binding branch 任一步失败 → version `failed`（或不可 ready）；preview 非 200；deploy job failed | **满足（代码路径）** | `processOne`：`runDeploy` 出错 → `StatusFailed` + `FailJob` + `compensateDeploy`（`orchestrator.go:90-96`）。默认 `deployFailClosed()==true`：`branchStep` 与 D1 branch/seed 失败 `return err`（`307-318`、`240-259`）。`runBindingBranches` 始终 `return err`，无 lenient 分支（`462-487`）。e2e `v5b` 断言子 version `failed` 且 Gateway ≠ 200。 |
| 2 | 保留显式 opt-out `CELLP_LENIENT_DEPLOY=1`，文档说明 | **满足** | `deploy_policy.go`；lenient 时 offshoot/D1 仅 warn 继续。`dev/.env.example`、`dev/README.md`、`site/docs/guides/environment-variables.md` 已说明。 |
| 3 | 更新 `dev/` 默认 env（需 lenient 时显式开）；`test-plan` / e2e 保持 fail-closed | **满足** | `dev/scripts/up.sh` 未设置 `CELLP_LENIENT_DEPLOY`（仅 `.env.example` 注释）。`docs/test-plan.md` TP-V5B；`e2e/scripts/MANIFEST` 含 `v5b-deploy-d1-branch-fail.sh`。 |
| 4 | 单元或集成测试覆盖「branch 失败 → failed」 | **部分满足** | **D1 branch：** `CELLP_E2E_INJECT_D1_BRANCH_FAIL` + `v5b-deploy-d1-branch-fail.sh`。**单元：** `deploy_policy_test.go` 只测 env→布尔，不测完整 saga。**Offshoot fork：** `stress/scripts/chaos-offshoot-fail.sh` 依赖默认 fail-closed（已去掉 `CELLP_STRICT_OFFSHOOT_FORK`），但**不在** `run-all.sh` 门禁内。 |
| 5 | `site/` 或 `docs/test-plan.md` 一句说明默认严格 | **满足** | `docs/test-plan.md` TP-V5B；`site/docs/guides/environment-variables.md` 段落。`site/docs/how-it-works.md` 未单独提及（plan 允许二选一）。 |

**issue 原文勾选框**（`ISSUE-02-deploy-fail-closed-branch.md`）仍为 `[ ]`，建议在合并时改为 `[x]` 或链到本审查。

---

## 2. 实现质量摘要

### 2.1 核心语义

- `strictOffshoot` 已移除，统一为 `deployFailClosed()`（`deploy_policy.go`），默认 `true`；`CELLP_STRICT_OFFSHOOT_FORK=1` 与默认等价，与 lenient 同设时 log 一次后 lenient 优先 — 与 plan 一致。
- Deploy 失败补偿链未改：`compensateDeploy` 在 job 失败时停用 route / 销毁 branch / Stop celld，与 TP-V5 一致。

### 2.2 与 plan 的差异（可接受）

- `runBindingBranches` 在 `CELLP_LENIENT_DEPLOY=1` 下**仍** fail-closed；`dev/README.md` 写的是「offshoot / D1 seed&branch」lenient，与实现一致。若产品期望 lenient = 含 KV/R2/Queue，需 follow-up（当前 plan 明确 binding 不加 lenient）。

### 2.3 已知非本 issue、仍影响「假就绪」的洞

- `runtime.Manager.D1Branch`：无 `celld` 可执行文件时 `return nil`（`manager.go:371-373`）。plan 已标 follow-up；e2e 有 `require_celld_cli`，门禁栈上通常不触发。

---

## 3. 阻塞问题

**无** — 在假定 `v5b` 与 `run-all.sh` 在完整 dev 栈上通过的前提下，可认为 ISSUE-02 验收达标。

若 CI/本地 **`v5b` 失败**，则视为阻塞，需先查 cellpd 重启后是否仍带齐 RustFS/registry 等环境（脚本仅 `env CELLP_E2E_INJECT_D1_BRANCH_FAIL=1 cellpd`，与 `v4b` 同模式，依赖 `run-all` 前栈已 healthy）。

---

## 4. 建议（非阻塞）

| 优先级 | 项 | 说明 |
|--------|-----|------|
| 高 | `v5b` 增加 `trap` 恢复 cellpd | `v4b-promote-offshoot-fail.sh` 有 `trap restore_cellpd EXIT`；`v5b` 若在断言前 `fail`，可能遗留 `CELLP_E2E_INJECT_D1_BRANCH_FAIL=1`，污染后续 MANIFEST 用例。 |
| 中 | 合并前跑门禁 | 见 §5。 |
| 低 | `e2e/README.md` 脚本列表补 `v5b-deploy-d1-branch-fail.sh` | plan 未强制，与 TP-V5B 对齐可读性更好。 |
| 低 | `v5b` 可选断言 deploy job `failed` | 验收写「deploy job failed」；当前只 poll version `status`，与多数 e2e 一致，可增强。 |
| 低 | lenient 冒烟 | plan 提到 `CELLP_LENIENT_DEPLOY=1` 冒烟；无自动化用例，可手工或 follow-up 单测（需可控 inject + lenient）。 |
| 低 | `dev/AGENTS.md` | plan 标可选，未改可接受。 |

---

## 5. 建议执行的验证

**合并 ISSUE-02 相关变更前建议至少：**

```bash
cd cellp && go test ./internal/orch/... -run DeployFailClosed -v
cd cellp && go test ./...
```

**完整门禁（含 TP-V5B）：**

```bash
./dev/scripts/health.sh   # 或 ./dev/scripts/up.sh
./e2e/scripts/run-all.sh
```

**不必为 ISSUE-02 单独全量跑 stress**，但若改动了 offshoot 路径，建议抽跑：

```bash
bash stress/scripts/chaos-offshoot-fail.sh
```

**不必**为 ISSUE-02 单独跑 `v1-d1-branch.sh`（除非怀疑 D1 branch 与 inject 交互）；`run-all` 已包含 `v1-d1-branch.sh` 与 `v5b`。

---

## 6. 结论

| 维度 | 判定 |
|------|------|
| 是否满足 ISSUE-02 验收 | **是**（待 `go test` + `run-all.sh` 绿） |
| 是否建议合并 | **是**，先修或接受「v5b 无 EXIT trap」风险 |
| 阻塞 | 无代码语义阻塞；**运行证据**缺失时以 e2e 为准 |

**审查人：** Composer（静态审查）
