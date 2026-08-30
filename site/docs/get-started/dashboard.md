# Dashboard

The Dashboard is a Vite SPA (`web/`). It is an operator console for **projects, versions, and data you already declared in wrangler** — not a place to create a Worker. Write the app first: [Write a Worker](/build/).

## Run it

```bash
./dev/scripts/up.sh          # API must be up
cd web && npm install && npm run dev
```

Open `http://127.0.0.1:5190`. Point it at your API with `VITE_CELLP_*` (see `web/.env.example`). Auth is the **admin token**.

## What you will find

| Area | You can |
|------|---------|
| **Projects** | List / create projects |
| **Overview** | Prod pointer, version counts, entry links |
| **Deployments / Versions** | Status, preview URL, parent, pin, archive, wake, promote |
| **Storage** | Bindings hub for a version |
| **D1 browser** | Tables, rows, SQL |
| **KV** | Namespaces and keys |
| **Queues** | Peek, pause, resume, redrive, purge |
| **Workflows** | Instance list (read-only) |
| **Settings** | Per-version Worker env overrides |
| **Platform** | Runtime / route health |

R2 is **visible on the bindings list**. There is no object browser (the runtime has no `celld r2` operator yet).

## What you will not find

- Login, members, roles — [no accounts](/reference/auth)
- Git commit browser — git is metadata on the version
- Live `wrangler tail` — see [Observability](/guides/observability)
- Pause / resume Workflows — list only

## Production note

Serve `web/dist` behind the same admin token story as the API. The Dashboard must **not** be exposed to the public internet without your own SSO/proxy in front — cellp will not do that for you.
