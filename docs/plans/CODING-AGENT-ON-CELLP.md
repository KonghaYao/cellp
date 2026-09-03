# Coding Agent on cellp (frontier)

> **Status:** exploratory · not a shipped product feature  
> **Updated:** 2026-09-02  
> **Related:** [support-star-queue.md](../support-star-queue.md) · [support-matrix.md](../support-matrix.md) · **[AGENT-SUPPORT.md](../AGENT-SUPPORT.md)** (OSS that already combine agents + Workers)

## Thesis

**Coding agents** on **Cloudflare** are productized as **[Agent Cloud](https://agents.cloudflare.com/)** ([docs](https://developers.cloudflare.com/agents/)): `Agent` + Durable Objects + Workflows + Workers AI / AI Gateway + MCP + **Dynamic Workers**. **cellp** versions **app + data** on every deploy with a real preview fork — the private control plane when agent output is a wrangler bundle:

```
Agent proposes change → CI builds artifact → POST /versions → preview Host
→ human or agent tests → promote (or branch again)
```

On SaaS, preview is vendor-bound. On cellp, preview is **your** RustFS + offshoot branch.

## Research targets (🔜 planned — not supported yet)

| Platform | Link | Role | Why study for cellp |
|----------|------|------|---------------------|
| **fx on Workers** | [codingstark-dev/fx-on-workers](https://github.com/codingstark-dev/fx-on-workers) | [fx.sh](https://fx.sh) agent: libfx wasm + DO + shell tool | **P1 deploy (`A04`)** — distinct from [Agent Cloud](https://agents.cloudflare.com/) |
| **Cloudflare Agents** | [agents.cloudflare.com](https://agents.cloudflare.com/) · [SDK](https://github.com/cloudflare/agents) | Official Agent Cloud | P0 via agents-starter |
| **Cloudflare OS** | [cloudflare/cloudflare-os](https://github.com/cloudflare/cloudflare-os) (~8.6k★) | Workspace agents, Gadgets, Gatekeepers | North-star · Dynamic Workers + multi Worker |
| **Eve** (Vercel) | [vercel/eve](https://github.com/vercel/eve) | Filesystem-first production agents | Non-CF deploy; architecture compare |
| **Pi** | [earendil-works/pi](https://github.com/earendil-works/pi) | Terminal harness; `pi-agent-core` in CF OS | Harness + pi-worker P0 |
| **Deep Agents** (LangChain) | [langchain-ai/deepagents](https://github.com/langchain-ai/deepagents) | LangGraph harness | Workers deploy TBD |

**Status:** none of the above are **supported on cellp** today. **🔜** = active research / future integration.

## Workers runtime reference (secondary)

| Project | Notes |
|---------|--------|
| [supermemoryai/cloudflare-saas-stack](https://github.com/supermemoryai/cloudflare-saas-stack) | SaaS template; Pages/Next hybrid |
| High-star OSS apps | [support-star-queue.md](../support-star-queue.md) (Sink, Counterscale, …) |

## Open questions (honest)

1. **Agent Cloud bindings** — Workers AI, AI Gateway, Vectorize, Browser Rendering vs celld **No** / **Partial** ([compat](../../celld/docs/cloudflare-compat.md)).
2. **Dynamic Workers / Facets** — Cloudflare OS + Agent Cloud runtime features beyond static wrangler bundles.
3. **Durable Objects** — celld **Partial**; load-bearing DO must be probed per stack (P0).
4. **Multi `[[services]]`** — Gatekeeper fleet; cellp does not orchestrate ([MULTI-WORKER-DEPLOY.md](./MULTI-WORKER-DEPLOY.md)).
5. **Version churn** — Many agent iterations/hour vs archive/wake and storage (`stress/`).

## Proposed phases

| Phase | Goal | Exit |
|-------|------|------|
| **P0 — Deploy** | **A01** agents-starter → **A02** pi-worker → **A03** opencode-do | [AGENT-SUPPORT §P0](../AGENT-SUPPORT.md#p0--cellp-deploy-support-validation-ordered) |
| **P1 — fx** | **A04** [fx-on-workers](https://github.com/codingstark-dev/fx-on-workers) ([fx.sh](https://fx.sh)) | AGENT-SUPPORT §P1 · wasm + DO |
| **R0 — Research** | [Agent Cloud](https://agents.cloudflare.com/), Eve, Pi, Deep Agents, Cloudflare OS vs cellp CD | This doc + README |
| **R1 — Agent loop sketch** | External agent → build → `POST /versions` | `dev/scripts/` or `e2e/` spike |
| **R2 — Cloudflare OS** | Gap analysis vs celld | Roadmap |
| **R3 — Eve mapping** | Non-CF agents → Workers + cellp | ADR or defer |

No frozen RPC until a repeatable deploy exists.

## Non-goals (this track)

- Hosted multi-tenant “cellp Agent Cloud”
- Replacing Cursor, Claude Code, Eve UIs, or the [Agent Cloud dashboard](https://agents.cloudflare.com/)
- Promising Cloudflare OS parity before runtime gaps are measured

## Evidence

```bash
mkdir -p docs/evidence
# future: coding-agent-*.log (gitignored)
```

## Links

- [Cloudflare OS README](https://github.com/cloudflare/cloudflare-os)
- [Introducing Eve](https://vercel.com/blog/introducing-eve)
- [Deep Agents docs](https://docs.langchain.com/oss/python/deepagents/overview)
- [Why cellp](https://konghayao.github.io/cellp/why)
