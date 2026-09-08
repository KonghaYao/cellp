# celld Native Product — Git Object Recovery

**Status:** BLOCKED — product commits were never pushed to `KonghaYao/celld`.

## Required SHAs

| SHA | Superproject ref |
|-----|------------------|
| `6ce39901a64edc79ee7607860a1fc862ae3d7bca` | `b98a2ba` |
| `84319111037b521aec11067e21564e2b8e0c813a` | `46f3e6a` |

`git fetch origin <sha>` → **not our ref**. Until recovered: merge cellp+e2e to `main`; v18 G-REL stays blocked.

## Verify after recovery

```bash
cd celld && git checkout 6ce39901a64edc79ee7607860a1fc862ae3d7bca
test -d crates/celld/native && cargo fmt --all -- --check
```
