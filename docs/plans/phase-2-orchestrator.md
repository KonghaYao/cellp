# Phase 2 — Orchestrator 与运行时集成

> **TP：** TP-V1–V7 · TP-V7-D · TP-SEC-3,4 · TP-API-5,6  
> **前置：** Phase 1 Exit · **`docs/evidence/celld-multi-fleet-spike.md`**（AD-1）

## Exit Criteria

- [x] AD-1 spike 文档：2 version 不同 body 同时 200
- [x] `cd cellp && go test ./internal/...`
- [x] `e2e/scripts/v1-*.sh` … `v7-*.sh` exit 0
- [x] POST /versions 驱动真实 pending→ready（非 stub）

## Execution Status

**Audit date:** 2026-08-28 (Track P2-AUDIT)

| Track | Status | Evidence |
|-------|--------|----------|
| **P2-T1** Branch | ✅ | `cellp/internal/branch/` · `v2-quiesce-fork.sh` |
| **P2-T2** Runtime | ✅ | `cellp/internal/runtime/` · AD-1 multi-port · `v1-d1-seed.sh` · `v3-dual-route.sh` |
| **P2-T3** Artifact | ✅ | `cellp/internal/artifact/` — S3 fetch, digest, SSRF |
| **P2-T4** Orchestrator | ✅ | `cellp/internal/orch/` — deploy worker, promote saga AD-5 |
| **P2-T5** E2E scripts | ✅ | `v4`–`v7` present; all pass via `run-all.sh` |

### AD-1 spike

See [`docs/evidence/celld-multi-fleet-spike.md`](../evidence/celld-multi-fleet-spike.md) — port table, bucket isolation, v3 dual-route evidence.

### Commands run (audit)

```bash
cd cellp && go test ./...          # exit 0
RUN_GATES=1 ./e2e/scripts/run-all.sh  # exit 0 (includes v1..v7)
```

### Known follow-ups (non-blocking M2)

| Item | Status |
|------|--------|
| TP2-S2 HTTP 429 | ✅ POST returns 429 when ready ≥ limit (`handleCreateVersion`) |
| AD-3 job reclaim | ✅ `ClaimJob` reclaims expired `claimed` leases |
| Promote compensate | `offshoot_promote` rollback is log-only |
| Unit tests | `orch`/`runtime`/`branch` lack dedicated tests |

## Parallel Tracks

| Track | ID | 并行 | Gate | 交付 |
|-------|-----|------|------|------|
| Branch Manager | **P2-T1** | ∥ T2,T3 | Phase 1 | `internal/branch` · `v2-quiesce-fork.sh` |
| Runtime Manager | **P2-T2** | ∥ T1,T3 | AD-1 spike | `internal/runtime` · `v1-d1-seed.sh` · `v3-dual-route.sh` |
| Artifact Store | **P2-T3** | ∥ T1,T2 | Phase 1 | `internal/artifact` |
| E2E scripts | **P2-T5** | ∥ T1–T3（早期） | API stable | `v4`–`v7` · `v6` · `counter-migrate/` |
| Orchestrator | **P2-T4** | **串行 T1,T2,T3,T5 契约** | 上列 | `internal/orch` |

```
T1 ──┐
T2 ──┼──> T4
T3 ──┘
T5 (scripts) ──> T4 集成验证
```

## AD-1 — Runtime Manager（P2-T2）

**每 ready version：**

1. 分配 `upstream_port`（Registry `routes`；基址 8792，+1 per version）
2. `CELLD_BUCKET=s3://cellp-celld/{project}/{version}` **或** 共享 bucket + version env
3. 启动 celld 子进程 `--listen 127.0.0.1:{port}`（≤5 进程 / project）
4. `Deploy` · `D1Execute` · `Health`

**Spike 交付：** `docs/evidence/celld-multi-fleet-spike.md` 含端口分配表与资源占用。

## P2-T1 — Branch Manager

offshoot CLI 包装：Drain（→ `SetRouteActive false`）· Checkpoint · Fork · Export · Promote · Destroy

**Store：** V0b 未过 → `OFFSHOOT_STORE=./dev/data/offshoot-store`

## P2-T3 — Artifact

RustFS fetch · digest verify · SSRF（TP-SEC-1）

## P2-T4 — Orchestrator

**状态机：** DESIGN §2.5

**Worker：** 从 SQLite `jobs` claim（lease）；重启恢复 AD-3

**Promote saga AD-5：**

```
forward:  validate → drain_old → deactivate_old_route → offshoot_promote → SetProdVersionCAS → activate_prod_route
compensate: 逆序 idempotent
```

**Version 上限 TP2-S2：** ready ≥5 → 429

**SQLite 争用：** registry 包统一 retry（50ms→2s，max 30）

## P2-T5 — Script ownership

| 脚本 | Owner | TP |
|------|-------|-----|
| `v1-d1-seed.sh` | T2 | V1 |
| `v2-quiesce-fork.sh` | T1 | V2 |
| `v3-dual-route.sh` | T2 | V3 |
| `v4-promote-cutover.sh` | T5 | V4 |
| `v5-saga-compensate.sh` | T5 | V5 |
| `v6-migrate-order.sh` | T5 | V6 |
| `v7-external-ci.sh` | T5 | V7, V7-D |

## Subagent prompt

```
Track P2-T{n}. cd cellp for Go. Implement only your package + assigned e2e scripts.
Read REVIEW AD-1, AD-5. Multi-version = separate celld ports, not single deploy swap.
```
