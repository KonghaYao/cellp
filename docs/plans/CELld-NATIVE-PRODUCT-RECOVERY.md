# celld Native Product — Git Object Recovery

**Status:** **REBUILD** — product commits were never pushed to `KonghaYao/celld`; local objects are gone (worktree `celld` @ `9b52684`, no `crates/celld/native/`).

## Required SHAs (historical; not recoverable from origin)

| SHA | Superproject ref |
|-----|------------------|
| `6ce39901a64edc79ee7607860a1fc862ae3d7bca` | `b98a2ba` |
| `84319111037b521aec11067e21564e2b8e0c813a` | `46f3e6a` |
| `44a3259bf2e50ade2204f978108e89b630de4b2e` | qualification G-Q |

`git fetch origin <sha>` → **not our ref**; `git cat-file` in submodule → missing. **Authoritative rebuild** on celld branch `adlc/native-wasm-runtime-product-r1`, spec from `.peri/adlc/tasks/2026-09-06-native-wasm-runtime-delivery/` + `e2e/scripts/v18-native-wasm.sh`.

Until celld rebuild lands and superproject gitlink updates: **v18 / G-REL wasm gate blocked** (cellp+e2e on `main` already merged).

## Verify after recovery

```bash
cd celld && git checkout 6ce39901a64edc79ee7607860a1fc862ae3d7bca
test -d crates/celld/native && cargo fmt --all -- --check
```
