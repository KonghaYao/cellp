# Phase 7 T2 — KV + Queue operator（包装 celld CLI）

> **规格：** [DESIGN.md §8.4](../../DESIGN.md) KV / Queues 表  
> **决策：** [decisions.md AD-6 · AD-7](../decisions.md) — 包装已有 CLI；**不做 inherit / copy / 挂父桶**  
> **母计划：** [phase-7-bindings.md](./phase-7-bindings.md)  
> **前置：** P7-T1 `ParseBindings` · `handleGetBindings`（已落地 `cellp/internal/runtime/bindings.go`）  
> **验收：** VALIDATION V9 · V10（本 track 交付 API；e2e 隔离归 T5）  
> **模式：** 与 D1 相同 — cellpd → `exec celld` → 解析 stdout；**禁止** Dashboard / API 直连 `:8792`

## Exit Criteria

- [ ] Runtime：`KvList` / `KvGet` / `KvPut` / `KvDelete` / `KvInfo`
- [ ] Runtime：`QueueInfo` / `QueuePeek` / `QueuePurge` / `QueuePause` / `QueueResume` / `QueueRedrive`
- [ ] HTTP：§8.4 全部 KV / Queue 路径经 `requireAdmin`；`{ns}` = `kv_namespaces[].id` verbatim；`{name}` = queue 名
- [ ] purge 无 JSON `{"force":true}` → **400** `force_required`；**不得**调用 celld
- [ ] peek / redrive `limit` 不在 1–100 → **400** `invalid_limit`
- [ ] wrangler 未声明的 ns / queue → **404**；**不得**对任意名字 exec celld
- [ ] 无 bulk 路由、无 inherit / parent-bucket
- [ ] `cd cellp && go test ./internal/runtime ./internal/api`

## 范围

| 做 | 不做 |
|----|------|
| 包装 `celld kv` get/put/delete/list/info | `celld kv bulk *`（第二轮） |
| 包装 `celld queue` info/peek/purge/pause/resume/redrive | inherit、copy、挂父 `s3://cellp-celld/{p}/{parent}` |
| wrangler 存在性校验 + OpenAPI 合同 | 改 `celld/`、`cellp/go.mod`、`D1-*-RPC.md` |
| fake celld argv 测试（同 `manager_test.go`） | Dashboard（T4）、e2e 脚本（T5）、Workflow/Cron（T3） |

```
T1 ParseBindings ──> T2 runtime wrap + handlers ──> T4 只打 :8790
```

## 布局

```
cellp/internal/runtime/
  celld_exec.go     # 私有 execCelld：LookPath + env + stdout/stderr（KV/Queue 用；不必改 D1ExecuteSQL）
  kv.go             # KvList/Get/Put/Delete/Info + HasKVNamespace
  queue.go          # QueueInfo/Peek/Purge/Pause/Resume/Redrive + HasQueue
  kv_test.go
  queue_test.go
cellp/internal/api/
  kv.go             # handlers + resolveKVContext
  queue.go          # handlers + resolveQueueContext
  kv_test.go
  queue_test.go
  server.go         # 路由 + CORS 加 PUT
cellp/api/openapi.yaml   # KV / Queue 路径（T4 合同）
```

D1 对照：`d1_query.go` `D1ExecuteSQL` · `database.go` `resolveDatabaseContext`。**不要**给 kv/queue 传 wrangler 目录 positional（`celld kv` / `celld queue` 没有 projectDir 参数）。

---

## 共享：exec + AWS env + fleet argv

`exec.LookPath("celld")` 失败 → 已有 `ErrCelldUnavailable`（API → **503** `celld_unavailable`）。

**Env 原样抄 `D1ExecuteSQL`**（`cmd.Env` 从 nil append，**不要** `os.Environ()`）：

```
CELLD_VAR_PROJECT_ID={project}
CELLD_VAR_VERSION_ID={version}
AWS_ACCESS_KEY_ID={m.accessKey}
AWS_SECRET_ACCESS_KEY={m.secretKey}
AWS_REGION={m.region}
```

**Fleet 后缀**（所有调用相同顺序，与 D1 `--json` 一样放最后）：

```
--bucket s3://cellp-celld/{project}/{version}
--endpoint {m.endpoint}
--region {m.region}
--json
```

