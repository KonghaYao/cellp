# Phase 7 T5 — Bindings E2E + TP

> **Track：** **P7-T5** · `e2e/scripts/` · `docs/test-plan.md` · 必要时 `dev/examples/`  
> **TP：** **TP-V9 · TP-V10 · TP-V11**（对齐 VALIDATION V9–V11）· **TP-VE-1** 健康路径修订  
> **规格：** [DESIGN.md §8.4](../../DESIGN.md) API · [§8.3 / AD-7](../decisions.md) 空起步 · [§8.6](../../DESIGN.md) health  
> **风格：** 端口级 curl，同 [`v1-d1-seed.sh`](../../e2e/scripts/v1-d1-seed.sh)（`lib.sh` · `create_version` · `poll_version` · 证据目录）  
> **前置：** T1 清单 + T2 KV/Queue operator + T3 Workflow/health 合同；脚本可先写，**全绿等 API 落地**  
> **不做：** 浏览器 / Playwright（那是 T4）；不改 `celld/` submodule

Dashboard 全绿 **不能** 勾 V9–V11。本 track 打 **cellpd `:8790`**（及 Gateway 仅用于 worker 发消息），证明真实 fleet 隔离。

## Exit Criteria

- [ ] `e2e/scripts/v9-kv.sh` exit 0：deploy KV example · API put/get · **兄弟 version 隔离**
- [ ] `e2e/scripts/v10-queue.sh` exit 0：bindings 含 queue · info/peek · purge 无 `force` → 400
- [ ] `e2e/scripts/v11-workflow-cron.sh` exit 0：workflow instances **不 500** · cron 出现在 bindings
- [ ] `dev/scripts/health.sh` + `e2e/scripts/health-all.sh` + `lib.sh require_celld` 使用 **`/.well-known/celld/health`**
- [ ] `docs/test-plan.md` 写入 TP-V9/V10/V11（及 TP-VE-1 路径更新）
- [ ] 证据落在 `docs/evidence/`（下表）
- [ ] `e2e/scripts/MANIFEST` 追加 v9/v10/v11（顺序在 v7 之后）
- [ ] `RUN_GATES=0 ./e2e/scripts/run-all.sh` 仍绿（含新脚本）

## 现状与缺口

| 项 | 现状 | T5 |
|----|------|-----|
| KV example | `celld/examples/kv`：`VALUES` / id `example-values`；Worker `PUT/GET/DELETE /KEY` | **用这个** 作 artifact |
| Workflow | `celld/examples/workflow`：`REPORTS` / name `report-builder` | list instances |
| Cron | `celld/examples/cron`：`triggers.crons: ["* * * * *"]` | bindings 含 cron |
| R2 | `celld/examples/r2` 有对象 Worker | **本期无** 对象 API；不必单开脚本 |
| **Queue** | **`celld/examples/` 无任何 queue wrangler** | **缺口：** 在 `dev/examples/queue/` 放 **最小 producer-only** worker（见下） |
| health | `health.sh` / `health-all.sh` / `lib.sh` 仍打 `/__celld/health` | **必须改** well-known（DESIGN §8.6） |
| `lib.sh` | 有 `api_get` / `api_post` / `api_delete` | 补 **`api_put`**（KV） |

celld 限制（原样遵守，不要在 e2e 里「修」）：**一个 consumer script 不能再 `export fetch()`**。因此 queue 示例必须是 **仅 producer**（有 `fetch` + `env.TASKS.send`），**不要**在同一 wrangler 里挂 `queues.consumers`。

## 顺序

```
改 health 路径（阻塞 TP-VE-1） ──┐
v9-kv.sh 骨架                 ──┼──> T2/T3 API 落地 ──> 全绿
v10 依赖 dev/examples/queue   ──┤
v11-workflow-cron.sh          ──┘
test-plan.md TP 条目 + MANIFEST + evidence
```

T5 **可先提交脚本**（`set -euo pipefail` + `fail` 信息明确）。未实现的 API → 脚本 fail 并指向 T2/T3，不要 soft-pass。

