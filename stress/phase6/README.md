# Phase 6 — Scale Stress Harness (6A-T5)

Implements **TP6-A5** seed + registry benchmark scripts from [test-plan-phase6.md](../../docs/test-plan-phase6.md).

## Prerequisites

1. Dev stack running: `./dev/scripts/up.sh`
2. Tools: `bash`, `curl`, `jq`, `sqlite3` (required for `seed-versions.sh`)
3. Optional: `vegeta` (preferred for `list-api-load.sh`)
4. Configure thresholds in [docs/evidence/scale-env.json](../../docs/evidence/scale-env.json)

All scripts:

- Source **`dev/.env`** for `PLATFORM_URL`, `PLATFORM_TOKEN` / `CELLP_ADMIN_TOKEN`
- Use **`scale-seed`** project prefix only (safety guard — refuses other IDs)

## Quick start — 6A ladder

```bash
# 1. Seed 1k projects (default)
./stress/phase6/seed-projects.sh

# Large offshoot SQLite branches (one project, 100MB × 50 forks)
./stress/phase6/offshoot-branch-scale.sh

# 100 MB binary D1 import
./stress/phase6/d1-import-scale.sh

# 100 MB D1 branch object-volume gate (default 8 MB; D1_IMPORT_SIZE_MB=100 for large)
./stress/phase6/d1-branch-scale.sh

# 2. Seed 10k versions across 10 projects (1000 each)
SCALE_SEED_PROJECTS=10 SCALE_SEED_VERSIONS=1000 ./stress/phase6/seed-versions.sh

# 3. Measure ListProjects / ListVersions p50/p95/p99
./stress/phase6/registry-bench.sh

# 4. Sustained cursor pagination load
./stress/phase6/list-api-load.sh
```

### Full 10k project ladder

```bash
SCALE_SEED_N=10000 ./stress/phase6/seed-projects.sh
SCALE_SEED_PROJECTS=10 SCALE_SEED_VERSIONS=10000 ./stress/phase6/seed-versions.sh
SCALE_BENCH_SAMPLES=500 ./stress/phase6/registry-bench.sh
SCALE_LOAD_RPS=200 SCALE_LOAD_DURATION=120 ./stress/phase6/list-api-load.sh
```

## Script map

| Script | Test ID | Notes |
|--------|---------|-------|
| `seed-projects.sh` | TP6-A5 | POST `/v1/projects` — metadata only |
| `seed-versions.sh` | TP6-A5 | SQLite bulk insert (`destroyed` status) — no deploy |
| `registry-bench.sh` | TP6-A5 | p50/p95/p99 vs scale-env thresholds |
| `d1-import-scale.sh` | TP-D1-IMP | 100 MB binary `celld d1 import` timing + G3 restore |
| `d1-branch-scale.sh` | TP-D1-BRANCH | Parent import → child branch; B2/B5/B6 object volume |
| `offshoot-branch-scale.sh` | TP-OB / TP-V0b-L | Single-project large SQLite CoW branches (local offshoot) |
| `offshoot-branch-scale-rustfs.sh` | TP-OB-R | Same ladder on `s3://cellp-offshoot` (RustFS) |
| `list-api-load.sh` | TP6-A5 | vegeta or curl cursor loop |

## Environment variables

| Variable | Default | Purpose |
|----------|---------|---------|
| `SCALE_SEED_N` | `1000` | Projects to create via API |
| `SCALE_SEED_BATCH` | `50` | Parallel batch size for seed-projects |
| `SCALE_SEED_VERSIONS` | `100` | Versions per project (seed-versions) |
| `SCALE_SEED_PROJECTS` | `10` | Projects to seed versions into |
| `SCALE_VERSION_STATUS` | `destroyed` | Metadata status (avoids ready cap) |
| `SCALE_BENCH_SAMPLES` | `200` | Registry bench iterations |
| `SCALE_BENCH_LIMIT` | `50` | Page size for bench requests |
| `SCALE_LOAD_RPS` | `100` | list-api-load target RPS |
| `SCALE_LOAD_DURATION` | `60` | list-api-load duration (seconds) |
| `SCALE_LOAD_TARGET` | `projects` | `projects` \| `versions` \| `both` |
| `STRESS_PROJECT_PREFIX` | `scale-seed` | **Do not change** unless cleaning up |
| `STRESS_RUN_ID` | timestamp | Unique suffix per run |
| `D1_IMPORT_SIZE_MB` | `100` / `8` | Seed size for `d1-import-scale.sh` / `d1-branch-scale.sh` |
| `D1_METRICS` | `docs/evidence/d1-import-metrics.jsonl` | D1 import scale metrics output |
| `D1_BRANCH_METRICS` | `docs/evidence/d1-branch-metrics.jsonl` | D1 branch scale metrics output |

Thresholds load from `docs/evidence/scale-env.json` → `thresholds.*`.

## Safety

- Project IDs: `scale-seed-{run_id}-{index}` only
- `seed-versions.sh` refuses non-`scale-seed-*` project IDs
- Version seeding uses **metadata-only** SQLite inserts — does not enqueue deploy jobs
- For API-deployed versions (small smoke), use Phase 5 harness with `stress-demo` prefix

## Evidence artifacts

| Path | Purpose |
|------|---------|
| `docs/evidence/scale-env.json` | Cluster topology, baselines, thresholds |
| `docs/evidence/scale-metrics.jsonl` | Per-run metrics (append-only) |
| `docs/evidence/d1-import-metrics.jsonl` | D1 import scale metrics (append-only) |
| `docs/evidence/scale-report-6A.md` | Human report (fill after runs) |

## Dependencies

**Required:** `bash`, `curl`, `jq`, `sqlite3`

**Optional:**

| Tool | Used by |
|------|---------|
| `vegeta` | `list-api-load.sh` (falls back to curl loop) |

Install vegeta: `go install github.com/tsenart/vegeta@latest`

## TP6-A5 gates (reference)

| Scale | Pass |
|-------|------|
| 10k projects | `ListProjects` cursor p99 **<200ms** |
| 100k versions/project (10×10k) | pagination p99 **<100ms** |
| 100k Gateway RPS (cached) | error rate **<0.1%** |

Gateway scale (`gateway-scale.sh`) and deploy storm scripts are planned for 6E/6C tracks.