`--bucket` 必须走 `m.versionBucket(project, version)` → `s3://cellp-celld/{project}/{version}`（AD-1 隔离；AD-7 **禁止**父 version bucket）。`m.endpoint` / `m.region` 与 D1 同一 Manager 字段。

`kv get` stdout 是 **原始 value 字节**，`--json` 对 get 无意义：**省略 `--json`**，以免和 value 混淆。put/delete 无结构化 stdout，也省略 `--json`。list/info 与全部 `queue *` **必须** `--json`。

失败：非 0 退出码 → `fmt.Errorf("celld kv …: %s", stderr优先)`，同 D1。

---

## Runtime 方法与精确 argv

下列 `{ns}` / `{name}` / `{key}` 均为 **一个 argv 元素**（不要 split）。`{FLEET}` = 上节四段 fleet 后缀。示例 project=`demo` version=`v1` endpoint=`http://127.0.0.1:9000` region=`us-east-1`。

### 存在性（先于 exec）

```go
HasKVNamespace(projectDir, ns) bool  // Binding.Type=="kv" && NamespaceID==ns（verbatim）
HasQueue(projectDir, name) bool      // producers[].queue || consumers[].queue || consumers[].dead_letter_queue
```

未命中 → sentinel `ErrKVNamespaceNotFound` / `ErrQueueNotFound`，**不**启动 celld。用 T1 `ParseBindings`；DLQ 若尚未进 `Binding`，在 `HasQueue` 里再扫 wrangler `queues.consumers[].dead_letter_queue`。

### KV

| 方法 | 签名要点 | argv（`celld` 之后） |
|------|----------|----------------------|
| **KvList** | `(ctx, project, version, projectDir, ns, prefix, cursor string, limit int) (*KvListResult, error)` | `kv list {ns} [--prefix P] [--limit N] [--after CURSOR] {FLEET} --json` |
| **KvGet** | `(…) (raw []byte, err error)` | `kv get {ns} {key} --bucket s3://cellp-celld/{p}/{v} --endpoint … --region …`（无 `--json`） |
| **KvPut** | `(…, ns, key string, in KvPutInput) error` | 见下 |
| **KvDelete** | `(…, ns, key string) error` | `kv delete {ns} {key} --bucket … --endpoint … --region …` |
| **KvInfo** | `(…) (*KvInfo, error)` | `kv info {ns} --bucket … --endpoint … --region … --json` |

**KvList 可选 flag：** `prefix==""` 则无 `--prefix`；`cursor==""` 则无 `--after`（HTTP `cursor` → CLI `--after`）；`limit<=0` 则无 `--limit`（celld 默认 1000）。`limit>1000` clamp 到 1000。禁止 `--all`。

stdout = NDJSON（每行一个 Key：`name` / `expiration` / `metadata`），解析同 `parseD1JSONOutput`。`KvListResult`：`keys` + 若 `len(keys)==limit` 则 `cursor`=最后一条 `name`。

**KvGet：** stdout **整段原始字节**（不要当 JSON）。celld `no key` → `ErrKVKeyNotFound`。

**KvPut argv：**

- UTF-8 文本、非空、不以 `-` 开头、且未标 binary/base64：  
  `kv put {ns} {key} {value} [--ttl SECONDS] [--metadata JSON] --bucket … --endpoint … --region …`
- 否则（binary/base64、空 value、前导 `-`、或内含 NUL）：写 temp 文件，进程结束后删：  
  `kv put {ns} {key} --path {tmp} [--ttl SECONDS] [--metadata JSON] --bucket … --endpoint … --region …`

`--ttl` 仅当 `ttl != nil`；celld 要求 **≥60**，更小在 runtime/API **400** `ttl_too_small`，不 exec。`--metadata` 为 **一条 JSON 文本**（≤1024 bytes，超则 400）。不要 `--expiration`（本期 body 只有 ttl）。

**KvInfo JSON：** `{ "keys": live, "bytes": …, "stored": … }`（celld Totals）。

### Queue

`limit` 在 Peek/Redrive **始终**传入（默认见 HTTP 节），便于测试 argv 稳定。

