# Example app

`dev/examples/commerce` is a small storefront plus JSON API. It exists so you can **click every binding** without writing a Worker first.

## Seed it

```bash
./dev/scripts/reset.sh   # optional, clean slate
./dev/scripts/up.sh
./dev/scripts/seed-commerce-store.sh
```

- Storefront: `http://127.0.0.1:8787/commerce-store/v1/`
- Dashboard: `http://127.0.0.1:5190/projects/commerce-store`

## Bindings used

Declared in [`wrangler.jsonc`](https://github.com/KonghaYao/cellp/blob/main/dev/examples/commerce/wrangler.jsonc); used from [`index.js`](https://github.com/KonghaYao/cellp/blob/main/dev/examples/commerce/index.js). Copy those two files as a template. How to declare your own: [Configure bindings](/build/wrangler). How to seed: [Platform data](/build/data).

| Binding | In the app |
|---------|------------|
| **D1** | Products, orders, inventory, audit log |
| **KV** | Cart + store banner |
| **Queue** | Fulfillment after checkout |
| **Workflow** | Revenue report |
| **R2** | Text asset uploads |
| **Cron** | Hourly heartbeat written to KV |

## HTTP surface

| Method | Path | Purpose |
|--------|------|---------|
| `GET` | `/` | Interactive UI |
| `GET` | `/health` | Liveness |
| `GET` | `/stats` | Row counts |
| `GET` / `POST` | `/api/products` | Catalog |
| `POST` | `/api/orders` | Place order (D1 + queue) |
| `GET` / `PUT` | `/api/kv/cart` | KV cart |
| `GET` / `PUT` | `/api/kv/banner` | KV banner |
| `POST` | `/api/queue/enqueue` | Manual queue message |
| `POST` | `/api/workflow/report` | Start workflow |
| `POST` | `/api/r2/upload` | Upload a text object |

## What to try

1. Add a product and place an order on the storefront.
2. Open Dashboard → Storage → D1 and see the new rows.
3. Deploy a **child** version (`simulate-cd` with a parent) and confirm preview data forked while `v1` stayed put.
4. [Promote](/concepts/promote) when you want `/commerce-store/` (no version segment) to serve that version.

This example is the fastest way to feel “App + Data, same version.”
