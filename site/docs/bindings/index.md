# Binding guides

These pages describe **Workers bindings** as you declare them in wrangler and use them in code. cellp does not replace wrangler keys—it runs **celld** per [version](/concepts/versions) and applies **version-level** data and scheduling policy on top.

## Runtime vs control plane

| Layer | What it does |
|-------|----------------|
| **celld** | Parses wrangler from the artifact, deploys the Worker, implements Cloudflare-style `env.*` APIs (within [celld compatibility](https://github.com/KonghaYao/cellp/blob/main/celld/docs/cloudflare-compat.md)). |
| **cellp** | One celld process + one object bucket per ready version; **branches** D1/KV/R2/Queue on child versions; **operator** HTTP API and Dashboard for D1/KV/Queues/Workflows (read-only); **cron arming** so only production (or pre-prod projects) schedules ticks; Gateway routing and promote. |

Judge API behavior against **celld**. Judge preview isolation, promote, and ops surfaces against **cellp**.

## Branching and scheduling (cellp)

When you deploy a **child** version (`parent_version_id` set):

| Binding / surface | Branched from parent? | Notes |
|-------------------|----------------------|--------|
| D1 | Yes | Copy-on-write via celld `d1 branch` |
| KV | Yes | Namespace branch; identities inherited |
| R2 | Yes | Overlay + tombstones |
| Queue | Yes | Preview messages stay on the child |
| Workflow **instances** | No | Empty in preview—no half-finished prod jobs |
| Cron **expressions** | N/A | Still listed from wrangler; **ticks** only on [production scheduling policy](/bindings/cron) |
| Worker script | No | This artifact only |
| Durable Objects | No | Per-version celld storage—not a D1-style branch |
| Images | No | Local transform binding only |

Root versions (no parent) start with **empty** KV/R2/Queue unless the Worker fills them; **D1** may use `seed.db` import or SQL. See [Platform data](/build/data) and [Data fork](/concepts/data-fork).

## Operator surfaces (cellp)

| Binding | Dashboard | REST (admin token) |
|---------|-----------|-------------------|
| D1 | Tables + SQL | `…/database/*` |
| KV | Key browser | `…/kv/*` |
| Queues | Peek, pause, resume, redrive, purge | `…/queues/*` |
| Workflows | Instance list | `…/workflows/*` (read-only) |
| R2 | Badge on Storage | Listed on `GET …/bindings` only |
| Cron | Expressions | Listed on `GET …/bindings` |
| Durable Objects | — | Not listed on bindings API |
| Images | — | Wrangler + Worker only |

Declared bindings (including cron strings stripped at deploy for non-prod) are visible on:

```bash
curl -sS -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$CELLP_URL/v1/projects/{project}/versions/{version}/bindings"
```

Conceptual overview: [Bindings](/concepts/bindings).

## Per-binding guides

| Guide | Topic |
|-------|--------|
| [D1](./d1) | SQLite, import vs branch, SQL operator |
| [KV](./kv) | Key/value, namespace branch |
| [R2](./r2) | Objects on your RustFS bucket |
| [Queues](./queues) | Producer HTTP Worker + separate consumer Worker |
| [Workflows](./workflows) | Classes, instances, read-only ops |
| [Cron](./cron) | `scheduled`, production-only arming |
| [Durable Objects](./durable-objects) | Partial celld support, no cellp branch |
| [Images](./images) | Local `env.IMAGES` transforms |

Configure wrangler: [Configure bindings](/build/wrangler). Platform comparison: [Compare](/compare) · [From Cloudflare](/migrate/cloudflare).
