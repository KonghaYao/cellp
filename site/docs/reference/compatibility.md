# Cloudflare compatibility

cellp runs Workers on **celld** (Rust runtime in the repo submodule). This table is the operator summary; celld’s authoritative gap list is [`celld/docs/cloudflare-compat.md`](https://github.com/KonghaYao/cellp/blob/main/celld/docs/cloudflare-compat.md).

**Legend:** **Yes** — applicable API implemented. **Partial** — some operations or configs missing. **No** — not implemented.

## Platform services

| Service | Status | cellp notes |
|---------|--------|-------------|
| Workers (`fetch`, runtime APIs) | Partial | See celld runtime table |
| Static assets (wrangler `assets`) | Yes | SPA / Workers Sites-style bundles |
| D1 | Partial | Root **import** on first version; preview **branch** on child versions |
| KV · R2 · Queues | Partial | **Branch** from parent on child versions |
| Workflows · Cron | Partial | **Not branched** across versions; cron schedules on **prod** (first ready before prod is set may arm) |
| Durable Objects | Partial | Validate before load-bearing DO — see [Durable Objects](/bindings/durable-objects) |
| Worker Loader | Partial | Opt-in via celld env; see compat doc |
| Images binding | Partial | See compat doc |
| Workers AI · Vectorize · Hyperdrive | No | |
| Browser Rendering · Email Workers · Python Workers | No | |

## cellp product vs Cloudflare

| Cloudflare expectation | cellp |
|------------------------|--------|
| `wrangler deploy` to CF edge | **No** — upload artifact to **your** RustFS, `POST /versions` |
| Multi-worker `[[services]]` in one deploy | **No** — one Worker bundle per version |
| Global edge PoPs | **No** — your machines |
| Account / API token per user | **No** — two shared tokens ([Auth](/reference/auth)) |
| `wrangler tail` | **No** — metrics + logs today; OTLP query/tail **planned** ([Observability](/guides/observability)) |
| R2 object browser in Dashboard | **No** — API/Worker only |
| Workflow pause/resume/restart in UI | **No** — list/read via API |

## Frameworks and migration

Validated stacks and tiers: [Supported stacks](/migrate/stacks) · [Framework tiers](/migrate/frameworks) · [From Cloudflare](/migrate/cloudflare).

## Bindings on the site

Per-binding operator docs: [D1](/bindings/d1) · [KV](/bindings/kv) · [R2](/bindings/r2) · [Queues](/bindings/queues) · [Workflows](/bindings/workflows) · [Cron](/bindings/cron) · [Durable Objects](/bindings/durable-objects).
