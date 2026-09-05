# Platform data

This is how data shows up in preview and production. You already [declared bindings in wrangler](/build/wrangler). Now you fill them.

## Mental model

```
wrangler.jsonc     →  names on env (DB, CACHE, …)
first version      →  empty resources, unless you seed
your Worker / UI   →  writes rows, keys, objects
child version      →  copy-on-write fork of D1 + KV + R2 + Queue
Dashboard          →  look at / patch data on a ready version
promote            →  prod pointer switches to that version’s bucket (not a merge)
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

If the artifact directory contains **`seed.db`** (a real SQLite file), the orchestrator runs `celld d1 import` for a **root** version (no `parent_version_id`). Before import, an optional **[offshoot](/concepts/offshoot) export** may write to that same path; when export succeeds, it **replaces** any `seed.db` you bundled. Child versions **branch** parent storage instead of re-importing `seed.db`.

Your CI can still generate `seed.db` (`sqlite3 seed.db < schema.sql`). Do not POST SQLite bytes through the JSON API.

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

Promote switches the production **pointer** to a ready version’s **existing** bucket. It is **not** a merge: writes that landed on the **previous** prod version **after** you forked the promoted version are **not** combined in.

Timeline example:

1. `v-staging` has 100 orders; you fork PR version `pr-9` from it (preview sees 100).
2. Live prod (`v-prod`) gets 5 more orders while you test `pr-9`.
3. You promote `pr-9`. Production now shows what `pr-9` had (e.g. 100 + preview test orders), **not** the 5 prod-only rows unless you wrote them in preview too.

Rolling back is promoting an older version — its frozen bucket comes back with it. [Rollback](/guides/rollback). See also [What promote does not do](/concepts/promote#what-promote-does-not-do).

## Checklist for a new app

1. Write `wrangler.jsonc` with the bindings you will call from `env`.
2. Deploy a **root** `v1`.
3. Put schema + seed in `seed.db` **or** run SQL in the Dashboard **or** `exec` in the Worker.
4. Pin `v1` (or a `v-staging`) if PRs will fork it.
5. PR deploys use `parent_version_id: "v-staging"` and the **same** binding ids.
6. Click around preview; promote when the data and code both look right.
