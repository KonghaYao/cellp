# KV

Workers KV on `env.<binding>`. **celld** stores keys in the version bucket; **cellp** **branches** namespaces on child versions and exposes key operator APIs.

[Bindings overview](/concepts/bindings) · [Binding guides](./index) · [Platform data](/build/data)

## 1. Declare it

```jsonc
"kv_namespaces": [
  {
    "binding": "CACHE",
    "id": "my-shop-cache"
  }
]
```

`id` is the namespace identity — keep it on child wranglers. [Configure bindings](/build/wrangler).

## 2. Use it in the Worker

```js
export default {
  async fetch(request, env) {
    const url = new URL(request.url)

    if (url.pathname === '/api/banner' && request.method === 'GET') {
      const value = (await env.CACHE.get('store:banner')) ?? ''
      return Response.json({ value })
    }

    if (url.pathname === '/api/banner' && request.method === 'PUT') {
      const { value } = await request.json()
      await env.CACHE.put('store:banner', String(value ?? ''))
      return Response.json({ ok: true })
    }

    return new Response('not found', { status: 404 })
  },
}
```

Commerce uses `CACHE` for cart + store banner (`dev/examples/commerce`).

## 3. Put keys in

| Method | When |
|--------|------|
| Worker `put` | App logic |
| Dashboard → Storage → KV | Flags, banners, one-off edits |
| Child version | **Branch** from parent (preview writes stay isolated) |

Root versions start **empty**. There is no “create namespace” button besides wrangler. [Platform data](/build/data).

## Operator API

Prefix: `/v1/projects/{project}/versions/{version}/`

```
GET    …/kv
GET    …/kv/{ns}                 # namespace info (wrangler `id`)
GET    …/kv/{ns}/keys?prefix=&cursor=&limit=
GET    …/kv/{ns}/keys/{key}
PUT    …/kv/{ns}/keys/{key}      # JSON body: value, optional ttl/metadata
DELETE …/kv/{ns}/keys/{key}
```

Dashboard and API require the version to be **ready**. `503` when celld is stopped.

## celld vs Cloudflare

- No edge cache — `cacheTtl` has no effect; `cacheStatus` is `null`.
- Values above **1 MiB** need the version fleet bucket (normal on cellp).
- One writer per namespace; scale writes with more namespaces.

Full list: [celld cloudflare-compat — KV](https://github.com/KonghaYao/cellp/blob/main/celld/docs/cloudflare-compat.md#kv).
