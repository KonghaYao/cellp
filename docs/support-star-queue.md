# High-star Cloudflare Workers — cellp validation queue

> **Purpose:** track OSS apps that are popular on Cloudflare but **not** yet in the S01–S21 verdict table.  
> **Research:** 2026-09-02 (GitHub topics, [zhuima/awesome-cloudflare](https://github.com/zhuima/awesome-cloudflare), CF templates, web-researcher pass).  
> **Status values:** **测试中** = active cellp validation · **排队** = next · **难** = likely blocked by WFP/DO/multi-service · **跳过** = email/mail Workers (out of scope)

**Authoritative verdict** once tested: add row to [support-matrix.md](./support-matrix.md) as **支持** or **不支持** (no partial).

---

## Priority queue (UI-first, single Worker preferred)

| Priority | Project | GitHub | ~Stars | Status | Notes |
|:--------:|---------|--------|-------:|--------|-------|
| P0 | **Sink** | [miantiao-me/Sink](https://github.com/miantiao-me/Sink) | 7.1k | **支持（S11）** | matrix · `prepare-artifact.sh` + overlay · prod v2 2026-09-03 |
| P0 | **Counterscale** | [jeffysl/counterscale](https://github.com/jeffysl/counterscale) | 2.1k | **不支持（S38）** | **S38** · `packages/server` · pnpm turbo + RR7 · overlay `dev/examples/support-counterscale/` · prod **v2** Host `support-counterscale.lvh.me` · `/` **200** · `/dashboard` **501** · celld 无 Analytics Engine · `docs/evidence/support-S38.log` |
| P0 | **CloudPaste** (unified SPA Worker) | [ling-drag0n/CloudPaste](https://github.com/ling-drag0n/CloudPaste) | 2.6k | **支持（S39）** | **S39** · `wrangler.spa.toml` 单 Worker + ASSETS · overlay `dev/examples/support-cloudpaste/` · prod **v1** Host `support-cloudpaste.lvh.me` **200** · `docs/evidence/support-S39.log`（非 Pages+Worker 拆分路径） |
| P1 | **CloudFlare-ImgBed** | [MarSeventh/CloudFlare-ImgBed](https://github.com/MarSeventh/CloudFlare-ImgBed) | 6.3k | **支持（S31）** | `deploy/worker` + `prepare-artifact.sh` · KV `img_url` + R2 + `IMAGES`（celld 本地 transform/output/info）· prod **v1** 2026-09-03 |
| P1 | **cf-workers-status-page** | [eidam/cf-workers-status-page](https://github.com/eidam/cf-workers-status-page) | 2.8k | **支持（S32）** | overlay + `prepare-artifact.sh` + `cellp-entry.mjs`（webpack IIFE → ESM default）· prod **v2** 2026-09-03 · Host `support-status-page.lvh.me` **200** |
| P1 | **UptimeFlare** | [lyc8503/UptimeFlare](https://github.com/lyc8503/UptimeFlare) | 3.8k | **⏸️ 暂缓** | **S33** 试验：v1 ready · prod **500**（Next `_error`）· **与 S30/OpenNext 同排后排** · overlay 未纳入矩阵 |
| P2 | **microfeed** | [microfeed/microfeed](https://github.com/microfeed/microfeed) | 4.1k | **支持（S34）** | Astro `@astrojs/cloudflare` · `dev/examples/support-microfeed/`（yarn build · wrangler dry-run slim · D1+Queue+R2 · 去 `.assetsignore`）· prod **v4** Host `support-microfeed.lvh.me` **200** · `docs/evidence/support-S34.log` |
| P2 | **Supermemory SaaS stack** | [supermemoryai/cloudflare-saas-stack](https://github.com/supermemoryai/cloudflare-saas-stack) | 3.7k | **排队** | Next-on-Pages pattern |
| P2 | **Serverless DNS** | [serverless-dns/serverless-dns](https://github.com/serverless-dns/serverless-dns) | 3.8k | **不支持（S37）** | **S37** · webpack + wrangler dry-run slim · overlay `dev/examples/support-serverless-dns/`（`pre.sh`/wget blocklist cfg · `WORKER_TIMEOUT` + dnsutil cap patch）· prod **v3** Host `support-serverless-dns.lvh.me` · `GET /` / `/configure` **408**（首请求拉取 blocklist trie 超时）· 正常应 **302** → `rethinkdns.com/configure` · `docs/evidence/support-S37.log` |
| P2 | **OpenStatus** | [openstatusHQ/openstatus](https://github.com/openstatusHQ/openstatus) | — | **支持（S35）** | 主仓 Docker · cellp 用官方 [astro-status-page](https://github.com/openstatusHQ/astro-status-page) Workers 模板 · prod **v2** Host `support-openstatus.lvh.me` **200** · `docs/evidence/support-S35.log` |
| P2 | **Triplit** | [aspen-cloud/triplit](https://github.com/aspen-cloud/triplit) | 3.1k | **不支持（S36）** | **S36** · `packages/cf-worker-server` · Yarn 4 + turbo · overlay `dev/examples/support-triplit/` · prod **v1** · Host `support-triplit.lvh.me` · `GET /health` **200** · `GET /` **401** JSON（无 Web 主界面）· `docs/evidence/support-S36.log` |

---

## Coding-agent / platform (frontier)

See **[plans/CODING-AGENT-ON-CELLP.md](./plans/CODING-AGENT-ON-CELLP.md)**. **🔜 计划支持** = research / future; not in matrix as 支持 yet.

| Platform | GitHub / product | ~Stars | Status | Notes |
|----------|------------------|-------:|--------|-------|
| **Cloudflare OS** | [cloudflare/cloudflare-os](https://github.com/cloudflare/cloudflare-os) | 8.6k | **🔜 计划支持** | Workers north-star · DO, Dynamic Workers, Gatekeepers |
| **[Cloudflare Agents](https://agents.cloudflare.com/)** | [cloudflare/agents](https://github.com/cloudflare/agents) | 5.4k | **🔜 对齐 Agent Cloud** | P0 via agents-starter |
| **fx on Workers** | [codingstark-dev/fx-on-workers](https://github.com/codingstark-dev/fx-on-workers) | — | **⚠️ P1 (`A04`)** | [fx.sh](https://fx.sh) wasm agent · AI Gateway key |
| **Eve** (Vercel) | [vercel/eve](https://github.com/vercel/eve) | — | **🔜 研究** | Filesystem-first production agents |
| **Pi** | [earendil-works/pi](https://github.com/earendil-works/pi) | — | **🔜 研究** | Terminal coding agent; pi-agent-core in CF OS |
| **Deep Agents** | [langchain-ai/deepagents](https://github.com/langchain-ai/deepagents) | — | **🔜 研究** | LangChain / LangGraph harness |

---

## Likely hard on cellp (still track for gaps)

| Project | Why |
|---------|-----|
| **cloudflare/wildebeest** | Archived; multiple Workers + DO worker |
| **G4brym/workers-firecrawl** | Browser Rendering (paid CF binding) |
| **cloudflare/partykit** | DO-heavy realtime |

---

## Excluded (by policy)

- **Email / disposable-mail Workers** (e.g. SaaSMail, ni-mail, Moemail) — not in validation queue
- **workers-sdk / workerd** — monorepos, not end-user apps
- **cloudflarebase** — already **不支持** in matrix (multi `[[services]]`)

---

## Frameworks (CF official guides)

Documented on Cloudflare; cellp framework slots in [support-matrix.md](./support-matrix.md) §S22–S25.

| Framework | Matrix |
|-----------|--------|
| Astro, SvelteKit, Remix | **支持** (when example overlay green) |
| Nuxt | 待验 |
| Hono, SolidStart, Qwik, Waku | **支持** — S26–S29 · 见 [support-matrix.md](./support-matrix.md) |
| Next (OpenNext) · UptimeFlare (Next/Pages) | **⏸️ 后排** — S30 / S33 暂停，见 [NEXT-OPENNEXT-CELLP.md](./plans/NEXT-OPENNEXT-CELLP.md) |

---

## When a row graduates

1. `deploy-support-app.sh` or dedicated overlay → ready + Host URL 200 on main UI  
2. Update **support-matrix.md** (new ID or flip S11 Sink)  
3. Update **README** ✅/⚠️ row (project names only, no internal IDs)  
4. Log under `docs/evidence/support-*.log`

---

## Sources

- [Cloudflare framework guides](https://developers.cloudflare.com/workers/framework-guides/)
- [cloudflare/templates](https://github.com/cloudflare/templates)
- [zhuima/awesome-cloudflare README-EN](https://github.com/zhuima/awesome-cloudflare/blob/master/README-EN.md)
- Internal web-researcher report 2026-09-02 (Sink, ImgBed, microfeed, UptimeFlare, …)
