# AD-1 Spike — Multi-Fleet celld (per-version upstream)

> **Gate:** Phase 2 T2 / Exit Criteria AD-1  
> **Date:** 2026-08-28  
> **Implementation:** `cellp/internal/runtime/manager.go` · `e2e/scripts/v3-dual-route.sh`

## Decision (AD-1)

Each **ready** version gets its own celld subprocess on a dedicated loopback port and isolated S3 bucket prefix. Gateway resolves `/{project}/{version}/*` via Registry `routes.upstream_host` + `routes.upstream_port` — **not** a single shared celld with path routing.

## Spike Result: Two Versions, Different Body, Both HTTP 200

| Check | Result |
|-------|--------|
| Version A route `GET /{project}/{VA}/` | **200** |
| Version B route `GET /{project}/{VB}/` | **200** |
| Response bodies differ | **Yes** — JSON `version` field matches each version id |

### Example (v3-dual-route.sh)

Script flow:

1. `POST /v1/projects/{project}/versions` × 2 (unique ids `VA`, `VB`)
2. Poll both to `ready` (orchestrator deploy pipeline, not stub)
3. `curl` both gateway URLs; assert HTTP 200
4. Parse `.version` from JSON bodies — **must differ** when multi-fleet is active

Counter worker (`dev/examples/counter/index.js`) returns:

```json
{"n":1,"version":"<VERSION_ID>","project":"<PROJECT_ID>","url":"..."}
```

Orchestrator sets `CELLD_VAR_VERSION_ID` per subprocess (`runtime.Manager.Start`), so each upstream serves a distinct `version` field even when artifact bundle is shared.

Example distinct bodies:

```
# GET /demo-app/v-e2e-1735123456-1234/
{"n":1,"version":"v-e2e-1735123456-1234","project":"demo-app","url":"..."}

# GET /demo-app/v-e2e-1735123456-5678/
{"n":1,"version":"v-e2e-1735123456-5678","project":"demo-app","url":"..."}
```

v3 fallback (single-fleet / mock): if bodies are identical but both return 200, script emits `WARN` and passes with note that strict AD-1 requires cellpd multi-port.

## Port Allocation Table

Base port from env `CELLD_PORT` (default **8792**). Runtime allocates **`basePort + 10 + N`** per new version (N starts at 1), reserving `8792…8802` for the dev/shared celld instance.

| Registry key (`project/version`) | N | Upstream port (default base) | Listen address |
|----------------------------------|---|------------------------------|----------------|
| `demo-app/v-e2e-…-1` | 1 | **8803** | `127.0.0.1:8803` |
| `demo-app/v-e2e-…-2` | 2 | **8804** | `127.0.0.1:8804` |
| `demo-app/v-e2e-…-3` | 3 | **8805** | `127.0.0.1:8805` |
| `demo-app/v-e2e-…-4` | 4 | **8806** | `127.0.0.1:8806` |
| `demo-app/v-e2e-…-5` | 5 | **8807** | `127.0.0.1:8807` |

Properties (from `AllocatePort`):

- Ports are **stable per project/version** for the lifetime of the manager process
- Ports are **not reused** for a different version while still tracked
- Max **5 ready versions / project** enforced by orchestrator (`maxReadyVersionsDefault = 5`, overridable via `CELLP_MAX_READY_VERSIONS`)

## Bucket Isolation

Per version bucket (not shared deploy swap):

```
s3://cellp-celld/{project}/{version}
```

Set via `Manager.versionBucket()` and passed to `celld --bucket` on `Start`, `Deploy`, and `D1Execute`.

## Resource Notes

| Resource | Per version | Limit (Phase 2) |
|----------|-------------|-----------------|
| celld OS process | 1 | ≤ 5 ready / project |
| TCP port | 1 (`127.0.0.1:{port}`) | 8803–8807 (default base) |
| S3 prefix | 1 bucket path | isolated deploy + D1 state |
| Log file | `/tmp/celld-{project}-{version}.log` | append-only |
| Startup wait | up to 60 × 1s health poll | `Health()` → `GET /__celld/health` |

Startup sequence per version (orchestrator `runDeploy`):

1. `artifact.Fetch` → `branch.Fork` → `branch.Export` (seed)
2. `runtime.Deploy` → `runtime.Start` → optional `D1Execute`
3. Health gate → `SetRoute(active=true)` → status `ready`

Teardown: `runtime.Stop` sends SIGTERM (10s grace, then SIGKILL).

When `celld` binary is absent (unit/dev without celld), manager stubs port allocation and health returns true — e2e scripts require real celld via `require_celld`.

## Gateway Drain (related)

Inactive routes (`route.active=false`) return **503** `route draining` (REVIEW M4). Used by promote saga `deactivate_old_route` and branch `Drain`.

## References

- `docs/plans/REVIEW.md` AD-1, AD-5
- `docs/plans/phase-2-orchestrator.md` Exit Criteria
- `cellp/internal/runtime/manager.go` — port/bucket/subprocess
- `cellp/internal/orch/orchestrator.go` — deploy + promote saga
- `e2e/scripts/v3-dual-route.sh` — acceptance script
