# Mastra on cellp (A05)

Classic **Mastra** demo without Studio:

- **Agent** `weather-agent` with **Tool** `get-weather` (Open-Meteo)
- **Workflow** `weather-workflow`: fetch forecast → **R2** cache (`CACHE`) → Agent suggests activities
- Registered **Tool** `get-forecast-cache` reuses the Workflow cache path for deterministic R2 miss → hit validation
- **D1** `D1Store` on binding `DB` backs Mastra Memory
- Custom static frontend in `public/`; this is not Mastra Studio
- **LLM** uses an OpenAI-compatible endpoint. Committed config contains only the safe placeholder `OPENAI_API_KEY=public`; configure a real key out-of-band when needed

## Deploy and verify

```bash
./dev/scripts/health.sh
export SUPPORT_VERSION="v-mastra-$(date +%s)"
export SUPPORT_SKIP_VERSION_PICK=1
./dev/scripts/deploy-support-app.sh A05
./dev/examples/support-mastra/acceptance.sh "$SUPPORT_VERSION"
```

`acceptance.sh` is strict. It separately verifies:

1. Agent and Tool registries
2. A real Agent generation (fallback does not count)
3. Real weather Tool execution
4. R2 cache miss → hit with a unique namespace
5. Workflow completion and `planningSource`
6. Mastra Memory thread/message write and read
7. The same Memory marker through cellp's authenticated version D1 query API

The Workflow reports `planningSource=agent` after a real model response. If the public endpoint returns HTTP 429, it reports `planningSource=rate-limit-fallback` so the orchestration remains demonstrable, but the separate Agent check still fails and the script exits non-zero.

## Current evidence

`v14` (2026-09-04, prod) passes `./dev/examples/support-mastra/acceptance.sh v14` with an OpenAI-compatible model (`composer-2.5` in local re-verify): Agent, Tool, Workflow (`planningSource=agent`), R2 miss → hit, Memory/D1, and cellp D1 query API. Earlier `v13` used OpenCode `big-pickle` and failed strict Agent with HTTP 429. Evidence: `docs/evidence/support-A05.log`.

Deploy with LLM env (example — set key out-of-band, never commit secrets):

```bash
export OPENAI_BASE_URL='https://cursor2api.freetavily.deno.net/v1'
export OPENAI_MODEL='composer-2.5'
export OPENAI_API_KEY='…'
export SUPPORT_VERSION="v-$(date +%s)"
export SUPPORT_SKIP_VERSION_PICK=1
./dev/scripts/deploy-support-app.sh A05
./dev/examples/support-mastra/acceptance.sh "$SUPPORT_VERSION"
```

## Build adapter

Mastra emits a multi-module Cloudflare artifact. `prepare-artifact.sh` performs:

1. `mastra build`
2. Merge the generated deployer Wrangler config with `wrangler.cellp.jsonc`
3. Wrangler 4 dry-run bundling into one ignored `.cellp-bundle/index.js`
4. Slim staging without `node_modules`

Telemetry is disabled for this demo via `MASTRA_TELEMETRY_DISABLED=1`. Mastra still inlines `posthog-node`; celld's `node:zlib` callback API supports its top-level `promisify(gzip)` path. See `docs/cf-worker-js-compat.md`.

| Check | Path |
|-------|------|
| Static shell | `GET /` |
| Agent registry/generate | `GET /api/agents` · `POST /api/agents/weather-agent/generate` |
| Tool registry/execute | `GET /api/tools` · `POST /api/tools/:toolId/execute` |
| Workflow | `POST /api/workflows/weather-workflow/start-async` |
| Memory | `/api/memory/threads` · `/api/memory/save-messages` |
