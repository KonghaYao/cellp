# Preview & production

Two URL shapes. Same gateway. Different pointers.

## Preview

```
GET http://gateway/{project}/{version}/…
```

Served by **that** version’s celld process. Use it for PR review, staging, and “does this migrate?”.

The path prefix is stripped when the request reaches the Worker: your app still sees `/` as its root, same as putting a Worker behind a subpath-aware proxy. If a link breaks, check that the Worker uses relative URLs.

## Production

```
GET http://gateway/{project}/…
```

Served by whatever `prod_version_id` currently is. Nothing is production until you [promote](/concepts/promote).

## What preview is for

Because child versions **branch data**, preview is a real environment:

- Checkout on a PR version writes orders into **that** D1 branch
- KV banners you tweak in preview do not appear on `/my-shop/`
- You can keep a long-lived `v-staging` pinned and fork PRs from it

## Data snapshot timeline

Preview is **not** “live production plus your PR code.” When you set `parent_version_id`, cellp forks **data bindings** (D1, KV, R2, Queue) from that parent during deploy. The child sees the parent celld bucket as it was at **`fork_txid`** (D1’s branch cut point). Writes on the parent **after** the child is created stay on the parent bucket — the preview **does not** see them.

Typical PR flow: parent = a **pinned staging or seed** version, not ad-hoc live prod. Production is still its own version and bucket; traffic on `/{project}/` does not stream into preview.

## Cron on preview

Preview ready versions **do not** run `scheduled` handlers. Only the **production** version arms cron (see [Cron](/bindings/cron)). After **promote**, scheduling moves to the new prod version automatically.

## Not like Git

| Git | cellp preview |
|-----|----------------|
| Branch can track `main` and merge/rebase often | Child version is a **one-time data snapshot** plus its own writes |
| `git merge` combines histories | [Promote](/concepts/promote) **switches** `prod_version_id` to that version’s existing bucket — no SQL/KV merge |
| You can pull latest `main` into a PR branch | Fork does **not** auto-update when prod moves forward |

## Custom domains

cellp does not issue certificates or write DNS. Point your load balancer:

- `shop.example.com` → `http://cellpd:8787/my-shop/`
- `preview.internal` → `http://cellpd:8787/` (keep the `/{project}/{version}` path)

Terminate TLS on the LB.

## Health

- API: `GET /v1/health` and `GET /v1/health/deep`
- Gateway: `GET /health` on the gateway port (Compose)

Deep health includes registry, object store, and queue depth. A `503` there means back off deploys.
