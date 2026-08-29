# P7-T1 — Bindings 清单 API + wrangler 解析

> **规格：** [DESIGN.md §8.4](../../DESIGN.md)（JSON 形状与路径的唯一来源）  
> **决策：** [decisions.md AD-1 · AD-6 · AD-7](../decisions.md)  
> **总表：** [phase-7-bindings.md](./phase-7-bindings.md)  
> **celld 字段：** `celld/crates/celld/deploy.rs` `SUPPORTED_KEYS` + `kv_namespaces` / `queues` / `workflows` / `r2_buckets` / `triggers`  
> **状态：** 计划中（2026-08-29）

只读清单。**不**包装 `celld kv` / `celld queue` / `celld cell list`（那是 T2 / T3）。解析必须走现有 `readWranglerConfig`，禁止第二套 wrangler I/O。

## Exit Criteria

- [ ] `GET /v1/projects/{id}/versions/{vid}/bindings` 对 **ready** version 返回 §8.4 六数组 JSON（`d1` / `kv` / `queues` / `workflows` / `r2` / `crons`），空声明为 `[]` 不是 `null`
- [ ] version 未 ready（含 `pending` / `failed` / `draining`）→ **404** `version_not_ready`（抄 `resolveDatabaseContext`）
- [ ] version 不存在 → **404** `version_not_found`
- [ ] 无 wrangler 文件 → **404** `wrangler_not_found`（见下方对照 D1 的取舍）
- [ ] wrangler 存在但未声明某类绑定 → **200**，对应数组 `[]`
- [ ] 非法 jsonc / 非 JSON → **500**（与 D1 `parse wrangler` 同级，不是 404）
- [ ] OpenAPI 增加该 GET，不改动既有 path
- [ ] `corsMiddleware` 的 `Allow-Methods` 含 **PUT**（T2 `PUT …/kv/{ns}/keys/{key}` 预检）
- [ ] `cd cellp && go test ./internal/runtime ./internal/api ./...`

## Parallel Tracks（本文件只做 T1）

| Track | ID | 并行 | Gate | 交付 |
|-------|-----|------|------|------|
| 清单 API + wrangler 解析 | **P7-T1** | 先于 T2/T3/T4 | DESIGN §8.4 | `internal/runtime` parse · `internal/api` GET · OpenAPI |
| KV + Queue operator | **P7-T2** | 等 T1 合同 | T1 JSON 字段冻结 | `celld kv` / `celld queue` 包装 |
| Workflow 只读 + Cron + health | **P7-T3** | 等 T1 合同 | T1 `workflows[]` / `crons[]` | `cell list` · health 路径 |
| Dashboard Bindings hub | **P7-T4** | 等 T1–T3 OpenAPI | OpenAPI 写入 | **仅 `web/`** |
| E2E + TP | **P7-T5** | 可先写脚本 | T1 路径稳定 | `e2e/scripts/` |

```
T1 清单 API ──┬──> T2 KV/Queue
              ├──> T3 Workflow/Cron/health
              └──> T5 脚本骨架
```

T1 **不得**注册 `…/kv` · `…/queues` · `…/workflows/{name}/instances` 路由。

## 现状（必须重写，不得当合同）

仓库里已有错误形状，T1 以 DESIGN 为准覆盖：

| 文件 | 现状 | T1 要求 |
|------|------|---------|
| `cellp/internal/runtime/bindings.go` | `BindingsManifest{bindings:[{type,name,…}]}`，还解析 DO / services / vars / assets | 六顶层数组；**不**把 DO / vars / assets 放进本 API |
| `cellp/internal/api/bindings.go` | `handleGetBindings` **不检查** `StatusReady`；任意 parse 错误都 404 | 抄 D1 ready-404；区分无文件 / 非法 JSON |
| `cellp/api/openapi.yaml` | 无 `/bindings` | 只加 GET 清单 |
| `corsMiddleware` | `GET, POST, DELETE, OPTIONS` | 加上 `PUT` |

`D1DatabaseID` / `D1DatabaseName` / `readWranglerConfig` / `stripJSONC` **保持**；D1「0 条空、>1 条 error」语义不动。

## Repo layout（本 track 可改）