## Artifact 约定（同 v1-d1-seed）

Orchestrator `destDir = $ARTIFACTS_DIR/{project}/{version}`。脚本：

1. `ensure_project` · `cleanup_e2e_versions`
2. `mkdir -p "$DEST_DIR"`，拷贝 example 的 `index.js` + `wrangler.jsonc`（必要时改 `name` / id，避免脏 demo-app guestbook）
3. `create_version` · `poll_version … ready`
4. **只** `curl` `PLATFORM_URL/v1/...`（ADMIN_TOKEN）；KV 隔离断言走 API，不要 `celld kv --bucket` 当主路径（主路径 = cellpd 包装 CLI）

Gateway 仅 queue 脚本需要：`POST ${GATEWAY_URL}/{project}/{version}/enqueue` 往队列塞一条，再 `GET …/queues/{name}/peek`。KV 的 put/get **以 API 为准**（V9：「经 `:8790`」）；可选再 `curl` Worker `/KEY` 作交叉检查，失败则 log WARN 不代替 API 断言。

## P7-T5a — health 公共路径

celld 0.4.0：**`GET /.well-known/celld/health`**。旧 `/__celld/health` 不再作为验收。

| 文件 | 改动 |
|------|------|
| `dev/scripts/health.sh` | celld 探活改为 well-known |
| `e2e/scripts/health-all.sh` | 同上 |
| `e2e/scripts/lib.sh` `require_celld` | 同上 |
| `docs/test-plan.md` **TP-VE-1** | 检查表改为 `/.well-known/celld/health` |

**顺手（同一 PR，避免跑脚本仍打旧路径）：** `dev/scripts/up.sh` · `up-native.sh` · `simulate-cd.sh` 启动等待循环。Runtime Manager 已用新路径，不必改 Go。

证据：`docs/evidence/v11-health-path.log`（`health.sh` transcript）。

## P7-T5b — `v9-kv.sh`（TP-V9 / V9）

**Example：** `celld/examples/kv`  
`{ns}` = wrangler `id` = **`example-values`**（verbatim）。

场景：

1. 两个 ready version：`VA`（无 parent）· `VB`（`parent_version_id=VA`），**各自** destDir 拷贝同一 kv example
2. `GET /v1/projects/{p}/versions/{VA}/bindings` → `bindings` 含 `type=kv` 且 `namespace_id=example-values`
3. `PUT …/kv/example-values/keys/e2e-greeting` body 含 value（UTF-8 或合同规定的 encoding）
4. `GET` 同 key → body 匹配
5. **隔离：** `GET …/versions/{VB}/kv/example-values/keys/e2e-greeting` → **404**（空起步，不是 prod 值）
6. 在 VB PUT 另一值；再 GET VA 仍为步骤 3 的值（互不写穿）

可选：`GET …/kv` 列表 · `GET …/kv/{ns}` info。

**不做：** bulk · inherit（AD-7）。

证据：`docs/evidence/v9-kv-e2e.log` + `docs/evidence/v9-kv-e2e.json`（project/version/ns/http codes）。

## P7-T5c — Queue 缺口 + `v10-queue.sh`（TP-V10 / V10）

### 缺口（必须写进实现注释）

`celld/examples/` **没有** queue worker。T5 **允许** 新增极小示例（**不是**改 submodule）：

```
dev/examples/queue/
  wrangler.jsonc    # queues.producers only
  index.js          # fetch: POST /enqueue → env.TASKS.send({...})
```

`wrangler.jsonc` 对齐 T1 解析器（见 `cellp/internal/runtime/bindings.go`）：

```jsonc
{
  "name": "queue-producer",
  "main": "index.js",
  "compatibility_date": "2026-01-01",
  "queues": {
    "producers": [{ "binding": "TASKS", "queue": "tasks" }]
  }
}
```

**禁止** 同脚本加 `consumers`（celld：consumer 不能 `export fetch()`）。pause/resume 测的是 broker；无 consumer 时 pause 仍应 200 或合同规定的可断言状态，以 T2 OpenAPI 为准。

