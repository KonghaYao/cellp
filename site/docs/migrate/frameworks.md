# Framework tiers (detail)

How cellp classifies front-end stacks relative to Cloudflare Workers.

## Tier 1 — first-class

cellp documents, recommends, and validates these stacks when they deploy as **one Worker + assets** per version (one **celld** process per [version](/concepts/versions)):

| Stack | Cloudflare | cellp |
|-------|------------|--------|
| React + Vite SPA | [React + Vite guide](https://developers.cloudflare.com/workers/framework-guides/web-apps/react/) | Default for new apps |
| Vue + Vite SPA | [Vue guide](https://developers.cloudflare.com/workers/framework-guides/web-apps/vue/) | Same SPA + Worker pattern |
| Astro | [Astro guide](https://developers.cloudflare.com/workers/framework-guides/web-apps/astro/) | Single Worker / static + server |
| SvelteKit | [SvelteKit guide](https://developers.cloudflare.com/workers/framework-guides/web-apps/sveltekit/) | **Single Worker only** |
| Remix | [Remix / React Router on Workers](https://developers.cloudflare.com/workers/framework-guides/web-apps/) | `@remix-run/cloudflare` bundle |
| Nuxt | [Nuxt on Workers](https://developers.cloudflare.com/workers/framework-guides/web-apps/nuxt/) | Nitro `cloudflare` preset |

### What “first-class” means

- Documented on this site and in [Supported stacks](/migrate/stacks).
- Expected path: **CI builds** the framework output → you upload the wrangler bundle → cellp runs **one celld** per version.
- **Not** a guarantee that every GitHub template works without a project-specific overlay or `prepare-artifact` script.

### What it does **not** mean

- **Multi-Worker** apps (`wrangler` `[[services]]` binding other Workers) — **not supported** on cellp today.
- **SvelteKit “platform” templates** that rely on separate auth/DB Workers — treat as unsupported until multi-worker orchestration exists.

## Default recommendation

For new projects on cellp, start with:

```text
Vite (React or Vue)  →  static dist/
Thin Worker          →  API, auth, bindings
wrangler assets      →  SPA fallback
```

This matches Cloudflare’s **Workers + Static assets** model and avoids celld re-bundling large framework graphs.

## Next.js — experimental, not tier 1

Cloudflare documents Next.js via **OpenNext** (and related tooling). cellp:

- Does **not** list Next as tier 1.
- Does **not** use Next for the Dashboard (`web/` is a **Vite SPA**, not Next.js).
- Allows **experimental** OpenNext artifacts if you pre-build a single Worker entry and static assets.
- **Lab only:** minimal OpenNext and pinned App Router fixtures have passed lab fixture checks (static assets, dynamic SSR route, Route Handler, 404). That is not tier-1 support or a promise that arbitrary Next/OpenNext versions work unchanged.

| Approach | cellp |
|----------|--------|
| Next on **Node** (Vercel-style SSR) | Out of scope |
| OpenNext **Worker** bundle + assets | Experimental; prebuild required |
| Next **Edge Middleware** as Next defines it | Not a goal |

## Other CF frameworks (Solid, Waku, Hono, Qwik, …)

Cloudflare may publish guides for additional frameworks. cellp assigns **tier 1** only to the table above. **Hono**, **SolidStart**, **Qwik City**, and **Waku** have passed community validation as single-Worker deploys — see [Supported stacks](/migrate/stacks). Others may still work if the output is a standard Workers bundle; judge against [celld compatibility](https://github.com/KonghaYao/cellp/blob/main/celld/docs/cloudflare-compat.md).

## Related

- [Migrate overview](/migrate/) — where to start
- [From Cloudflare](/migrate/cloudflare) — deploy path and bindings
- [Supported stacks](/migrate/stacks) — short summary
- [Binding guides](/bindings/) — celld vs cellp per binding