```
cellp/
├── api/openapi.yaml                 # 追加 GET bindings + schemas
└── internal/
    ├── runtime/
    │   ├── manager.go               # 只复用 readWranglerConfig / stripJSONC；不复制读文件
    │   ├── bindings.go              # 重写：ParseBindings → DESIGN 类型
    │   └── bindings_test.go         # table-driven fixtures
    └── api/
        ├── server.go                # 路由已有；改 CORS
        ├── bindings.go              # handleGetBindings
        └── bindings_test.go         # httptest：ready / 404 / 形状
```

## Go 类型（响应 = DESIGN §8.4，字段名 = celld wrangler）

放在 **`cellp/internal/runtime`**（解析结果即 API JSON，`api` 不要再定义一套）。handler `writeJSON` 直接编码此值。

**每个切片必须 `make(..., 0)`**，禁止 nil → JSON `null`。DESIGN：「空数组表示未声明，不是错误。」

```go
// Bindings is GET /v1/projects/{id}/versions/{vid}/bindings (DESIGN §8.4).
type Bindings struct {
	D1        []D1Binding       `json:"d1"`
	KV        []KVNamespace     `json:"kv"`
	Queues    []QueueBinding    `json:"queues"`
	Workflows []WorkflowBinding `json:"workflows"`
	R2        []R2Bucket        `json:"r2"`
	Crons     []string          `json:"crons"`
}

// wrangler d1_databases[] — celld: binding, database_name, database_id
type D1Binding struct {
	Binding      string `json:"binding"`
	DatabaseName string `json:"database_name"`
	DatabaseID   string `json:"database_id,omitempty"`
}

// wrangler kv_namespaces[] — celld: binding + id（verbatim；禁止发明 namespace_name）
// T2 路径 {ns} == ID
type KVNamespace struct {
	Binding string `json:"binding"`
	ID      string `json:"id"`
}

// 按 wrangler queues.*.queue 去重合并。
// producers: binding, queue（忽略 delivery_delay）
// consumers: queue, dead_letter_queue（忽略 max_batch_* / retry_delay / script_name）
type QueueBinding struct {
	Name            string  `json:"name"`                         // queues.producers[].queue 或 queues.consumers[].queue
	Binding         string  `json:"binding,omitempty"`            // 仅 producer 的 env 名
	Consumer        bool    `json:"consumer"`                     // 本 script 是否在 consumers[] 里声明了该 queue
	DeadLetterQueue *string `json:"dead_letter_queue,omitempty"`  // 仅 consumer
}

// wrangler workflows[] — celld: binding, name, class_name（script_name 忽略）
// T3 路径 {name} == Name（资源名，不是 binding）
type WorkflowBinding struct {
	Binding   string `json:"binding"`
	Name      string `json:"name"`
	ClassName string `json:"class_name"`
}

// wrangler r2_buckets[] — celld: binding, bucket_name（拒绝 jurisdiction；清单也不输出）
type R2Bucket struct {
	Binding    string `json:"binding"`
	BucketName string `json:"bucket_name"`
}
```

### 示例 JSON

```json
{
  "d1": [
    {"binding": "DB", "database_name": "main", "database_id": "db-demo-v1"}
  ],
  "kv": [
    {"binding": "KV", "id": "ns-1"}
  ],
  "queues": [
    {"name": "tasks", "binding": "TASKS", "consumer": true, "dead_letter_queue": "tasks-dlq"},
    {"name": "events", "binding": "EVENTS", "consumer": false}
  ],
  "workflows": [
    {"binding": "WF", "name": "order-flow", "class_name": "OrderWorkflow"}
  ],
  "r2": [
    {"binding": "FILES", "bucket_name": "files"}
  ],
  "crons": ["0 * * * *"]
}
```

consumer-only queue：`{"name":"events","consumer":true}`（无 `binding`）。  
producer-only：`consumer: false`，无 `dead_letter_queue`。

同名 queue 既有 producer 又有 consumer → **一条** `queues[]`。多个 producer 绑同一 `queue` → 保留 **第一条** `binding`（celld 允许多个 producer binding；清单不发明数组字段）。

## wrangler 解析（扩展 runtime，禁止复制）

