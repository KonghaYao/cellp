# Projects

A **project** is the durable name of an application. Versions live under it. Production is a pointer on the project, not a separate environment object.

## Create

```bash
curl -sS -X POST "$CELLP_URL/v1/projects" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"id":"my-shop","git_remote":"https://github.com/acme/my-shop"}'
```

`id` is the **project id** used in ingress Host names: `{project}.{base-domain}` (prod) and `{version}.{project}.{base-domain}` (preview). Path prefixes `/{id}/` are **deprecated** — use [Host-based routing](/concepts/routing).

`git_remote` is optional **metadata**. cellp never clones it.

## Production pointer

`GET /v1/projects/{id}` returns `prod_version_id` and `prod_url`.

| Situation | Behavior |
|-----------|----------|
| **No prod yet** | The **first** version that reaches `ready` is assigned production automatically (prod Host is created). You do not need a separate “bootstrap promote.” |
| **Prod already set** | New versions are preview-only until you [promote](/concepts/promote) one of them. |

The prod **Host** stays the same across promote; only the backing version changes.

## Listing

`GET /v1/projects` is cursor-paginated (`limit`, `cursor`). Dashboard uses the same endpoint.

## Mental model

```
project: my-shop
  versions: v-seed, pr-42-abc, v-2026-08-30
  prod_version_id: v-2026-08-30    →  Host: my-shop.ingress.local
  preview (pr-42-abc):             →  Host: pr-42-abc.my-shop.ingress.local
```

There are no “Preview / Production / Development” environment records like Vercel env targets. Isolation is **per version**. Env vars are [per version](/guides/environment-variables) as well.
