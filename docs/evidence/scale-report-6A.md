# Phase 6A — SQLite 基线与止血验收报告

> **Run ID:** 20260829-064349  
> **日期:** 2026-08-29  
> **范围:** 6A only（**无 PG、无多租户**）

## 环境

| 项 | 值 |
|----|-----|
| Registry | SQLite `./dev/data/cellp-registry.sqlite` |
| API | `http://127.0.0.1:8790` |
| Gateway | `http://127.0.0.1:8787` |
| 拓扑 | 单节点 · **~10,020 projects** |

## TP6-A1 — API 游标分页

| 检查 | 结果 |
|------|------|
| `GET /v1/projects?limit&cursor` | ✅ 实现 + `go test ./internal/registry` |
| `GET /v1/projects/{id}/versions?limit&cursor` | ✅ 实现 + API 测试 |
| 默认 limit=50，最大 200 | ✅ |

## TP6-A2 — Gateway 路由缓存

| 检查 | 结果 |
|------|------|
| `route_cache.go` + 写路径 invalidate | ✅ `go test ./internal/gateway` |

## TP6-A3 — Job/Version GC

| 检查 | 结果 |
|------|------|
| GC worker + `dev/scripts/gc.sh` | ✅ `go test ./internal/gc` |

## TP6-A4 — Dashboard

| 检查 | 结果 |
|------|------|
| Load more 分页 | ✅ |
| Playwright e2e | ✅ **7/7** |

## TP6-A5 — Registry 基准（实跑）

**Seed:**

```text
seed-projects: 10,020 total (scale-seed-*)
seed-versions: 10,000 rows (10 projects × 1000)
```

**Bench @ 10k projects (200 samples, limit=50):**

| 指标 | p50 | p95 | p99 | Gate | 结果 |
|------|-----|-----|-----|------|------|
| ListProjects（优化后） | 180ms | 225ms | **238ms** | <200ms | ❌ |
| ListProjects（ANALYZE 后） | 203ms | 241ms | **262ms** | <200ms | ❌ |
| ListVersions | 0ms | 1ms | **1ms** | <100ms | ✅ |

**Bench @ 1k projects（早期 run）：** ListProjects p99 **64ms** ✅

**查询优化（已提交，待重启 cellpd 复测）：**

- 去掉 `ListProjects` correlated `COUNT(*)` subquery
- 分页后批量 `IN (...)` 统计 `version_count`
- 新增索引 `idx_projects_created(created_at, id)`

## 结论

| 项 | 状态 |
|----|------|
| 6A 代码 + 单测 + e2e | ✅ |
| TP6-A5 @ 1k | ✅ |
| TP6-A5 @ 10k ListProjects | ❌（SQLite 上限；已优化待复测） |
| 6B–6F | **OUT OF SCOPE**（不做 PG / 多租户） |

证据：`docs/evidence/scale-metrics.jsonl` · `docs/evidence/scale-env.json`
