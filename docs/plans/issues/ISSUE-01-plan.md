# ISSUE-01 实现计划：Promote 硬门禁 offshoot promote

**对应 issue：** [ISSUE-01-promote-offshoot-gate.md](./ISSUE-01-promote-offshoot-gate.md)  
**状态：** 计划（未实施）  
**日期：** 2026-08-31

---

## 1. 范围与风险

### 1.1 目标

将 AD-5 promote saga 与 `docs/decisions.md` §6 表述对齐：**`offshoot_promote` 失败时必须中止 saga**，不得执行 `SetProdVersionCAS` / `activate_prod_route`；已执行的 `drain_old` / `deactivate_old_route` 须走既有 compensation 恢复旧 prod 路由。

### 1.2 根因（现状）

`cellp/internal/orch/orchestrator.go` 中 `Promote` 在 `branch.Promote` 失败时仅 `log.Printf` warn，随后仍执行 CAS 与激活新 prod 路由（约 L379–L400）。与 AD-5「任一步失败按逆序补偿」矛盾。

对比：deploy 路径对 offshoot 步骤使用 `branchStep` + `CELLP_STRICT_OFFSHOOT_FORK=1` 可失败 fast；**promote 路径未复用该模式**。

### 1.3 范围（in）

| 项 | 说明 |
|----|------|
| Orchestrator | `Promote` 失败分支 + compensation |
| API | 非 200 + 稳定 `error` 码 |
| 单测 | `go test` 覆盖 promote 失败路径（无需真实 offshoot） |
| E2E | 新脚本或扩展现有门禁，断言 prod 未切换 |
| 文档 | `docs/decisions.md` §6（及必要时 `docs/plans/REVIEW.md` AD-5 一句） |

### 1.4 非目标（issue 已列 + 本计划确认）

- offshoot promote **逆向**补偿（reverse promote）
- 合并 celld bucket / D1 数据修复
- Dashboard（`web/`）、公开站点大改（除非补一句 AD-5 行为说明）
- **ISSUE-04**（多 ready cron）— 见 §5

### 1.5 风险与注意点

| 风险 | 缓解 |
|------|------|
| **`hasOffshoot()==false` 时 `branch.Promote` 恒为 nil** | 无 CLI 环境仍可无门禁完成 CAS。一期：在 decisions 写明「有 offshoot 时 promote 为硬门禁」；可选 follow-up：`CELLP_REQUIRE_OFFSHOOT=1` 在 prod 配置（非 issue 必交，可记 backlog） |
| E2E 依赖 offshoot 真实失败 | 优先 **进程内注入**（`CELLP_E2E_INJECT_OFFSHOOT_PROMOTE_FAIL`），与 `CELLP_E2E_INJECT_DEPLOY_FAIL` / `git_sha=fail` 模式一致；备选只读 store（参考 `stress/scripts/chaos-offshoot-fail.sh`，成本高、需重启 cellpd） |
| 旧 prod 路由在失败后被 deactivate | 修复后 `runCompensation` 应把 `oldProd` 的 `route.active` 设回 `true`；单测断言 registry 状态 |
| OpenAPI 仅 200/409 | 补充 5xx + `offshoot_promote_failed` 响应描述（与实现同步） |

---

## 2. 具体文件与函数改动清单

### 2.1 核心逻辑（P0）

**文件：** `cellp/internal/orch/orchestrator.go`

| 位置 | 改动 |
|------|------|
| `Promote` L379–L385 | `branch.Promote` 返回 `err != nil` 时：`o.runCompensation(ctx, compensated)`，`return` 包装错误（勿 log-only 继续） |
| `Promote` L383–L385 | **仅**在 offshoot promote **成功**后再 `append` 空 compensate（或删除无意义的空 fn）；失败路径不应向 `compensated` 追加无效项 |
| （可选统一） | 抽取 `promoteBranchStep` 或复用 `branchStep` 语义：promote 是否受 `CELLP_STRICT_OFFSHOOT_FORK` 约束 — **建议 promote 始终 strict**（与 issue 硬门禁一致），deploy 仍用现有 env |

**建议错误类型（同包或 `orch/errors.go`）：**

```go
var ErrOffshootPromote = errors.New("offshoot_promote_failed")
// return fmt.Errorf("%w: %v", ErrOffshootPromote, err)
```

**文件：** `cellp/internal/branch/manager.go`（测试钩子，小改）

| 位置 | 改动 |
|------|------|
| `Promote` 入口 | 若 `os.Getenv("CELLP_E2E_INJECT_OFFSHOOT_PROMOTE_FAIL") == "1"`，返回固定错误（便于 e2e / 单测，无需 mock 整个 Manager） |