| 函数 | 文件 | 职责 |
|------|------|------|
| `readWranglerConfig(projectDir)` | `manager.go` | 读 `wrangler.jsonc` 优先，否则 `wrangler.json`；jsonc 走 `stripJSONC` |
| `readWranglerConfigFile` | `manager.go` | 返回 path+bytes；两文件都不存在 → error 文案 `no wrangler.jsonc or wrangler.json in …` |
| `stripJSONC` | `manager.go` | `//` 与 `/* */` |
| **`ParseBindings(projectDir string) (*Bindings, error)`** | `bindings.go` | **唯一**清单入口。内部只调用 `readWranglerConfig`，再 `json.Unmarshal` 到 wrangler 镜像 struct |
| `D1DatabaseID` / `D1DatabaseName` | `manager.go` | **不改**；继续只处理 `d1_databases` 0/1/>1 |

建议导出 sentinel，供 API 映射：

```go
var ErrNoWrangler = errors.New("wrangler not found")
```

`readWranglerConfigFile` 在两个文件都不存在时 `return "", nil, fmt.Errorf("%w: %s", ErrNoWrangler, projectDir)`（保留原字符串亦可，但 API 必须能 `errors.Is`）。

### 内部 unmarshal 镜像（仅 parse，不直接当 HTTP JSON）

字段名必须与 celld `deploy.rs` 一致（不是 Cloudflare 文档里的别名）：

```go
type wranglerFile struct {
	D1Databases  []wranglerD1 `json:"d1_databases"`
	KVNamespaces []wranglerKV `json:"kv_namespaces"`
	Queues       *wranglerQueues `json:"queues"`
	Workflows    []wranglerWorkflow `json:"workflows"`
	R2Buckets    []wranglerR2 `json:"r2_buckets"`
	Triggers     *struct {
		Crons []string `json:"crons"`
	} `json:"triggers"`
}

type wranglerD1 struct {
	Binding      string `json:"binding"`
	DatabaseName string `json:"database_name"`
	DatabaseID   string `json:"database_id"`
}

type wranglerKV struct {
	Binding   string `json:"binding"`
	ID        string `json:"id"`
	PreviewID string `json:"preview_id"` // celld 接受但 deploy 忽略；清单不输出
}

type wranglerQueues struct {
	Producers []struct {
		Binding string `json:"binding"`
		Queue   string `json:"queue"`
	} `json:"producers"`
	Consumers []struct {
		Queue           string  `json:"queue"`
		DeadLetterQueue *string `json:"dead_letter_queue"`
	} `json:"consumers"`
}

type wranglerWorkflow struct {
	Binding   string `json:"binding"`
	Name      string `json:"name"`
	ClassName string `json:"class_name"`
}

type wranglerR2 struct {
	Binding    string `json:"binding"`
	BucketName string `json:"bucket_name"`
}
```

**宽松策略（清单 ≠ deploy）：** celld 缺字段会 `bail!`；cellp 只读跳过不完整条目（缺 `binding` / 缺 kv `id` / 缺 queue `queue` / 缺 workflow `name`+`class_name` / 缺 r2 `bucket_name`）。整文件不是 JSON → 返回 `fmt.Errorf("parse wrangler: %w", err)`。不校验 cron 表达式（那是 deploy 的事）。

**明确不解析进响应：** `durable_objects` · `services` · `vars` · `assets` · `migrations` · `compatibility_*` · queue `delivery_delay` / `max_batch_size` 等。`preview_id`、workflow `script_name` 读入可丢弃。

删除当前 `Binding` / `WorkerManifest` / `parseDOBindings` / `parseVars` / `parseAssetsBinding` / `parseServiceBindings`。旧测试 `TestParseBindingsCommerce` / `TestParseBindingsFullTopology` 改为下表 fixture。

## API handler

**文件：** `cellp/internal/api/bindings.go`  
**函数：** `handleGetBindings`

抄 `database.go` `resolveDatabaseContext` 的前半（不要复用 D1「无 database → 404」）：