| 方法 | argv（`celld` 之后） |
|------|----------------------|
| **QueueInfo** | `queue info {name} --bucket s3://cellp-celld/{p}/{v} --endpoint … --region … --json` |
| **QueuePeek** | `queue peek {name} --limit {n} --bucket … --endpoint … --region … --json` |
| **QueuePurge** | `queue purge {name} --force --bucket … --endpoint … --region … --json` |
| **QueuePause** | `queue pause {name} --bucket … --endpoint … --region … --json` |
| **QueueResume** | `queue resume {name} --bucket … --endpoint … --region … --json` |
| **QueueRedrive** | `queue redrive {name} --limit {n} --bucket … --endpoint … --region … --json` |

stdout = **一行 JSON object**（`--json` + `Output::row`）。方法返回 `json.RawMessage`（或 `map[string]any`），API 原样写入 body。celld 已 unwrap `result`。

**QueuePurge 不接收 force 参数** — 只有 HTTP 校验过后才调用，argv **恒带** `--force`。

Peek JSON 形态（透传）：`{ "messages": [ { "id", "bodyBase64", "contentType", … } ] }`。Redrive：`{ "redriven": N, "metrics": … }`。

---

## HTTP：context、路由、handler 名

一律 **ready version**（抄 `resolveDatabaseContext`）：无 version → 404 `version_not_found`；`status != ready` → 404 `version_not_ready`。`projectDir = filepath.Join(s.cfg.ArtifactsDir, projectID, versionID)`。

```go
resolveKVContext(r)    // ready + HasKVNamespace(ns)；失败 404 kv_namespace_not_found
resolveQueueContext(r) // ready + HasQueue(name)；失败 404 queue_not_found
```

`{ns}` = `chi.URLParam(r, "ns")` **不做**大小写折叠。`{name}` 同理。

`server.go` `routes()` 挂在 `/{versionID}` 下（与 `/database` 并列），全部 `requireAdmin`：

```go
r.Route("/kv", func(r chi.Router) {
    r.Get("/", s.requireAdmin(s.handleListKVNamespaces))
    r.Route("/{ns}", func(r chi.Router) {
        r.Get("/", s.requireAdmin(s.handleGetKVInfo))
        r.Get("/keys", s.requireAdmin(s.handleListKVKeys))
        r.Get("/keys/{key:*}", s.requireAdmin(s.handleGetKVValue))
        r.Put("/keys/{key:*}", s.requireAdmin(s.handlePutKVValue))
        r.Delete("/keys/{key:*}", s.requireAdmin(s.handleDeleteKVValue))
    })
})
r.Route("/queues", func(r chi.Router) {
    r.Get("/", s.requireAdmin(s.handleListQueues))
    r.Route("/{name}", func(r chi.Router) {
        r.Get("/", s.requireAdmin(s.handleGetQueueInfo))
        r.Get("/peek", s.requireAdmin(s.handlePeekQueue))
        r.Post("/pause", s.requireAdmin(s.handlePauseQueue))
        r.Post("/resume", s.requireAdmin(s.handleResumeQueue))
        r.Post("/redrive", s.requireAdmin(s.handleRedriveQueue))
        r.Post("/purge", s.requireAdmin(s.handlePurgeQueue))
    })
})
```

`{key:*}`：允许 key 含 `/`。空 key → 400 `key_required`。CORS `Access-Control-Allow-Methods` **加上 PUT**（现仅 GET, POST, DELETE, OPTIONS）。

**不要**注册 `kv bulk`。

### KV handlers

| Handler | 方法 · 路径 | 行为 |
|---------|-------------|------|
| **handleListKVNamespaces** | `GET /v1/projects/{id}/versions/{vid}/kv` | 只读 wrangler；**不**调 celld。`{"namespaces":[{"id","binding"}]}`。无 KV → **200** `[]`（不是 404） |
| **handleGetKVInfo** | `GET …/kv/{ns}` | `KvInfo` → `{"keys","bytes","stored"}` |
| **handleListKVKeys** | `GET …/kv/{ns}/keys?prefix=&cursor=&limit=` | `KvList`；query `cursor` → `--after` |
| **handleGetKVValue** | `GET …/kv/{ns}/keys/{key}` | `KvGet` + 编码（下节） |
| **handlePutKVValue** | `PUT …/kv/{ns}/keys/{key}` | body 下节；`KvPut`；成功 **204** 或 `{"ok":true}`（选一，OpenAPI 写死） |
| **handleDeleteKVValue** | `DELETE …/kv/{ns}/keys/{key}` | `KvDelete`；成功 **204** |

### Queue handlers

