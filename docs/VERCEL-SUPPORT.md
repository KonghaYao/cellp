# Vercel open source on cellp

> **Status:** secondary track (after Cloudflare Workers + Agent Cloud) · **Updated:** 2026-09-02  
> **Plan:** [VERCEL-FRAMEWORK-ON-CELLP.md](./plans/VERCEL-FRAMEWORK-ON-CELLP.md) · **Agents:** [AGENT-SUPPORT.md](./AGENT-SUPPORT.md) (fx on Workers)

cellp is **not** Vercel. This catalog tracks **Vercel Labs / Vercel OSS** when the **deploy artifact** can be a **wrangler bundle** on **celld**, or when we only study architecture for a future bridge.

**Legend:** **✅** validated path · **⚠️** partial / testing · **🔜** planned research · **❌** out of scope on cellp today

---

## Core stack

| Project | URL | Role | On cellp |
|---------|-----|------|----------|
| **Next.js** | [vercel/next.js](https://github.com/vercel/next.js) | React framework | **❌** Not a hosted Next platform · **⚠️** pre-built OpenNext → single Worker ([NEXT-OPENNEXT-CELLP.md](./plans/NEXT-OPENNEXT-CELLP.md)) |
| **AI SDK** | [vercel/ai](https://github.com/vercel/ai) | TypeScript AI toolkit (`ai`, `@ai-sdk/*`) | **⚠️** Runs **inside** your Worker bundle if you wire providers; no Vercel account required · provider keys are yours |
| **Workflow SDK** | [vercel/workflow](https://github.com/vercel/workflow) | Durable steps / hooks (Workflow DevKit) | **🔜** Vercel-managed runtime ≠ cellp · compare to **CF Workflows** on celld (**Partial**) · see [workflow-examples](https://github.com/vercel/workflow-examples) |
| **Vercel Workflows (product)** | [docs](https://vercel.com/docs/workflows) | Managed durable platform | **❌** Not self-hosted on cellp |

---

## Agents & coding

| Project | URL | Role | On cellp |
|---------|-----|------|----------|
| **fx** | [vercel-labs/fx](https://github.com/vercel-labs/fx) (~2.5k★) | Zig CLI + wasm embed (`createFxAgent`, `createFxTerminal`) · [fx.sh](https://fx.sh) | **⚠️** **P1** on Workers via [codingstark-dev/fx-on-workers](https://github.com/codingstark-dev/fx-on-workers) (`A04`) · needs **AI Gateway** key for model calls |
| **Eve** | [vercel/eve](https://github.com/vercel/eve) | Filesystem-first production agents (Sandbox, Connect) | **🔜** Research — default deploy is **Vercel**, not wrangler |
| **open-agents** | [vercel-labs/open-agents](https://github.com/vercel-labs/open-agents) | Cloud agent template (Workflow SDK + sandbox + tools) | **🔜** Research — assumes Vercel sandbox / managed workflow |

---

## Related (not tier 1)

| Project | Notes |
|---------|--------|
| [supermemoryai/cloudflare-saas-stack](https://github.com/supermemoryai/cloudflare-saas-stack) | Next-on-Pages pattern; hybrid Pages + D1 |
| [nicolasmontone/fx-inside-function](https://github.com/nicolasmontone/fx-inside-function) | fx wasm on **Vercel Function** — precursor to fx-on-workers |

---

## Validation queue (Vercel track)

| Order | Focus | cellp action |
|------:|-------|--------------|
| — | **fx wasm on Workers** | `deploy-support-app.sh A04` · [AGENT-SUPPORT §P1](./AGENT-SUPPORT.md) |
| 1 | OpenNext single Worker | Follow [NEXT-OPENNEXT-CELLP.md](./plans/NEXT-OPENNEXT-CELLP.md) when scheduled |
| 2 | AI SDK + provider in thin Worker | Community pattern; no dedicated ID yet |
| 3 | Workflow SDK vs CF Workflows | Architecture spike only |

---

## Honest gaps

1. **Vercel Sandbox / Connect / Fluid** — no cellp equivalent; Eve and open-agents assume them.  
2. **Vercel AI Gateway** — fx-on-workers uses `ai-gateway.vercel.sh`; on cellp you supply keys or replace the transport.  
3. **Workflow SDK** — durable semantics may map to **CF Workflows** binding, not a port of Vercel’s managed executor.  
4. **Next.js SSR** — still **not** AD-10 target; only **pre-built** Worker + assets.

---

## Links

- User migration: [vercel-migration.md](./vercel-migration.md) · public [From Vercel](https://konghayao.github.io/cellp/migrate/) (if exists) or compare  
- Framework tiers (CF-first): [framework-coverage-cellp.md](./framework-coverage-cellp.md) · [site migrate/frameworks](https://konghayao.github.io/cellp/migrate/frameworks)
