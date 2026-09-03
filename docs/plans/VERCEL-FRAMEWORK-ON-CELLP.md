# Vercel Framework on cellp (secondary track)

> **Status:** planned / research · runs **after** Workers-first support and [Coding Agent on cellp](./CODING-AGENT-ON-CELLP.md) P0  
> **Catalog:** [VERCEL-SUPPORT.md](../VERCEL-SUPPORT.md)  
> **Updated:** 2026-09-02

## Why a separate track

Vercel’s open ecosystem (**Next.js**, **AI SDK**, **Workflow SDK**, **Eve**, **fx**) is built around **Vercel’s control plane** (deployments, sandbox, managed workflows, AI Gateway). **cellp** is a **private Workers control plane** (version + data fork + promote on **celld**).

This track answers: *which Vercel OSS projects can ship as **wrangler artifacts** on cellp, and which are architecture-only?*

---

## Principles

1. **No Vercel account required** for cellp itself — same as [AD-10](../decisions.md#15-ad-10--产品边界权威否定与核心范畴).  
2. **Pre-build, then POST /versions** — cellp does not run `next build` or Vercel CLI deploy.  
3. **Workers runtime only** — Node serverless on Vercel ≠ celld; map only what fits Workers + bindings.  
4. **Honest ❌** — managed Vercel Workflows platform, Vercel Sandbox, and default Eve deploy are **not** cellp products.

---

## Ecosystem map (summary)

See full table in **[VERCEL-SUPPORT.md](../VERCEL-SUPPORT.md)**.

| Area | Flagship OSS | cellp stance |
|------|--------------|--------------|
| Web framework | Next.js | ❌ host · ⚠️ OpenNext pre-build |
| AI libraries | [vercel/ai](https://github.com/vercel/ai) | ⚠️ in-bundle |
| Durable logic | [vercel/workflow](https://github.com/vercel/workflow) | 🔜 vs CF Workflows |
| Coding agent | [vercel-labs/fx](https://github.com/vercel-labs/fx) | ⚠️ **P1** Workers port |
| Production agents | [vercel/eve](https://github.com/vercel/eve) | 🔜 research |
| Agent template | [vercel-labs/open-agents](https://github.com/vercel-labs/open-agents) | 🔜 research |

**fx chain:** upstream **[vercel-labs/fx](https://github.com/vercel-labs/fx)** → embed wasm → community **[fx-on-workers](https://github.com/codingstark-dev/fx-on-workers)** → cellp **`A04`** (shared with Agent track).

---

## Phases

| Phase | Goal | Exit |
|-------|------|------|
| **V0 — fx** | `A04` ready + preview Host | [AGENT-SUPPORT §P1](../AGENT-SUPPORT.md) |
| **V1 — AI SDK** | Document minimal Worker + `@ai-sdk/openai` (or gateway) on cellp | VERCEL-SUPPORT row ✅/⚠️ |
| **V2 — OpenNext** | One reproducible Next → single Worker path | [NEXT-OPENNEXT-CELLP.md](./NEXT-OPENNEXT-CELLP.md) evidence |
| **V3 — Workflow SDK** | Decision doc: map to CF Workflows or defer | ADR |
| **V4 — Eve / open-agents** | Gap list vs sandbox + cellp version model | VERCEL-SUPPORT update |

---

## Non-goals

- Replacing Vercel dashboard, Analytics, or Edge Config as hosted services  
- Promising **Workflow SDK** parity without CF Workflows proof on celld  
- Listing Next.js as tier-1 on cellp ([AD-13](../decisions.md#18-ad-13--前端框架一等公民与-nextjs-边界))

---

## Public doc

English summary (when published): `site/docs/research/vercel-on-cellp.md`