| Handler | 方法 · 路径 | 行为 |
|---------|-------------|------|
| **handleListQueues** | `GET …/queues` | wrangler 去重 queue 名；**不**调 celld。无 queue → **200** `[]` |
| **handleGetQueueInfo** | `GET …/queues/{name}` | `QueueInfo` |
| **handlePeekQueue** | `GET …/queues/{name}/peek?limit=` | 默认 **10**；校验 1–100；`QueuePeek` |
| **handlePauseQueue** | `POST …/queues/{name}/pause` | `QueuePause` |
| **handleResumeQueue** | `POST …/queues/{name}/resume` | `QueueResume` |
| **handleRedriveQueue** | `POST …/queues/{name}/redrive?limit=` | 默认 **100**；校验 1–100；`QueueRedrive`（body 的 limit **忽略**，只用 query） |
| **handlePurgeQueue** | `POST …/queues/{name}/purge` | 先读 JSON；`force === true`（布尔）才 `QueuePurge` |

pause/resume 无 body。celld JSON 响应 **200** 原样返回。

### purge 强制

`handlePurgeQueue`：

1. JSON decode 失败、body 空、缺 `force`、`force` 不是 JSON boolean `true`（含 `false`、`1`、`"true"`）→ **400** `{"error":"force_required"}`，**不** exec。
2. 仅 `{"force":true}` → `QueuePurge` → argv 含 `--force`。

### peek / redrive limit

缺省：peek **10**、redrive **100**（与 celld cell `_operatorLimit` 一致）。有 `limit` query：`Atoi` 后不在 **1–100** → **400** `invalid_limit`，不 exec。argv **总是**带 `--limit {n}`。

---

## PUT body · GET value 编码

### PUT `handlePutKVValue`

```json
{
  "value": "string",          // 必填（允许 ""）
  "ttl": 3600,                // 可选 number，秒，≥60 → --ttl
  "metadata": { },            // 可选 object 或 string；object 则 Marshal 成 --metadata
  "base64": false,            // 可选
  "binary": false             // 与 base64 同义；任一 true 则 value 当 base64 解码
}
```

- 缺 `value` → 400 `value_required`。
- `base64||binary`：StdEncoding 解码失败 → 400 `invalid_base64`；得到的字节再 `KvPut`。
- 默认：`value` 的 UTF-8 字节（JSON string）。
- `metadata` 序列化后 >1024 → 400 `metadata_too_large`。
- 本期无 bulk 数组。

成功建议 **204 No Content**（与 DELETE 一致）。

### GET `handleGetKVValue`

```json
{
  "key": "<decoded key>",
  "value": "<string>",
  "encoding": "utf-8" | "base64"
}
```

- 原始字节 **合法 UTF-8 且无 NUL** → `encoding=utf-8`，`value` 为该字符串。
- 否则 → `encoding=base64`，`value` 为 StdEncoding。
- `ErrKVKeyNotFound` → **404** `key_not_found`。

---

## OpenAPI

在 `cellp/api/openapi.yaml` 写入上表全部 path（T4 不得猜路径）。query：`prefix` `cursor` `limit`。PUT schema 含 `value` `ttl` `metadata` `base64` `binary`。purge requestBody **required** `force: true`。错误码 400/404/503 与上列 `error` 字符串一致。

---

## 测试（fake celld，同 `manager_test.go`）

脚本：

```sh
#!/bin/sh
printf '%s\n' "$@" >> "$ARGS_LOG"
# 按 $1/$2 写 stdout：kv list/info NDJSON|JSON；kv get 原始字节；queue * 一行 JSON
```

`t.Setenv("PATH", bin)`。`New(8792, "http://127.0.0.1:9000", "us-east-1", "s3://cellp-celld/demo", "k", "s")`。wrangler 最小：

```json
{
  "kv_namespaces": [{ "binding": "KV", "id": "ns-1" }],
  "queues": {
    "producers": [{ "binding": "TASKS", "queue": "tasks" }],
    "consumers": [{ "queue": "events", "dead_letter_queue": "events-dlq" }]
  }
}
```

### `./internal/runtime`

