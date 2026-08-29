# Phase 7 — celld 0.4.0 Bindings（KV · Queue · Workflow · Cron）

> **规格：** [DESIGN.md §8](../../DESIGN.md)（唯一设计）  
> **决策：** [decisions.md AD-6 · AD-7](../decisions.md)  
> **验收：** [VALIDATION.md V9–V11](../../VALIDATION.md)  
> **状态：** 计划中（2026-08-29）

沿用 celld 运行时与 operator CLI。**D1 仍是唯一有 branch 的绑定**；KV / R2 / Queue / Workflow 子 version **空起步**。R2 无 CLI → 无对象浏览器。Workflow 无 CLI → 只读 list。

## Tracks

| ID | 范围 | 计划文件 | 实现边界 |
|----|------|----------|----------|
| **P7-T1** | Bindings 清单 API + wrangler 解析 | [phase-7-t1-api.md](./phase-7-t1-api.md) | `cellp/internal/runtime` parse · `cellp/internal/api` · OpenAPI |
| **P7-T2** | KV + Queue operator（包装 `celld kv` / `celld queue`） | [phase-7-t2-runtime.md](./phase-7-t2-runtime.md) | `cellp/internal/runtime` · API handlers · tests |
| **P7-T3** | Workflow 只读 + Cron 清单 + health 脚本 | [phase-7-t3-readonly.md](./phase-7-t3-readonly.md) | `cell list` wrap · `dev/` `e2e/` health 路径 |
| **P7-T4** | Dashboard Bindings hub | [phase-7-t4-dashboard.md](./phase-7-t4-dashboard.md) | **仅 `web/`** |
| **P7-T5** | E2E + TP | [phase-7-t5-e2e.md](./phase-7-t5-e2e.md) | `e2e/scripts/` · `docs/test-plan.md` · examples |

## 顺序

```
T1 清单 API ──┬──> T2 KV/Queue
              ├──> T3 Workflow/Cron/health
              └──> T5 e2e 脚本骨架
T2 + T3 ──> T4 Dashboard
T2 + T3 + T4 ──> T5 全绿 · verify
```

T4 **不得**在 T1–T3 API 合同未写入 OpenAPI 前开工写死路径。T5 可先写脚本，等 API 落地再跑。

## 禁止

- 见 AD-6 / AD-7
- KV/R2/Queue/Workflow inherit、copy、挂父 bucket
- `celld r2` 对象浏览器、Workflow pause/resume/restart
- Dashboard 直连 `:8792` / S3 / offshoot
- 改 `cellp/go.mod`（除非 deps owner）
- 改冻结契约 `D1-*-RPC.md`
- 改 `celld/` submodule（bindings 已在 0.4.0）

## Exit

- [ ] `GET /v1/projects/{id}/versions/{vid}/bindings` 
- [ ] KV list/get/put/delete/info 经 cellpd
- [ ] Queue info/peek/pause/resume/redrive/purge（purge 需 force）
- [ ] Workflow instances 只读不 500
- [ ] Dashboard Storage 总览 + KV + Queue + Workflow 页
- [ ] `cd cellp && go test ./...`
- [ ] Playwright：新 TP-UI 不直连 `:8792`
- [ ] health 脚本使用 `/.well-known/celld/health`
