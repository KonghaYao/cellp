# P7-T3 — Workflow 只读 + Cron 清单 + health 路径修正

> **规格：** [DESIGN.md §8.4 Workflows](../../DESIGN.md) · [§8.6 health](../../DESIGN.md)  
> **决策：** [AD-7](../decisions.md) — 无 `celld workflow` → 只读 `cell list`；无 `celld r2` → 无对象浏览器  
> **验收：** [VALIDATION.md V11](../../VALIDATION.md)  
> **父计划：** [phase-7-bindings.md](./phase-7-bindings.md)  
> **状态：** 计划中（2026-08-29）  
> **Module root：** `cellp/` — 本 track **不写** Dashboard（T4）· **不写** E2E 新场景（T5）

沿用 celld 0.4.0。控制面只包装已有 `celld cell list`，并从 T1 的 wrangler 解析结果读出 workflow / cron / r2 清单。不发明 operator 协议。

## 依赖

| 前置 | 本 track 怎么用 |
|------|-----------------|
| **P7-T1** `runtime.ParseBindings` | `workflows[]` → `Binding{Type:"workflow", Name, WorkflowName, ClassName}`；`triggers.crons` → `Crons` + `Binding{Type:"cron"}`；`r2_buckets` → `Binding{Type:"r2"}` |
| **P7-T1** `GET .../bindings` | Cron / R2 **只出现在这里**。T3 **不**再加 `/crons`、`/r2` 路由 |
| celld ≥ 0.4.0 | `celld cell list [CLASS] --all --json --bucket …`；`cli.rs` **无** `workflow` / `r2` 子命令（已核对 `Action`：`deploy/dev/cell/d1/kv/queue/connect/credentials/token/disconnect`） |
| Runtime `Health()` | **已是** `GET /.well-known/celld/health`（`cellp/internal/runtime/manager.go`）。本 track 只改 **dev / e2e 脚本与验收表** |

T1 未合入时不得假设新路径；解析函数已在 `cellp/internal/runtime/bindings.go`，T3 只消费，不重写 wrangler 解析。

## celld 如何命名 Workflow cell（过滤依据）

已核对 `celld/docs/README.md`「List Durable Objects」、`celld/crates/celld/cell_cli.rs`、`deploy.rs`、`js.rs`、`js/harness.js`。

### 清单命令

```
celld cell list [CLASS] --all --json --bucket s3://cellp-celld/{project}/{version} \
  --endpoint "$S3_ENDPOINT" --region "$AWS_REGION"
```

- stdout：**NDJSON**，每行一个对象；stderr 才是进度 / `1000 cells shown`。
- `--all`：跨页读完整清单（每页最多 1000）。client-side 过滤必须全量，否则漏实例。
- 可选 positional `CLASS`：只列出该类。匹配是 **整段相等**（`split_once(':')` 的 class 侧）。

JSON 行（`cell_cli.rs` `Record::json`）：

```json
{"scope":"<class>:<id>","class":"<class>","id":"<id>","reserved":true}
```

`reserved == true` 当 class 是 D1 / KV / Queue / **Workflow**（含 script-scoped 名）。

### Scope 形状

| 层 | 实际值 | 来源 |
|----|--------|------|
| 运行时 DO class 常量 | `__Workflow` | `deploy::WORKFLOW_CLASS` |
| **列出的 class** | `__Workflow.{script_name}` | `deploy::workflow_class(script)`。`.` 分隔：JS 类名不能含 `.`，且不是 scope 的 `:` |
| namespace key | `cells:v1:<len>:<script>:__Workflow.{script}` | script-scoped；两 Worker 互不撞 cell |
| 内部 getByName | `{workflow_name}/{instance_id}` | `harness.js`；`/` 不在 workflow 名字 charset 内 |
| **列出的 id** | 64 位 hex（DO id） | `durable_object_id_for_name(namespace, "{workflow_name}/{instance_id}")` 的哈希 |

因此 `cell list` **看不到** workflow 名或 instance 名，只看到：

```
__Workflow.{wrangler.name}:{64-hex}
```

`is_workflow_class`：class 以 `__Workflow.` 开头（裸 `__Workflow` 在 `RESERVED_CLASSES` 里，但 **实例行用的是 script-scoped 名**）。

### 过滤能否对准 `{name}`？

| 情况 | 能否把实例划给某一个 wrangler workflow |
|------|----------------------------------------|
| 该 version 的 wrangler **只有 1 条** `workflows[]`，且 path `{name}` 等于其 `name` 或 `binding` | **能**（该类下所有 reserved workflow cell 即该 workflow） |
| 同一 script **多条** `workflows[]` | **不能**。id 是 `{workflow_name}/{instance_id}` 的哈希，不可逆 |

这不是 cellp 缺陷，是 celld 没有 `celld workflow`、list 也不回传内部 name。

