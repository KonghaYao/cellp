# Promote

Promote makes a **ready** version the production pointer. It is explicit. Merging to `main` does nothing unless your CI calls this API.

## Call

```bash
curl -sS -X POST \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$CELLP_URL/v1/projects/my-shop/versions/v-2026-08-30/promote"
```

You get back `prod_version_id` and `prod_url`. Gateway `/{project}/` now hits that version.

The version must be `ready`. Archived versions need a [wake](/concepts/archive) first. `409` if it is not ready.

## What happens

Promote is a **saga** with compensation:

1. Validate
2. Drain the old production route
3. Promote data-plane (offshoot) pointers
4. Compare-and-swap `prod_version_id`
5. Activate the new production route

If a step fails, cellp rolls the saga back in reverse. You should not see a split-brain prod pointer.

Cutover is designed to be **short** (seconds), not a multi-minute mesh drain.

## After promote

- Old production stays around (ready or later idle-archived). It is your [rollback](/guides/rollback) candidate.
- New production will **not** be auto-archived.
- Preview URLs for other versions keep working.

## Dashboard

On the version page: **Promote**. Same API.

## CI pattern

Typical `main` pipeline:

1. Build + upload artifact as version `sha-abc`
2. Poll until `ready`
3. Run smoke against `/{project}/sha-abc/`
4. `POST …/promote`
5. Smoke against `/{project}/`

PR pipelines **stop before step 4**. See [Deploy from CI](/guides/ci).
