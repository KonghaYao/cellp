# Coding Agent on cellp

cellp explores a **private control plane** for coding agents: build → `POST /versions` → **preview with forked D1/KV/R2** → promote. We align with Cloudflare **[Agent Cloud](https://agents.cloudflare.com/)** ([docs](https://developers.cloudflare.com/agents/)) — not a Cloudflare account on your metal.

**Status:** research and lab validation only. See [Research](/research/) for scope.

## Lab targets (ordered)

These upstream projects are exercised on the local dev stack in contributor workflows. Order reflects dependency risk (Durable Objects, WebSockets, bundle size), not a public release date.

1. [cloudflare/agents-starter](https://github.com/cloudflare/agents-starter) — partial: SPA and HTTP paths; Workers AI binding removed in lab overlay; chat inference needs your own model wiring on celld
2. [qaml-ai/pi-worker](https://github.com/qaml-ai/pi-worker) (`examples/hello-agent`) — supported in lab
3. [southpolesteve/opencode-do](https://github.com/southpolesteve/opencode-do) — partial: HTTP and SSE; model path uses placeholders unless you add providers

**Next tier (research):** [fx on Workers](https://github.com/codingstark-dev/fx-on-workers) — [fx.sh](https://fx.sh) coding agent in a Worker (wasm + Durable Object session); needs gateway credentials for full model turns.

## Agent & platform targets (research)

| Platform | Status | Notes |
|----------|--------|-------|
| **[Mastra](https://mastra.ai/)** | Lab only | Lab fixture checks (agent, tools, workflows, D1/R2/memory on celld); bring your own LLM keys — not a product support tier |
| **[fx on Workers](https://github.com/codingstark-dev/fx-on-workers)** | Lab only (partial) | fx.sh wasm agent · not Agent Cloud |
| **[Cloudflare Agents](https://agents.cloudflare.com/)** | Research / planned | Agent Cloud platform; lab path via agents-starter |
| **[Cloudflare OS](https://github.com/cloudflare/cloudflare-os)** | Research / planned | Workspace north-star |
| **[Eve](https://github.com/vercel/eve)** (Vercel) | Research / planned | [eve.dev/docs](https://eve.dev/docs) |
| **[Pi](https://github.com/earendil-works/pi)** | Research / planned | [pi.dev](https://pi.dev) · `pi-agent-core` in CF OS |
| **[Deep Agents](https://github.com/langchain-ai/deepagents)** (LangChain) | Research / planned | LangGraph harness |

**Lab only** = contributor lab fixture checks, not a hosting promise · **Research / planned** = not supported as product capability yet

## Related

- [Why cellp](/why) · [From Cloudflare](/migrate/cloudflare) · [Vercel OSS on cellp](/research/vercel-on-cellp)
