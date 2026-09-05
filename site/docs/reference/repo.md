# Repository map

Public docs live here. The GitHub repo also has design and acceptance material that is deliberately *not* this site.

**App authors:** start at [Write a Worker](/build/), then [Configure bindings](/build/wrangler) and [Platform data](/build/data).

## Layout

| Path | What |
|------|------|
| [`cellp/`](https://github.com/KonghaYao/cellp/tree/main/cellp) | Go control plane (API, orchestrator, gateway, registry) |
| [`celld/`](https://github.com/KonghaYao/cellp/tree/main/celld) | Rust Workers runtime (**git submodule**) |
| [`web/`](https://github.com/KonghaYao/cellp/tree/main/web) | Dashboard (Vite + React SPA) |
| [`dev/`](https://github.com/KonghaYao/cellp/tree/main/dev) | Local stack scripts + examples |
| [`docker/`](https://github.com/KonghaYao/cellp/tree/main/docker) | Image + Compose |
| [`site/`](https://github.com/KonghaYao/cellp/tree/main/site) | **This website** (VitePress) |

## Docs you want as a user

Start on this site:

- [REST API](/reference/api) · [Auth](/reference/auth) · [CLI](/reference/cli) · [Compatibility](/reference/compatibility)
- OpenAPI: `cellp/api/openapi.yaml`
- CI example: `dev/examples/ci-pr-preview.example.yml`
- Commerce Worker: `dev/examples/commerce/`
- celld compat: `celld/docs/cloudflare-compat.md`

## Run this site locally

```bash
# from repository root (pnpm workspace includes site/)
pnpm install
pnpm --filter cellp-docs docs:dev   # http://localhost:5173/cellp/
```

Production URL: `https://konghayao.github.io/cellp/` (GitHub Pages).

## Architecture deep dives (GitHub)

For contributors and agents, the repository root includes `DESIGN.md`, `docs/decisions.md`, and `AGENTS.md`. Those files are not mirrored on this site.