## API（`:8790/v1` · ADMIN_TOKEN · ready version）

一律挂在 **ready version** 上（与 D1 相同：未 ready / 不存在 → **404** `version_not_found` / `version_not_ready`）。Dashboard 禁止直连 `:8792` / S3。`--bucket` 必须是 **该 version** 的 `s3://cellp-celld/{project}/{version}`（AD-1；子 version Workflow **空起步**，AD-7）。

### GET `/projects/{id}/versions/{vid}/workflows`

从 T1 清单过滤 `type == "workflow"`，**不**调 celld。

```json
{
  "workflows": [
    {
      "binding": "WF",
      "workflow_name": "wf",
      "class_name": "WfClass"
    }
  ]
}
```

空数组 = wrangler 未声明，不是错误（200）。无 wrangler 文件 → 404 `bindings_not_found`（与 T1 bindings 一致）。

path `{name}` 解析顺序：先匹配 `workflow_name`（wrangler `name`），再匹配 `binding`。两者都不中 → 404 `workflow_not_found`。

### GET `/projects/{id}/versions/{vid}/workflows/{name}/instances`

1. 解析 wrangler，确认 `{name}` 是该 version 的 workflow（否则 404）。
2. 取 `script_name` = `BindingsManifest.Worker.Name`。
3. 调 wrapper：

   ```
   celld cell list __Workflow.{script_name} --all --json --bucket … --endpoint … --region …
   ```

   `Worker.Name` 为空时：退回 **不带 CLASS** 的 `--all --json`，再在进程内丢掉非 workflow 行。
4. 只保留 `reserved == true` 且 class 满足 `strings.HasPrefix(class, "__Workflow.")` 的行（丢掉 `__D1Database` / KV / Queue / 用户 DO）。
5. **按 workflow 名再过滤：见下一节 fallback。**
6. V11：**不得 500**。无实例 → 200 `instances: []`。PATH 上无 `celld`（dev）→ 200 空列表（与 `Health()` 在缺 binary 时放行一致）。celld 非 0 退出 → **502** `celld_cell_list_failed`，不要 500。

建议响应：

```json
{
  "workflow_name": "wf",
  "binding": "WF",
  "script_name": "commerce",
  "filter": "workflow",
  "limitation": null,
  "wrangler_workflows": ["wf"],
  "instances": [
    {
      "scope": "__Workflow.commerce:0123…64hex",
      "class": "__Workflow.commerce",
      "id": "0123…64hex",
      "reserved": true
    }
  ]
}
```

`filter`：`workflow`（单 workflow，可归因）| `script`（多 workflow，fallback）。

### 安全 fallback（过滤歧义时 **必须** 采用）

当该 version wrangler 声明了 **≥2** 条 workflow 时，**禁止猜测**哪条 NDJSON 行属于 `{name}`（哈希不可逆，猜错会漏或张冠李戴）。

**Fallback：**

- HTTP **仍 200**（V11：不 500）。
- `instances` = 该 version bucket 上、该类 `__Workflow.{script}` 的 **全部** reserved workflow cell（不是全 bucket 的 D1/KV）。
- `wrangler_workflows` = 该 version 全部 wrangler `workflows[].name`（没有 `name` 则用 `binding`）。
- `filter`: `"script"`。
- `limitation` 固定文案（API 与 OpenAPI description 各写一遍）：

  > celld `cell list` 只给出 `__Workflow.{script}:<64-hex>`；实例内部名 `{workflow_name}/{instance_id}` 被哈希，无法按 workflow 名精确过滤。本响应返回该 version / 该 script 下全部 Workflow cell，并附 wrangler 声明的 workflow 名。精确归属需等 `celld workflow`。

单条 workflow 且 `{name}` 命中时：`filter: "workflow"`，`limitation: null`，实例集合与「该类全部 workflow cell」相同。

`{name}` 未在 wrangler 中声明：**404**，即使 bucket 里有 `__Workflow.*` cell（避免用未知名字扫出实例）。

## Cron

- **唯一来源：** T1 已解析的 wrangler `triggers.crons`（`BindingsManifest.Crons` + `bindings[]` 里 `type:"cron"`）。
- T3 **不**增加 `GET /crons`、**不**增加「触发一次」/ pause cron / 跨 version 选举。
- Dashboard（T4）从 `GET /bindings` 读 cron 表达式列表。
- celld 仍按 **该 version 的 fleet** 触发 cron（V11 只做可见性）。

## R2

- **只出现在** `GET /bindings` 的 `type:"r2"`（`bucket_name` 来自 wrangler）。
- **禁止** 任何对象 API：list / get / put / delete prefix `r2/<bucket>/`。
- 无 `celld r2`（`cli.rs` 已确认）。不做对象浏览器、不做独立 `/r2` 路由。空态 / 徽章归 T4。

