# AI Coding Agents on Cloudflare Workers — OSS Landscape for cellp

> **Purpose:** Identify open-source projects that combine **AI coding agents** with **Cloudflare Workers / wrangler-deployable** stacks, suitable for forking and deploying to validate **“Coding Agent on cellp”** (build → version → preview with forked D1/KV/R2 → promote).
>
> **Research date:** 2026-09-02. Star counts are approximate snapshots from GitHub UI / search; verify before citing in external docs.

---

## P0 — cellp deploy-support validation (ordered)

Run on local dev stack: `./dev/scripts/deploy-support-app.sh <id>` · evidence: `docs/evidence/support-<id>.log`.

| Order | ID | Upstream | Why first |
|------:|----|----------|-----------|
| **—** | — | [agents.cloudflare.com](https://agents.cloudflare.com/) · [Agents docs](https://developers.cloudflare.com/agents/) | **Reference platform** for P0 (SDK, DO, Workflows, Dynamic Workers) |
| **1** | **A01** | [cloudflare/agents-starter](https://github.com/cloudflare/agents-starter) | Official minimal `AIChatAgent` + DO SQLite + UI |
| **2** | **A02** | [qaml-ai/pi-worker](https://github.com/qaml-ai/pi-worker) → `examples/hello-agent` | Pi-style coding agent on Workers (picked over `pi-agent-cf` for richer harness) |
| **3** | **A03** | [southpolesteve/opencode-do](https://github.com/southpolesteve/opencode-do) | Small Worker + DO; OpenCode SSE protocol |

### P0 cellp 生产验收（本地 ingress）

`./dev/scripts/health.sh` → `./dev/scripts/deploy-support-app.sh A01|A03`（`GITHUB_CLONE_DIRECT=1 SUPPORT_SKIP_GIT_FETCH=1`）→ prod Host `support-*.lvh.me:8787`（非 preview）。

| ID | **支持？** | **version** | 失败根因 / 边界 | 证据 |
|----|:--------:|-------------|-----------------|------|
| **A01** | **支持（部分）** | **v10** | overlay 去掉 Workers AI；`GET /` SPA **200**，`/agents/*` 需 WebSocket 升级；聊天推理在 celld 上无 `env.AI` | `docs/evidence/support-A01.log` · `support-A01-acceptance.log` |
| **A02** | **支持** | （见 matrix） | — | `docs/evidence/support-A02.log` |
| **A03** | **支持（部分）** | **v1** | HTTP OpenCode 面 **200/JSON**（`POST /session`、`.../message`）；模型回复占位错误；`GET /event` SSE 在 gateway 上 **超时** | `docs/evidence/support-A03.log` · `support-A03-acceptance.log` |

### P1 — [fx on Workers](https://github.com/codingstark-dev/fx-on-workers) (`A04`)

Runs **[fx.sh](https://fx.sh)** upstream **[vercel-labs/fx](https://github.com/vercel-labs/fx)** (not Cloudflare Agent Cloud) inside a Worker via **[codingstark-dev/fx-on-workers](https://github.com/codingstark-dev/fx-on-workers)**: **libfx WebAssembly** + [`just-bash`](https://www.npmjs.com/package/just-bash) in **Durable Object** `FxSession`, TUI over WebSocket. Vercel track: [VERCEL-SUPPORT.md](./VERCEL-SUPPORT.md).

| Item | Detail |
|------|--------|
| **Deploy** | `./dev/scripts/deploy-support-app.sh A04` |
| **Overlay** | `dev/examples/support-fx-on-workers/wrangler.cellp.jsonc` |
| **Secrets** | `AI_GATEWAY_API_KEY` (fx wasm → **Vercel AI Gateway only**; **not** OpenCode — cellp 不做协议适配，见 [FX-LLM-CREDENTIALS.md](./plans/FX-LLM-CREDENTIALS.md)) |
| **cellp bar** | ready + `GET /?key=` **200**；**HTTP** `POST /api/prompt?key=`（overlay，见 `dev/examples/support-fx-on-workers/README.md`）收集 `command` 事件；浏览器 TUI 依赖 **WebSocket `/session`**（本地栈常 **502**，见 `platform-defects-log`） |
| **Size** | ~2.2 MiB gzip bundle — watch Workers bundle limits |

**Overlays (P0):** `dev/examples/support-agents-starter|support-pi-worker|support-opencode-do/wrangler.cellp.jsonc` — **Workers AI** bindings omitted (celld gap). **Agent 验收：** 多轮 + 工具调用。Pi（A02）在 cellp 用 **OpenAI 兼容** 接 Zen：`OPENAI_BASE_URL=https://opencode.ai/zen/v1`、`OPENAI_API_KEY=public`、`OPENAI_MODEL=big-pickle`，overlay `hello-agent.src/index.ts` 走 **pi-worker `Agent` + `getModel`**（非手写 fetch 循环）。

**Later (not P0):** Cloudflare OS 🔜 · [Agent Cloud](https://agents.cloudflare.com/) full stack · Eve / Pi / Deep Agents research in [CODING-AGENT-ON-CELLP.md](./plans/CODING-AGENT-ON-CELLP.md).

---

## 1. Search methodology

### 1.1 GitHub search queries (web + GitHub)

| Query family | Example queries |
|--------------|-----------------|
| Official CF agent products | `cloudflare agents-starter`, `cloudflare vibesdk`, `cloudflare cloudflare-os`, `cloudflare agents` |
| Pi on Workers | `pi-agent-core Cloudflare Workers`, `pi-worker`, `pi-agent-cf` |
| Community harness | `wrangler agent`, `workers agent` durable object, `coding-agent cloudflare-workers` |
| MCP on edge | `MCP server Cloudflare Workers`, `cloudflare mcp codemode` |
| LangGraph / LangChain | `LangGraph Cloudflare Workers`, `langchain-cloudflare`, `js-cloudflare langsmith` |
| OpenCode / Kimi | `opencode durable objects`, `kimiflare cloudflare` |
| Code Mode | `code mode cloudflare agents`, `@cloudflare/codemode` |

### 1.2 GitHub Topics browsed

- `cloudflare-workers`, `durable-objects`, `coding-agent`, `vibe-coding`, `workers-ai`, `cloudflare-workers-ai`, `codemode`, `ai-agent`

### 1.3 Curated lists & docs (not exhaustive)

- Cloudflare [Agents docs](https://developers.cloudflare.com/agents/), [agent-setup](https://developers.cloudflare.com/agent-setup/) (Copilot/Cursor skills)
- Cloudflare blog: VibeSDK, Cloudflare OS, Code Mode, Dynamic Workers, Artifacts
- LangChain: [Deploy with Cloudflare Workers](https://docs.langchain.com/langsmith/deploy-cloudflare-workers) (`js-cloudflare` template)
- Product Hunt / community posts for VibeSDK (cross-check only)

### 1.4 Exclusions (per scope)

- **Email-only** agent demos (e.g. generic Email Workers) unless they are full agent harnesses — `agentic-inbox` included as reference UI+agent pattern but **not** a coding-agent primary.
- **`cloudflare/workers-sdk`** monorepo (Wrangler only) — excluded as a validation target.
- Pure **MCP control-plane** servers with no agent loop (listed only where they inform deploy patterns).

### 1.5 cellp validation lens

| Fit | Meaning |
|-----|---------|
| **High** | `wrangler deploy` (or documented CF deploy), agent loop edits code / deploys previews, maps to cellp `POST /versions` + data fork narrative |
| **Medium** | CF-native agent but multi-worker/platform-heavy, CLI-first (not wrangler bundle), or missing deploy-preview loop |
| **Low** | MCP-only, chat-only, POC, or wrong deploy target |

**Single vs multi:** “Single” = one primary Worker entry + DOs as bindings; “Multi” = separate workers (gateways, containers, WfP, feedback-worker, etc.).

---

## 2. Project table (25 entries)

| # | Name | URL | Stars~ | Agent type | CF integration | Single / Multi | cellp fit | One line |
|---|------|-----|--------|------------|----------------|--------------|-----------|----------|
| 1 | **agents-starter** | https://github.com/cloudflare/agents-starter | 1.3k | Chat agent (`AIChatAgent`) + tools/scheduling | Workers, wrangler, DO SQLite, Workers AI | Single (+ Vite UI) | **High** | Minimal wrangler template: best “hello agent on Workers” for cellp smoke tests |
| 2 | **agents** (SDK monorepo) | https://github.com/cloudflare/agents | 5.4k | Framework: `Agent`, Think, Code Mode, MCP | DO, Workflows, Worker Loader, `@cloudflare/shell` | Multi (examples) | **Medium** | Source of truth for patterns; fork **examples/** not whole monorepo |
| 3 | **vibesdk** | https://github.com/cloudflare/vibesdk | 5.3k | Vibe coding platform (Think agent loop) | DO, D1, R2, KV, Artifacts, Dynamic Workers, Sandboxes | **Multi** | **Medium** | Full build→preview→verify loop; heavy deps (WfP, containers) |
| 4 | **cloudflare-os** | https://github.com/cloudflare/cloudflare-os | 8.6k | Workspace agent (Pi + Code Mode) + Gadgets | DO, Dynamic Workers, Facets, many Gatekeeper Workers | **Multi** | **Low–Med** | North-star architecture; too large for first cellp fork |
| 5 | **cloudflare-os-starter** | https://github.com/cloudflare/cloudflare-os-starter | (see GH) | Deploy helper for OS | wrangler / deploy flow | Multi | **Low** | Install path for OS, not a minimal coding agent |
| 6 | **agentic-inbox** | https://github.com/cloudflare/agentic-inbox | 6.9k | Email `AIChatAgent` (9 tools) | Workers, DO, R2, Email Routing | Single app worker | **Low** | Great Agents SDK reference; not code-generation |
| 7 | **pi-worker** | https://github.com/qaml-ai/pi-worker | 183 | Pi-style **coding** agent on Workers | DO, SQLite FS, Worker Loader sandboxes | Multi (examples) | **High** | Closest OSS to “Pi harness on CF”; `terminal-agent` is flagship |
| 8 | **pi-agent-cf** | https://github.com/funtuan/pi-agent-cf | 17 | `pi-agent-core` in DO sessions | Workers, DO, WebSocket/REST | Single worker pattern | **High** | Thin SDK wrapper; good minimal Pi+DO deploy |
| 9 | **kimiflare** | https://github.com/sinameraji/kimiflare | 168 | Terminal coding agent (Kimi on Workers AI) | CLI + optional remote; Workers AI, AI Gateway | CLI (not wrangler app) | **Medium** | Coding harness on *your* CF account; deploy story is npm CLI not cellp Worker bundle |
| 10 | **opencode-do** | https://github.com/southpolesteve/opencode-do | 114 | OpenCode remote server POC | Workers + DO SQLite, Workers AI | Single | **Medium** | `opencode attach` protocol; no file tools yet |
| 11 | **cloud-code** | https://github.com/miantiao-me/cloud-code | 558 | OpenCode in **Containers** | Workers + `@cloudflare/containers`, wrangler | Multi (Worker + container) | **Medium** | Agent in container; CF Worker is orchestrator |
| 12 | **clopinette-ai** | https://github.com/marceloeatworld/clopinette-ai | 28 | General agent + codemode | DO, Workflows, Vectorize, Workers AI | Multi | **Medium** | Hermes-like; coding secondary to personal assistant |
| 13 | **langchain-cloudflare** | https://github.com/cloudflare/langchain-cloudflare | 39 | LangChain/LangGraph **integrations** | Workers AI, Vectorize, D1 checkpoints (Python) | Library | **Low** | Bring-your-own-agent; no wrangler coding harness |
| 14 | **LangSmith js-cloudflare** | https://docs.langchain.com/langsmith/deploy-cloudflare-workers | — | Deep agent template (Hono, React, DO SSE) | wrangler, DO `ThreadSession` | Single template | **Medium** | Official LangGraph-on-CF deploy guide; Vercel-adjacent tooling |
| 15 | **auth0-lab/cloudflare-agents-starter** | https://github.com/auth0-lab/cloudflare-agents-starter | ~low | Chat agent + Auth0 | Same as agents-starter + auth | Single | **Medium** | agents-starter + identity |
| 16 | **vinext-agents-example** | https://github.com/cloudflare/vinext-agents-example | ~low | Chat agent (vinext + Agents) | Workers, DO, `AIChatAgent` | Single | **High** | Alternative frontend stack; same agent model as starter |
| 17 | **workers-mcp** | https://github.com/cloudflare/workers-mcp | ~low | MCP bridge (stdio proxy to Worker) | Workers + local Node proxy | Single Worker + local | **Low** | Expose Worker APIs to desktop agents |
| 18 | **mcp** (CF API) | https://github.com/cloudflare/mcp | ~low | MCP + **Code Mode** for CF API | Workers, Dynamic Worker Loader | Single | **Low** | Infra agent tooling, not app coding |
| 19 | **mcp-server-cloudflare** | https://github.com/cloudflare/mcp-server-cloudflare | ~low | Hosted MCP servers (docs, bindings, builds) | Workers (multiple apps in repo) | Multi | **Low** | Account management via MCP |
| 20 | **building-mcp-server-on-cloudflare** | https://github.com/cloudflare/building-mcp-server-on-cloudflare | ~low | Skill/template for remote MCP on Workers | wrangler | Single | **Low** | Pattern reference for agent tools |
| 21 | **codemode example** | https://github.com/cloudflare/agents/tree/main/examples/codemode | (in agents) | Code Mode agent loop | AIChatAgent + codemode | Single example | **High** | Minimal “agent writes code to call tools” on Workers |
| 22 | **dynamic-workers example** | https://github.com/cloudflare/agents/tree/main/examples/dynamic-workers | (in agents) | Sandbox / dynamic isolate | Worker Loader | Single example | **High** | Maps to “run generated worker” previews |
| 23 | **playground** | https://github.com/cloudflare/agents/tree/main/examples/playground | (in agents) | Kitchen-sink agent UI | Full SDK surface | Single | **Medium** | Broad feature testbed; heavier than starter |
| 24 | **innovatorved/chat-cloudflare-stack** | https://github.com/innovatorved/chat-cloudflare-stack | ~low | AI chat webapp | D1, KV, DO, wrangler | Single | **Medium** | Chat + encrypted API keys; not coding-first |
| 25 | **mattzcarey/cloudflare-mcp** | https://github.com/mattzcarey/cloudflare-mcp | ~low | CF API MCP (code execution) | Workers, codemode-style executor | Single | **Low** | API automation, not repo coding agent |

### Non–Cloudflare deploy target (relevant architecture)

| Name | URL | Stars~ | Agent type | Deploy target | cellp fit | One line |
|------|-----|--------|------------|---------------|-----------|----------|
| **Eve** | https://github.com/vercel/eve | growing | Filesystem-first durable agents | Vercel (Sandbox, Connect) | **Low** (CF) | Production agent framework; compare lifecycle to cellp, not wrangler |

---

## 3. Top 8 recommended for cellp deploy-support validation

Ranked for **fork/deploy on cellp** with preference for **optional UI** and **simpler topology** (single primary Worker where possible).

### 1. `cloudflare/agents-starter` — **High**

- **Why:** Official minimal `wrangler.jsonc` + `AIChatAgent`, streaming chat, tools, scheduling; `npm run deploy` is documented.
- **cellp angle:** Replace “deploy to CF” with `cellp` version upload; agent loop stays identical; bindings become cellp fork targets.
- **UI:** Included (Vite/React); can strip UI and keep Worker+DO only.

### 2. `qaml-ai/pi-worker` → `examples/hello-agent` or `examples/terminal-agent` — **High**

- **Why:** Explicit **coding agent** on Workers using Pi forks (`pi-coding-agent-worker`); terminal-agent shows DO sessions, sandbox execution, publish worker from session.
- **cellp angle:** Strongest alignment with “Pi on private control plane”; terminal-agent is multi-binding but one logical app.
- **UI:** `terminal-agent` has browser terminal; `hello-agent` is smallest.

### 3. `funtuan/pi-agent-cf` — **High**

- **Why:** Small surface: `createAgentWorker` + DO session + `pi-agent-core`; clear REST/WebSocket API.
- **cellp angle:** Easiest to reason about per-session deploy and storage mapping to cellp preview branches.
- **UI:** None required (API-first).

### 4. `cloudflare/agents` → `examples/codemode` — **High**

- **Why:** Documents Code Mode on Workers (agent generates TS to orchestrate tools)—same pattern as Cloudflare OS / VibeSDK internals.
- **cellp angle:** Validates Dynamic Worker / loader assumptions against celld.
- **UI:** Example-specific; often minimal.

### 5. `cloudflare/agents` → `examples/dynamic-workers` or `worker-bundler-playground` — **High**

- **Why:** Runtime bundle + Worker Loader previews—direct analog to “agent output is a Worker artifact.”
- **cellp angle:** Close to cellp’s “artifact → celld instance” path.
- **UI:** Playground optional.

### 6. `cloudflare/vinext-agents-example` — **High**

- **Why:** Same Agents SDK chat agent as starter, different UI bundling (vinext); proves alternate frontend + one Worker.
- **cellp angle:** Confirms agent protocol is UI-agnostic.
- **UI:** Yes (vinext).

### 7. `southpolesteve/opencode-do` — **Medium**

- **Why:** Single Worker + DO, wrangler deploy, documents OpenCode SSE protocol; very small codebase.
- **cellp angle:** Good protocol test; extend with file tools later.
- **UI:** OpenCode TUI (external).

### 8. `cloudflare/agents-starter` derivative: **auth0-lab/cloudflare-agents-starter** — **Medium**

- **Why:** Same deploy shape as #1 with auth boundary—useful if cellp adds access control before agent endpoints.
- **cellp angle:** Auth layer without changing agent loop.
- **UI:** Same as agents-starter.

**Honorable mentions (not top 8):**

- **vibesdk** — best end-to-end “coding platform” but **multi-service** and CF-account-specific features (Artifacts beta, WfP).
- **cloudflare-os** — architectural north star; fork for research, not first validation.
- **kimiflare** — excellent coding agent UX but **CLI/npm**-first, not a single wrangler bundle.

---

## 4. Explicit gaps (what almost no OSS does)

These are **underserved** relative to cellp’s thesis (**private control plane + agent + data fork**):

| Gap | What exists today | What cellp could validate |
|-----|-------------------|---------------------------|
| **Private control plane** | Everything assumes **Cloudflare dashboard** account, API tokens, or Vercel project | Same wrangler-shaped app deployed via **cellp** with **preview Host + branched D1/KV/R2** |
| **Versioned data fork per agent run** | VibeSDK has per-app Facet SQLite; OS has per-gadget DO—not **parent/child version fork** of all bindings | Agent run → new **cellp version** with **offshoot data**; promote to prod |
| **Promote / rollback of agent-built apps** | CF: deploy to `workers.dev` or WfP; git via Artifacts in VibeSDK | cellp **promote** semantics for agent output artifacts |
| **Unified “harness + deploy” on self-hosted Workers runtime** | Pi is local; Pi-on-CF is community; CF official stacks need CF cloud | **celld** + **cellp** as the runtime/control plane |
| **Single-tenant enterprise fork** | cloudflare-os is “fork for your company” but still **CF-hosted** | OSS rarely combines **on-prem control plane** + **coding agent** + **binding branch** |
| **LangGraph “production server” on Workers** | JS runs on Workers; **LangGraph Cloud** is separate product; D1 checkpoint package is Python-centric | No popular **single wrangler** “deep agent” repo with checkpoint + preview fork |
| **Agent loop that targets wrangler without CF API** | Agents use CF API / dashboard for deploy buttons | Agent emits **cellp API** (`POST /versions`) instead of `wrangler deploy` |

**Almost no OSS repo** implements all three: **(1)** coding agent loop, **(2)** wrangler-compatible Worker output, **(3)** **private** lifecycle with **data branching**—that is cellp’s differentiation vs Cloudflare-hosted and Vercel-hosted stacks.

---

## 5. Suggested cellp validation sequence

1. **Smoke:** Deploy `agents-starter` on cellp; one chat turn with Workers-AI equivalent (or external model via AI SDK).
2. **Coding:** `pi-agent-cf` or `pi-worker/examples/hello-agent`; session create → prompt → tool stub.
3. **Artifact:** `examples/dynamic-workers` or minimal custom Worker that returns bundled script from agent.
4. **Stretch:** Subset of vibesdk deploy loop (without WfP) or document blockers in celld compat matrix.

---

## 6. References

- cellp: [CODING-AGENT-ON-CELLP.md](./plans/CODING-AGENT-ON-CELLP.md), [support-matrix.md](./support-matrix.md)
- Cloudflare: [Agents](https://developers.cloudflare.com/agents/), [Dynamic Workers](https://developers.cloudflare.com/dynamic-workers/), [Code Mode example](https://developers.cloudflare.com/dynamic-workers/examples/codemode)
- Pi: [earendil-works/pi](https://github.com/earendil-works/pi) (runtime used in Cloudflare OS)
- LangChain CF deploy: [Deploy with Cloudflare Workers](https://docs.langchain.com/langsmith/deploy-cloudflare-workers)
