# support-fx-on-workers（cellp overlay）

上游 [fx-on-workers](https://github.com/codingstark-dev/fx-on-workers) 默认是 **浏览器 TUI + WebSocket**（`/session` → `FxSession` DO + fx wasm）。

## cellp 本地栈：WebSocket 限制

当前 **Gateway → celld** 对 Durable Object 的 **WebSocket 升级** 在验收中表现为 **`/session` → 502**（TUI 页可 200，但 xterm 一直 `connecting…`）。

- 平台跟踪：见仓库根目录 [`docs/platform-defects-log.md`](../../../docs/platform-defects-log.md)（fx / DO WebSocket 条目）。
- **不要**把「fx 在 cellp 上不可用」等同于「fx 必须 WebSocket」——是平台 WS 通路未就绪。

## cellp 产品形态（HTTP 验收）

本目录在 `deploy-support-app.sh A04` 时注入：

| 文件 | 作用 |
|------|------|
| `cellp-overlay/index.js` | 增加 `GET /api/health`、`POST /api/prompt` |
| `cellp-overlay/patch-session.mjs` | 为 `FxSession` 增加 `X-Cellp-Mode: http-prompt`（不依赖浏览器 WS） |

### 用法

```bash
KEY=cellp-dev-fx-on-workers
HOST=support-fx-on-workers.lvh.me
GW=http://127.0.0.1:8787

curl -sS -H "Host: $HOST" "$GW/api/health?key=$KEY"

curl -sS -m 180 -X POST -H "Host: $HOST" -H "Content-Type: application/json" \
  "$GW/api/prompt?key=$KEY" \
  -d '{"prompt":"run: echo cellp-fx-ok","waitMs":90000}'
```

**验收看点：** HTTP 200；`events` 含 `ready`；若模型实际跑了 shell，`commands[]` 非空。完整 fx 回合仍需 **`AI_GATEWAY_API_KEY`**（Vercel AI Gateway，非 OpenCode Zen）。

### 与上游差异

| 上游 | cellp overlay |
|------|----------------|
| 仅 `/` + WS `/session` | 额外 **HTTP `/api/prompt`** |
| 长连接 TUI | HTTP 一轮 prompt + 收集 `command` 事件 / TUI 输出尾部 |

上游 WebSocket 路径**保留**；cellp 修通 WS 后浏览器 TUI 可继续用，HTTP 路径仍可作为自动化门禁。
