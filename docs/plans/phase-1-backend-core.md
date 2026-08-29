# Phase 1 — 后端核心（cellpd）

> **TP：** TP-API-* · TP-SEC-1,2,5 · TP-DEV-1（部分）  
> **Module root：** `cellp/` — `cd cellp && go test ./...`

## Exit Criteria

- [ ] `cd cellp && go build -o cellpd ./cmd/cellpd`
- [ ] `cd cellp && go test ./internal/registry/... ./internal/api/... ./internal/gateway/...`
- [ ] `curl :8790/v1/health` · `curl :8787/health` → 200
- [ ] Gateway **`/{project}/`** prod 路由可用（REVIEW AD-2）
- [ ] Gateway drain：`route.active=false` → **503**
- [ ] `cellp/api/openapi.yaml` + `make openapi-check` exit 0
- [ ] TP-API-1..4 通过

## Parallel Tracks

| Track | ID | 并行 | Gate | 交付 |
|-------|-----|------|------|------|
| Scaffold + Registry + jobs schema | **P1-T1** | **首 track** | 无 | `cellp/go.mod` · registry · schema |
| HTTP API + OpenAPI | **P1-T2** | ∥ T3 | **T1 Store 接口冻结** | `internal/api` · `api/openapi.yaml` |
| Gateway + prod route + drain | **P1-T3** | ∥ T2 | T1 Store | `internal/gateway` |
| 组装 + job queue 接线 | **P1-T4** | 串行 | T1,T2,T3 | `cmd/cellpd` · **唯一 go.mod deps 合并点** |

```
T1 ──┬──> T2 (API)
     └──> T3 (Gateway)
T1+T2+T3 ──> T4 (main)
```

## Repo layout

```
cellp/
├── go.mod                    # module github.com/cellp/cellp
├── Makefile                  # openapi-check, test
├── api/openapi.yaml
├── cmd/cellpd/main.go
└── internal/
    ├── config/
    ├── registry/             # T1 owner
    ├── api/                  # T2 owner — 不得改 Store 接口
    ├── gateway/              # T3 owner — 不得改 main.go
    └── job/                  # T1 定义 Job + Queue 接口
```

## P1-T1 — Registry + Schema

**表：**

- `projects(id, git_remote, prod_version_id, created_at)`
- `versions(...)` — 见 DESIGN §4
- `routes(project_id, version_id, active, upstream_host, upstream_port)`
- `jobs(id, project_id, version_id, step, status, lease_until, updated_at)` — REVIEW AD-3

**Store 接口（冻结后 T2/T3 只读引用）：**

```go
type Store interface {
    CreateProject(...)
    GetProject(...)
    CreateVersion(...)
    UpdateVersionStatus(...)
    SetRouteActive(projectID, versionID string, active bool) error
    GetRoute(...)
    ListActiveRoutes(...)
    SetProdVersionCAS(projectID, expected, new string) error  // AD-5
    // jobs
    EnqueueJob(...)
    ClaimJob(...) (*Job, error)
    CompleteJob(...)
}
```

**SQLite：** WAL · `_busy_timeout=60000` · 路径 `CELLP_REGISTRY_DB`

**测试：** `cd cellp && go test ./internal/registry/...`

## P1-T2 — HTTP API

**不得：** 编辑 `go.mod`（deps 列表交 T4）· 修改 `Store` 接口

**POST /versions：** 202 · 写 `pending` · `job.Enqueue` · artifact URI 服务端构造

**OpenAPI：** `cellp/api/openapi.yaml` — P4-T1 消费

**鉴权 TP-API-3 / TP-SEC-2**

## P1-T3 — Gateway

**路由（全部必做）：**

| 模式 | 行为 |
|------|------|
| `/health` | 200 |
| `/{project}/{version}/*` | strip prefix → upstream from `routes` |
| `/{project}/*` | **prod_version_id** upstream（AD-2） |
| inactive route | **503**（drain · TP-V2） |

**不得：** 编辑 `main.go` · `go.mod`

**测试：** httptest + mock upstream

## P1-T4 — 组装

- 合并 T2/T3 依赖到 `go.mod`
- `main.go`：API :8790 · Gateway :8787 · job queue goroutine（consumer stub → Phase 2）
- `dev/scripts/build-cellpd.sh`

## Job queue 契约（→ Phase 2 T4）

```go
// internal/job/job.go
type DeployJob struct {
    ProjectID, VersionID string
    Step string // fetching|branching|...
}
type Queue interface {
    Enqueue(ctx context.Context, j *DeployJob) error
}
```

Phase 1：channel 实现 + SQLite `jobs` 双写（enqueue 时 persist）。

## Subagent 约束

| Track | 可改 | 不可改 |
|-------|------|--------|
| T1 | registry/, job/, schema, go.mod 初始 | — |
| T2 | internal/api/, openapi.yaml | Store, main.go, go.mod |
| T3 | internal/gateway/ | Store, main.go, go.mod |
| T4 | main.go, go.mod 合并 | — |
