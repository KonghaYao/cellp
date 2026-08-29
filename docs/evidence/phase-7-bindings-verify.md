# Phase 7 Bindings — VERIFY

**Date:** 2026-08-29  
**Workspace:** `tmp-3ab3a94c22413688`  
**Scope:** DESIGN.md §8 · AD-6 · AD-7 · `docs/plans/phase-7-bindings.md` T1–T5  
**Fixes applied:** none (no merge-break, no one-line regression).

---

## Commands (pass/fail)

| # | Command | Result |
|---|---------|--------|
| 1 | `cd cellp && go test ./...` | **PASS** — `internal/api` `ok` 18.8s; `internal/runtime` `ok` 28.4s; other packages ok / no tests |
| 2 | `cd web && npm run build` | **PASS** — vite production build, exit 0 (~921ms) |
| 3 | `cd web && npm run test:e2e` | **PASS** — 25 passed (1.1m), Chromium |
| 4 | `rg ':8792\|offshoot' web/src/` | **PASS with documented exception** — no `:8792`; only `offshoot_export` (D1 `branch_method` union in `cellp-api.ts`, not a runtime host) |
| 5 | `rg '/__celld/health' dev e2e` | **PASS** — empty |
| 6 | `server.go` bindings + kv + queues + workflows | **PASS** — see spot-check |
| 7 | purge-without-force 400; no WF pause/resume buttons; no `/r2` route | **PASS** — see spot-check |
| 8 | live stack `health.sh` / `:8790` then `v9-kv.sh` | **SKIP** — stack down |

### 4 — grep (verbatim)

```
web/src/lib/cellp-api.ts:export type DatabaseBranchMethod = "d1_branch" | "offshoot_export";
```

No `:8792` in `web/src/`. Same D1 enum also appears in `web/e2e/mock-api-server.mjs` (`branch_method: "offshoot_export"` for fixture v3). That is the Phase 6 D1 API field, not celld/S3/offshoot CLI.

### 5 — grep (verbatim)

No matches under `dev/` or `e2e/` for `/__celld/health`. Scripts use `/.well-known/celld/health`.

---

## Spot-checks

### server.go routes (T1–T3)

`cellp/internal/api/server.go` under `/v1/projects/{projectID}/versions/{versionID}`:

- `GET /bindings`
- `GET /workflows`, `GET /workflows/{name}/instances`
- `GET /kv`, `GET /kv/{ns}`, `GET|PUT|DELETE /kv/{ns}/keys/{key:*}`
- `GET /queues`, `GET /queues/{name}`, `GET …/peek`, `POST …/pause|resume|redrive|purge`

CORS `Allow-Methods` includes **PUT**. OpenAPI lists the same KV / Queue / Workflow paths. No `/r2` object API, no `/crons` operator, no KV bulk, no workflow pause/resume/restart.

### Purge without force → 400

`TestPurgeRequiresForce` in `cellp/internal/api/queue_test.go`: bodies `""`, `{}`, `{"force":false}` → **400** `force_required` and **must not** exec celld. `{"force":true}` → 200. Covered by `go test ./...`.

### WorkflowsPage — no control buttons

`web/src/pages/WorkflowsPage.tsx` has no Pause/Resume/Restart. `workflow-panel.tsx` states instances are read-only (copy only). Playwright asserts those buttons have count 0.

### App.tsx — no R2 browser route

Routes: Storage hub, D1 `…/browser`, `…/kv`, `…/queues`, `…/workflows`, legacy `/bindings` → hub. **No** `storage/:vid/r2` or `…/cron`. Hub R2/Cron badges have **no `href`** (tooltip only).

---

## What passed (in-repo)

**T1 API:** six-array bindings JSON (`d1` / `kv` / `queues` / `workflows` / `r2` / `crons`) in `runtime.Bindings`. Ready-version 404s covered by Go tests.

**T2 KV/Queue:** operator wrap + HTTP; undeclared ns/queue not exec’d (tests); no bulk routes in Go/OpenAPI.

**T3 Workflow/Cron/health:** list + instances; Go tests reject workflow control / `/crons` / R2 object paths as non-2xx. Dev/e2e health path is well-known.

**T4 Dashboard (code + mock Playwright):** Storage hub UI with six badges; KvPage / QueuesPage / WorkflowsPage; AD-7 banner; sidebar still Overview · Deployments · Storage. `npm run test:e2e` green:

