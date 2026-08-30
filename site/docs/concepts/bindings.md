# Bindings

Bindings are how a Worker talks to storage and side systems. cellp does not invent a new binding format: it reads **wrangler** keys from the deploy bundle and asks **celld** to honor them.

## What is declared vs what is branched

Declared in `wrangler.jsonc` (or `wrangler.json`) **inside the artifact**:

- `d1_databases`
- `kv_namespaces`
- `r2_buckets`
- `queues`
- `workflows`
- `triggers.crons`

When you create a **child** version (`parent_version_id` set):

| Binding | Branched from parent? |
|---------|----------------------|
| D1 | Yes |
| KV | Yes |
| R2 | Yes (overlay + tombstones) |
| Queue | Yes |
| Workflow **instances** | **No** — empty |
| Cron | **No** — follows **this** script |
| Worker script | **No** — this artifact |

Binding **identities** (`database_id`, KV namespace id, queue name, R2 bucket name) are inherited from the parent wrangler so the Worker code keeps working.

## Operator surfaces

| Binding | Dashboard | API |
|---------|-----------|-----|
| D1 | Table browser + SQL | `/database`, `/database/query`, … |
| KV | Key browser | `/kv`, `/kv/{ns}/keys`, … |
| Queues | Peek / pause / redrive / purge | `/queues/...` |
| Workflows | Instance list | `/workflows/...` (read-only) |
| R2 | Badge on the list | Listed on `GET …/bindings` |
| Cron | Display | Listed on `GET …/bindings` |

```bash
curl -sS -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$CELLP_URL/v1/projects/my-shop/versions/v1/bindings"
```

## What cellp does not add

- A second “inherit bindings” button — branch happens during Start
- An R2 file manager
- Workflow pause/resume/restart
- Workers AI, Vectorize, Hyperdrive, Email Workers, Python Workers

Compatibility matrix: [Supported stacks](/migrate/stacks). Deep runtime gaps: [celld cloudflare-compat](https://github.com/KonghaYao/cellp/blob/main/celld/docs/cloudflare-compat.md).

Per-binding guides: [D1](/bindings/d1) · [KV](/bindings/kv) · [R2](/bindings/r2) · [Queues](/bindings/queues) · [Workflows](/bindings/workflows) · [Cron](/bindings/cron).
