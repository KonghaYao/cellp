# Data fork (parent & child versions)

A **child** version is created with `parent_version_id`. cellp treats that deploy as a **fork**: new **application** code (Worker bundle) from **this** artifact, and **copy-on-write data** from the parent.

## What forks

| Piece | Child version |
|-------|----------------|
| **Worker script** | From **this** upload (not a diff of the parent) |
| **D1** | Branch from parent bucket (`fork_txid` cut point) |
| **KV** | Namespace branch; large values can chain to parent blobs |
| **R2** | Prefix overlay + tombstones on the child |
| **Queue** | Branch; messages enqueued in preview stay in preview |
| **Workflow instances** | **Not** branched — empty for this version |
| **Cron schedules** | Defined by **this** script; **only production** runs `scheduled` handlers while a version is prod |

Binding **names and ids** in `wrangler.jsonc` stay aligned with the parent so `env.DB`, `env.CACHE`, and so on keep working without code changes.

## Copy-on-write (offshoot)

Under the hood, cellp uses **offshoot** for SQLite-style branching and coordinates celld for D1/KV/R2/Queue. You do not call offshoot from application code. The operator-facing idea is simple: **preview data is a fork**, not a shared live database.

Root versions (no parent) use **D1 import** or empty bindings as configured for your first seed—not a fork.

## Timeline: not “live prod + PR code”

When the child is created, data bindings reflect the parent as of **`fork_txid`** (D1’s branch point). Writes on the parent **after** the child exists stay on the parent; the preview does **not** see them.

Use a **staging seed** or pinned non-prod parent for PR previews. Forking live production is rejected or scrubbed in some cases on purpose.

Read: [Preview data snapshot timeline](/concepts/preview#data-snapshot-timeline) · [What promote does not do](/concepts/promote#what-promote-does-not-do).

## Cron and workflows

- **Cron:** Preview-ready versions do **not** run cron. After **promote**, scheduling moves to the new production version. See [Cron](/bindings/cron).
- **Workflows:** New instances start from this deploy; parent workflow state is not copied.

## Operator surfaces

Forking happens during version **start** (orchestrator). There is no separate “inherit bindings” button. Per-binding behavior: [Bindings](/concepts/bindings) and the guides under [Bindings](/bindings/d1).
