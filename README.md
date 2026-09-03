# cellp

**Self-hosted Workers control plane — version the app and the data on every deploy.**

Preview forks D1, KV, R2, and Queues. Production is an explicit **promote**. You keep Git, CI, and TLS; cellp receives artifacts, runs isolated **[celld](https://github.com/KonghaYao/celld)** processes, and routes traffic.

```bash
curl -fsSL https://raw.githubusercontent.com/KonghaYao/cellp/main/scripts/install.sh | sh
export PATH="$HOME/.local/bin:$PATH"
cellp doctor && cellp dev
```

**Docs:** [konghayao.github.io/cellp](https://konghayao.github.io/cellp/) · [Install](https://konghayao.github.io/cellp/guides/install) · [From Cloudflare](https://konghayao.github.io/cellp/migrate/cloudflare) · [Vercel on cellp](https://konghayao.github.io/cellp/research/vercel-on-cellp) · [Supported stacks](https://konghayao.github.io/cellp/migrate/stacks)

---

## What cellp is

Private **control plane** on your hardware — **not** a Cloudflare account, **not** “self-hosted Cloudflare,” **not** Vercel or a Git host.

| cellp does | cellp does not |
|------------|----------------|
| Version **App + Data** together; preview **branch**; **promote** / rollback | Global edge, built-in DNS/TLS/CDN/WAF |
| Gateway **Host** ingress (preview + prod) · **WebSocket** Upgrade → celld | User accounts, SSO, RBAC |
| Dashboard + REST (`:8790`) | Hosted cellp cloud |

> **cellp versions Worker + D1/KV/R2/Queue on every deploy** — preview is a real data fork, production is an explicit promote.

**cellp + celld:** celld runs the isolate and bindings; cellp holds the SQLite registry, orchestrator, Gateway, and offshoot-backed forks (**one celld process per ready version**). Storage: **RustFS** (private S3). Full stack: [How it works](https://konghayao.github.io/cellp/how-it-works).

### Coding Agent on cellp (frontier)

cellp explores a **private control plane** for agent loops: build → `POST /versions` → **preview Host with forked D1/KV/R2** → promote. We track Cloudflare’s **Agent Cloud** surface — [agents.cloudflare.com](https://agents.cloudflare.com/) · [Agents docs](https://developers.cloudflare.com/agents/) — (Agents SDK, Durable Objects, Workflows, Workers AI / AI Gateway, MCP, Dynamic Workers) and map what can run on **celld + cellp** without a Cloudflare account.

**Legend:** **✅** validated · **⚠️** testing or blocked today · **🔜** planned on cellp (not supported yet)

**P0 deploy validation (in order):** [agents-starter](https://github.com/cloudflare/agents-starter) → [pi-worker](https://github.com/qaml-ai/pi-worker) `hello-agent` → [opencode-do](https://github.com/southpolesteve/opencode-do). **P1:** [**fx on Workers**](https://github.com/codingstark-dev/fx-on-workers) — [fx.sh](https://fx.sh) coding agent (libfx wasm + DO session). See [AGENT-SUPPORT.md](./docs/AGENT-SUPPORT.md).

| Platform | What | | Notes |
|----------|------|:-:|-------|
| **[Cloudflare Agents](https://agents.cloudflare.com/)** | Official **Agent Cloud** platform (`Agent` class, MCP, Dynamic Workers) | 🔜 | [Agents SDK](https://github.com/cloudflare/agents); P0 via agents-starter |
| **[Cloudflare OS](https://github.com/cloudflare/cloudflare-os)** | Company agent workspace (Gadgets, Gatekeepers, Pi + Code Mode) | 🔜 | ~8.6k★ · Dynamic Workers + multi Gatekeeper Workers · **North-star** |
| **[Pi](https://github.com/earendil-works/pi)** | Terminal coding agent (`pi-agent-core`; used in Cloudflare OS) | 🔜 | Research harness |
| **[Deep Agents](https://github.com/langchain-ai/deepagents)** (LangChain) | LangGraph agent harness | 🔜 | Research — Workers deploy TBD |

**High-star Workers apps** (Sink, Counterscale, CloudPaste, …): see [support-star-queue.md](./docs/support-star-queue.md). **Community matrix:** [support-matrix.md](./docs/support-matrix.md).

Plan: **[CODING-AGENT-ON-CELLP.md](./docs/plans/CODING-AGENT-ON-CELLP.md)** · **[AGENT-SUPPORT.md](./docs/AGENT-SUPPORT.md)** · [Pages](https://konghayao.github.io/cellp/research/coding-agent-on-cellp)

---

## Cloudflare Workers compatibility

**You keep the Workers programming model.** Source shape, wrangler binding names, and most APIs come from **[celld](https://github.com/KonghaYao/celld)** (Rust runtime, git submodule). **cellp** adds version lifecycle, preview/prod Host routing, and **binding branch** on child versions — it does not replace the Workers engine.

Judge your app against **[celld Cloudflare compat](./celld/docs/cloudflare-compat.md)** (authoritative). Summary:

| Capability | In celld | Notes |
|------------|----------|--------|
| Workers (`fetch`, runtime APIs) | **Partial** | Gaps listed in compat doc; fail at deploy or first use |
| **Static assets** (wrangler `assets`) | **Yes** | SPA / Workers Sites-style bundles |
| **D1** | **Partial** | cellp: root **import**, child **branch** |
| **KV · R2 · Queues** | **Partial** | cellp: child **branch** from parent |
| **Workflows · Cron** | **Partial** | Not branched across versions (by design) |
| **Durable Objects** | **Partial** | Read compat before load-bearing DO |
| Workers AI · Vectorize · Hyperdrive · Browser · Email · Python Workers | **No** | |

**Deploy path:** there is no `wrangler deploy` target for cellp. CI builds a wrangler bundle → upload to your RustFS → `POST /v1/projects/{project}/versions`. See [From Cloudflare](https://konghayao.github.io/cellp/migrate/cloudflare).

**cellp limits on top of celld:** one Worker entrypoint per version (no multi-Worker `[[services]]` orchestration); Next.js / Node SSR is out of scope — use pre-built Workers bundles ([framework tiers](https://konghayao.github.io/cellp/migrate/frameworks)).

### Notable projects (community validation)

Real open-source Workers apps and framework stacks we exercise on cellp before calling them supported. **[support-matrix.md](./docs/support-matrix.md)** · **[support-star-queue.md](./docs/support-star-queue.md)** (high-star OSS + agent targets). **✅** = validated · **⚠️** = gap, out of scope, or **testing on cellp**.

**Frameworks**

| Framework | | Notes |
|-----------|:-:|--------|
| **Astro** (`@astrojs/cloudflare`) | ✅ | |
| **SvelteKit** (`adapter-cloudflare`, single Worker) | ✅ | |
| **React / Vue + Vite SPA** + wrangler `assets` | ✅ | Default pattern; see apps below |
| **Hono** (+ optional SPA via Workers Assets) | ✅ | **S26** · C3 `create-hono` + Assets · prod `GET /message` → `Hello Hono!` |
| **Remix** (`@remix-run/cloudflare`) | ✅ | |
| **Nuxt** (Nitro `cloudflare`) | ✅ | **S25** · prod `support-nuxt.lvh.me`（celld `node:timers` / Nitro loopback） |
| **SolidStart** | ✅ | S27 · `dev/examples/support-solidstart/` · prod **200** · `Hello world!` · celld 动态 `import()` + `nodejs_als` |
| **Qwik City** | ✅ | **S28 v7** · `templates/qwik/workers` · prod `Welcome to Qwik` · 无 `nodejs_compat` |
| **Waku** | ✅ | `create-waku` + C3 overlay · **S29 v9** · `docs/evidence/support-S29.log` |
| **Next.js** (OpenNext / vinext) | ⏸️ | **后排** · **S30 暂停** · prod v22 **400** · 非 AD-13 门禁 · [plan](./docs/plans/NEXT-OPENNEXT-CELLP.md) |

**Apps & stacks**

| Project | Kind | |
|---------|------|:-:|
| **Memos** | Notes / lightweight backend | ✅ |
| **Monolith** | Blog + API (separate wrangler) | ✅ |
| **SonicJS** | Headless CMS (Vite admin) | ✅ |
| **FlareMo** | React monorepo on Workers | ✅ |
| **EdgeEver** | Bun-bundled Worker app | ✅ |
| **Relay** | Short links + admin UI | ✅ |
| **workflows-starter** | Cloudflare Workflows demo SPA | ✅ |
| **R2-Explorer** | R2 file browser (Vue) | ✅ |
| **r2filebox** | R2 upload UI (Vue) | ✅ |
| **FileWorker** | Pages-style build + Worker | ✅ |
| **webhookflare** | Webhook / utility UI | ✅ |
| **NodeWarden** | Worker ops / API tool | ✅ |
| **cloudflarebase** | SvelteKit + multi-Worker `[[services]]` | ⚠️ |
| **pastebin-worker** | Hono SSR + Tailwind in Worker graph | ⚠️ |
| **request-bin** | HTTP bin (no root UI) | ⚠️ |

**Popular on Cloudflare — testing on cellp**

High-visibility Workers OSS (GitHub stars / CF templates / [awesome-cloudflare](https://github.com/zhuima/awesome-cloudflare)). **⚠️** = not signed off on cellp yet. Email / disposable-mail Workers apps are out of scope for this list.

| Project | Kind | | Notes |
|---------|------|:-:|--------|
| **Sink** | Short links + analytics UI | ✅ | ~7k★ · matrix S11 |
| **CloudFlare-ImgBed** | Image & file hosting (R2/D1) | ✅ | ~6k★ · **S31** · prod v1 |
| **microfeed** | Blog / podcast CMS | ✅ | **S34** · D1, Queues, R2 · `support-microfeed.lvh.me` |
| **UptimeFlare** | Uptime checks + status page | ⏸️ | ~3.8k★ · **S33 暂缓** · Next prod **500** · 后排 |
| **Supermemory SaaS stack** | Full-stack SaaS starter | ⚠️ | ~3.7k★ · auth + D1 + R2 |
| **CloudPaste** | File share / WebDAV UI | ✅ | ~2.6k★ · **S39** · unified SPA Worker · prod v1 · `support-cloudpaste.lvh.me` |
| **Counterscale** | Privacy-friendly analytics | ❌ | **S38** · prod v2 · `support-counterscale.lvh.me` · `/` **200** · 无 Analytics Engine · 见 matrix |
| **cf-workers-status-page** | Status page + alerts | ✅ | ~2.8k★ · **S32** · flareact · ESM wrap · prod v2 |
| **Serverless DNS** | DNS resolver on Workers | ❌ | **S37** · prod v3 · `/` `/configure` **408**（blocklist 冷启动超时）· 见 matrix |
| **OpenStatus** | Open-source status pages | ✅ | **S35** · official Astro CF Workers template · prod v2 · `support-openstatus.lvh.me` |
| **Triplit** | Edge sync / data layer | ⚠️ | Workers client |

More: [Cloudflare templates](https://github.com/cloudflare/templates) · [Framework guides](https://developers.cloudflare.com/workers/framework-guides/) · [Full star queue](./docs/support-star-queue.md).

**Summary:** ✅ on **14** apps in the core matrix (~67%). **⚠️ testing** rows tracked in [support-star-queue.md](./docs/support-star-queue.md). [Compare](https://konghayao.github.io/cellp/compare) · [Supported stacks](https://konghayao.github.io/cellp/migrate/stacks).

---

## Vercel framework on cellp

cellp is **not** Vercel. This track maps **[Vercel open source](https://github.com/vercel)** when the deliverable can be a **wrangler bundle** on **celld** (or when we only study architecture for a later bridge). **Not** Vercel hosting, Sandbox, or managed Workflows.

**Legend:** **✅** validated path · **⚠️** partial / testing · **🔜** research · **❌** out of scope

| Vercel OSS | What | | On cellp |
|------------|------|:-:|----------|
| **[Next.js](https://github.com/vercel/next.js)** | React framework | ⏸️ | **后排** · 不做 Next 托管 · OpenNext 实验 **S30 暂停**（[plan](./docs/plans/NEXT-OPENNEXT-CELLP.md)） |
| **[AI SDK](https://github.com/vercel/ai)** | TypeScript AI toolkit (`ai`, `@ai-sdk/*`) | ⚠️ | Bundle in your Worker + **your** provider keys |
| **[Workflow SDK](https://github.com/vercel/workflow)** | Durable steps (Workflow DevKit) | 🔜 | Compare to **CF Workflows** on celld — not Vercel’s managed executor |
| **[fx](https://github.com/vercel-labs/fx)** | [fx.sh](https://fx.sh) coding agent (Zig + wasm embed) | ⚠️ | **P1** on Workers: [fx-on-workers](https://github.com/codingstark-dev/fx-on-workers) · `./dev/scripts/deploy-support-app.sh A04` |
| **[Eve](https://github.com/vercel/eve)** | Filesystem-first production agents | 🔜 | Default deploy is Vercel — research only |
| **[open-agents](https://github.com/vercel-labs/open-agents)** | Cloud agent template (workflow + sandbox) | 🔜 | Assumes Vercel sandbox / managed workflow |

**Catalog:** [VERCEL-SUPPORT.md](./docs/VERCEL-SUPPORT.md) · **Plan:** [VERCEL-FRAMEWORK-ON-CELLP.md](./docs/plans/VERCEL-FRAMEWORK-ON-CELLP.md) · **Pages:** [research/vercel-on-cellp](https://konghayao.github.io/cellp/research/vercel-on-cellp)

---

## Contributors

| | |
|--|--|
| Architecture | [DESIGN.md](./DESIGN.md) · [docs/decisions.md](./docs/decisions.md) |
| Coding Agent (frontier) | [docs/plans/CODING-AGENT-ON-CELLP.md](./docs/plans/CODING-AGENT-ON-CELLP.md) · [AGENT-SUPPORT.md](./docs/AGENT-SUPPORT.md) |
| Vercel framework | [docs/plans/VERCEL-FRAMEWORK-ON-CELLP.md](./docs/plans/VERCEL-FRAMEWORK-ON-CELLP.md) · [VERCEL-SUPPORT.md](./docs/VERCEL-SUPPORT.md) |
| Agents | [AGENTS.md](./AGENTS.md) · [docs/README.md](./docs/README.md) |
| Repo layout | [site → Repository map](https://konghayao.github.io/cellp/reference/repo) |

---

## License

See each subtree. `celld/` tracks [KonghaYao/celld](https://github.com/KonghaYao/celld).
