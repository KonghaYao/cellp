# Limits

Intentional constraints so you do not plan on a Cloudflare-shaped roadmap that will not ship inside cellp.

## Product boundary

| You might want | cellp |
|----------------|--------|
| Signup, orgs, SSO, RBAC | **No** — two tokens |
| Git host, GitHub App, PR comments | **No** — CI calls HTTP |
| DNS, ACME, CDN, WAF, DDoS | **No** — your LB |
| Global edge PoPs | **No** — your region, your machines |
| Next.js / Node serverless | **No** |
| Hosted cellp cloud | **No** — you run the binary |

## Runtime (celld)

| You might want | Status |
|----------------|--------|
| Workers AI, Vectorize, Hyperdrive | **No** |
| Browser Rendering, Email Workers, Python Workers | **No** |
| Full Cloudflare KV/R2/Queue/DO/Workflows | **Partial** — see [Compatibility](/reference/compatibility) |
| `wrangler tail` | **No** |
| R2 object manager | **No** |
| Workflow pause/resume/restart | **No** |
| Wake-on-request for archived versions | **No** in v1 (explicit `wake`) |

## Scale (v1 honesty)

- **Ready versions** have no small hard cap; idle ones **archive** to drop processes.
- Registry is **SQLite**. Fine for a serious single-node (or VIP) control plane; not a multi-region consensus product.
- Deep health returns **503** when the deploy queue is past `CELLP_QUEUE_MAX`.
- `POST /versions` returns **503** `queue_full` when the queue is saturated.
- Very large multi-tenant registry stress beyond SQLite scope is **out of v1** honesty for this release.

## Elastic replicas (planned, default off)

Optional **0→N serving replicas** per version (scale-to-zero and surge) exist in code behind `CELLP_ELASTIC_RUNTIME`. **Default is disabled:** every `ready` version still maps to **one celld process** on your machines. `POST …/wake` only applies to **archived** versions, not a future “cold but not archived” elastic state. When elastic serving is productized, this page and [Archive & wake](/concepts/archive) will describe how archive and cold interact.

## Data plane

- Child versions should fork a **seed/staging** parent, not casually fork prod.
- Destroyed versions cannot be rolled back.
- Local celld watch dirs are cache. **S3/RustFS** is durable.

## What we *do* guarantee as the product

- Isolated runtime per ready version
- Preview and production URL shapes (Host ingress)
- D1/KV/R2/Queue branch on children
- Promote saga + re-promote rollback
- REST + Dashboard over the same API
- Self-host without AWS/Cloudflare accounts