1. `chi.URLParam`：`projectID` · `versionID`
2. `s.store.GetVersion` → err 则 500；`v == nil` → 404 `version_not_found`
3. `v.Status != registry.StatusReady` → 404 `{"error":"version_not_ready"}`
4. `projectDir := filepath.Join(s.cfg.ArtifactsDir, projectID, versionID)`
5. `runtime.ParseBindings(projectDir)`
   - `errors.Is(err, runtime.ErrNoWrangler)` → 404 `wrangler_not_found`
   - 其它 err → 500 `{"error": err.Error()}`
6. 200 + `Bindings` JSON  
鉴权：已有 `s.requireAdmin`（ADMIN_TOKEN），与 D1 相同。

### 错误映射（对照 D1）

| 条件 | D1 `GET …/database` | 本清单 `GET …/bindings` |
|------|---------------------|-------------------------|
| version 不存在 | 404 `version_not_found` | **同** |
| status ≠ `ready` | 404 `version_not_ready` | **同**（DESIGN §8.4：「version 未 ready → 404」） |
| wrangler 在但无 `d1_databases` | 404 `database_not_found` | **200**，`d1: []` 及其余按声明 |
| **无 wrangler 文件** | `D1DatabaseName` 失败 → **500**（把 I/O 当 parse 错） | **404 `wrangler_not_found`** |
| 非法 jsonc | 500 `parse wrangler` | **500**（同 D1） |

**取舍：无 wrangler 选 404，不选空数组。**

- DESIGN 说空数组 = **「未声明」**，前提是已经从 wrangler 抽出 key。文件不存在不是「未声明」，是没有清单可读。
- D1 把缺文件打成 500 过重；清单是 Dashboard hub 入口，缺 artifact 用 404 让 UI 走空态/错误态，而 **不会**把「没 KV」和「artifact 丢了」混成六个 `[]`。
- 与 D1「无 d1 → 404」对齐的是 **资源不可用**；与 D1 不同的是：无 KV 声明仍 200 + `kv: []`，因为 Bindings 资源本身还在。

## Route registration（`server.go`）

已有（保留，不要挪到 `/database` 下面）：

```go
r.Get("/bindings", s.requireAdmin(s.handleGetBindings))
```

完整前缀：`/v1/projects/{projectID}/versions/{versionID}/bindings`。  
T1 **不要**加 `r.Put` / `r.Route("/kv"` 等。

### CORS

```go
w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
```

T1 本身只有 GET；**PUT 必须现在写上**。否则 T2 Dashboard `PUT …/kv/…/keys/…` 的浏览器预检会被拒。`Allow-Headers` 已有 `Authorization, Content-Type`，不动。

## OpenAPI（只追加，不改既有 path）

在 `cellp/api/openapi.yaml` 的 `/projects/{projectID}/versions/{versionID}/database` **之后**插入（不要改 Version schema、不要改 securitySchemes）：

```
/projects/{projectID}/versions/{versionID}/bindings:
  get:
    operationId: getBindings
    security: [adminToken]
    responses:
      "200": Bindings
      "404": version_not_found | version_not_ready | wrangler_not_found
      "500": parse wrangler
```

`components.schemas` 增加：`Bindings` · `BindingsD1` · `BindingsKV` · `BindingsQueue` · `BindingsWorkflow` · `BindingsR2`，属性名与上表 JSON 一致。`crons`：`type: array, items: {type: string}`。六个数组 `default: []`。

**不要**在 T1 写入 `…/kv` · `…/queues` · `…/workflows` operator path（T2/T3 合同）。T4 在 T1 落地后即可打 GET bindings；operator 页等 T2/T3 OpenAPI。

若仓库有 `make openapi-check`，改完跑一次；失败不改 `go.mod` 去加 linter。

## Unit tests

### `internal/runtime/bindings_test.go` — table-driven

`TestParseBindings`：每行写临时目录 + `wrangler.jsonc` 或 `.json`，调 `ParseBindings`。

