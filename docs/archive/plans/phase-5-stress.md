# Phase 5 — 生产压测

> **TP：** test-plan-phase2 全部 TP2-*  
> **Gate：** **M2**（test-plan.md 全绿）

## Exit Criteria

- [x] `docs/evidence/stress-env.json` 填写 baseline
- [x] `./stress/scripts/run-all.sh -short` exit 0（不含 24h）
- [ ] `./stress/scripts/soak-24h.sh` 或 nightly 记录 — **deferred to nightly CI** (see Execution Status)
- [x] `docs/evidence/stress-report.md` + `stress-metrics.jsonl`
- [x] **M3 达成**

## Execution Status (P5-AUDIT)

**Date:** 2026-08-28  
**Run ID:** `p5-audit-20260828-044246`  
**Branch:** `cursor/p5-audit-stress-4299`

### Commands & Results

| Step | Command | Result |
|------|---------|--------|
| Dev stack | `./dev/scripts/up-native.sh` | OK — cellpd :8790, gateway :8787, RustFS :19000 |
| Short suite | `./stress/scripts/run-all.sh -short` | **exit 0** (~25 min) |
| Evidence | `docs/evidence/stress-env.json` | baseline updated |
| Evidence | `docs/evidence/stress-report.md` | generated |
| Evidence | `docs/evidence/stress-metrics.jsonl` | 96 records (21 from this run) |
| Evidence | `docs/evidence/stress-run-all-short.log` | full transcript |

### Key metrics (this run)

| Test | Metric | Value | Threshold | Pass |
|------|--------|-------|-----------|------|
| L1 | p95 deploy | 2s | ≤ 600s | ✓ |
| L2/L3 | concurrent + cross-talk | 3/3, 0 failures | — | ✓ |
| L4 | p99 / error rate | 0ms / 0% | < 500ms / < 0.1% | ✓ |
| L5 | cutover / 5xx | 1s / 0.067% | ≤ 5s / ≤ 1% | ✓ |
| S1 (-short) | RSS ratio | 0.913 | < 1.10 | ✓ |
| S2 | version limit | 202 (queue) | documented | ✓ |
| S3 | TTL GC | 0s | ≤ 300s | ✓ |
| C1–C5 | chaos | all recovered | — | ✓ |
| D1 | counter drift | 1 | ≤ tolerance | ✓ |

### 24h soak — deferred

Full `./stress/scripts/soak-24h.sh` (86400s) is **not run in agent sessions**. Scheduled for **nightly CI** pipeline. CI short soak (`soak-24h.sh -short`, 300s) satisfies S1 for M3 gate; see `stress-report.md` §24h soak.

## Parallel Tracks

| Track | ID | 并行 | Gate | 交付 |
|-------|-----|------|------|------|
| 负载脚本 | **P5-T1** | ∥ T2 | M2 | `stress/scripts/L*.sh` · `D*.sh` |
| 指标采集 | **P5-T2** | ∥ T1 | M2 | `collect-metrics.sh` · jsonl |
| 混沌 | **P5-T3** | **L1–L5 基线后** | P5-T1 green | `chaos-*.sh` |

## 目录

```
stress/
├── scripts/
│   ├── run-all.sh          # -short skips soak
│   ├── sequential-cd.sh
│   ├── concurrent-cd.sh
│   ├── gateway-load.sh
│   ├── soak-24h.sh
│   ├── version-limit.sh
│   ├── ttl-gc.sh
│   ├── chaos-celld-kill.sh
│   ├── chaos-rustfs-pause.sh
│   ├── chaos-cellpd-restart.sh
│   ├── chaos-offshoot-fail.sh
│   ├── chaos-sqlite-contention.sh
│   └── data-counter-load.sh
└── README.md
```

## stress-env.json 模板

```json
{
  "hardware": { "cpu": "", "ram_gb": 0, "disk": "" },
  "rustfs_tag": "1.0.0-rc.1",
  "offshoot_tier": "local|rustfs",
  "baseline": {
    "l1_p95_deploy_sec": 600,
    "l4_p99_ms": 500,
    "rss_t0_mb": 0
  },
  "thresholds": {
    "STRESS_P95_DEPLOY_SEC": 600,
    "STRESS_GATEWAY_RPS": 500
  }
}
```

## P5-T3 — 混沌（L 基线后）

| 脚本 | TP |
|------|-----|
| `chaos-celld-kill.sh` | C1 |
| `chaos-rustfs-pause.sh` | C2 |
| `chaos-cellpd-restart.sh` | C3 |
| `chaos-offshoot-fail.sh` | C4 |
| `chaos-sqlite-contention.sh` | C5 |

**安全：** 仅 `stress-*` project prefix

## 报告

`docs/evidence/stress-report.md` 必含 TP2-MET-1 字段；注明 **offshoot tier**（AD-4）

## Subagent prompt

```
Phase 5. Gate: M2 complete. stress/ scripts only. Record metrics to docs/evidence/.
24h soak may run outside subagent session (nightly).
```
