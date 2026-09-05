# Dashboard

The Dashboard is a **Vite + React SPA** (`web/`). It is an operator console for **projects, versions, and data you already declared in wrangler** — not a place to create a Worker. Write the app first: [Write a Worker](/build/).

## API boundary

All Dashboard traffic goes to **cellpd on port `:8790`** (`VITE_CELLP_API_URL` and related `VITE_CELLP_*` in `web/.env.example`). It does **not** call celld listen ports (`8792+`), offshoot, or object storage directly. Storage browsers and binding actions are proxied through the REST API. See [Architecture at a glance](/get-started/architecture#components).

## Run it

```bash
./dev/scripts/up.sh          # API must be up
pnpm install && pnpm --filter cellp-dashboard dev
```

Open `http://127.0.0.1:5190`. Point it at your API with `VITE_CELLP_*` (see `web/.env.example`). Auth is the **admin token**. Run from the **repository root** after `pnpm install` (workspace package `cellp-dashboard`).

For the full click-path from deploy to promote, see [Operator journey](/get-started/operator-journey).

## What you will find

| Area | You can |
|------|---------|
| **Projects** | List / create projects |
| **Overview** | Prod pointer, version counts, operator checklist, **Inspect** entry |
| **Inspect** | Per-project fleet, runtime routes, prod bindings snapshot, attention list |
| **Deployments / Versions** | Status, preview URL, parent, pin, archive, wake, promote |
| **Storage** | Bindings hub for a version |
| **D1 browser** | Tables, rows, SQL |
| **KV** | Namespaces and keys |
| **Queues** | Peek, pause, resume, redrive, purge |
| **Workflows** | Instance list (read-only) |
| **Settings** | Per-version Worker env overrides |
| **Platform** | Global metrics, deep health, runtime routes (filter by project) |

R2 is **visible on the bindings list**. There is no object browser (the runtime has no `celld r2` operator yet).

## What you will not find

- Login, members, roles — [no accounts](/reference/auth)
- Git commit browser — git is metadata on the version
- Live `wrangler tail` — see [Observability](/guides/observability)
- Pause / resume Workflows — list only

The Dashboard only calls the **`:8790` API**; it never talks to celld or S3 directly (see [API boundary](#api-boundary)).

## Production note

Serve `web/dist` behind the same admin token story as the API. The Dashboard must **not** be exposed to the public internet without your own SSO/proxy in front — cellp will not do that for you.
