# `_routes.json` contract (celld static assets)

Cloudflare Workers static assets may ship `_routes.json` in the asset directory
(Astro `@astrojs/cloudflare` and Wrangler publish it at deploy time).

## File location

`<assets.directory>/_routes.json` at the project root of the asset tree (same
level as `_headers` and `_redirects`). `celld deploy` does **not** upload this
file as a public asset.

## Schema (version 1)

```json
{
  "version": 1,
  "include": ["/*"],
  "exclude": ["/", "/_astro/*", "/blog/*", "/about"]
}
```

- **`include`**: path patterns (leading `/`, `*` wildcards) where the Worker
  may run **before** static assets when no exclude matches.
- **`exclude`**: path patterns served **only** from the asset index. On miss,
  ingress returns **404** and does **not** invoke the Worker.

## Compilation

At deploy, celld stores the parsed `include` / `exclude` arrays in
`assets.json` (`AssetConfig.routes`) and compiles ingress routing to
`run_worker_first` as `include` rules plus `!exclude` rules (same semantics as
Wrangler route lists).

## Ingress

1. If the path matches any **exclude** pattern → try static assets; **404** on
   miss.
2. Else if compiled **include** / `run_worker_first` matches → Worker first.
3. Else → static assets, then Worker fallback (default).

## Related

- `celld/docs/cloudflare-compat.md` — Static assets section
- PD-20260902-05 — Astro S22 `/blog/`, `/about/` routing