| Spec label (as written) | What it actually covers vs test-plan |
|-------------------------|--------------------------------------|
| `dashboard.spec.ts` TP-UI-1..5 + D1 | still green (incl. legacy database redirect) |
| `kv.spec.ts` “TP-UI-7” | **TP-UI-8** KV browser + **TP-UI-11** AD-7 empty KV |
| `queues-workflows.spec.ts` “TP-UI-8” | **TP-UI-9** queue console (no silent purge) |
| `queues-workflows.spec.ts` “TP-UI-9” | **TP-UI-10** workflow read-only |

**T5 scripts exist:** `e2e/scripts/v9-kv.sh`, `v10-queue.sh`, `v11-workflow-cron.sh` in MANIFEST; `dev/examples/queue/` producer-only; health scripts on well-known. **Not executed against a live fleet** (see SKIP).

---

## SKIPPED (live e2e stack)

`:8790` and `:8792` are not listening.

```
curl :8790/v1/health     → connection refused
curl :8792/.well-known/celld/health → connection refused
./dev/scripts/health.sh  → FAIL gateway :8787, FAIL platform :8790, FAIL celld :8792
                           OK rustfs :19000, OK registry file
                           exit 1
```

Per instructions, **`v9-kv.sh` was not run**. `require_stack_or_skip` would print SKIP and exit 0.

Therefore **TP-V9 / TP-V10 / TP-V11 remain unchecked** against real cellpd + celld. No `docs/evidence/v9-kv-e2e.log` (or v10/v11) from this verify. `docs/test-plan.md` still has `[ ]` on TP-V9–V11.

---

## Remaining gaps vs DESIGN §8

### Playwright vs TP-UI-7..12

| ID | DESIGN / T4 intent | This verify |
|----|--------------------|-------------|
| **TP-UI-7** Storage hub badges | Playwright: `/storage` shows d1/kv/queue/workflow/r2/cron | **UI exists** (`StoragePage` + mock fixture has all six, v1 has cron). **No Playwright** hits `/projects/demo-app/storage` for badges. `bindings.spec.ts` was never added. |
| **TP-UI-8** KV | Playwright | **Covered** (mislabelled TP-UI-7 in `kv.spec.ts`) |
| **TP-UI-9** Queue | Playwright | **Covered** (mislabelled TP-UI-8) |
| **TP-UI-10** Workflow read-only | Playwright | **Covered** (mislabelled TP-UI-9) |
| **TP-UI-11** AD-7 banner | Playwright | **Covered** on v2 KV and v2 Queue |
| **TP-UI-12** R2/Cron no browser | Hub badges not links; goto `/r2` `/cron` → 404 or hub | **Hub: badges have no href** (code). **No Playwright** for hub or for navigating `/storage/:vid/r2`. Unmatched nested routes likely blank Outlet, not an explicit 404/redirect. |

`docs/test-plan.md` TP-UI-7..12 are still `[ ]` (“未勾直到 FE verify”). Specs also use the wrong TP-UI numbers.

### Bulk KV

**Not done (by design, §8.4 / T2 “第二轮”).** No `celld kv bulk` wrap, no API path, no Dashboard bulk UI.

### Live isolation (V9–V11)

Sibling-version KV empty-start, queue purge-400 on real celld, workflow instances not-500 on a real fleet: **not proven here**. Unit tests + mock Playwright only.

---

## Honest “not done” (DESIGN §8.7 / AD-7 — expected)

These are **out of Phase 7 scope**, not regressions:

1. **R2 object browser** — no `celld r2`; no `/r2` route; hub tooltip only.
2. **Workflow control** — no pause / resume / restart / delete API or buttons (`celld` has no `workflow` CLI).
3. **Inherit / branch for KV · R2 · Queue · Workflow** — empty-start only; no copy, no parent bucket. D1 branch unchanged. UI copy states preview does not inherit Production.

Also still deferred (DESIGN matrix): Queue pull/HTTP API; Cron platform scheduler / run-once; DO browser.

---

## Verdict

In-repo Phase 7 **API + Dashboard + mock e2e + Go tests are green**. Invariants 4–7 hold. Live cellpd/celld **was down**, so fleet e2e (**TP-V9–V11**) is **SKIP**. FE gaps vs T4: **no Storage-hub Playwright (TP-UI-7)** and **no TP-UI-12 goto-`/r2` assertion**; spec IDs are off-by-one vs `docs/test-plan.md`. Bulk KV, R2 browser, workflow control, and non-D1 inherit are **not done** and should stay that way until a new ADR / later round.
