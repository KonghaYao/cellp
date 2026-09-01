# Preview & production

cellp selects the target version with **HTTP Host** ([AD-12](https://github.com/konghayao/cellp/blob/main/docs/decisions.md#17-ad-12--hostname--port-ingress废弃-path-选-version)). Gateway forwards the request path **unchanged** from `/`. Using `/{project}/{version}/` or `/{project}/` as a version selector is **deprecated**.

## Preview

Preview Host (synthetic FQDN):

```
{version}.{project}.{base-domain}
```

Default base domain: `ingress.local` (`CELLP_INGRESS_BASE_DOMAIN`).

Example:

```
GET http://v-abc123.demo-app.ingress.local/…
```

Local dev without DNS: add one **`/etc/hosts`** line pointing at the gateway (loopback):

```
127.0.0.1 v-abc123.demo-app.ingress.local
```

Then open `http://v-abc123.demo-app.ingress.local:8787/` (or terminate TLS on an outer proxy in production). The Worker sees `/` as its root—no path prefix to strip.

The API returns `preview_url` when you create a version; after deploy it matches the Host binding.

## Production

```
GET http://{project}.{base-domain}/…
```

Example: `http://demo-app.ingress.local/` → current `prod_version_id`.

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

## Local dev

For each preview Host and the prod Host `{project}.{base-domain}`, add **`127.0.0.1`** entries in `/etc/hosts` (contributor stacks: `cellp dev ingress hosts-print` prints suggested lines). Gateway listens on `GATEWAY_PORT` (default **8787**). Without editing hosts, you can still probe with `curl -H "Host: v-abc123.demo-app.ingress.local" http://127.0.0.1:8787/`.

Full normative design: [INGRESS-ROUTING.md](https://github.com/konghayao/cellp/blob/main/docs/plans/INGRESS-ROUTING.md).
