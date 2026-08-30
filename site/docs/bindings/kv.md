# KV

Workers KV (`env.KV.get` / `put`) is implemented by celld. cellp lists namespaces and offers a key operator for humans and scripts.

## Branching

Child versions **branch** KV from the parent: reads can chain to parent blobs; writes stay on the child. Preview carts and flags do not leak into production.

Root versions start empty (unless you seed via the Worker or API).

## Worker

```js
await env.CART.put('session:1', JSON.stringify({ items: [] }))
const raw = await env.CART.get('session:1')
```

Declare `kv_namespaces` in wrangler. Child versions inherit namespace **ids**.

## Operator API

```
GET    …/kv
GET    …/kv/{ns}
GET    …/kv/{ns}/keys
GET    …/kv/{ns}/keys/{key}
PUT    …/kv/{ns}/keys/{key}
DELETE …/kv/{ns}/keys/{key}
```

Dashboard → Storage → KV.

## Gaps vs Cloudflare

celld KV is **partial** (see celld compat). Do not assume every CF KV edge-case (limits, metadata, list cursors) matches production Cloudflare. Treat it as “good enough for app state and config,” and verify your list/pagination needs on the example app.

There is no global “copy prod KV into this preview” besides creating a child version.
