# Phase 5 — Production Stress Harness

Implements [test-plan-phase2.md](../docs/test-plan-phase2.md) against a **single-node** cellpd stack.

## Prerequisites

1. Dev stack running: `./dev/scripts/up.sh`
2. Optional: `celld`, `offshoot`, `sqlite3`, `vegeta`
3. Configure thresholds in [docs/evidence/stress-env.json](../docs/evidence/stress-env.json)

All scripts:

- Source `dev/.env` (API `:8790`, Gateway `:8787`)
- Use **`stress-demo`** project prefix only (safety guard in `lib/common.sh`)

## Quick start

```bash
# Full suite except real 24h (runs 5min soak instead)
./stress/scripts/run-all.sh -short

# Individual tests
./stress/scripts/sequential-cd.sh      # TP2-L1
./stress/scripts/concurrent-cd.sh      # TP2-L2, L3
./stress/scripts/gateway-load.sh       # TP2-L4, L5
./stress/scripts/soak-24h.sh -short    # TP2-S1 (CI)
./stress/scripts/collect-metrics.sh    # TP2-MET-1
```

## Script map

| Script | Test IDs | Notes |
|--------|----------|-------|
| `run-all.sh` | TP2-R2 | `-short` skips real 24h soak |
| `sequential-cd.sh` | TP2-L1 | 10 deploys, p95 vs `STRESS_P95_DEPLOY_SEC` |
| `concurrent-cd.sh` | TP2-L2, L3 | 3-way + multi-project isolation |
| `gateway-load.sh` | TP2-L4, L5 | vegeta or bash loop; promote under load |
| `soak-24h.sh` | TP2-S1 | `STRESS_SOAK_SECONDS`, `-short` for CI |
| `version-limit.sh` | TP2-S2 | 6th POST → 429 |
| `ttl-gc.sh` | TP2-S3 | destroy/TTL → route GC |
| `chaos-*.sh` | TP2-C1–C5 | Run after L baseline |
| `data-counter-load.sh` | TP2-D1 | 100 workers; `n == successful_requests` |
| `collect-metrics.sh` | TP2-MET-1 | Appends `docs/evidence/stress-metrics.jsonl` |

## Environment variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `STRESS_PROJECT_PREFIX` | `stress-demo` | Project ID prefix |
| `STRESS_RUN_ID` | timestamp | Unique suffix per run |
| `STRESS_SOAK_SECONDS` | `86400` / `300` (short) | Soak duration |
| `STRESS_SOAK_SHORT` | `0` | Set `1` with soak script |
| `STRESS_GATEWAY_DURATION` | `300` | L4 load duration (seconds) |
| `STRESS_COUNTER_WORKERS` | `100` | D1 concurrency |
| `STRESS_COUNTER_DURATION` | `300` | D1 duration (seconds) |

Thresholds load from `docs/evidence/stress-env.json` → `thresholds.*`.

## Evidence artifacts

| Path | Purpose |
|------|---------|
| `docs/evidence/stress-env.json` | Hardware, tier, baselines, thresholds |
| `docs/evidence/stress-metrics.jsonl` | Per-test metrics (append-only) |
| `docs/evidence/stress-report.md` | Human report (TP2-R1 — fill after runs) |

## Dependencies

**Required:** `bash`, `curl`, `jq`

**Optional:**

| Tool | Used by |
|------|---------|
| `vegeta` | `gateway-load.sh` (falls back to bash loop) |
| `sqlite3` | route counts, contention checks |
| `docker` | `chaos-rustfs-pause.sh` |
| `celld` | deploy chaos + gateway fixtures |
| `cellpd` | production target (mock OK for harness smoke) |

Install vegeta: `go install github.com/tsenart/vegeta@latest`

## Offshoot tier

Record `offshoot_tier` (`local` | `rustfs`) in `stress-env.json` before comparing runs.
