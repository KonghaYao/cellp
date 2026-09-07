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
- Preview URL returns **`503 version_archived`** (prod Host is unaffected)
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

## Elastic serving (unsupported internal scaffold)

`CELLP_ELASTIC_RUNTIME` is off by default and currently exposes only an **unsupported internal E1–E5 scaffold**. It does not provide a remote HTTP+mTLS Node Agent, real celld lifecycle management, a Scheduler or complete 0→N scaling, and it is not production-ready. Do not enable it to operate workloads.

A future productized elastic runtime may let a version be **cold**—no live serving replica—while object storage is retained. That is **not** the same as **archived** in the v1 API: archive means celld stopped and preview returns `503` until [wake](/concepts/archive#wake).

For supported releases, use **archive** and **wake** to stop and restart processes. See [Versions — one process per ready version](/concepts/versions#one-process-per-ready-version).