> 备选：钩子只放在 `Orchestrator.Promote` 内、不碰 `branch` 包 — 单测可 `t.Setenv` 后调用 `branch.Promote` 的注入需在 orchestrator 层。推荐 **branch.Manager 注入 + orchestrator 单测集成**，与 `runtime.Manager` 的 `CELLP_E2E_INJECT_DEPLOY_FAIL` 对称。

### 2.2 API 层（P0）

**文件：** `cellp/internal/api/server.go` — `handlePromote`

| 现状 | 目标 |
|------|------|
| 任意 `orch.Promote` 错误 → `500` + `err.Error()` | `errors.Is(err, orch.ErrOffshootPromote)`（或 `strings.Contains` 稳定码）→ **`502` 或 `503`** + `{"error":"offshoot_promote_failed","message":"..."}` |
| | 其余错误保持 `500` / 现有 `404`/`409` |

**文件：** `cellp/api/openapi.yaml` — `POST .../promote`

- 增加 `502`/`503` 响应 schema，`error` 枚举含 `offshoot_promote_failed`

### 2.3 单测（P0）

**新建：** `cellp/internal/orch/promote_test.go`（或扩展现有 `orch` 测试）

| 用例 | 步骤 | 断言 |
|------|------|------|
| `TestPromote_OffshootFail_NoCAS` | 内存 sqlite store；两 version ready；`SetProdVersion`/`CAS` 设 `oldProd`；`t.Setenv("CELLP_E2E_INJECT_OFFSHOOT_PROMOTE_FAIL","1")`；`orch.Promote(ctx, demo, newVer)` | `Promote` 返回错误；`GetProject().ProdVersionID` 仍为 `oldProd`；`oldProd` route `active=true`（或至少新 version 未成为 prod route） |
| `TestPromote_OffshootFail_CompensatesOldRoute` | 同上，显式检查 `routes` 表 | `deactivate_old` 后失败时旧路由恢复 |

**可选：** `cellp/internal/api/server_test.go` — `TestPromoteOffshootFail`：httptest + 注入 env，期望 HTTP ≠ 200 且 body `error==offshoot_promote_failed`

**不强制：** 为 `branch.Manager` 引入 interface 重构 `Orchestrator`（工作量大；一期 env 注入足够）。

### 2.4 E2E（P0）

**新建：** `e2e/scripts/v4b-promote-offshoot-fail.sh`（或 `ve-promote-fail.sh`）

建议流程：

1. `require_platform` + `require_celld` + `ensure_project`
2. 创建 `V_OLD`、`V_NEW`，poll `ready`
3. 成功 promote `V_OLD` → prod（与 `ve-promote.sh` 一致）
4. 记录 `PROD_BODY_OLD`、`GET /v1/projects/{p}` 的 `prod_version_id`（应为 `V_OLD`）
5. **重启 cellpd** 并设置 `CELLP_E2E_INJECT_OFFSHOOT_PROMOTE_FAIL=1`（在 `dev/scripts` 或脚本内 kill + 启动 `cellpd`，与 `chaos-offshoot-fail.sh` 类似但 env 不同）
6. `POST .../versions/${V_NEW}/promote` → 期望 **HTTP 502/503/500**（脚本内允许集合），body 含 `offshoot_promote_failed`
7. 再次 GET project：`prod_version_id` 仍为 `V_OLD`
8. `curl` prod URL：body 仍与 step 4 一致（或仍等于 `V_OLD` preview body）
9. **teardown：** 去掉注入 env，重启 cellpd，避免污染后续 MANIFEST

**MANIFEST：** 在 `v4-promote-cutover.sh` **之后**插入一行（promote 成功路径仍由 v4/ve 覆盖）。

**test-plan：** 新增小节 **TP-V4b**（或并入 TP-V5 描述并改名 — 建议独立 TP-V4b 便于勾选 issue）。

**复用评估：**

| 资产 | 结论 |
|------|------|
| `v5-saga-compensate.sh` | 仅 deploy 失败；**不覆盖** promote |
| `chaos-offshoot-fail.sh` | fork 失败 + `CELLP_STRICT_OFFSHOOT_FORK`；可抄 **重启 cellpd** 模式，逻辑不直接复用 |
| `CELLP_STRICT_OFFSHOOT_FORK` | 仅 `branchStep`（deploy）；**不能**单独满足 issue，除非 promote 也走 strict（本计划采用 promote 硬失败，不必依赖该 env） |

### 2.5 文档（P0）

