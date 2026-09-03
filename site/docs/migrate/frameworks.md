# Framework tiers (detail)

How cellp classifies front-end stacks relative to Cloudflare Workers — **AD-13**.

## Tier 1 — first-class

cellp documents, recommends, and validates (support IDs **S22–S25**) these stacks when they deploy as **one Worker + assets** per version:

| Stack | Cloudflare | cellp validation |
|-------|------------|------------------|
| React + Vite SPA | [React + Vite guide](https://developers.cloudflare.com/workers/framework-guides/web-apps/react/) | Community apps S15+ |
| Vue + Vite SPA | [Vue guide](https://developers.cloudflare.com/workers/framework-guides/web-apps/vue/) | e.g. S17 |
| Astro | [Astro guide](https://developers.cloudflare.com/workers/framework-guides/web-apps/astro/) | **S22** |
| SvelteKit | [SvelteKit guide](https://developers.cloudflare.com/workers/framework-guides/web-apps/sveltekit/) | **S23** (single Worker only) |
| Remix | [Remix / React Router on Workers](https://developers.cloudflare.com/workers/framework-guides/web-apps/) | **S24** |
| Nuxt | [Nuxt on Workers](https://developers.cloudflare.com/workers/framework-guides/web-apps/nuxt/) | **S25** |

### What “first-class” means

- Documented on this site and in [Supported stacks](./stacks.md).
- Expected path: **CI builds** the framework output → you upload the wrangler bundle → cellp runs **one celld** per version (AD-1).
- **Not** a guarantee that every GitHub template works without a `dev/examples/<project>/` overlay.

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
- Does **not** use Next for the Dashboard (`web/` is Vite SPA).
- Allows **experimental** OpenNext artifacts if you pre-build a single Worker entry and static assets (see repo doc `docs/plans/NEXT-OPENNEXT-CELLP.md`).

| Approach | cellp |
|----------|--------|
| Next on **Node** (Vercel-style SSR) | Out of scope |
| OpenNext **Worker** bundle + assets | Experimental; prebuild required |
| Next **Edge Middleware** as Next defines it | Not a goal |

## Other CF frameworks (Solid, Waku, …)

Cloudflare may publish guides for additional frameworks. cellp does not assign tier-1 status until there is a validation slot and matrix entry. You can still deploy if the output is a standard Workers bundle.

## Related

- [From Cloudflare](./cloudflare.md) — deploy path and bindings
- [Supported stacks](./stacks.md) — short summary
- Contributor matrix: [framework-coverage-cellp.md](https://github.com/KonghaYao/cellp/blob/main/docs/framework-coverage-cellp.md) · [support-matrix.md](https://github.com/KonghaYao/cellp/blob/main/docs/support-matrix.md)
