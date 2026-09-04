# Real Hard Problem — S30 OpenNext `GET /` 不 settle

> **状态：** 未关闭 · **2026-09-04 进展**：preview **0-byte hang** 在 lab celld + `process.setImmediate` 补丁下 **可 settle**（instrumented v42 bundle、prod v22 均在 &lt;1s 返回）；**prod `GET /` 仍 400** proto-rel `//`（与 hang 分离）  
> **产品口径：** Next / OpenNext **非一等公民**（AD-13）；矩阵 **不支持**  
> **对照：** [NEXT-OPENNEXT-CELLP.md](./NEXT-OPENNEXT-CELLP.md) · [NITRO-CELLD-COMPAT.md](./NITRO-CELLD-COMPAT.md)（PD-06，**机制不同**）

本文记录 **原始问题、已排除路径、做过的尝试、当前最可能热路径**。续作前先读完，禁止再空等长 curl / 连打 deploy。

---

## 1. 原始问题

用户要在 **cellp 私有化 Workers** 上跑 **Next.js（OpenNext / `@opennextjs/cloudflare`）**。

| 层 | 期望 | 实际 |
|----|------|------|
| 构建 | CI 预构建 + `no_bundle` | **已通**（`prepare-artifact.sh`） |
| deploy | version `ready` | **已通**（含 PD-08 sibling wasm） |
| 历史 308 / cookie 500 | 对齐 CF | **已消**（ingress `//` + `node_http.js` `8a7bfaa`） |
| **prod / preview `GET /`** | **200 HTML** | **未通** |

两条并存的失败形态（**不是同一 bug**）：

1. **prod v22（当前指针）** — `~100ms` **400**  
   `"url" parameter cannot be a protocol-relative URL (//)"`  
   `X-Opennext: 1`  
   来自 **bundle 内 Next ImageOptimizer `validateParams`**，**不是 celld**。
2. **preview v38–v42（去掉 / 未打半套 proto 补丁后）** — **0 byte、无响应头**  
   直打 celld `:8839` 与 Gateway `:8787` **一样**。客户端 `-m 8` 先断；`CELLD_HANDLER_BUDGET_S` 默认 **300s**，故 **看不到** `handler exceeded`。

Gateway **没有** `ResponseHeaderTimeout`。所谓「upstream 超时 >45s」是 **客户端空等**，不是平台 45s 门闩。

**产品边界（不要忘）：** Dashboard 禁止 Next；用户 Worker 只允许 **自构建单 Worker artifact**；不做 Node SSR 托管、不做多 `[[services]]`。

---

## 2. 复现（短超时）

```bash
# prod（旧 artifact，快失败）
curl -sS -m 3 -D - -o /tmp/s30-prod.html \
  -H 'Host: support-opennext.lvh.me' http://127.0.0.1:8787/

# preview（正确 Host，禁止 path /support-opennext/v42/）
curl -sS -m 8 -D - -o /tmp/s30-v42.html \
  -H 'Host: v42.support-opennext.lvh.me' http://127.0.0.1:8787/
# 或直打 celld
curl -sS -m 8 -D - \
  -H 'Host: v42.support-opennext.lvh.me' http://127.0.0.1:8839/

# 对照：ASSETS 命中
curl -sS -m 2 -o /dev/null -w '%{http_code} %{time_total}\n' \
  -H 'Host: v42.support-opennext.lvh.me' http://127.0.0.1:8839/favicon.ico
```

| 目标 | 结果（2026-09-03 末） |
|------|----------------------|
| prod v22 `GET /` | **400 / ~1–150ms** proto-rel |
| v22 favicon | **200 / ~2ms** |
| v42 health `:8839` | **200 / <1ms** |
| v42 favicon | **200 / <1ms** |
| v42 `GET /` | **8s / 0 byte**（curl 28）· **2026-09-04**：lab celld + patch 后 instrumented bundle **~45ms 返回**（404/Next 头，非 hang） |
| v42 `/_next/static/...` miss | 曾同 hang；**2026-09-04** 直打 celld **200**（~24KB CSS） |
| v38 Gateway | 曾 **503 `route draining`**（route inactive，勿与 hang 混） |

**专用日志（不是 `dev/data/logs/celld.log`）：**

```
$TMPDIR/celld-support-opennext-v42.log
```

挂起窗口内只有 lease/ship 心跳；curl 断开后 `http_connection_failures` / `incomplete_message`。

**换二进制陷阱：** `POST …/archive` + `/wake` **不保证**换 celld 进程。必须 `kill` 旧 PID，再 archive/wake，核对 **新 start 时间 + `lsof` → `celld/target/lab/celld`**。

---

## 3. 已排除 / 部分排除

| 假设 | 结论 |
|------|------|
| Gateway 反代超时 / path 预览 308 | **排除**；直打 celld 同样 hang |
| celld ingress `pathname === "//"` 308 环 | **已修**；不解释 400 文案，也不解释 0-byte |
| PD-06 Nitro `setImmediate` stub | Nitro 走 h3/`node:timers`；OpenNext 另需 **`process.setImmediate`**（unenv `process` 无此字段）→ **见 §4.2 `__celldPatchProcessTimers`** |
| `PUBLIC_BASE_URL` :8787 vs 直连 :8839 → Egress 打自己 | **弱**。canonical 命中先 **Loopback**；preview Host 对不上时应 **Reject**（快失败），解释不了 hang |
| ImageOptimizer 400 | **仅 v22**；去掉校验后进入更深路径 |
| `_next/image` BYOB | 对 **`GET /` 间接**。`handleImageRequest` 才 `getReader({mode:"byob"}).readAtLeast(32)` |
| 同源 `GET /` 再 selfFetch | **第 1 轮证伪**：`op_fetch_plan` **零命中** → SSR **没走到** `globalThis.fetch` |
| `IncomingMessage` 不是 `Readable` | **第 2 轮未过**：改 `node:stream` 后仍 8s/0 byte |

