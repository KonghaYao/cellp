# Support 框架用户行为验收（AD-13）

> **日期：** 2026-09-02（末批复验 **2026-09-03**）  
> **Gateway：** `http://127.0.0.1:8787`（prod Host：`*.lvh.me`）  
> **验收方：** verification subagent（端口 + HTML 内容，非仅 HTTP 码）  
> **健康检查：** `./dev/scripts/health.sh` — 全部 OK（deep health 503 允许）

## 2026-09-03 同步全量复验

证据：`docs/evidence/verify-restart-20260903.log`、`verify-full-20260903.log`

| 范围 | 总评 |
|------|------|
| S22–S26 prod | **PASS**（fleet 冷启动时需先 reconcile / 拉起 upstream celld） |
| S27 | **FAIL**（未改） |
| S28 / S29 | **FAIL**（deploy health timeout + worker load 错误；prod `ingress_unknown`） |
| S30 OpenNext **v10** | **FAIL**（promote ready 后 prod 仍 **308** `Location: ?`） |
| A01 / A03 / A04 prod HTTP | **PASS**（与既有 PARTIAL 口径一致） |
| `go test ./...` | **PASS** |
| `e2e/run-all.sh` | **FAIL**（本机缺 offshoot CLI） |

## 验收标准（用户行为）

| 结论 | 条件 |
|------|------|
| **fail** | 502/503/500、`ingress_unknown`、空 body、页面与预期场景不符 |
| **pass** | HTML 含预期标题/正文；导航路径可跟；robots 等为合理内容类型 |

---

## S22 — support-astro.lvh.me（读者模拟）

> **复验：** 2026-09-03 同步 verify 批次 · prod grep **PASS**

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

> **复验：** 2026-09-03 · `prepare-artifact.sh` 构建前重置 `wrangler.jsonc`（`main` → `.svelte-kit/cloudflare/_worker.js`）· `deploy-support-app.sh S23` promote **v1** · 2026-09-03 同步 verify **PASS**

| 步骤 | URL | HTTP | 用户可见结果 | Pass |
|------|-----|------|--------------|------|
| 首页 | `/` | 200 | h1「Welcome to SvelteKit」 | **PASS** |
| robots | `/robots.txt` | 200 | `text/plain`，`User-agent: *` | **PASS** |
| 内链 | — | — | 首页无额外站内路径（N/A） | — |

**S23 总评：PASS**

---


## S24 — support-remix.lvh.me（Remix starter）

> **复验：** 2026-09-03 同步 verify 批次 · prod **PASS**

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

## S26 — support-hono.lvh.me（C3 Hono · Workers Assets）

> **复验：** 2026-09-03 · `GITHUB_CLONE_DIRECT=1 SUPPORT_SKIP_GIT_FETCH=1 SUPPORT_POLL_SECS=300 ./dev/scripts/deploy-support-app.sh S26`（promote **v3**）· 2026-09-03 同步 verify **PASS**

| 步骤 | URL | HTTP | 用户可见结果 | Pass |
|------|-----|------|--------------|------|
| API | `/message` | 200 | body `Hello Hono!` | **PASS** |
| 静态首页 | `/` | 200 | `<!doctype html>` · `Hello, World!` · 内联脚本 fetch `/message` | **PASS** |
| 读者模拟 | `/` + `/message` | 200 | API 与静态资源同源可访问 | **PASS** |

**S26 总评：PASS**

---

## S27 — support-solidstart.lvh.me（SolidStart · C3）

> **复验：** 2026-09-03 · `dev/examples/support-solidstart/prepare-artifact.sh` → `CELP_C3_FRAMEWORK=solid`

| 步骤 | 结果 | Pass |
|------|------|------|
| 非交互 scaffold | `create-cloudflare --framework=solid` → `create-solid` **ENOENT** / 仅 hello-world stub | **FAIL** |
| prod Host | 未部署 | **FAIL** |
| 读者模拟 | 未 `grep 'Hello world'` | **FAIL** |

