# D1 branch scale report

> **Run ID:** 20260829-130656  
> **Date:** 2026-08-29T05:07:11Z  
> **Result:** PASS

## Environment

| Item | Value |
|------|-------|
| celld | /Users/mino/.local/bin/celld |
| fleet | :8792 |
| seed | 8 MB |
| project | `d1-br-20260829-130656` |
| parent bucket | `s3://cellp-celld/d1-br-20260829-130656/parent` |
| child bucket | `s3://cellp-celld/d1-br-20260829-130656/child` |

## Notes

- parent import OK (661 ms)
- parent prefix bytes=8463032
- branch OK (446 ms)
- B6 child prefix 2589 <= 20% seed (1677721)
- B2 no min_txid==1 LTX on child prefix
- B2 child prefix has base.json
- B5 restore OK blob_rows=8
- B2 still no min_txid==1 LTX after B5
- B6 child prefix after B5 3412 <= 20% seed (1677721)

## Metrics

JSONL: `docs/evidence/d1-branch-metrics.jsonl`