---

## 4. 做过的尝试（按时间）

### 4.1 Overlay / 构建（不换 celld 语义）

- `dev/examples/support-opennext/prepare-artifact.sh`：slash / Location / proto-rel / localPatterns / cookie / `images.unoptimized`。
- proto-rel 改为 **pathname** 归一化；开关 `CELLP_OPENNEXT_PROTO_PATCH_ONLY` / `SKIP_PATCH`。
- `deploy-support-app.sh`：`pick_support_version` **跳过 archived**。
- 新 artifact **v38–v42**：400 串可去掉；**preview `GET /` 变 hang**。prod **回滚 v22** 作基线。

### 4.2 celld（submodule，需 `cargo build -p celld --profile lab`）

| 主题 | 文件 | 验收 |
|------|------|------|
| ingress / fetch URL 折叠 `//` | `logic/http.rs` · `main.rs` · `js.rs` | 308 已消；**不治 hang** |
| loopback 重入：`finish_turn` 持 `CurrentGuard`；`wake` 在 `event_depth>1` 时 Poll；`__invokeSelfFetch` 继承 ctx | `js.rs` · `runtime.rs` · `harness.js` | **rebuild 后 v42 仍 hang** |
| `CelldHttpBodyStream` BYOB / `readAtLeast` | `harness.js` | 未单独打通 `/_next/image`；**不治 `GET /`** |
| 同 path loopback 拒绝 + `inboundPathname` + `fetch_plan` info | `harness.js` · `js.rs` | **无 fetch_plan 日志** |
| `IncomingMessage`/`ServerResponse` 继承 `node:stream` | `node_http.js` | **仍 hang**（补丁前） |
| **`process.setImmediate` on unenv `process`** | `harness.js` · `bootstrap.rs` `patch_process_timers` | **2026-09-04**：`requestHandler` 可 settle；需 **rebuild celld + kill/wake** 换二进制 |

**不要把 PD-10 写成「S30 已好」。** hang 形态可消除；S30 用户验收 **仍不过**（prod **400** proto-rel、矩阵 **不支持**）。

### 4.3 运维踩坑

- 长 `curl -m 120` / 无 `-m` 把墙钟吃掉（上一轮 subagent）。
- archive/wake **复用旧 PID**（21:00 进程 vs 21:35 二进制）。
- 看错日志文件（共享 `celld.log` 无 request 行）。

---

## 5. 当前最可能热路径（未证）

```
ingress GET /
  ├── favicon → ASSETS（快 200）
  └── fetch_worker → worker_default.fetch
        → handler2(middleware)
        → OpenNext IncomingMessage + createServerResponse
        → Next node:http SSR
        → Promise 永不 settle
```

bundle 入口：`dev/data/artifacts/support-opennext/v42/.cellp-bundle/index.js`  
`worker_default.fetch` ~115317；`handler2` ~114796；OpenNext `IncomingMessage` ~111234（自带 `_read`/`push`）。

**卡在进 `globalThis.fetch` 之前**（无 `fetch_plan`）。候选：

1. Next / OpenNext 等 **`res.end` / 流式 `finish`**，而 celld `node:http` 或 stream 泵未把 handler Promise 推到 settle。  
2. ~~**`process.setImmediate` 缺失**~~ → **2026-09-04 已修**；仍可能有 **`process.nextTick` / 微任务** 与 `finish_turn` 顺序问题。  
3. OpenNext **`DetachedPromiseRunner` / `awaitAllDetachedPromise`** 等内部队列永不 resolve。  
4. `ServerResponse` 与 OpenNext **自写的 response 类** 双轨，我们改的 `node:http` **没被这条路径用到**。

---

## 6. 续作纪律

1. **`-m ≤ 8`**；禁止循环探测；禁止本问题再 `prepare-artifact` / promote，除非刻意换 artifact。  
2. 日志用 **`$TMPDIR/celld-support-opennext-v42.log`**。  
3. 先确认 **新 celld PID + lab 二进制**。  
4. 下一刀应是 **bundle 内 `handler2` / `createServerResponse` 进出日志**（或最小 OpenNext fixture），不要再盲改 loopback/BYOB。  
5. **不**把 S30 标成矩阵「支持」；**不**把 Next 升 AD-13 一等公民。

---

## 7. 关键路径

| 用途 | 路径 |
|------|------|
| 本记录 | `docs/plans/S30-OPENNEXT-HARD-PROBLEM.md` |
| 实验计划 | `docs/plans/NEXT-OPENNEXT-CELLP.md` |
| overlay | `dev/examples/support-opennext/prepare-artifact.sh` |
| celld | `celld/crates/celld/js.rs` · `runtime.rs` · `js/harness.js` · `js/node_http.js` · `fetch_loopback.rs` |
| artifact | `dev/data/artifacts/support-opennext/v42/` |
| 验收 Host | `v42.support-opennext.lvh.me` · prod `support-opennext.lvh.me` |