### 脚本步骤

1. 拷贝 `dev/examples/queue` → destDir · create+poll ready
2. `GET …/bindings` 含 `type=queue` · `queue_name=tasks`
3. `GET …/queues/tasks` info → 200
4. Gateway `POST /enqueue` 一条；`GET …/queues/tasks/peek?limit=10` 能见到积压（body base64 可解码）
5. `POST …/purge` **无** `{force:true}` → **400**
6. `POST …/purge` 带 `force` → 200；再 peek 为空（或 count=0）

可选：pause 后再 enqueue，peek 仍可见积压（VALIDATION V10：「pause 后 peek 仍可见」）——若 T2 语义不同，以 OpenAPI 为准并在证据里写清。

**不做：** pull consumer；跨 version 共享同一 queue broker（第二 version 再 deploy 应空队列 = AD-7）。建议短断言：`VC` child 的 peek 不含 VA 的消息。

证据：`docs/evidence/v10-queue-e2e.log` · `docs/evidence/v10-queue-e2e.json`。

## P7-T5d — `v11-workflow-cron.sh`（TP-V11 / V11）

可 **一个脚本两段**（推荐）或拆 `v11-workflow.sh` + `v11-cron.sh`（MANIFEST 两条）。推荐单脚本减 version 配额压力。

### Workflow

1. 拷贝 `celld/examples/workflow` → destDir · ready
2. `GET …/bindings` 含 `type=workflow` · `workflow_name=report-builder`（或 binding `REPORTS`）
3. `GET …/workflows` 200
4. `GET …/workflows/report-builder/instances` → **HTTP ≠ 500**（空数组合法）
5. 可选：Gateway `GET /create?url=…` 造一条实例后再 list 非空（不阻塞；create 依赖出站 fetch）

**禁止** 调 pause/resume/restart（无 API）。

### Cron

1. 另起 version（或同脚本第二 destDir）拷贝 `celld/examples/cron`
2. `GET …/bindings`：`crons` 含 `* * * * *` **或** `bindings[]` 含 `type=cron`
3. **无**「触发一次」API 断言（不存在则 404 即可记 WARN）

证据：`docs/evidence/v11-workflow-cron-e2e.log` · `docs/evidence/v11-workflow-cron-e2e.json`。

## `lib.sh` 增量

```bash
api_put() {  # path, body, token
  curl -sf -X PUT "${PLATFORM_URL}${path}" \
    -H "$(api_auth "$token")" -H "Content-Type: application/json" -d "$body"
}
```

`require_celld` 改为 well-known。不要新增 offshoot 依赖（KV/Queue/WF **无** branch）。

## MANIFEST

在 `v7-external-ci.sh` 之后追加：

```
v9-kv.sh
v10-queue.sh
v11-workflow-cron.sh
```

`run-all.sh` 无需改逻辑（读 MANIFEST）。新脚本必须 `chmod +x`。

`e2e/README.md` 补一小节 Bindings（V9–V11），避免后人只记得 D1。

## `docs/test-plan.md` 条目（T5 负责写入）

在 **§H D1** 之后新增 **§I. Bindings（Phase 7 · celld 0.4.0）**。勾选保持 `[ ]` 直到脚本绿。

### [ ] TP-V9 — celld KV operator（经 cellpd）

| 命令 | `e2e/scripts/v9-kv.sh` |
| 通过 | bindings 含 kv；put/get 200；子 version GET 同 key **404**；父值不变 |
| 证据 | `docs/evidence/v9-kv-e2e.log` · `v9-kv-e2e.json` |
| 对齐 | VALIDATION **V9** |
| 不做 | bulk · inherit（AD-7） |

### [ ] TP-V10 — celld Queue operator

| 命令 | `e2e/scripts/v10-queue.sh` |
| 通过 | bindings 含 queue；info/peek 200；purge 无 force → **400** |
| 证据 | `docs/evidence/v10-queue-e2e.log` · `v10-queue-e2e.json` |
| 对齐 | VALIDATION **V10** |
| 缺口 | `celld/examples` 无 queue → 使用 `dev/examples/queue` |
| 不做 | pull consumer · 跨 version 共享 queue |

