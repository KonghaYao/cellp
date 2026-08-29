# Offshoot large-branch scale report

> **Run ID:** 20260829-075332  
> **Date:** 2026-08-28T23:53:46Z  
> **Suite:** all  
> **Tier:** rustfs  
> **Result:** PASS

## Environment

| Item | Value |
|------|-------|
| offshoot | /Users/mino/go/bin/offshoot |
| store | `s3://cellp-offshoot/ob-scale/20260829-075332` |
| db | `obscale-20260829-075332` |
| size | 100 MB seed |
| fan-out | 50 |
| chain | 20 |
| concurrent | 4 |
| checkouts | 8 |

## Notes

- TP-OB-1 seed OK (105005056 bytes, 181 ms)
- TP-V0b-L fork+export OK (fork 46 ms, export 416 ms)
- TP-OB-2 fan-out 50/50 p50=48ms p99=97ms
- TP-OB-4 CoW OK (RustFS): 50 storage=shared branches after fan-out 50
- TP-OB-5 concurrent 4/4 in 44 ms
- TP-OB-3 chain 20/20 first=44ms last=49ms
- checkout 8/8 p50=509ms du_added=840073216 bytes
- TP-OB-6 sqlite3 .dump 34 ms → 16777596 bytes SQL from 8409088 byte sample. celld d1 --file needs SQL; a 100MB incompressible dump is ~2× hex and is not executed against a live fleet in this harness.
- TP-OB-6 D1 execute skipped (needs running celld fleet); dump cost recorded
- TP-OB-7 destroy+gc OK in 2330 ms, store 0 bytes, no shared leftovers
- cellpd API up; live ready cap remains CELLP_MAX_READY_VERSIONS (default 5, env 20). Not deploying 100MB into celld fleet.

## Metrics

JSONL: `docs/evidence/offshoot-branch-metrics.jsonl` (this run appended).
