# D1 branch e2e evidence

> **Date:** 2026-08-29  
> **Result:** PASS  
> **Run:** parent `v-e2e-1787979996-24614` → child `v-e2e-1787979996-9593`

## B1 — Smoke CLI

| ID | Status | Command | Exit | Notes |
|----|--------|---------|------|-------|
| B1 | **PASS** | `export PATH="$HOME/.local/bin:$PATH" && celld d1 branch -h` | 0 | Help lists `celld d1 branch DATABASE --parent-bucket URI [PROJECT] --bucket NAME` without requiring DATABASE |

## B3/B4/B5 — `e2e/scripts/v1-d1-branch.sh`

```bash
export PATH="$HOME/.local/bin:$HOME/go/bin:$PATH"
export CELLD_ESBUILD="$PWD/dev/examples/d1-seed/node_modules/.bin/esbuild"
bash e2e/scripts/v1-d1-branch.sh
```

```
==> D1 branch e2e project=demo-app parent=v-e2e-1787979996-24614 child=v-e2e-1787979996-9593
==> parent seed.db entries=42
==> parent worker count OK=42
==> child worker count OK=42
==> child INSERT OK count=43
==> parent isolation OK count=42
==> B5 kill child celld :8815 then wipe .../celld-watch/demo-app/v-e2e-1787979996-9593
==> B5 wipe-watch restore OK count=43
PASS: D1 branch parent=42 child=42 isolation OK
```

| ID | Status | Notes |
|----|--------|-------|
| B3 | **PASS** | Child Worker `GET /count` = 42 (same as parent seed) |
| B4 | **PASS** | Child INSERT → 43; parent Worker still 42 |
| B5 | **PASS** | Kill child celld, wipe watch, new process on same port/bucket; count still 43 |

## B7b — orchestrator path (this run)

From `dev/data/logs/cellpd.log`:

```
orch: d1 seed took 337.945541ms
orch: offshoot export skipped (d1 branch from parent v-e2e-1787979996-24614)
orch: d1 branch took 345.387333ms
```

Child deploy skipped offshoot export and used `d1 branch` (no `d1 seed took` on the child).

## Environment

| Item | Value |
|------|-------|
| celld | `~/.local/bin/celld` 0.4.0 (release build after handoff-skip) |
| cellpd | `CELLP_STRICT_OFFSHOOT_FORK=1` |
| offshoot | `$HOME/go/bin/offshoot` |
| RustFS | `http://127.0.0.1:19000` |
| Platform | `http://127.0.0.1:8790/v1/health` |

## Object store (independent list after B5)

Child `s3://cellp-celld/demo-app/v-e2e-1787979996-9593` cells prefix:

| Size | Key |
|------|-----|
| 1155 | `.../ltx/e1/0000/0000000000000003-0000000000000003.ltx` (child INSERT, MinTXID=3) |
| 249 | `.../ltx/e1/base.json` |
| 58 | `.../own.json` |

No MinTXID==1 full LTX on the child. Parent keeps `0001-0001` / `0002-0002` L0.

## Log paths

- `docs/evidence/d1-branch-offshoot.log`
- `dev/data/logs/cellpd.log`
- `docs/evidence/d1-branch-child-insert.log`
- `docs/evidence/d1-branch-b5-celld.log`
