# SQLite-scope deploy-path: checkout / export

> **Date:** 2026-08-29T01:31:00Z  
> **Result:** measured; checkout cannot be skipped without changing offshoot

## What cellp does now

Orchestrator tries `offshoot checkpoint` on the parent branch first. Only if that fails does it `checkout` then `checkpoint`. Export still writes a full `seed.db` for `celld d1 import` (celld must not open the offshoot store).

## Evidence

| Probe | Result |
|-------|--------|
| `offshoot checkpoint db@main` with **empty** `checkouts/` | **fails:** `no checkout for … (run checkout first)` |
| `offshoot checkout` of 100 MB scale branch `obscale-20260829-074717@main` | **+102 548 KB** (`105 005 056` byte working copy) |
| e2e `v1-d1-seed` after seeding `demo-app@main` (checkout already present) | `orch: offshoot checkpoint without checkout` — checkouts **8 → 16 KB** |
| 100 MB D1 import (no SQL dump) | **5911 ms**; sqlite3 `.dump` baseline **209 718 708** bytes SQL |

## Bound

Skipping checkout would require offshoot to snapshot without a working copy. That CLI cannot do that today, and changing offshoot is out of scope. The remaining deploy-path cost for a cold 100 MB branch is still one checkout plus one export, not eight extra checkouts per D1 dump.

Default Worker bundle remains `dev/examples/counter`. D1Execute still skips when wrangler has zero `d1_databases`.
