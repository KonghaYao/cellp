# Small e-commerce storefront + API backed by all celld bindings.

## Bindings

| Binding | Usage in app |
|---------|----------------|
| **D1** | Products, orders, inventory, audit log |
| **KV** | Shopping cart + store banner |
| **Queue** | Fulfillment tasks on checkout |
| **Workflow** | Order revenue report |
| **R2** | Upload asset notes from UI |
| **Cron** | Hourly heartbeat written to KV |

## Storefront

Open the production URL root (`/`) for the interactive UI — catalog, cart checkout (D1 writes), bindings playground.

## API (JSON)

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Liveness |
| GET | `/stats` | Row counts |
| GET | `/api/products` | Product catalog |
| POST | `/api/products` | Add product + inventory |
| GET | `/api/customers` | Customer list |
| POST | `/api/orders` | Place order (D1 + queue) |
| GET/PUT | `/api/kv/cart` | KV cart |
| GET/PUT | `/api/kv/banner` | KV banner |
| POST | `/api/queue/enqueue` | Manual queue message |
| POST | `/api/workflow/report` | Start workflow |
| POST | `/api/r2/upload` | Upload text asset |

## Local deploy

```bash
./dev/scripts/reset.sh
./dev/scripts/up.sh
./dev/scripts/seed-commerce-store.sh
open http://127.0.0.1:8787/commerce-store/v1/
```

Dashboard: `http://127.0.0.1:5190/projects/commerce-store`
