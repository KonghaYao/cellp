# Rollback

There is no hidden “instant alias” besides **promote**. Rolling back production means promoting a **previous** version that still exists.

## Decision table

| What you see | What to do |
|--------------|------------|
| New Worker is live on **prod Host** | Confirm `prod_version_id` |
| Previous version is still `ready` | **Re-promote** it (fastest) |
| Previous version is `archived` | **Wake**, wait for `ready`, promote |
| Previous version is `destroyed` | Cannot roll back. Redeploy from artifact |

## Fast path (still ready)

```bash
curl -sS -X POST \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$CELLP_URL/v1/projects/$PROJECT/versions/$OLD_VERSION/promote"
```

`CELLP_URL` must include `/v1`. Promote returns **202** while the saga runs; poll project or version state until `prod_version_id` updates.

Verify production Host routing (not deprecated path URLs):

```bash
curl -sS -H "Host: ${PROJECT}.ingress.local" "http://127.0.0.1:8787/" | head
# or GET /v1/projects/$PROJECT → prod_version_id, prod_url
```

See [Preview & production](/concepts/preview) for Host patterns in your environment.

## Archived previous prod

```bash
curl -sS -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$CELLP_URL/v1/projects/$PROJECT/versions/$OLD_VERSION/wake"

# poll GET …/versions/$OLD_VERSION until status=ready

curl -sS -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$CELLP_URL/v1/projects/$PROJECT/versions/$OLD_VERSION/promote"
```

## Practice

After a risky promote, keep the previous production **pinned** for a window (`POST …/pin` or `CELLP_ROLLBACK_KEEP` on cellpd) so wake is unnecessary. See [Archive](/concepts/archive).

Do not destroy versions you might need to roll back to.
