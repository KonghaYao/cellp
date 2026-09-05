# Supported stacks

Runtime: **celld** (Cloudflare Workers semantics on your hardware). cellp documents **tier 1** stacks as first-class; other CF framework guides may work when you ship a **single Worker + assets** bundle built in CI.

**Status legend:** **Works** — validated on cellp · **Experimental** — lab fixtures only, not a hosting promise · **Not a goal** — use another platform

## Tier 1 (first-class)

Aligned with [Cloudflare Workers framework guides](https://developers.cloudflare.com/workers/framework-guides/). cellp documents, recommends, and runs a dedicated validation track for these when they deploy as **one Worker per version**:

| Framework | Typical artifact | Notes |
|-----------|------------------|--------|
| **React + Vite SPA** | `dist` + thin Worker | **Default recommendation** for new apps |
| **Vue + Vite SPA** | same | same pattern |
| **Astro** | `@astrojs/cloudflare` single Worker | static or adapter single deploy |
| **SvelteKit** | `adapter-cloudflare` **single** Worker | not multi-`services` consoles |
| **Remix** | `@remix-run/cloudflare` bundle | single bundle per version |
| **Nuxt** | Nitro `cloudflare` preset | single Worker |

**Deploy pattern:** build in CI → upload artifact → `POST /versions`. Prefer **pre-built** worker + `assets`; avoid relying on celld to re-bundle Tailwind, `.md`, or full SSR graphs inside the Worker.

Details: [Framework tiers (detail)](/migrate/frameworks).

## Also works (not tier 1)

Cloudflare documents additional stacks. cellp does **not** treat these as product defaults, but community validation has passed for single-Worker artifacts:

| Framework | Status | Notes |
|-----------|:------:|-------|
| **Hono** | Works | API or API + static assets |
| **SolidStart** | Works | single Worker + `nodejs_compat` where needed |
| **Qwik City** | Works | Workers template, pre-built bundle |
| **Waku** | Works | pre-build + slim wrangler layout |

If a template uses **`[[services]]`** (multiple Workers), treat it as **unsupported** until cellp gains multi-worker orchestration.

## Next.js

| Dimension | Status |
|-----------|--------|
| OpenNext / vinext **Worker** bundles (not Node `next start`) | Experimental |
| App Router on **Node** (Vercel-style SSR) | Not a goal |

**Experimental** — lab fixture checks only for minimal OpenNext and pinned App Router samples; that does **not** mean arbitrary Next versions deploy unchanged. See [Framework tiers](/migrate/frameworks) and [From Vercel](/migrate/vercel).

## Recommended default

**Vite SPA (React or Vue) + one Worker API** with wrangler static `assets` — same mental model as Cloudflare **Workers + Static Assets**.

## Supported runtime kinds (summary)

| Kind | Notes |
|------|--------|
| Cloudflare-style Workers | `export default { fetch }` (+ scheduled/queue as celld allows) |
| `wrangler.json` / `wrangler.jsonc` | Parsed from the **bundle** at deploy |
| D1, KV, R2, Queue, Workflows, Cron | See [bindings](/concepts/bindings) |
| Static assets | wrangler `assets` within celld support |
| Durable Objects | **Partial** in celld — verify before migration |

## Not supported / not a goal

| Kind | What to do |
|------|------------|
| **Multiple Workers** (`[[services]]` stacks) | Single main Worker per version today |
| Worker-in-Worker SSR graphs without prebuild | Pre-bundle + `no_bundle` or use tier-1 adapters |
| Pages Functions that are not Workers | Rewrite as a Worker |
| Workers AI, Vectorize, Hyperdrive | Not in celld — use D1 or call an external DB/API from the Worker |
| Arbitrary npm needing Node built-ins | Workers-compatible packages + `nodejs_compat` where supported |
| Python Workers | No |

## Compatibility

celld marks many Cloudflare APIs **Partial**. Read [celld cloudflare-compat](https://github.com/KonghaYao/cellp/blob/main/celld/docs/cloudflare-compat.md) and the [binding guides](/bindings/).

## Languages

JavaScript / TypeScript compiled to a Worker bundle. Bring your own bundler as long as the artifact matches what `celld deploy` expects.
