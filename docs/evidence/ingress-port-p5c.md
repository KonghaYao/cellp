# Ingress Port P5c — evidence

**Date:** 2026-09-01  
**Plan:** [INGRESS-PORT-P5c-impl-plan.md](../plans/INGRESS-PORT-P5c-impl-plan.md)

## Implemented

- `gateway.ListenerManager`: boot + event-driven `ReconcileIngressListeners`, `127.0.0.1` bind only, orphan ledger `ReleasePort(orphan_reconcile)`, shutdown via `CloseAll`.
- `ingress_resolve.go`: dedicated listener port wins over Host (supports `prod_port` with global `CELLP_INGRESS_TIER_B=host`).
- `serve.Run`: boot reconcile before main gateway; `lm.CloseAll` on shutdown; orch wired via `SetIngressListenerReconciler`.
- `orch`: reconcile after preview/prod attach and after preview teardown; strict error on deploy path before gateway verify.

## Tests (pass)

```bash
cd cellp && go test ./internal/gateway/... ./internal/orch/... -count=1
```

```
ok  	github.com/cellp/cellp/internal/gateway	1.671s
ok  	github.com/cellp/cellp/internal/orch	3.060s
```

Gateway coverage: `listeners_test.go` (L1–L4), `ingress_listen_resolve_test.go` (L5 local port → prod binding).

## Deferred (per plan)

- `web/format.ts` dedicated URL handling
- `e2e/scripts/v1-ingress-port-preview.sh` (opt-in `INGRESS_PORT_E2E=1`)
