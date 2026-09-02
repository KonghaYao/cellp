# Support 框架用户行为验收（AD-13）

> **日期：** 2026-09-02  
> **Gateway：** `http://127.0.0.1:8787`（prod Host：`*.lvh.me`）  
> **验收方：** verification subagent（端口 + HTML 内容，非仅 HTTP 码）  
> **健康检查：** `./dev/scripts/health.sh` — 全部 OK

## 验收标准（用户行为）

| 结论 | 条件 |
|------|------|
| **fail** | 502/503/500、`ingress_unknown`、空 body、页面与预期场景不符 |
| **pass** | HTML 含预期标题/正文；导航路径可跟；robots 等为合理内容类型 |

---

## S22 — support-astro.lvh.me（读者模拟）

| 步骤 | URL | HTTP | 用户可见结果 | Pass |
|------|-----|------|--------------|------|
| 首页 | `/` | 200 | `<title>Astro Blog</title>`，h1「Hello, Astronaut!」 | **PASS** |
| 博客索引 | `/blog/` | 200 | 博文列表，链到 `/blog/first-post/` 等 | **PASS** |
| 首篇 | `/blog/first-post/` | 200 | 「First post」+ 正文 | **PASS** |
| About | `/about/` | 200 | `<title>About Me</title>` | **PASS** |
| 首页 → /blog | `/blog`（-L） | 200 | 同源跳转正常 | **PASS** |

**S22 总评：PASS**

---

## S23 — support-sveltekit.lvh.me（首次访问）

| 步骤 | URL | HTTP | 用户可见结果 | Pass |
|------|-----|------|--------------|------|
| 首页 | `/` | 200 | h1「Welcome to SvelteKit」 | **PASS** |
| robots | `/robots.txt` | 200 | `text/plain`，`User-agent: *` | **PASS** |
| 内链 | — | — | 首页无额外站内路径（N/A） | — |

**S23 总评：PASS**

---


## S24 — support-remix.lvh.me（Remix starter）

| 步骤 | URL | HTTP | 用户可见结果 | Pass |
|------|-----|------|--------------|------|
| 首页 | `/` | 200 | `<title>New Remix App</title>`，「Welcome to Remix」、Remix logo | **PASS** |
| 静态 CSS | `/assets/root-CS6YCbpS.css` | 200 | 样式资源可加载 | **PASS** |
| 静态图 | `/logo-light.png` | 200 | logo 可加载 | **PASS** |
| 读者模拟 | `/`（`curl -L` + grep） | 200 | body 含 `Remix` / `New Remix App` / `__remixContext` | **PASS** |

**S24 总评：PASS**

---

## S25 — support-nuxt.lvh.me（Nuxt / Nitro SSR）

> **复验：** 2026-09-03 · 新 `celld`（`node:timers` `setImmediate`）· `kill` 旧 `support-nuxt` celld · `GITHUB_CLONE_DIRECT=1 SUPPORT_SKIP_GIT_FETCH=1 SUPPORT_POLL_SECS=180 ./dev/scripts/deploy-support-app.sh S25`（promote v1；smoke `GET /` → **200**）

| 步骤 | URL | HTTP | 用户可见结果 | Pass |
|------|-----|------|--------------|------|
| 首页 SSR | `/` | **200**（`curl -m 15` → 105018 bytes，~11ms，`Content-Type: text/html`） | `<!DOCTYPE html>` · `<title>Welcome to Nuxt!</title>` · `X-Powered-By: Nuxt` | **PASS** |
| robots | `/robots.txt` | 200 | `text/plain`，`User-Agent: *` / `Disallow:` | **PASS** |
| 静态图标 | `/favicon.ico` | 200 | 4286 bytes | **PASS** |
| Nuxt 构建元数据 | `/_nuxt/builds/latest.json` | 200 | 71 bytes | **PASS** |
| 读者模拟（首页） | `/` | 200 | ≤15s 内可见可渲染 HTML 首页 | **PASS** |

**缺陷对齐：** [PD-20260902-06](./platform-defects-log.md) **fixed**（`node:timers` 真实 `setImmediate`，h3 `send()` 可 `res.end`）。

**S25 总评：PASS**

---

## A02 — support-pi-worker (Pi / Zen + R2 工具)

