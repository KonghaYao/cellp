# Coding Agent on cellp

cellp explores a **private control plane** for coding agents: build → `POST /versions` → **preview with forked D1/KV/R2** → promote. We align with Cloudflare **[Agent Cloud](https://agents.cloudflare.com/)** ([docs](https://developers.cloudflare.com/agents/)) — not a Cloudflare account on your metal.

## P0 — deploy on cellp (ordered)

1. [cloudflare/agents-starter](https://github.com/cloudflare/agents-starter)  
2. [qaml-ai/pi-worker](https://github.com/qaml-ai/pi-worker) (`examples/hello-agent`)  
3. [southpolesteve/opencode-do](https://github.com/southpolesteve/opencode-do)  

**P1:** [fx on Workers](https://github.com/codingstark-dev/fx-on-workers) — [fx.sh](https://fx.sh) coding agent (`A04`).

Contributor runbook: **[AGENT-SUPPORT.md](https://github.com/KonghaYao/cellp/blob/main/docs/AGENT-SUPPORT.md)** (`A01` … `A05`).

## Agent & platform targets (research 🔜)

| Platform | | Notes |
|----------|:-:|-------|
| **[fx on Workers](https://github.com/codingstark-dev/fx-on-workers)** | ⚠️ | P1 · fx.sh wasm agent · not Agent Cloud |
| **[Mastra](https://mastra.ai/)** | ✅ | A05 · prod **v14** · strict acceptance pass || **[Cloudflare Agents](https://agents.cloudflare.com/)** | 🔜 | Agent Cloud platform |
| **[Cloudflare OS](https://github.com/cloudflare/cloudflare-os)** (~8.6k★) | 🔜 | Workspace north-star |
| **[Eve](https://github.com/vercel/eve)** (Vercel) | 🔜 | [eve.dev/docs](https://eve.dev/docs) |
| **[Pi](https://github.com/earendil-works/pi)** | 🔜 | [pi.dev](https://pi.dev) · `pi-agent-core` in CF OS |
| **[Deep Agents](https://github.com/langchain-ai/deepagents)** (LangChain) | 🔜 | LangGraph harness |

**⚠️** = partial validation / blocked — **not supported yet** · **🔜** = planned / research — **not supported yet**

Contributor plan: **[CODING-AGENT-ON-CELLP.md](https://github.com/KonghaYao/cellp/blob/main/docs/plans/CODING-AGENT-ON-CELLP.md)** · **[AGENT-SUPPORT.md](https://github.com/KonghaYao/cellp/blob/main/docs/AGENT-SUPPORT.md)** · [Star queue](https://github.com/KonghaYao/cellp/blob/main/docs/support-star-queue.md)

## Related

- [Why cellp](/why) · [From Cloudflare](/migrate/cloudflare) · [Community matrix](https://github.com/KonghaYao/cellp/blob/main/docs/support-matrix.md)
