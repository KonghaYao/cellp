# Peri workflows (cellp)

```bash
npx -y @peri-code/workflow@0.2.0 validate scripts/workflows/doc-full-refresh.mjs
npx -y @peri-code/workflow@0.2.0 validate scripts/workflows/doc-full-refresh-verify.mjs
```

## Limits (important)

Full refresh uses **~800+ tool calls**. Launch with:

```json
"maxToolCalls": 1200,
"maxAgents": 20,
"maxElapsedMs": 3600000
```

A run that stops at **400** `maxToolCalls` may still have finished most phases; check `docs/evidence/doc-refresh/SUMMARY.md` anyway.

## `doc-full-refresh.mjs`

Inventory → **6 parallel writers** → **6 parallel verifiers** → build + nav gates → summary.

Evidence: `docs/evidence/doc-refresh/`.

## `doc-full-refresh-verify.mjs`

After manual or partial fixes: **verify + gates only** (~200 tool calls). Use `maxToolCalls: 400` is usually enough.

## Example launch (full)

```json
{
  "scriptPath": "scripts/workflows/doc-full-refresh.mjs",
  "args": { "scope": "full public doc sync" },
  "maxConcurrency": 6,
  "maxToolCalls": 1200,
  "maxAgents": 20,
  "writeIntent": {
    "kind": "write",
    "repo_root": "/path/to/cellp",
    "cwd": "/path/to/cellp",
    "path_allowlist": ["site/", "README.md", "docs/evidence/doc-refresh/"]
  }
}
```
