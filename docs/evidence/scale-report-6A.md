# Phase 6A — SQLite 基线与止血验收报告

> **Run ID:** 20260829-161942  
> **日期:** 2026-08-29  
> **范围:** 6A only（**无 PG、无多租户**）  
> **结论:** **6A 实现完成（SQLite scope）** · 6B–6F OUT OF SCOPE

## 环境

| 项 | 值 |
|----|-----|
| Registry | SQLite `./dev/data/cellp-registry.sqlite`（**5.07 MB**） |
| API | `http://127.0.0.1:8790` |
| Gateway | `http://127.0.0.1:8787` |
| 拓扑 | 单节点 · **11,202 projects** · **10,026 versions** |

## TP6-A1 — API 游标分页

| 检查 | 结果 |
|------|------|
| `GET /v1/projects?limit&cursor` | ✅ |
| `GET /v1/projects/{id}/versions?limit&cursor` | ✅ |

## TP6-A2 — Gateway 路由缓存

| 检查 | 结果 |
|------|------|
| `route_cache.go` + invalidate | ✅ |

## TP6-A3 — Job/Version GC

| 检查 | 结果 |
|------|------|
| GC worker + `dev/scripts/gc.sh` | ✅ |

## TP6-A4 — Dashboard

| 检查 | 结果 |
|------|------|
| Playwright e2e | ✅ **16/16** |

## TP6-A5 — Registry 基准（2026-08-29 复测）

**Registry snapshot** (`registry-size-report.sh`):

| 表 | 行数 |
|----|------|
| projects | 11,202 |
| versions | 10,026 |
| jobs | 26 |
| routes (active) | 21 |

**Bench @ ~11k projects (200 samples, limit=50):**

| 指标 | p50 | p95 | p99 | Gate | 结果 |
|------|-----|-----|-----|------|------|
| ListProjects | 4ms | 6ms | **7ms** | <200ms | ✅ |
| ListVersions | 5ms | 8ms | **8ms** | <100ms | ✅ |

> 早期 run（`20260829-064349`）在 correlated `COUNT(*)` 优化前测得 ListProjects p99 **238–262ms**。当前树已优化（批量 `IN` + `idx_projects_created`），复测通过。

**Gateway dev baseline** (`gateway-scale.sh -short`):

| 指标 | 值 | 说明 |
|------|-----|------|
| RPS | 50 | dev 上限，非 prod gate |
| p99 | 105ms | cached route |
| error_rate | 0 | 2850/2850 OK |

> **100k RPS gate** 属 6E+，需水平 Gateway + LB，**不在 SQLite dev scope**。

## 里程碑

| 里程碑 | 状态 |
|--------|------|
| **M4** 6A Gate（SQLite） | ✅ 实现 + 基准通过 |
| **M5–M7** | **OUT OF SCOPE**（无 PG / 无千万 infra） |

## 证据

- `docs/evidence/scale-metrics.jsonl`
- `docs/evidence/scale-env.json`
- `stress/phase6/registry-size-report.sh`
- `stress/phase6/gateway-scale.sh`
