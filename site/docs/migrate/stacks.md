# Supported stacks

Runtime: **celld** (Workers semantics). Not Node. Not Next.js hosting.

## Supported

| Kind | Notes |
|------|--------|
| Cloudflare-style Workers | `export default { fetch }` (and scheduled/queue handlers as celld allows) |
| `wrangler.json` / `wrangler.jsonc` | Parsed from the **bundle** at deploy. No wrangler.toml lifecycle in cellp |
| D1 | Import + branch, Dashboard SQL |
| KV / R2 / Queue | Operators + branch on children (R2: no object UI) |
| Workflows | Run in celld; cellp list-only |
| Cron | Declared in wrangler; celld fires |
| Static assets | wrangler `assets` / Workers Sites **within celld support** |
| Any Git + CI | As long as you `POST /versions` |

## Not supported / not a goal

| Kind | What to do |
|------|------------|
| Next.js SSR / App Router on Node | Vercel or Node host |
| Next.js Edge Middleware (as Next defines it) | Not a Workers bundle |
| Pages Functions that are not Workers | Rewrite as a Worker |
| Workers AI, Vectorize, Hyperdrive | Not in celld |
| Arbitrary npm that needs Node built-ins | Workers-compatible packages only |
| Python Workers | No |

## Compatibility

celld marks many CF APIs **Partial**. “It ran on Cloudflare” is not a guarantee. Read [celld cloudflare-compat](https://github.com/KonghaYao/cellp/blob/main/celld/docs/cloudflare-compat.md) for Durable Objects, KV list, R2, Queues, and cron syntax.

## Languages

JavaScript / TypeScript compiled to a Worker bundle (esbuild in the image/dev toolchain). Bring your own bundler as long as the artifact is what `celld deploy` expects.