| name | fixture 要点 | 断言 |
|------|----------------|------|
| `kv-only` | `kv_namespaces: [{binding:"KV", id:"ns-1"}]` | `kv==[{KV,ns-1}]`；其余 `len==0` |
| `queues-producers-consumers` | `producers:[{binding:"TASKS",queue:"tasks"}]` + `consumers:[{queue:"tasks",dead_letter_queue:"tasks-dlq"},{queue:"events"}]` | `tasks`：binding TASKS、consumer true、dlq；`events`：无 binding、consumer true |
| `workflows` | `{binding:"WF", name:"order-flow", class_name:"OrderWorkflow"}` | 三字段；`kv`/`r2` 空 |
| `r2` | `{binding:"FILES", bucket_name:"files"}` | `r2[0].bucket_name=="files"` |
| `crons` | `triggers: {crons: ["0 * * * *", "*/5 * * * *"]}` | `crons` 两段字符串 |
| `empty` | `{"name":"empty","main":"index.js"}` | 六数组长度均为 0，JSON  marshal 为 `[]` 不是 `null` |
| `invalid-jsonc` | `/* broken` 或 `{ binding:` | `err != nil`；`errors.Is(ErrNoWrangler)==false` |
| `jsonc-comments` | `// comment` + 合法 `kv_namespaces` | 与 kv-only 相同（证明仍走 `stripJSONC`） |
| `no-file` | 空目录 | `errors.Is(err, ErrNoWrangler)` |
| `d1-passthrough` | 一条 `d1_databases` | `d1[0].binding/database_name/database_id`；不改变 `D1DatabaseName` 测试 |

另：`TestParseBindingsJSONNotNull` 对 empty fixture `json.Marshal`，body 含 `"d1":[]` 等。

**删掉**依赖扁平 `bindings[].type` 的旧测试。

### `internal/api/bindings_test.go` — httptest

复用 `testAPI` / `setupDatabaseVersion`（或本地 `setupReadyVersion`）。重写 `TestGetBindings`：断言顶层 `d1`/`kv`/… 而不是 `resp["bindings"]`。

| 测试 | 期望 |
|------|------|
| `TestGetBindingsReady` | ready + wrangler（含 d1）→ 200，`d1[0].binding=="DB"`，缺的 key 为 `[]` |
| `TestGetBindingsVersionNotFound` | 404 `version_not_found` |
| `TestGetBindingsVersionNotReady` | 只 CreateVersion、不 Update ready → 404 `version_not_ready`（抄 `TestDatabaseVersionNotReady`） |
| `TestGetBindingsNoWrangler` | ready、删掉 wrangler → 404 `wrangler_not_found`（替换当前 `bindings_not_found`） |
| `TestGetBindingsEmptyDeclared` | ready + `{"name":"x"}` → **200** 全空数组（对照 D1 `TestDatabaseNoD1` 的 404） |
| `TestGetBindingsInvalidJSONC` | ready + 坏文件 → 500 |
| `TestCORSAllowsPUT` | `OPTIONS /v1/projects/demo/versions/v1/bindings`，`Allow-Methods` 含 `PUT` |

`TestGetBindings` 旧断言 `bindings[0].type=="d1"` **必须删**。

## Forbidden（本 track）

- Registry 存 KV 内容；见 AD-6 / AD-7
- inherit / copy / 挂父 version bucket（AD-7）
- R2 对象 list/get/put；任何 `…/r2/{bucket}/…` 路由
- Workflow pause/resume/restart/delete
- 改冻结契约 `docs/plans/D1-*-RPC.md`
- 改 `cellp/go.mod`（除非 deps owner 另派）
- 改 `celld/` submodule
- Dashboard / `web/`（T4）
- `celld kv` / `celld queue` / `celld cell list` 包装（T2/T3）
- 改 `D1DatabaseID` / `D1DatabaseName` 的 0/1/>1 行为

## Verify

```bash
cd cellp && go test ./internal/runtime ./internal/api ./...
```

预期：`runtime` 新 fixture 全绿；`api` 旧 bindings 测试已改写；`./...` 回归 D1 / orch 无红。不跑 e2e、不启 celld。

## Subagent 约束

```
Track P7-T1. 只改 cellp/internal/runtime/bindings*.go、manager.go（仅 ErrNoWrangler 包装）、
cellp/internal/api/bindings*.go、server.go CORS、cellp/api/openapi.yaml。
合同：DESIGN.md §8.4。解析字段跟 celld deploy.rs，不要跟旧 BindingsManifest。
```