## 明确禁止（本 track 不得实现、不得在 OpenAPI 占位成可调用）

| 禁止 | 原因 |
|------|------|
| Workflow **pause / resume / restart / delete** | AD-7 · 无 `celld workflow`；DESIGN §8.4「不做」 |
| `POST/PUT/PATCH/DELETE .../workflows…` | 只读 GET |
| R2 **list / get / put**（含预签名、前缀列举） | 无 `celld r2` |
| Dashboard 或 cellpd 直连 `:8792` / S3 读 cell | P2 包 CLI |
| 改 `celld/`、给 celld 加 `workflow`/`r2` 子命令 | 冻结 submodule |
| Workflow inherit / 拷父桶 | AD-6 / AD-7 |
| 改 `cellp/go.mod` | 除非 deps owner |

chi **不要注册** 这些路径。测试用 GET 以外的 method 打 `/workflows/{name}/instances` → 405 或 404（未注册即可），**不要**实现成「假的 501 handler」。

## 实现边界

| 路径 | 职责 |
|------|------|
| `cellp/internal/runtime/cell_list.go`（新） | `ListCells` / `ListWorkflowInstances`：拼 argv、跑 `celld`、解析 NDJSON、按 reserved + `__Workflow.` 过滤、计算 `filter`/`limitation` |
| `cellp/internal/runtime/cell_list_test.go`（新） | **假 binary**（见下） |
| `cellp/internal/api/workflows.go`（新） | 两 GET；ready 检查照抄 `resolveDatabaseContext` 的 version 闸 |
| `cellp/internal/api/workflows_test.go`（新） | httptest：清单 / 实例 / 404 / 禁止写方法 |
| `cellp/internal/api/server.go` | 在 `{versionID}` 下 `GET /workflows`、`GET /workflows/{name}/instances`（`requireAdmin`） |
| `cellp/api/openapi.yaml` | 上述两路径 + 响应 schema + `limitation` 说明 |
| `dev/scripts/*.sh` · `e2e/scripts/*.sh` | health 一行替换 |
| `docs/test-plan.md` · `VALIDATION.md` | 验收表里的旧 health 路径（见下表） |

**不要改：** `celld/` · `web/` · `D1-*-RPC.md` · `manager.go` 的 `Health()`（已正确）· T2 的 kv/queue 文件（除非为了共用 `exec.LookPath("celld")` 抽 3 行 helper；非必须）。

argv 必须包含：`cell` `list`、`--json`、`--all`、`--bucket`（version bucket）、`--endpoint`、`--region`。有 script 名时 **CLASS 为第一 positional**：`__Workflow.{script}`。Env 与 D1 相同：`AWS_*`、`CELLD_VAR_PROJECT_ID` / `VERSION_ID`。

解析：按行 `json.Unmarshal`（同 `parseD1JSONOutput`）。忽略空行。`class` JSON `null`（无 `:` 的 scope）视为非 workflow。

## `/__celld/health` 残留与一行修复

celld 0.4.0 公共路径：**`/.well-known/celld/health`**。替换时只改 path，host/port 不变。

一行：

```
/__celld/health  →  /.well-known/celld/health
```

| 文件 | 用途 | T3 |
|------|------|----|
| `e2e/scripts/lib.sh`（`require_celld`） | e2e 等 celld ready | **必改** |
| `e2e/scripts/health-all.sh` | TP-VE-1 | **必改** |
| `dev/scripts/up.sh` | 启动轮询 | **必改** |
| `dev/scripts/up-native.sh` | 启动轮询 | **必改** |
| `dev/scripts/simulate-cd.sh` | 启动轮询 | **必改** |
| `dev/scripts/health.sh` | 本地 health | **必改** |
| `docs/test-plan.md` TP-VE-1 | `:8792/__celld/health` | **应改**（否则门禁文档与脚本不一致） |
| `VALIDATION.md` 端口表 `:8792` | 仍写旧 path | **应改** |
| `DESIGN.md` §8.6 | 新旧对照说明 | **不改** |
| `docs/evidence/celld-multi-fleet-spike.md` | 历史证据 | **不改** |
| `stress/phase6/d1-import-scale.sh` | 注释里的旧 gate 叙述 | **不改** |

`cellp/internal/runtime/manager.go` **已使用新路径**，不要「再修」Health。

改完后：`rg '/__celld/health' dev e2e` 必须无命中。

## 测试（假 binary）

沿用 `manager_test.go` `TestD1BranchPassesParentBucket`：`t.TempDir()` 下写可执行 `bin/celld`，`t.Setenv("PATH", bin)`（不要依赖真实 celld、不要打 S3）。

假 binary 最低行为：

