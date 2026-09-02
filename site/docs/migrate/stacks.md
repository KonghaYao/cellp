# Framework tiers on cellp

Runtime: **celld** (Workers semantics). **AD-13** defines which stacks are first-class on cellp.

## First-class (tier 1)

These align with [Cloudflare Workers framework guides](https://developers.cloudflare.com/workers/framework-guides/) and have a dedicated support validation track (S22–S25 where noted):

| Framework | Typical artifact | Notes |
|-----------|------------------|--------|
| **React + Vite SPA** | `dist` + thin Worker | **Default recommendation** for new apps |
| **Vue + Vite SPA** | same | same pattern |
| **Astro** | `@astrojs/cloudflare` single Worker | S22 validation |
| **SvelteKit** | `adapter-cloudflare` **single** Worker | Not multi-`services` consoles (e.g. cloudflarebase) |
| **Remix** | `@remix-run/cloudflare` bundle | S24 validation |
| **Nuxt** | Nitro `cloudflare` preset | S25 validation |

**Deploy pattern:** build in CI → upload artifact → `POST /versions`. Prefer **pre-built** worker + `assets`; avoid relying on celld to re-bundle Tailwind, `.md`, or full SSR graphs.

See [Framework tiers (detail)](./frameworks.md) and the internal [framework coverage](https://github.com/KonghaYao/cellp/blob/main/docs/framework-coverage-cellp.md) doc.

## Recommended default

**Vite SPA (React or Vue) + one Worker API** with wrangler static `assets` — same mental model as Cloudflare **Workers + Static Assets**.

## Next.js (not tier 1)

Cloudflare hosts Next via **OpenNext** or **vinext**. cellp does **not** treat Next as first-class:

- No Next.js Dashboard or official Next template in cellp.
- You may still ship an OpenNext-built artifact if it is a **single** Worker bundle + assets and passes celld deploy.
- **Node SSR / App Router on Node** belongs on Vercel or a Node host, not cellp.

Optimization notes: [OpenNext on cellp (experimental)](https://github.com/KonghaYao/cellp/blob/main/docs/plans/NEXT-OPENNEXT-CELLP.md).

## Supported runtime kinds (summary)

| Kind | Notes |
|------|--------|
| Cloudflare-style Workers | `export default { fetch }` (+ scheduled/queue as celld allows) |
| `wrangler.json` / `wrangler.jsonc` | Parsed from the **bundle** at deploy |
| D1, KV, R2, Queue, Workflows, Cron | See [bindings](/concepts/bindings) |
| Static assets | wrangler `assets` within celld support |

## Not supported / not a goal

| Kind | What to do |
|------|------------|
| **Multiple Workers** (`[[services]]` stacks) | Single main Worker per version today |
| Worker-in-Worker SSR graphs without prebuild | Pre-bundle + `no_bundle` or use tier-1 adapters |
| Pages Functions that are not Workers | Rewrite as a Worker |
| Workers AI, Vectorize, Hyperdrive | Not in celld |
| Arbitrary npm needing Node built-ins | Workers-compatible packages + `nodejs_compat` where supported |
| Python Workers | No |

## Compatibility

celld marks many CF APIs **Partial**. Read [celld cloudflare-compat](https://github.com/KonghaYao/cellp/blob/main/celld/docs/cloudflare-compat.md).

## Languages

JavaScript / TypeScript compiled to a Worker bundle. Bring your own bundler as long as the artifact matches what `celld deploy` expects.
