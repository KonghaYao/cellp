# cellp

**Self-hosted Workers control plane** — version **app + data** (D1/KV/R2/Queue) on every deploy. Preview **branches** bindings; the **first** `ready` version becomes production automatically, later cutovers via **promote**. Not Cloudflare edge, Vercel, or a Git host — you keep Git, CI, and TLS; cellp runs artifacts on **celld** and routes by Host. [How it works](https://konghayao.github.io/cellp/how-it-works).

> **`celld/`** → [KonghaYao/celld](https://github.com/KonghaYao/celld) ([celld.dev](https://celld.dev) lineage, extended for cellp). Use the celld build that matches your cellp release.

```bash
curl -fsSL https://raw.githubusercontent.com/KonghaYao/cellp/main/scripts/install.sh | sh
export PATH="$HOME/.local/bin:$PATH"
cellp doctor && cellp dev
```

**Docs:** [konghayao.github.io/cellp](https://konghayao.github.io/cellp/) · [Install](https://konghayao.github.io/cellp/guides/install) · [From Cloudflare](https://konghayao.github.io/cellp/migrate/cloudflare) · [Supported stacks](https://konghayao.github.io/cellp/migrate/stacks) · [Compare](https://konghayao.github.io/cellp/compare)

## Cloudflare Workers compatibility

**You keep the Workers programming model.** APIs and binding names come from that **cellp-maintained celld**. **cellp** adds version lifecycle, preview/prod routing, and **binding branch** on preview versions — it does not replace the Workers engine.

Authoritative gap list: **[celld Cloudflare compat](./celld/docs/cloudflare-compat.md)**.

| Capability | Status | Notes |
|------------|--------|--------|
| Workers (`fetch`, runtime APIs) | Partial | See compat doc |
| Static assets (wrangler `assets`) | Yes | SPA / Workers Sites-style bundles |
| D1 | Partial | Root import; preview **branch** |
| KV · R2 · Queues | Partial | Preview **branch** from parent |
| Workflows · Cron | Partial | Not branched across versions (by design) |
| Durable Objects | Partial | Check compat before load-bearing DO |
| Workers AI · Vectorize · Hyperdrive · Browser · Email · Python Workers | No | |

**Deploy path:** there is no `wrangler deploy` target for cellp. CI builds a wrangler bundle → upload to your storage → `POST /v1/projects/{project}/versions` (deploy token). See [From Cloudflare](https://konghayao.github.io/cellp/migrate/cloudflare).

**Scope:** one Worker entrypoint per version (no multi-Worker `[[services]]` orchestration in cellp). Next.js / Node SSR is out of scope — use pre-built Workers bundles ([framework tiers](https://konghayao.github.io/cellp/migrate/frameworks)).

---

## Frameworks & community apps

We validate real open-source Workers apps and common framework stacks on cellp before listing them as supported on the docs site.

**Legend:** **✅** works on cellp · **⚠️** gaps, partial, or in progress · **⏸️** paused / not a current focus · **❌** blocked or out of scope for now

### Frameworks

| Framework | Status | Notes |
|-----------|:------:|-------|
| Astro | ✅ | `@astrojs/cloudflare` |
| SvelteKit | ✅ | `adapter-cloudflare`, one Worker per deploy |
| React / Vue (Vite) | ✅ | Static SPA + wrangler `assets` |
| Hono | ✅ | API or API + static assets |
| Remix | ✅ | `@remix-run/cloudflare` |
| Nuxt | ✅ | Nitro `cloudflare` preset |
| SolidStart | ✅ | |
| Qwik City | ✅ | |
| Waku | ✅ | |
| Next.js | ⏸️ | OpenNext / vinext only; not a Next hosting platform on cellp |

Full matrix and migration notes: **[Supported stacks](https://konghayao.github.io/cellp/migrate/stacks)**.

### Notable apps (sample)

| Project | Kind | Status |
|---------|------|:------:|
| Memos, Monolith, SonicJS, FlareMo, EdgeEver, Relay | Apps / CMS / tools | ✅ |
| workflows-starter, R2-Explorer, r2filebox, FileWorker | Demos / R2 / Workflows | ✅ |
| Sink, CloudFlare-ImgBed, CloudPaste, OpenStatus, microfeed | Popular Workers OSS | ✅ |
| cloudflarebase, pastebin-worker, request-bin | Multi-worker or edge cases | ⚠️ |
| UptimeFlare, Supermemory, Triplit, Serverless DNS, Counterscale | Various | ⚠️ / ❌ |

More detail: [Compare](https://konghayao.github.io/cellp/compare) on the docs site.

---

## Coding agents & Vercel OSS (research)

cellp is exploring a **private control plane** for agent loops: build → deploy version → **preview with forked D1/KV/R2** → promote. We track Cloudflare **Agent Cloud** and common agent stacks (Agents SDK, Workflows, MCP, Pi, Mastra, LangGraph, etc.) for what can run on **celld + cellp** without a Cloudflare account.

| Area | Status | Notes |
|------|--------|-------|
| Cloudflare Agents / Agent Cloud | Research | agents-starter paths in contributor lab; partial |
| Mastra on Workers | Lab only | Fixture on celld; you bring LLM keys—not a product support tier |
| Pi, Deep Agents, Cloudflare OS | Research | Not offered as product capability |
| Vercel AI SDK (in your Worker) | Partial | Your keys and providers |
| Vercel Workflow SDK · fx on Workers | Research / Lab only | Workflows vs celld; fx port partial in lab |
| Next.js, Eve, Vercel open-agents | Research | Not Vercel-style hosting on cellp |

**Lab only** = lab fixture checks, not a hosting promise · **Research** = exploration only · **Partial** = some paths work; see docs

**Public write-ups:** [Research overview](https://konghayao.github.io/cellp/research/) · [Coding agent on cellp](https://konghayao.github.io/cellp/research/coding-agent-on-cellp) · [Vercel on cellp](https://konghayao.github.io/cellp/research/vercel-on-cellp)

## License

**Apache License 2.0** — see [`LICENSE`](./LICENSE). Submodule **`celld/`** ([KonghaYao/celld](https://github.com/KonghaYao/celld)): [`celld/LICENSE`](./celld/LICENSE).
