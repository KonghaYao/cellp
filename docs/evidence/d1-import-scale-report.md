# D1 import scale report

> **Run ID:** 20260829-092342  
> **Date:** 2026-08-29T01:24:12Z  
> **Result:** PASS

## Environment

| Item | Value |
|------|-------|
| celld | /Users/mino/.local/bin/celld |
| fleet | :8792 |
| project | `/Users/mino/code/remote/tmp-3ab3a94c22413688/dev/data/d1-import-scale/20260829-092342/d1-project` |
| database | `guestbook` |
| seed | 100 MB |
| bucket | `s3://cellp-celld/d1-imp-20260829-092342` (isolated; not shared `demo-app`) |
| local watch | wiped under `dev/data/d1-import-scale/20260829-092342/celld-watch` before successor start |

## Notes

- G3 fixture /Users/mino/code/remote/tmp-3ab3a94c22413688/dev/data/d1-import-scale/20260829-092342/d1-project database=guestbook
- deploy OK (289 ms)
- seed OK (105005056 bytes, 184 ms, page_size=4096)
- sqlite3 .dump baseline produced 209718708 bytes SQL (not used for import)
- import OK in 5911 ms
- no .dump.sql after import
- blob row count 100 matches seed
- execute SELECT 1 OK (1362 ms)
- G3 restore OK blob_rows=100 (18947 ms)
- G3 restore_plan (empty watch → LTX download): `cell="__D1Database:32a0422df013cf0b3afdf06cf99af8faeaab49e25157c060fc36d78b8a8b37f6" epoch=1 to=2 objects=1 bytes=105750979 levels=L9:1 download_us=384307 apply_us=1692874`

## Metrics

JSONL: `docs/evidence/d1-import-metrics.jsonl` (this run appended).

## Follow-up (same day)

- **G2 e2e** `e2e/scripts/v1-d1-seed.sh`: version `v-e2e-1787967057-17612` — D1 `SELECT count(*)` **42** and Worker `GET /count` **42**.
- **G4** cellp `D1Execute` uses `celld d1 import` for SQLite seeds; dump path absent (`go test ./internal/runtime ./internal/orch`).
- **Checkout:** `docs/evidence/checkout-skip-report.md`.

