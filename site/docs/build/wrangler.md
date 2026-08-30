# Configure bindings

Platform data is **not** a separate product you click together in the Dashboard. You declare it in **`wrangler.jsonc`** next to the Worker (`wrangler.json` is parsed for listings but **not** used as the deploy bundle). On deploy, celld creates those bindings on **this version**. The Dashboard is for inspecting and editing **values** after the version is `ready`.

## The rule

| Lives in wrangler (your repo) | Lives on the version (after deploy) |
|-------------------------------|--------------------------------------|
| Binding **names** (`env.DB`) | Actual D1 rows, KV keys, R2 objects, queue messages |
| Resource ids (`database_id`, KV `id`, queue name, R2 `bucket_name`) | Forked copies on child versions |
| Cron expressions, workflow class names | Cron ticks / workflow **instances** |
| `vars` | Overridable per version in Settings |

If it is not in wrangler, `env.FOO` is `undefined`. Creating a “database” only in the UI does nothing.

**Deploy filename:** the orchestrator only treats **`wrangler.jsonc`** as the Worker bundle. Use that name.

cellp currently honors **one** `d1_databases` entry. A second database in the array fails deploy.

## Minimal file

```jsonc
{
  "name": "my-shop",
  "main": "index.js",
  "compatibility_date": "2026-01-01",
  "vars": {
    "STORE_NAME": "Demo shop"
  }
}
```

`vars` become `env.STORE_NAME`. Do not put production secrets here — inject them from CI or [version env](/guides/environment-variables).

## D1

```jsonc
"d1_databases": [
  {
    "binding": "DB",
    "database_name": "commerce",
    "database_id": "a1b2c3d4-e5f6-7890-abcd-ef1234567890"
  }
]
```

In the Worker: `env.DB.prepare('select …')`, `env.DB.exec(schema)`.

`binding` is the `env` key. `database_name` is what celld uses for import. `database_id` must stay **stable** across child versions so the same Worker keeps working — copy it from the parent wrangler; do not mint a new UUID on every PR.

[Use D1 in code →](/bindings/d1) · [Seed / browse data →](/build/data)

## KV

```jsonc
"kv_namespaces": [
  {
    "binding": "CACHE",
    "id": "my-shop-cache"
  }
]
```

In the Worker: `env.CACHE.get(key)`, `env.CACHE.put(key, value)`. The `id` is the namespace identity (inherited on branch). [KV guide](/bindings/kv).

## R2

```jsonc
"r2_buckets": [
  { "binding": "ASSETS", "bucket_name": "my-shop-assets" }
]
```

In the Worker: `env.ASSETS.put(key, body)`, `get`, `list`. There is no object browser in the Dashboard. [R2 guide](/bindings/r2).

## Queues

```jsonc
"queues": {
  "producers": [{ "binding": "FULFILLMENT", "queue": "fulfillment" }]
}
```

`env.FULFILLMENT.send(payload)` from a **fetch** Worker.

::: warning Consumer scripts cannot also export `fetch`
celld: a queue **consumer** script must not export `fetch()`. Keep a producer Worker for HTTP and a separate consumer Worker if you need `queue()`. See [Queues](/bindings/queues) and [Handlers](/build/handlers).
:::

## Workflows

```jsonc
"workflows": [
  {
    "binding": "REPORTS",
    "name": "order-report",
    "class_name": "OrderReport"
  }
]
```

Export the class from the same module (`class_name` must match). `env.REPORTS.create({ params })`. Instances **do not** fork to preview versions. [Workflows](/bindings/workflows).

## Cron

```jsonc
"triggers": { "crons": ["0 * * * *"] }
```

Implement `scheduled(controller, env, ctx)` on the default export. celld fires it; cellp only displays the expression. [Cron](/bindings/cron).

## Durable Objects (optional)

```jsonc
"durable_objects": {
  "bindings": [{ "name": "COUNTER", "class_name": "Counter" }]
},
"migrations": [{ "tag": "v1", "new_sqlite_classes": ["Counter"] }]
```

See the repo example `dev/examples/counter`. celld DO support is **partial**. Durable Objects **do not** show up on `GET …/bindings` (the control-plane parser has no `durable_objects` field). Confirm [celld compat](https://github.com/KonghaYao/cellp/blob/main/celld/docs/cloudflare-compat.md) before you bet the product on them.

## Full example (commerce)

The storefront in this repo wires all of the above. Copy [wrangler.jsonc](https://github.com/KonghaYao/cellp/blob/main/dev/examples/commerce/wrangler.jsonc), [index.js](https://github.com/KonghaYao/cellp/blob/main/dev/examples/commerce/index.js), and [storefront.js](https://github.com/KonghaYao/cellp/blob/main/dev/examples/commerce/storefront.js).

## Child versions

When CI sends `parent_version_id`, cellp **copies parent identities** onto the child wrangler for D1 `database_id`, KV `id`, and R2 `bucket_name`. You should still ship the same ids in your file so listings match what you expect. **Queue names must already match** the parent or Start fails.

Changing those ids in a PR does **not** give you a quietly empty preview for D1/KV/R2 (they get overwritten). An empty preview is usually a missing `parent_version_id`, a failed branch, or a count mismatch.

The **script** always comes from this artifact (`main`). You can change routes and SQL.

## What the Dashboard is for

After `ready`:

- See the binding list (Storage)
- Browse / SQL D1, edit KV keys, peek queues
- Override `vars` ([env](/guides/environment-variables))

It will not add a new `d1_databases` entry. Change wrangler and deploy a new version.
