# Projects

A **project** is the durable name of an application. Versions live under it. Production is a pointer on the project, not a separate environment object.

## Create

```bash
curl -sS -X POST "$CELLP_URL/v1/projects" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"id":"my-shop","git_remote":"https://github.com/acme/my-shop"}'
```

`id` is the path segment you will see on the gateway: `/{id}/` and `/{id}/{version}/`.

`git_remote` is optional **metadata**. cellp never clones it.

## Production pointer

`GET /v1/projects/{id}` returns `prod_version_id` and `prod_url`. Until you [promote](/concepts/promote) something, there is no production.

## Listing

`GET /v1/projects` is cursor-paginated (`limit`, `cursor`). Dashboard uses the same endpoint.

## Mental model

```
project: my-shop
  versions: v-seed, pr-42-abc, v-2026-08-30
  prod_version_id: v-2026-08-30    →  GET /my-shop/
  preview:                         →  GET /my-shop/pr-42-abc/
```

There are no “Preview / Production / Development” environment records like Vercel env targets. Isolation is **per version**. Env vars are [per version](/guides/environment-variables) as well.
