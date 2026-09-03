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
| P0 | **Counterscale** | [benvinegar/counterscale](https://github.com/benvinegar/counterscale) | 2.1k | **不支持** | turbo pnpm monorepo |
| P0 | **CloudPaste** (unified SPA Worker) | [ling-drag0n/CloudPaste](https://github.com/ling-drag0n/CloudPaste) | 2.6k | **不支持** | Pages+Worker 拆分 |
| P1 | **CloudFlare-ImgBed** | [MarSeventh/CloudFlare-ImgBed](https://github.com/MarSeventh/CloudFlare-ImgBed) | 6.3k | **支持（S31）** | `deploy/worker` + `prepare-artifact.sh` · KV `img_url` + R2 · prod **v1** 2026-09-03 · 无 `IMAGES` binding |
| P1 | **cf-workers-status-page** | [eidam/cf-workers-status-page](https://github.com/eidam/cf-workers-status-page) | 2.8k | **不支持** | flareact/webpack `[site]` 未适配 |
| P1 | **UptimeFlare** | [lyc8503/UptimeFlare](https://github.com/lyc8503/UptimeFlare) | 3.8k | **排队** | Workers + Pages — more moving parts |
| P2 | **microfeed** | [microfeed/microfeed](https://github.com/microfeed/microfeed) | 4.1k | **排队** | D1 + Queues + R2 CMS |
| P2 | **Supermemory SaaS stack** | [supermemoryai/cloudflare-saas-stack](https://github.com/supermemoryai/cloudflare-saas-stack) | 3.7k | **排队** | Next-on-Pages pattern |
| P2 | **Serverless DNS** | [serverless-dns/serverless-dns](https://github.com/serverless-dns/serverless-dns) | 3.8k | **排队** | Edge DNS resolver |
| P2 | **OpenStatus** | [openstatusHQ/openstatus](https://github.com/openstatusHQ/openstatus) | — | **排队** | Status pages (verify Workers deploy) |
| P2 | **Triplit** | [aspen-cloud/triplit](https://github.com/aspen-cloud/triplit) | 3.1k | **排队** | Edge sync client |

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
| Hono, SolidStart, Qwik, Waku, Next (OpenNext) | **测试中** / experimental — see README |

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