| 文件 | 改动 |
|------|------|
| `docs/decisions.md` §6 | 在 forward 列表后增加：**「`offshoot_promote` 为硬门禁：失败则不 CAS、不激活新 prod 路由，并执行已注册步骤的补偿。」** |
| `docs/plans/REVIEW.md` AD-5 | 若与 decisions 一致，同步一句（可选） |
| `docs/test-plan.md` | TP-V4b 行 + 命令 |
| `e2e/README.md` | 列出新脚本（若有维护列表） |

### 2.6 明确不改（除非发现连带 bug）

- `web/` Dashboard promote UI（已透传 API 错误即可）
- `celld/` submodule
- `site/docs`（ISSUE-01 非必须；promote 失败错误码可在 API 文档节补一句，低优先级）

---

## 3. 测试 / E2E 步骤（实施顺序）

```text
1. 改 orchestrator + branch 注入 → cd cellp && go test ./internal/orch/... ./internal/api/...
2. 全量 go test → cd cellp && go test ./...
3. 本地栈 → ./dev/scripts/up.sh && ./dev/scripts/health.sh
4. 新 e2e 单独 → bash e2e/scripts/v4b-promote-offshoot-fail.sh
5. 回归 promote 成功路径 → bash e2e/scripts/ve-promote.sh && bash e2e/scripts/v4-promote-cutover.sh
6. 全门禁 → ./e2e/scripts/run-all.sh
```

**证据（可选）：** `docs/evidence/issue-01-promote-gate.log`（issue 未强制，与仓库惯例一致即可）。

---

## 4. 验收勾选（映射 ISSUE-01）

| Issue 条目 | 本计划交付物 |
|------------|----------------|
| `branch.Promote` 失败不 CAS + 运行 compensation | §2.1 `orchestrator.go` + §2.3 单测 |
| `POST …/promote` 非 200 + 可区分错误码 | §2.2 `handlePromote` + OpenAPI |
| 新增 e2e 断言 prod 不变 | §2.4 `v4b-promote-offshoot-fail.sh` + MANIFEST |
| decisions / AD-5 硬门禁表述 | §2.5 |
| `go test ./...` + e2e 可单独跑 | §3 |

---

## 5. ISSUE-04 边界说明（同仓库、不同 issue）

**ISSUE-04**（多 ready 时 Cron 重复触发）与 ISSUE-01 **无共享代码路径**；promote 修复不解决 cron 倍增。

若在同一迭代处理 ISSUE-04，建议一期边界：

| 一期交付（文档 + 决策） | **Defer（代码）** |
|-------------------------|-------------------|
| `docs/decisions.md` 附录：策略选型（推荐「仅 prod version arm cron」） | orchestrator `Start` / deploy 后按 prod 剥离 wrangler `triggers.crons` |
| `site/docs` preview 与 cron 行为说明 | celld 行为变更、env `CELLPD_VARS` 全量 reconcile |
| 调研笔记：celld deploy triggers 与 `manager.go` | e2e「两 ready 仅触发一次」脚本 |

**ISSUE-01 实施不包含 ISSUE-04 任何代码**；避免在 `Promote` 改动中顺带改 cron。

---

## 6. 工作量粗估

| 块 | 估时 |
|----|------|
| Orchestrator + API + 单测 | 0.5–1d |
| E2E 脚本 + cellpd 重启/env | 0.5d |
| 文档 + OpenAPI + run-all 验证 | 0.25d |

**合计：** ~1–1.5 人日。

---

## 实施记录

**日期：** 2026-08-31

- **`cellp/internal/orch/orchestrator.go`**：`offshoot_promote` 失败时 `runCompensation` 并返回 `ErrOffshootPromote`，不再 log-only 后继续 CAS。
- **`cellp/internal/orch/errors.go`**：新增 `ErrOffshootPromote`。
- **`cellp/internal/branch/manager.go`**：`CELLP_E2E_INJECT_OFFSHOOT_PROMOTE_FAIL=1` 注入失败（与 deploy 注入对称）。
- **`cellp/internal/api/server.go`**：`handlePromote` 对 `ErrOffshootPromote` 返回 **502** + `offshoot_promote_failed`。
- **`cellp/api/openapi.yaml`**：promote 增加 502 响应。
- **`cellp/internal/orch/promote_test.go`**：覆盖无 CAS、旧路由补偿。
- **`e2e/scripts/v4b-promote-offshoot-fail.sh`** + **MANIFEST**；文档 `decisions.md` §6、`REVIEW.md` AD-5、`test-plan.md` TP-V4b、`e2e/README.md`。
- **验证：** `cd cellp && go test ./...`（实施时执行）。
