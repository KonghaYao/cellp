# support-fx-on-workers（cellp overlay）

上游 [fx-on-workers](https://github.com/codingstark-dev/fx-on-workers) 默认是 **浏览器 TUI + WebSocket**（`/session` → `FxSession` DO + fx wasm）。

## WebSocket 冒烟（Worker / DO 通路）

**cellp：** WebSocket ingress **已支持**（Gateway 101 → DO 会话帧；见 `docs/plans/WEBSOCKET-SUPPORT-ANALYSIS.md`）。

```bash
./dev/scripts/fx-websocket-smoke.sh
```

证据追加：`docs/evidence/fx-websocket-worker-smoke.log`。仅出现 `AI_GATEWAY_API_KEY` 相关 `error` 时脚本仍 **PASS**（通路 OK），并打印 `NEEDS_AI_GATEWAY=1`。

### 推理凭证（Vercel AI Gateway）

fx wasm **只认** `AI_GATEWAY_API_KEY`（[Vercel AI Gateway](https://vercel.com/docs/ai-gateway)），**不是** OpenAI / OpenCode Zen。

| 项 | 说明 |
|----|------|
| **cellp 立场** | **不做** fx → OpenCode（OpenAI 兼容）协议适配；见 `docs/plans/FX-LLM-CREDENTIALS.md` |
| **完整 TUI 回合** | 配置 Gateway key 后重跑 `fx-websocket-smoke.sh` 或浏览器 TUI |
| **本地注入** | `dev/support-corpus/.../.dev.vars` 或部署前在 `wrangler.cellp.jsonc` 增加 `vars`（勿提交密钥）；也可用 Platform version env API |

```bash
# 示例（勿把真实 key 提交仓库）
# echo 'AI_GATEWAY_API_KEY=...' >> dev/support-corpus/support-fx-on-workers/.dev.vars
# 然后 redeploy A04
```

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
