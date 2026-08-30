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

## Custom domains

cellp does not issue certificates or write DNS. Point your load balancer:

- `shop.example.com` → `http://cellpd:8787/my-shop/`
- `preview.internal` → `http://cellpd:8787/` (keep the `/{project}/{version}` path)

Terminate TLS on the LB.

## Health

- API: `GET /v1/health` and `GET /v1/health/deep`
- Gateway: `GET /health` on the gateway port (Compose)

Deep health includes registry, object store, and queue depth. A `503` there means back off deploys.