**根因：** C3 SolidStart 依赖 `create-solid` 交互选模板；CI/`deploy-support-app` 无法完成。

**S27 总评：FAIL**

---

## S28 — support-qwik.lvh.me（Qwik City · C3）

> **复验：** 2026-09-03 同步 verify · `deploy-support-app.sh S28` **v4** · celld `445569a`

| 步骤 | URL | HTTP | 用户可见结果 | Pass |
|------|-----|------|--------------|------|
| slim artifact | — | — | wrangler dry-run + `.cellp-assets`（无 `.assetsignore`） | **PASS** |
| deploy | platform | — | `start celld: celld health timeout` **8833** | **FAIL** |
| worker load | 手动 celld | — | `Cannot set property stdin of #<Process>`（unenv / `nodejs_compat`） | **FAIL** |
| prod | `support-qwik.lvh.me` | **404** | `ingress_unknown` | **FAIL** |
| 读者模拟 | `/` | — | 未 `grep 'Welcome to Qwik'` | **FAIL** |

**S28 总评：FAIL**（构建链 OK；runtime load + prod route 未通）

---

## S29 — support-waku.lvh.me（Waku · C3）

> **复验：** 2026-09-03 同步 verify · `deploy-support-app.sh S29` **v6** · celld `445569a`

| 步骤 | URL | HTTP | 用户可见结果 | Pass |
|------|-----|------|--------------|------|
| slim artifact | — | — | `dist/server/wrangler.json` dry-run · `nodejs_als` · 无 `rules`/`.assetsignore` | **PASS** |
| deploy | platform | — | `start celld: celld health timeout` **8834** | **FAIL** |
| worker load | 手动 celld | — | `does not provide an export named 'r'` | **FAIL** |
| prod | `support-waku.lvh.me` | **404** | `ingress_unknown` | **FAIL** |
| 读者模拟 | `/` | — | 未验收 `Waku` / `An internet website!` | **FAIL** |

**S29 总评：FAIL**（构建链 OK；runtime load + prod route 未通）

---

## S30 — support-opennext.lvh.me（Next.js · OpenNext / C3 template）

> **复验：** 2026-09-03 同步 verify · `SUPPORT_VERSION=v10` · `deploy-support-app.sh S30` promote **v10** · celld `445569a` · 无 `global_fetch_strictly_public` · `CELLD_TRUST_FORWARDED_HEADERS=1`

| 步骤 | URL | HTTP | 用户可见结果 | Pass |
|------|-----|------|--------------|------|
| deploy | platform | — | version **v10** **ready** + promote ok | **PASS** |
| prod 首页 | `/` | **308** | `Location: ?` · body `?` · `X-Opennext: 1` · **非** Next HTML | **FAIL** |
| 读者模拟 | `/` | **308** | 无 `<!DOCTYPE html>` / Next 标题 | **FAIL** |

**根因（当前）：** OpenNext/Next 重复斜杠或 URL 规范化链在 cellp ingress 下仍产出 **`Location: ?`**（见 [NEXT-OPENNEXT-CELLP.md](./plans/NEXT-OPENNEXT-CELLP.md) · [support-validation-lessons.md](./support-validation-lessons.md)）。wasm（PD-08）已 fixed。

**S30 总评：FAIL**（celld ready；prod 语义未达 tier-1）

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

> **复验：** 2026-09-03 同步 verify · prod `GET /` **PASS**

| 步骤 | URL | HTTP | 用户可见 | Pass |
|------|-----|------|----------|------|
| 部署 | `deploy-support-app.sh A01` | — | `npx vite build` + `prepare-artifact.sh` → **v10** promoted | **PASS** |
| 生产首页 | `GET /`（Host prod） | 200 | HTML `Agent Starter` + theme script | **PASS** |
| 静态资源 | `GET /assets/index-*.js` | 200 | 前端 bundle | **PASS** |
| Agent 路由 | `GET /agents/ChatAgent/default` | 400 | `Invalid request`（非 WS 探测；非 404） | **PASS**（结构） |
| 多轮推理 | WebSocket + chat | — | 无 Workers AI 绑定，推理不可用 | **FAIL**（平台） |

