# Nitro / Nuxt SSR on celld（S25）

> **状态：** Phase 2 已修（`node:timers` `setImmediate`）· S25 `GET /` 200  
> **缺陷：** [PD-20260902-06](../platform-defects-log.md#pd-20260902-06--nitro-localfetch-ssr-挂起celld)  
> **验收应用：** `dev/examples/support-nuxt/` → artifact `dev/data/artifacts/support-nuxt/v1/`

---

## 1. 现象

| 请求 | 预期 | celld 实测（`Host: support-nuxt.lvh.me:8787`） |
|------|------|-----------------------------------------------|
| `GET /` | 200 HTML（SSR） | **挂起**（curl `-m 5` → 000，直至客户端超时） |
| `GET /robots.txt` | 200 | 200 |
| `GET /favicon.ico` | 200 | 200 |
| `GET /_nuxt/builds/latest.json` | 200 | 200 |

celld 日志在挂起期间多为常规 lease/ship 心跳；客户端断开时出现 `http_connection_failures` / `read header from client timeout`，**未见**稳定的 `handler exceeded` 或 `handler is waiting on nothing`（客户端超时先于 `CELLD_HANDLER_BUDGET_S` 默认 300s）。

---

## 2. 复现

```bash
# 网关（示例）
curl -m 5 -v -H "Host: support-nuxt.lvh.me" http://127.0.0.1:8787/

# 对照：静态资产
curl -sS -o /dev/null -w "%{http_code}\n" \
  -H "Host: support-nuxt.lvh.me" http://127.0.0.1:8787/robots.txt
```

部署：`support-nuxt` v1，`wrangler.jsonc` 见 artifact（`no_bundle: true`，`assets.binding: ASSETS`，**无** `_routes.json`）。

---

## 3. 请求流（文本）

```
Client GET /
  → cellp ingress :8787
  → celld handle_ingress (main.rs)
       → ingress_assets: run_worker_first=false（默认）
       → 先试静态资产解析 → `/` 无 index → Ok(None)
       → fetch_worker → RuntimeManager::fetch_worker_pool
            → isolate: Worker default export fetch(req, env, ctx)
                 → Nitro Vr.fetch (bundle ~7845)
                      → 非 ASSETS 白名单路径
                      → useNitroApp().localFetch(pathname+search, { context: { waitUntil… } })
                           → h3 toNodeListener → b2(listener, { url: "/…" })  【进程内，不经 op_fetch】
                           → lazy `/**` → defineRenderHandler → Vue renderToString …
                 ← Promise<Response>（须 settle 后 celld 才回包）
  ← HttpResponse（若 Promise 永不 settle → 客户端一直等）
```

静态路径（`robots.txt` 等）在 Worker 入口即 `env.ASSETS.fetch(req)` → harness `__asset_fetch` → `dispatch_asset_call` → **不进入** Nitro SSR。

---

## 4. 产物侧（support-nuxt v1 bundle）

- 入口：`.cellp-bundle/index.js`，`export { Vr as default }`。
- SSR：`localFetch` 对以 `/` 开头的 URL 走 **h3 `b2()` 进程内调用**，**不是** `globalThis.fetch`（见 bundle ~7809–7812）。
- 对外层：`fetch` handler 对非 `Qr`/`Jr` 白名单路径调用 `localFetch`（~7851–7856）。
- `compatibility_flags`：`nodejs_compat`，`nodejs_als`（wrangler overlay）。

---

## 5. 根因（celld，非应用 patch）

### 5.1 与 PD 初稿的差异

PD-20260902-06 初稿将挂起归因于「Nitro `localFetch` 同源子 fetch」。对 **当前** S25 bundle，**主路径上的 `localFetch` 不经过 celld `op_fetch`**，而是 Nitro 内置 h3 监听器。因此「递归 HTTP 子请求」不是对 `localFetch` 本身的准确描述。

### 5.2 可确认的 celld 缺口

1. **顶层 `fetch()` 无同源 loopback**  
   - harness `globalThis.fetch` → `__op_fetch` → **reqwest 出网 HTTP**（`js.rs` `op_fetch`）。  
   - 同脚本 **service binding** 却有 `__cell.selfFetch` 进程内路径（`harness.js` ~2236–2250）。  
   - **workerd / Miniflare** 对 Worker 子请求通常走运行时内部调度，而非再占一条对外 HTTP 连接；celld 未对齐。

2. **挂起表现**  
   - `GET /` 时 Worker `fetch` 返回的 **Promise 长期 pending**（客户端超时，非快速 5xx）。  
   - 可能组合：  
     - SSR 某步仍调用 **`globalThis.fetch` / `$fetch` 默认链**（`createFetch` 默认 `globalThis.fetch`，仅 event 上被替换为 `localFetch`）；若命中 **同源 URL**，在 **单 isolate 占满 + 子请求经 ingress 再要 worker** 时可形成 **池内死锁或长时间等待**（`op_fetch` 默认超时 120s）。  
     - 或 **Vue/Nitro 异步链**在 celld V8 事件循环下未推进（`nodejs_als` / microtask / `waitUntil` 与 `end_event` 交互）——需用 `CELLD_HANDLER_BUDGET_S` 缩短预算 + isolate 日志进一步钉死。

3. **部署侧排除**  
   - 无 `_routes.json`：`/` **不会**被错误标为 asset-exclusive；会先 miss 静态再进 Worker，与「静态 200、SSR 挂」一致。  
   - **不是** `run_worker_first` 误配（artifact 未设，默认 `false`）。

### 5.3 结论（一句话）

**S25 SSR 挂起是 celld 运行时问题：** Worker 入口 `fetch` 的 Promise 在 Nitro SSR 路径上未在客户端时限内 settle；静态资产因 `ASSETS` 绑定旁路 SSR 仍正常。celld 缺少与 workerd 等价的 **同源 Worker `fetch` loopback**（`globalThis.fetch` 一律 HTTP），且对 Nitro 进程内 `localFetch` 与外层事件循环的交互尚未验收通过。

---

## 6. 建议修复（celld，最小方向）

| 优先级 | 改动 | 说明 |
|--------|------|------|
| P0 | **`op_fetch` / `globalThis.fetch` 同源 loopback** | 若 URL 指向本 generation 当前 Worker 的 ingress 主机（或相对 `/` 重写为当前请求 URL），在 isolate 内调用 `__cell.selfFetch`（新 event + `__beginEvent`），与 service binding 同源路径一致；避免 reqwest 再入 ingress 占第二 isolate。 |
| P1 | **回归测试** | `celld` 集成测：最小 Nitro `cloudflare_module` bundle（或裁剪的 `support-nuxt` 入口），`GET /` 断言 200 且 body 含 `<!DOCTYPE html>`。 |
| P2 | **可观测性** | 挂起时打 log：`op_fetch` URL、isolate 占用、pending op 类型（fetch vs asset）。 |

**不宜：** 在 `prepare-artifact.sh` 长期 polyfill `cloudflare:workers` / 改写 Nitro `localFetch`（见 PD 缓解条款）。

---

## 7. 测试想法

```bash
# 缩短 handler 预算，观察是 500/504 还是仍客户端超时
CELLD_HANDLER_BUDGET_S=30 … redeploy … curl -m 35 …

cargo test -p celld  # 新增 nitro/ssr loopback 用例后
```

---

## 8. 关键代码索引

| 区域 | 路径 |
|------|------|
| Ingress 资产 → Worker | `celld/crates/celld/main.rs` `handle_ingress` |
| 无状态 Worker 池 | `celld/crates/celld/runtime.rs` `fetch` / `drive` |
| `globalThis.fetch` | `celld/crates/celld/js/harness.js` ~565–593 |
| 同源 service `selfFetch` | `celld/crates/celld/js/harness.js` ~2190–2250 |
| `op_fetch`（HTTP） | `celld/crates/celld/js.rs` ~6502+ |
| ASSETS | `harness.js` `__makeAssetsBinding` → `op_asset_fetch` |
| S25 bundle 入口 | `dev/data/artifacts/support-nuxt/v1/.cellp-bundle/index.js` `Vr.fetch` / `localFetch` |

---

## 变更 log

| 日期 | 变更 |
|------|------|
| 2026-09-03 | Phase 2：`node:timers` 懒模块；h3 `send()`/`Xt2` 不再挂起；S25 PASS |
| 2026-09-01 | 初版：复现、流图、根因（修正 localFetch 机制）、修复与测试建议 |
