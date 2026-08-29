# Offshoot large-branch scale report

> **Run ID:** 20260829-074717  
> **Date:** 2026-08-28T23:47:28Z  
> **Suite:** all · incompressible 1MB blobs × 100  
> **Result:** PASS

## Environment

| Item | Value |
|------|-------|
| offshoot | `/Users/mino/go/bin/offshoot` |
| store | `dev/data/offshoot-scale-store` (local, isolated) |
| db | `obscale-20260829-074717` |
| seed | 100 MB (`os.urandom`, 105,005,056 bytes) |
| fan-out | 50 |
| chain | 20 |
| concurrent | 4 |
| checkouts | 8 |

## Results

| ID | What | Result |
|----|------|--------|
| TP-OB-1 | Seed + `create --from` | 150 ms seed · **366 ms** import |
| TP-V0b-L | 100MB fork + export | fork **44 ms** · export **323 ms** · 105,005,056 bytes |
| TP-OB-2 | Fan-out 50 from main | 50/50 · p50 **40 ms** · p99 **42 ms** |
| TP-OB-4 | CoW amplification | **8,192 bytes/fork** (limit 10.5 MB) · store/logical **~1%** |
| TP-OB-5 | Concurrent fork ×4 | 4/4 in **62 ms** wall |
| TP-OB-3 | Chain fork ×20 | 20/20 · first 39 ms · last 41 ms (no degradation) |
| TP-OB-6a | Checkout 8 branches | p50 **359 ms** · p99 **377 ms** · **+840 MB** disk |
| TP-OB-6 | sqlite3 `.dump` 8MB sample | 30 ms · 16.8 MB SQL (~2× hex) |
| TP-OB-6 D1 | `celld d1 execute` | skipped (needs running fleet) |
| TP-OB-7 | destroy + `gc --grace 0s` ×2 | 1.7 s · store 946 MB → **106 MB** · 0 shared left |
| TP-OB-P | cellpd live | API up · did **not** deploy 100MB into celld (ready cap 5–20) |

JSONL: `docs/evidence/offshoot-branch-metrics.jsonl`

## Findings

1. **Fork is cheap.** 50 incompressible 100MB branches add ~8 KB each in the store. Concurrent and chained forks stay ~40 ms. This is not the bottleneck.
2. **Checkout is the bottleneck.** Materializing 8 working copies added **840 MB** and ~360 ms p50 each. Extreme “多开” of *usable* sqlite files is full-size disk and hundreds of ms, not CoW metadata.
3. **Export is the second cost.** 323 ms to copy 100 MB out — this is what orchestrator does before D1.
4. **D1 path is SQL, not sqlite.** celld `--file` needs a dump. 8 MB db → 16.8 MB SQL; a 100 MB blob dump would be ~200 MB of hex and is capped by `CELLP_D1_DUMP_MAX_MB` (default 32) in cellpd.
5. **GC works** if you run `gc --grace 0s` twice (tombstone then delete). Store returns to ~one materialized main (~106 MB).
6. **Live celld is a different ceiling.** Default max 5 ready versions; 8 checkouts already used 840 MB without starting workers.
