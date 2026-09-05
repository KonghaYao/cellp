# Gateway routing

User traffic hits **cellpd Gateway** (default port **8787**). The gateway is a reverse proxy: it picks a **version** from the request, then forwards the URL path and query **unchanged** to that version’s celld process.

## Host-based routing (default)

cellp selects the target version from the **HTTP Host** header, not from a path prefix.

| Host pattern | Routes to |
|--------------|-----------|
| `{version}.{project}.{base-domain}` | That version (preview) |
| `{project}.{base-domain}` | Current production (`prod_version_id`) |

Default base domain: `ingress.local` (`CELLP_INGRESS_BASE_DOMAIN`). The API returns `preview_url` and `prod_url` with the correct Host (and `:8787` in dev when nothing listens on 80/443).

Registry rows (`ingress_bindings`) tie each version to a **synthetic hostname** and optional dev listen port. The gateway forwards to celld with that **synthetic `Host`** (and trusted `X-Forwarded-*` headers) so `request.url` inside the Worker matches the public preview or prod origin—not `127.0.0.1:8792`.

Details and local setup: [Preview & production](/concepts/preview).

## Deprecated path selectors

These used to select a version by URL path. They are **deprecated** and removed from the default gateway:

- `/{project}/{version}/…`
- `/{project}/…` (production)

Use Host routing instead. CI smoke tests and the Dashboard should use **`preview_url`** / **`prod_url`**, not path-shaped URLs.

## WebSockets

The gateway **forwards WebSocket upgrades** to celld: hop-by-hop headers (`Upgrade`, `Connection`, `Sec-WebSocket-*`) are passed through, and the proxy supports **101 Switching Protocols** on the same Host route as HTTP.

Requirements:

- Use the same **preview or prod Host** as for normal HTTP (for example `wss://` through your outer TLS proxy, or `ws://` in dev).
- The Worker (or Durable Object) must implement the upgrade on the path you request; cellp does not add a separate “WebSocket URL” besides the version’s ingress Host.

Runtime limits (partial APIs, DO behavior) are documented in [celld cloudflare-compat](https://github.com/KonghaYao/cellp/blob/main/celld/docs/cloudflare-compat.md).

## Optional listen-port routing (dev)

For local debugging only, a version can bind a **dedicated loopback port** (`127.0.0.1`, high port range) in addition to—or instead of—Host routing. This is **opt-in**, not the default laptop or CI path.

Production-shaped deployments should use **one gateway listener** plus Host (or synthetic FQDN + `/etc/hosts`). Put TLS and public DNS on Nginx, HAProxy, or a cloud load balancer in front of the gateway; forward `Host` unchanged.

Operator-facing behavior: [Preview & production](/concepts/preview) · [How it works](/how-it-works).

## Outer proxy checklist

- `proxy_pass` to gateway **without** stripping a path prefix
- `proxy_set_header Host $host` (or the synthetic preview Host you intend)
- Terminate TLS outside cellp; gateway speaks HTTP on `:8787` in dev

See also: [How it works](/how-it-works) · [API ingress notes](/reference/api).
