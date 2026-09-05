# offshoot (data plane)

**offshoot** is the copy-on-write layer cellp uses to **fork and promote application data** between versions. It is not something Worker authors configure in `wrangler.jsonc`.

## What it does for you

| Operation | Meaning |
|-----------|---------|
| **Branch** | Child version gets D1/KV/R2/Queue state derived from a parent version’s bucket |
| **Promote** | Production cutover updates data-plane pointers as part of the [promote saga](/concepts/promote) |
| **Export / import** | Root versions can seed D1 from SQLite files via the orchestrator + celld |

cellp runs offshoot as a **CLI integration** next to celld. The Dashboard and your Worker never talk to offshoot directly.

## What you configure instead

- **Parent version** — `parent_version_id` on `POST /versions`
- **Artifacts and seeds** — upload bundle + optional D1 seed for root deploys ([Platform data](/build/data))
- **Storage** — private S3 (**RustFS** in self-hosted stacks) holds artifacts, per-version celld blobs, and offshoot objects

## Mental model

```
version A (parent)  ──branch──►  version B (child preview)
       │                              │
       └── S3 prefix + offshoot ──────┘  copy-on-write, not a full copy
```

For binding-level behavior and the parent/child matrix, see [Data fork](/concepts/data-fork).

Technical storage tiers and RustFS notes for operators: [Self-hosting](/guides/self-hosting) · [Limits](/reference/limits).
