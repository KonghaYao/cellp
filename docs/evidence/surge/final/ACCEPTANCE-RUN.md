# SURGE — Final acceptance run

| Field | Value |
|-------|--------|
| **Date** | 2026-09-05 |
| **Host** | macOS (local dev stack) |
| **Repo** | `/Users/mino/code/remote/cellp` |
| **Overall** | **FAIL** |

## Summary

| Gate | Result | Notes |
|------|--------|--------|
| `cd cellp && go test ./...` | **PASS** | All packages ok (orch ~2.5s uncached on first run; remainder cached) |
| `cd web && npm run test -- --run` | **PASS** | First run: 10 files, 31 tests (vitest 3.2.7, ~1.8s) |
| `./dev/scripts/health.sh` | **PASS** | Before e2e: gateway, platform, celld, rustfs, S3 skew, operator API |
| `./e2e/scripts/run-all.sh` | **FAIL** | Stopped at `v6-migrate-order.sh` (exit 1); log: `final/run-all.log` |

## Commands

### Go

```bash
cd cellp && go test ./...
```

Exit 0.

### Web (requested)

```bash
cd web && npm run test -- --run
```

Exit 0 — all 31 tests passed at session start.

### Dev health

```bash
./dev/scripts/health.sh
```

Exit 0 before e2e. Stack briefly unhealthy after a long e2e attempt; `./dev/scripts/up.sh` + 15s wait restored health for the recorded e2e run.

### E2E

```bash
./e2e/scripts/run-all.sh
```

Not skipped (health passed). `RUN_GATES=0` (Phase 0 storage gates skipped).

**Failure:** `v6-migrate-order.sh` — version ended `failed` instead of `ready`:

```text
deploy: read .../dev/examples/counter/node_modules/esbuild: is a directory
```

Scripts through `v5b-deploy-d1-branch-fail.sh` reported PASS in this run. See `run-all.log` for full transcript (WARN: cutover > 2s on v4).

## Follow-up (not part of gate table)

Re-running `web` tests later hit 1 failure in `src/flows/cellp-api.flow.test.ts` (`promoteVersion` URL assertion; vitest 4.1.11). Not re-run as part of the ordered acceptance sequence above.

## Artifacts

| Path | Description |
|------|-------------|
| `docs/evidence/surge/final/ACCEPTANCE-RUN.md` | This report |
| `docs/evidence/surge/final/run-all.log` | E2e stdout (failed run) |
