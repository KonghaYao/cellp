# ISSUE-02: Deploy 默认 fail-closed（D1 branch / offshoot / binding branch 失败不得 ready）

**优先级:** P0 · **类型:** 可靠性 / DX  
**关联:** `orchestrator.go` strictOffshoot、`D1Branch` warn 路径、`CELLP_STRICT_OFFSHOOT_FORK`

## 问题

默认未设 `CELLP_STRICT_OFFSHOOT_FORK` 时，offshoot fork、D1 import/branch、binding branch 失败可 warn 后继续 **ready**。用户 Gateway 200 但 D1/数据空或错 — **假就绪**。

## 验收标准

- [ ] 默认行为：上述任一步失败 → version `failed`（或不可 ready），preview 非 200 / deploy job failed
- [ ] 保留显式 opt-out 环境变量（如 `CELLP_LENIENT_DEPLOY=1`）用于本地调试，文档说明
- [ ] 更新 `dev/` 默认 env 若需要 lenient；`test-plan` / e2e 保持 fail-closed
- [ ] 单元或集成测试覆盖「branch 失败 → failed」
- [ ] `site/` 或 `docs/test-plan.md` 一句说明默认严格

## 非目标

- 改变 celld RPC 契约
- 自动重试 branch（可 follow-up）

## 调研提示

- `orchestrator.go` 303-318, 237-251, `binding_branch.go`
- `e2e/scripts/v1-d1-branch.sh` 与默认 env 差异
