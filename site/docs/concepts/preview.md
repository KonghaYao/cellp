# Preview & production

cellp selects the target version with **HTTP Host** ([ingress routing plan](https://github.com/KonghaYao/cellp/blob/main/docs/plans/INGRESS-ROUTING.md)). Gateway forwards the request path **unchanged** from `/`. Using `/{project}/{version}/` or `/{project}/` as a version selector is **deprecated**.

## Preview

Preview Host (synthetic FQDN):

```
{version}.{project}.{base-domain}
```

Default base domain: `ingress.local` (`CELLP_INGRESS_BASE_DOMAIN`).

Example:

```
GET http://v-abc123.demo-app.ingress.local:8787/…
```

The API returns `preview_url` when you create a version; after deploy it matches the Host binding (includes `:8787` in dev when Gateway is not on port 80/443).

## Production

```
GET http://{project}.{base-domain}:8787/…
```

Example: `http://demo-app.ingress.local:8787/` → current `prod_version_id`.

Nothing is production until you [promote](/concepts/promote). **Promote does not change the prod Host**—only which version backs it.

## What preview is for

Because child versions **branch data**, preview is a real environment:

- Checkout on a PR version writes orders into **that** D1 branch
- KV banners you tweak in preview do not appear on prod Host
- You can keep a long-lived `v-staging` pinned and fork PRs from it

## Data snapshot timeline

Preview is **not** “live production plus your PR code.” When you set `parent_version_id`, cellp forks **data bindings** (D1, KV, R2, Queue) from that parent during deploy. The child sees the parent celld bucket as it was at **`fork_txid`** (D1’s branch cut point). Writes on the parent **after** the child is created stay on the parent bucket — the preview **does not** see them.

Typical PR flow: parent = a **pinned staging or seed** version, not ad-hoc live prod. Production is still its own version and bucket; prod Host traffic does not stream into preview.

## Cron on preview

Preview ready versions **do not** run `scheduled` handlers. Only the **production** version arms cron (see [Cron](/bindings/cron)). After **promote**, scheduling moves to the new prod version automatically.

## Local dev (hosts & LAN)

Contributor stacks: **[dev/INGRESS-HOST.md](https://github.com/KonghaYao/cellp/blob/main/dev/INGRESS-HOST.md)** (unified guide).

| Mode | Setup |
|------|--------|
| **Default** | `CELLP_INGRESS_BASE_DOMAIN=ingress.local` + `/etc/hosts` → `127.0.0.1` |
| **LAN (no per-client hosts)** | `./dev/scripts/ingress-host-init.sh magic` (nip.io / sslip.io) |
| **Clash / system proxy** | [dev/clash/README.md](https://github.com/KonghaYao/cellp/blob/main/dev/clash/README.md) — nip.io needs **DIRECT** or browser **502** |

Quick init:

```bash
./dev/scripts/ingress-host-init.sh local   # or magic
./dev/scripts/reset.sh && ./dev/scripts/up.sh && ./dev/scripts/seed-demo.sh
```

Example `/etc/hosts` (local):

```
127.0.0.1 v1.demo-app.ingress.local demo-app.ingress.local
```

Probe without editing hosts:

```bash
curl -H "Host: v1.demo-app.ingress.local" http://127.0.0.1:8787/
curl -H "Host: demo-app.ingress.local" http://127.0.0.1:8787/health
```

**Always use `http://` and include `:8787` in the browser** unless TLS terminates on an outer proxy.

Full normative design: [INGRESS-ROUTING.md](https://github.com/konghayao/cellp/blob/main/docs/plans/INGRESS-ROUTING.md).