1. 把 `$@` 追加到 `celld-args.log`（一行一个 argv，或整行再 split）。
2. 若 argv 含 `cell` 与 `list`：向 **stdout** 打 NDJSON fixture，exit 0。
3. stderr 可写一句 `note`，解析器必须忽略。

Fixture 至少三行，覆盖过滤：

```json
{"scope":"__Workflow.full:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","class":"__Workflow.full","id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","reserved":true}
{"scope":"__D1Database:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","class":"__D1Database","id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb","reserved":true}
{"scope":"Room:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","class":"Room","id":"cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc","reserved":false}
```

**必写用例：**

| 用例 | 断言 |
|------|------|
| argv | 含 `cell`,`list`,`--json`,`--all`,`--bucket`,`s3://cellp-celld/{p}/{v}`；有 worker name 时含 CLASS `__Workflow.{name}` |
| 过滤 | 返回 1 条 workflow 实例；D1 / `Room` 不出现 |
| 单 workflow | wrangler 一条 `workflows`，`filter=="workflow"`，`limitation` 空 |
| 多 workflow fallback | wrangler 两条；`GET .../workflows/wf-a/instances` 仍 200；`instances` 含该类全部 workflow cell；`filter=="script"`；`wrangler_workflows` 含两个名字；`limitation` 非空 |
| 未知 `{name}` | 404，且 **不**调用 celld（args log 空）或即使调用也不把结果当成功体 |
| 无 celld | `PATH` 不含假 binary → 200 空 `instances`（或明确的 unavailable→空），**不是** 500 |
| API 只读 | `POST .../workflows/wf/pause`（及 resume/restart/delete）不是 2xx |
| Cron 无新路由 | `GET .../crons` 404（清单只在 `/bindings`） |
| R2 无对象路由 | `GET .../r2/{bucket}/objects` 404 |

解析函数可单测（不 exec）：给定 NDJSON bytes → 过滤结果。API 层用假 PATH 或对 `runtime` 注入；优先 PATH 假 binary，少 mock。

## OpenAPI

在 `cellp/api/openapi.yaml` 增加（T1 若已加 `/bindings` 则只追加 workflow）：

- `GET /projects/{projectID}/versions/{versionID}/workflows`
- `GET /projects/{projectID}/versions/{versionID}/workflows/{name}/instances`

写清：ADMIN_TOKEN；404 version/workflow；502 celld；`filter` enum；`limitation` 歧义说明。**不要**列出 pause/resume/restart/delete/R2 object paths。

`cd cellp && make openapi-check` 必须通过。

## Verify

```bash
# 单元：wrapper + API
cd cellp && go test ./internal/runtime/ ./internal/api/ -count=1

# 全量（本 track 不得弄红既有包）
cd cellp && go test ./... -count=1

cd cellp && make openapi-check

# health：dev/e2e 不得再出现旧 path
rg -n '/__celld/health' dev e2e
# 期望：无输出（exit 1 from rg is success for "no matches"）

# 文档应改项（允许 DESIGN/evidence/stress 仍出现旧字符串）
rg -n '/__celld/health' docs/test-plan.md VALIDATION.md
# 期望：无输出

# 禁止面：源码不得出现这些 handler 名 / 路径片段
rg -n 'workflow (pause|resume|restart|delete)|/r2/.*/(objects|keys)' cellp --glob '*.go'
# 期望：无实现（测试里作为 URL 字符串打 404 的除外）

# 手工（有 stack 时）：T1 bindings 已含 workflows + crons 的 fixture
# curl -sS -H "Authorization: Bearer $ADMIN_TOKEN" \
#   http://127.0.0.1:8790/v1/projects/{id}/versions/{vid}/workflows
# curl -sS -H "Authorization: Bearer $ADMIN_TOKEN" \
#   http://127.0.0.1:8790/v1/projects/{id}/versions/{vid}/workflows/{name}/instances
# 后者不得 500；无实例时 instances=[]
```

有本地 celld 时可选：对已 deploy 且跑过 workflow 的 version bucket 跑一次真 `celld cell list --all --json`，确认 wrapper 的 CLASS / 过滤与 CLI 一致。无实例时两边都是空列表，仍算通过。

## Exit

- [ ] `GET .../workflows` 来自 wrangler，不调 celld
- [ ] `GET .../workflows/{name}/instances` 包装 `celld cell list --json --all`，只保留 reserved `__Workflow.*`
- [ ] 多 workflow 时走 **script 级 fallback** + `limitation` 文案；不 500
- [ ] Cron 仅 T1 bindings；无 trigger API
- [ ] R2 仅 bindings；无 list/get/put
- [ ] 无 pause/resume/restart/delete
- [ ] 假 binary 测试覆盖 argv + 过滤 + fallback
- [ ] `dev/` `e2e/` 无 `/__celld/health`；`Health()` 保持 well-known
- [ ] `cd cellp && go test ./...` · `make openapi-check`
