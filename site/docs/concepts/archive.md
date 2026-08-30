# Archive & wake

A `ready` version is a live process. That does not scale if every PR from last month is still running.

## Archive

```bash
curl -sS -X POST \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$CELLP_URL/v1/projects/my-shop/versions/pr-42-abc/archive"
```

Effects:

- celld **stops**
- Local watch cache is dropped
- Object storage **kept**
- Preview URL returns **`503 version_archived`**
- The version can still be a **branch parent**

You cannot archive production (`422`). Pinned versions refuse archive (`409`).

## Wake

```bash
curl -sS -X POST \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$CELLP_URL/v1/projects/my-shop/versions/pr-42-abc/wake"
```

Poll until `status` is `ready` again. Gateway does **not** lazily wake on the first request — wake is explicit. That is a v1 honesty constraint, not a hidden edge cache.

## Idle reaper

Unpinned, unused previews are archived after an idle window (on the order of tens of minutes; production gets a longer grace after promote). Disable with `CELLP_ARCHIVE_REAPER=0` if you are debugging.

## Pin / unpin

```bash
curl -sS -X POST -H "Authorization: Bearer $ADMIN_TOKEN" \
  "$CELLP_URL/v1/projects/my-shop/versions/v-qa-long/pin"
```

Use pin for:

- Shared staging seeds you fork PRs from
- A QA version stakeholders will hit for days
- The previous production you want hot for instant rollback

Unpin when that job is done so the reaper can reclaim the process.
