# Rollback

There is no hidden “instant alias” besides **promote**. Rolling back production means promoting a **previous** version that still exists.

## Decision table

| What you see | What to do |
|--------------|------------|
| New Worker is live on `/{project}/` | Confirm `prod_version_id` |
| Previous version is still `ready` | **Re-promote** it (fastest) |
| Previous version is `archived` | **Wake**, wait for `ready`, promote |
| Previous version is `destroyed` | Cannot roll back. Redeploy from artifact |

## Fast path (still ready)

```bash
curl -sS -X POST \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$CELLP_URL/v1/projects/$PROJECT/versions/$OLD_VERSION/promote"
```

Verify:

```bash
curl -sS "$GATEWAY/$PROJECT/" | head
# or GET /v1/projects/$PROJECT → prod_version_id
```

## Archived previous prod

```bash
curl -sS -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$CELLP_URL/v1/projects/$PROJECT/versions/$OLD_VERSION/wake"

# poll GET …/versions/$OLD_VERSION until status=ready

curl -sS -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$CELLP_URL/v1/projects/$PROJECT/versions/$OLD_VERSION/promote"
```

## Practice

Pin the previous production for an hour after a risky promote so wake is unnecessary. See [Archive](/concepts/archive).

Do not destroy versions you might need to roll back to.

Internal runbook in the repo: [`docs/runbooks/rollback.md`](https://github.com/KonghaYao/cellp/blob/main/docs/runbooks/rollback.md).
