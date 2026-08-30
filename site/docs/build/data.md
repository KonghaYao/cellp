# Platform data

This is how data shows up in preview and production. You already [declared bindings in wrangler](/build/wrangler). Now you fill them.

## Mental model

```
wrangler.jsonc     →  names on env (DB, CACHE, …)
first version      →  empty resources, unless you seed
your Worker / UI   →  writes rows, keys, objects
child version      →  copy-on-write fork of D1 + KV + R2 + Queue
Dashboard          →  look at / patch data on a ready version
promote            →  that version’s data becomes production
```

Production does **not** share a database with a PR. A PR with `parent_version_id` gets its **own** fork.

## Root version (first deploy, no parent)

| Binding | Starts as | How you put data in |
|---------|-----------|---------------------|
| **D1** | Empty | `seed.db` in the artifact, or `env.DB.exec` / `prepare` in the Worker, or Dashboard SQL |
| **KV** | Empty | Worker `put`, or Dashboard KV browser |
| **R2** | Empty | Worker `put` (no object UI) |
| **Queue** | Empty | Worker `send` |
| **Workflow / Cron** | No instances / waiting for schedule | Worker `create` / celld cron |

### Seed D1 with a SQLite file

If the artifact directory contains **`seed.db`** (a real SQLite file), the orchestrator runs `celld d1 import` for a **root** version.

Local commerce does exactly that:

```bash
./dev/scripts/seed-commerce-store.sh
# writes seed.db next to wrangler.jsonc under
# dev/data/artifacts/commerce-store/v1/
```

Your app can do the same: generate `seed.db` in CI (`sqlite3 seed.db < schema.sql` then inserts), upload it **beside** `wrangler.jsonc`.

Do not POST SQLite bytes through the JSON API. Path/file import only.

### Seed D1 from the Worker

Fine for schema and tiny fixtures:

```js
async function ensureSchema(db) {
  await db.exec(`
    CREATE TABLE IF NOT EXISTS products (
      id INTEGER PRIMARY KEY AUTOINCREMENT,
      name TEXT NOT NULL,
      price_cents INTEGER NOT NULL
    );
  `)
}

export default {
  async fetch(request, env) {
    await ensureSchema(env.DB)
    // …
  },
}
```

This runs on every request unless you guard it. Prefer `seed.db` for catalogs; use `CREATE TABLE IF NOT EXISTS` so a fork still has a schema if import was skipped.

## Child version (PR / staging fork)

Set `parent_version_id` to a **pinned seed or staging** version, not casually to live prod.

Then, automatically:

- D1 / KV / R2 / Queue **branch** (preview writes stay in the preview)
- Workflow instances and Cron **do not** copy in-flight work
- Worker code is **this** artifact

Keep parent `database_id` / KV `id` / queue / bucket names in wrangler. Details: [Versions](/concepts/versions).

## Using the Dashboard to configure data

Once the version is `ready`, open Dashboard → project → **Storage** (pick the version):

| You want | Where |
|----------|--------|
| Run SQL, browse tables | D1 browser |
| Set a banner / feature flag | KV |
| Look at queued messages | Queues (peek / pause / purge) |
| See workflow runs | Workflows (read-only) |
| Change `API_ORIGIN` | Settings → env (restarts that version) |

R2 has **no** file manager — upload from the Worker.

This is how a human configures “platform data” day to day **without** a new deploy. A new **binding** still requires a new wrangler + version.

## Env vs data

| | Env vars | D1 / KV / R2 / Queue |
|--|----------|----------------------|
| Declared | wrangler `vars` + overrides | wrangler binding blocks |
| Per version? | Yes | Yes (and forked on children) |
| Secrets | CI / PUT env — not a vault | Don’t store secrets in D1 if you can avoid it |
| Preview isolation | Child copies parent overrides | Child **branches** storage |

See [Environment variables](/guides/environment-variables).

## Promote and rollback

Promote switches the production **pointer**. The data that goes live is whatever that version already has (its D1/KV/…). Rolling back is promoting an older version — its data comes back with it. [Rollback](/guides/rollback).

## Checklist for a new app

1. Write `wrangler.jsonc` with the bindings you will call from `env`.
2. Deploy a **root** `v1`.
3. Put schema + seed in `seed.db` **or** run SQL in the Dashboard **or** `exec` in the Worker.
4. Pin `v1` (or a `v-staging`) if PRs will fork it.
5. PR deploys use `parent_version_id: "v-staging"` and the **same** binding ids.
6. Click around preview; promote when the data and code both look right.