**A01 总评：PARTIAL**（生产 SPA **200**；完整 agent 对话 blocked by celld Workers AI）

---

## A03 — support-opencode-do

> **复验：** 2026-09-03 同步 verify · `/` + `/global/health` **PASS**

| 步骤 | URL | HTTP | 用户可见 | Pass |
|------|-----|------|----------|------|
| 部署 | `deploy-support-app.sh A03` | — | overlay only · **v1** promoted | **PASS** |
| 落地 | `GET /`（Host prod） | 200 | attach 说明 HTML | **PASS** |
| 健康 | `GET /global/health` | 200 | `{"healthy":true,"version":"0.0.4"}` | **PASS** |
| 会话 | `POST /session` | 200 | `ses_*` JSON | **PASS** |
| 多轮消息 | `POST /session/{id}/message` | 200 | user+assistant JSON；assistant 为 Workers AI 错误占位 | **PASS**（协议） |
| 历史 | `GET /session/{id}/message` | 200 | 两轮消息数组 | **PASS** |
| SSE | `GET /event` | 超时 | 3s 内无字节（gateway） | **FAIL** |

**A03 总评：PARTIAL**（生产 HTTP/OpenCode JSON **OK**；SSE + 真实推理未通）

---

## A04 — support-fx-on-workers (fx agent)

> **复验：** 2026-09-03 同步 verify · 401/200 TUI **PASS**

| 步骤 | URL | HTTP | 用户可见结果 | Pass |
|------|-----|------|--------------|------|
| 无 key | `GET /` | 401 | `unauthorized — append ?key=<ACCESS_KEY>` | **PASS** |
| TUI 首页 | `GET /?key=cellp-dev-fx-on-workers` | 200 | HTML `fx on Cloudflare` + xterm 容器 | **PASS** |
| Session（非 WS） | `GET /session?key=...` | 426 | `expected websocket` | **PASS** |
| 终端 WebSocket | `GET /session` + Upgrade | **101** | 握手成功（2026-09-03 WS-M2 复验） | **PASS** |
| **WS 会话帧** | `dev/scripts/fx-websocket-smoke.sh` | — | 101 + JSON 事件（无 key 时为 `AI_GATEWAY` error 帧） | **PASS** |
| ingress | 各 Host 正文 | — | 无 `ingress_unknown` | **PASS** |

**A04 总评：支持**（WebSocket 101 + 会话帧 smoke PASS；完整 LLM 回合需 **Vercel AI Gateway** key，非 OpenCode）

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
# S25
curl -sS -m 15 -H "Host: support-nuxt.lvh.me" http://127.0.0.1:8787/ | grep -i 'Welcome to Nuxt'
curl -sS -H "Host: support-nuxt.lvh.me" http://127.0.0.1:8787/robots.txt
# S30 OpenNext（须 200 HTML，非 308 Location: ?）
curl -sS -D - -o /tmp/s30.html -H "Host: support-opennext.lvh.me" http://127.0.0.1:8787/ | head -15
grep -iE 'next|<!DOCTYPE' /tmp/s30.html | head -3
# A02 工具多轮
curl -sS -m 300 -X POST -H "Host: support-pi-worker.lvh.me" -H "Content-Type: application/json" \
  http://127.0.0.1:8787/ -d '{"prompt":"Use write tool to create cellp-agent-test.txt with line: ok. Then ls path . and report filenames."}'
# A04
curl -sS -H "Host: support-fx-on-workers.lvh.me" "http://127.0.0.1:8787/?key=cellp-dev-fx-on-workers" | head
bash dev/scripts/fx-websocket-smoke.sh
```