### [ ] TP-V11 — Workflow 只读 + Cron 清单

| 命令 | `e2e/scripts/v11-workflow-cron.sh` |
| 通过 | bindings 含 workflow 与 cron；`GET …/workflows/{name}/instances` 不 500 |
| 证据 | `docs/evidence/v11-workflow-cron-e2e.log` · `v11-workflow-cron-e2e.json` |
| 对齐 | VALIDATION **V11** |
| 不做 | workflow 控制 · R2 对象浏览器 · Cron 平台调度 |

### TP-VE-1（修订，非新 ID）

| 检查 | `:8790/v1/health` · `:8787/health` · `:8792/.well-known/celld/health` → 200 |
| 命令 | `e2e/scripts/health-all.sh` · `dev/scripts/health.sh` |

Dashboard TP-UI-7..12 **不在本文件实现**（T4 写 Playwright；test-plan §F 由 T4 或文档 owner 补 `[ ]` 条目）。T5 若顺手在 §F 加一行「见 phase-7-t4」以免漏，可以。

## 证据路径汇总

| 路径 | 来源 |
|------|------|
| `docs/evidence/v9-kv-e2e.log` | `v9-kv.sh` |
| `docs/evidence/v9-kv-e2e.json` | 结构化结果 |
| `docs/evidence/v10-queue-e2e.log` | `v10-queue.sh` |
| `docs/evidence/v10-queue-e2e.json` | 结构化结果 |
| `docs/evidence/v11-workflow-cron-e2e.log` | `v11-workflow-cron.sh` |
| `docs/evidence/v11-workflow-cron-e2e.json` | 结构化结果 |
| `docs/evidence/v11-health-path.log` | `health.sh` / `health-all.sh` |

JSON 最小字段：`project` · `versions[]` · `exit` · `timestamp` · 相关 HTTP code。目录已 gitignore `*.log`；**json 可提交**（与 `d1-branch-multi-100mb.json` 同类）。

## 验证

```bash
./dev/scripts/health.sh          # celld well-known
./e2e/scripts/health-all.sh
./e2e/scripts/v9-kv.sh
./e2e/scripts/v10-queue.sh
./e2e/scripts/v11-workflow-cron.sh
RUN_GATES=0 ./e2e/scripts/run-all.sh
```

栈：`./dev/scripts/up.sh` 或 `up-native.sh`。version 上限 5 ready/project：脚本必须 `cleanup_e2e_versions`，KV 隔离用 **2** 个 id，queue+workflow+cron 控制并发，必要时错开 `DEV_PROJECT`。

## 禁止

- 改 `celld/` · `go.mod` · 冻结 D1 RPC
- Dashboard / Playwright（T4）
- `celld kv|queue` 直连当 **唯一** 断言（允许 debug log，pass/fail 看 `:8790`）
- 见 AD-6 / AD-7；inherit/copy
- R2 对象 e2e；Workflow 控制 API
- 在 queue example 里同时挂 consumer + `fetch`

## Subagent prompt

```
Track P7-T5 Bindings E2E. Port-level curl only, same style as e2e/scripts/v1-d1-seed.sh.
Read DESIGN.md §8.4 §8.6, VALIDATION V9–V11, docs/plans/phase-7-t5-e2e.md,
celld/examples/kv, workflow, cron wrangler.jsonc.
GAP: celld/examples has no queue — add tiny producer-only worker at
dev/examples/queue/ (no consumers; consumer cannot export fetch).
Scripts: v9-kv.sh (put/get + sibling isolation), v10-queue.sh,
v11-workflow-cron.sh. Health: /.well-known/celld/health in health.sh,
health-all.sh, lib.sh require_celld. Add api_put. MANIFEST after v7.
Write TP-V9/V10/V11 in docs/test-plan.md. Evidence under docs/evidence/.
Do not touch web/ or celld/ submodule. Pass/fail via cellpd :8790.
```