| 测试 | 断言 |
|------|------|
| `TestKvListArgv` | 精确切片：`kv`,`list`,`ns-1`,`--prefix`,`app/`,`--limit`,`50`,`--after`,`k0`,`--bucket`,`s3://cellp-celld/demo/v1`,`--endpoint`,`http://127.0.0.1:9000`,`--region`,`us-east-1`,`--json` |
| `TestKvGetArgv` | `kv get ns-1 my-key --bucket … --endpoint … --region …`（无 `--json`）；stdout 字节原样返回 |
| `TestKvPutArgvInline` | `kv put ns-1 k hello --ttl 60 --metadata {"seeded":true} --bucket …` |
| `TestKvPutArgvPath` | `base64` 值走 `--path`；读该文件字节 |
| `TestKvDeleteArgv` / `TestKvInfoArgv` | 含 `--json` 仅 info |
| `TestKvSkipsUnknownNamespace` | `HasKVNamespace` 失败；**无** args log |
| `TestQueuePeekArgv` | `queue peek tasks --limit 10 --bucket s3://cellp-celld/demo/v1 --endpoint … --region … --json` |
| `TestQueuePurgeArgv` | 含 `--force` 与 `--json` |
| `TestQueueRedriveArgv` | `--limit 100` |
| `TestQueuePauseResumeInfoArgv` | 三组精确 argv |
| `TestQueueSkipsUnknownName` | `bogus` 不写 args log；`events-dlq` **允许**（dead_letter_queue） |
| `TestKvQueueUnavailable` | PATH 无 celld → `ErrCelldUnavailable` |

### `./internal/api`

沿用 `testAPI` + 扩展 wrangler（可复用 `setupDatabaseVersion` 或新 `setupKVQueueVersion`）。

| 测试 | 断言 |
|------|------|
| `TestListKVNamespaces` | 200；`id=ns-1`；fake 无 kv 子命令 |
| `TestGetKVValueEncoding` | UTF-8 vs 含 NUL → `encoding` |
| `TestPutKVValue` | 204；runtime 侧能看到 put |
| `TestUnknownKVNamespace404` | `ns-nope` → 404；不调 celld |
| `TestKVVersionNotReady404` | 同 D1 |
| `TestPeekLimitValidation` | `limit=0` / `101` → 400 `invalid_limit` |
| `TestPurgeRequiresForce` | 无 body、`{}`、`{"force":false}` → 400 `force_required`；`{"force":true}` → 200 且 argv 有 `--force` |
| `TestQueueNotDeclared404` | `{name}=other` → 404 |
| `TestListQueuesEmptyOk` | 无 queues 的 wrangler → 200 `[]` |

---

## 错误映射

| 条件 | HTTP | `error` |
|------|------|---------|
| version 缺失 / 非 ready | 404 | `version_not_found` / `version_not_ready` |
| ns 不在 wrangler | 404 | `kv_namespace_not_found` |
| queue 不在 wrangler | 404 | `queue_not_found` |
| kv get 无 key | 404 | `key_not_found` |
| purge 无 force | 400 | `force_required` |
| peek/redrive limit | 400 | `invalid_limit` |
| ttl < 60 | 400 | `ttl_too_small` |
| celld 不在 PATH | 503 | `celld_unavailable` |
| celld 其它非 0 | 400 | stderr 文本（trim `celld kv …: ` 前缀） |

---

## 禁止（复查）

- `celld kv bulk get|put|delete`
- `--parent-bucket`、读 prod KV；见 AD-6 / AD-7
- 改 `celld/` submodule、`go.mod`、冻结 D1 RPC
- Dashboard 直连 `:8792`
- 未声明 ns/queue 仍 exec
- purge 不带 `--force` 调 celld
- T3 Workflow、T4 web、T5 e2e（本文件不派发）

## Verify

```bash
cd cellp && go test ./internal/runtime ./internal/api
```

（有 `make openapi-check` 则顺手跑；OpenAPI 是 T4 合同。）

## Subagent prompt

```
Track P7-T2. 只改 cellp/internal/runtime（kv.go queue.go celld_exec.go + tests）、
cellp/internal/api（kv.go queue.go server.go CORS PUT + tests）、cellp/api/openapi.yaml。
包装 celld kv/queue，argv/env 抄 D1ExecuteSQL。{ns}=kv_namespaces[].id verbatim。
purge 必须 JSON {"force":true} 否则 400。peek/redrive limit 1–100。无 bulk、无 inherit。
Verify: cd cellp && go test ./internal/runtime ./internal/api
```