| 步骤 | URL | HTTP | 用户可见结果 | Pass |
|------|-----|------|--------------|------|
| 用法 | `GET /` | 200 | Zen multi-turn tools 说明 | **PASS** |
| 缺 prompt | `POST /` `{}` | 400 | `Missing 'prompt'` | **PASS** |
| **多轮 + 工具** | `POST /` write+ls prompt | 200 | `meta.toolCalls ≥ 1`，正文含创建的文件名 | **PASS**（v5 · `big-pickle` · `OPENCODE_API_KEY=public`） |

证据：`docs/evidence/support-A02-agent-tools.log`

**A02 总评：PASS**（agent 工具多轮已通）

---

## A01 — support-agents-starter

| 步骤 | URL | HTTP | 用户可见 | Pass |
|------|-----|------|----------|------|
| 部署 | `deploy-support-app.sh A01` | — | 需 `npx vite build`（非 `npm run build`） | 待复测 |
| Agent/工具 | `/agents/*` | — | 依赖 Workers AI；cellp overlay 无 AI 绑定时仅结构验收 | 待复测 |

**A01 总评：待复测**（构建脚本已修于 `deploy-support-app.sh`）

---

## A03 — support-opencode-do

| 步骤 | URL | HTTP | 用户可见 | Pass |
|------|-----|------|----------|------|
| 落地 | `GET /` | 200 | attach 说明 | **PASS** |
| 会话 | `POST /session` | 200 | `ses_*` | **PASS** |
| 多轮消息 | `POST /session/{id}/message` | 200 | user+assistant 持久化 | **PASS** |
| SSE | `GET /event` | 超时/空 | Gateway 上长连接未通 | **FAIL** |

**A03 总评：PARTIAL**（HTTP 多回合 OK；SSE/推理需外部模型）

---

## A04 — support-fx-on-workers (fx agent)

| 步骤 | URL | HTTP | 用户可见结果 | Pass |
|------|-----|------|--------------|------|
| 无 key | `GET /` | 401 | `unauthorized — append ?key=<ACCESS_KEY>` | **PASS** |
| TUI 首页 | `GET /?key=cellp-dev-fx-on-workers` | 200 | HTML `fx on Cloudflare` + xterm 容器 | **PASS** |
| Session（非 WS） | `GET /session?key=...` | 426 | `expected websocket` | **PASS** |
| 终端 WebSocket | `GET /session` + Upgrade | **502** | `bad gateway` | **FAIL** |
| ingress | 各 Host 正文 | — | 无 `ingress_unknown` | **PASS** |

**A04 总评：FAIL**（落地页达标；TUI 依赖的 `/session` WebSocket 502，用户无法连终端）

---

*不自动改写 [support-matrix.md](./support-matrix.md)「支持」列；与 deploy 判定一致时由批次合并更新。*

## 复现

```bash
./dev/scripts/health.sh
# S22
curl -sS -H "Host: support-astro.lvh.me" http://127.0.0.1:8787/ | head
curl -sS -H "Host: support-astro.lvh.me" http://127.0.0.1:8787/blog/first-post/ | grep -o '<title>[^<]*</title>'
# S23
curl -sS -H "Host: support-sveltekit.lvh.me" http://127.0.0.1:8787/ | grep -i sveltekit
# S24
curl -sS -H "Host: support-remix.lvh.me" http://127.0.0.1:8787/ | grep -i remix
curl -sS -o /dev/null -w '%{http_code}\n' -H "Host: support-remix.lvh.me" http://127.0.0.1:8787/assets/root-CS6YCbpS.css
# S25（预期首页超时）
curl -sS -m 10 -H "Host: support-nuxt.lvh.me" http://127.0.0.1:8787/ || echo timeout
curl -sS -H "Host: support-nuxt.lvh.me" http://127.0.0.1:8787/robots.txt
# A02 工具多轮
curl -sS -m 300 -X POST -H "Host: support-pi-worker.lvh.me" -H "Content-Type: application/json" \
  http://127.0.0.1:8787/ -d '{"prompt":"Use write tool to create cellp-agent-test.txt with line: ok. Then ls path . and report filenames."}'
# A04
curl -sS -H "Host: support-fx-on-workers.lvh.me" "http://127.0.0.1:8787/?key=cellp-dev-fx-on-workers" | head
```
