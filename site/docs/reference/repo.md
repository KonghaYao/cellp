# Repository map

Public docs live here. The GitHub repo also has **contributor** material (design, ADRs, test gates) that is deliberately *not* this site.

**App authors:** start at [Write a Worker](/build/), then [Configure bindings](/build/wrangler) and [Platform data](/build/data).

## Layout

| Path | What |
|------|------|
| [`cellp/`](https://github.com/KonghaYao/cellp/tree/main/cellp) | Go control plane (API, orchestrator, gateway, registry) |
| [`celld/`](https://github.com/KonghaYao/cellp/tree/main/celld) | Rust Workers runtime (**git submodule**) |
| [`web/`](https://github.com/KonghaYao/cellp/tree/main/web) | Dashboard (Vite + React SPA) |
| [`dev/`](https://github.com/KonghaYao/cellp/tree/main/dev) | Local stack scripts + examples |
| [`e2e/`](https://github.com/KonghaYao/cellp/tree/main/e2e) | Integration gates |
| [`stress/`](https://github.com/KonghaYao/cellp/tree/main/stress) | Load tests |
| [`docker/`](https://github.com/KonghaYao/cellp/tree/main/docker) | Image + Compose |
| [`docs/`](https://github.com/KonghaYao/cellp/tree/main/docs) | Internal design, decisions, evidence |
| [`site/`](https://github.com/KonghaYao/cellp/tree/main/site) | **This website** (VitePress) |

## Docs you want as a user

Start on this site. Deep links into GitHub when you need a file:

- OpenAPI: `cellp/api/openapi.yaml`
- CI example: `dev/examples/ci-pr-preview.example.yml`
- Commerce Worker: `dev/examples/commerce/`
- celld compat: `celld/docs/cloudflare-compat.md`

## Docs you want as a contributor / agent

- [`DESIGN.md`](https://github.com/KonghaYao/cellp/blob/main/DESIGN.md) — architecture
- [`docs/decisions.md`](https://github.com/KonghaYao/cellp/blob/main/docs/decisions.md) — AD-1…13
- [`docs/support-matrix.md`](https://github.com/KonghaYao/cellp/blob/main/docs/support-matrix.md) — community Workers validation
- [`docs/test-plan.md`](https://github.com/KonghaYao/cellp/blob/main/docs/test-plan.md) — acceptance
- [`AGENTS.md`](https://github.com/KonghaYao/cellp/blob/main/AGENTS.md) — how to change the repo

## Run this site locally

```bash
cd site
npm install
npm run docs:dev
```

Production URL: `https://konghayao.github.io/cellp/` (GitHub Pages).
