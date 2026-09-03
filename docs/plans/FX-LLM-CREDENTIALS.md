# fx 推理凭证 — cellp 范围说明

> **状态：** 已记录 · **不做** cellp 侧 OpenCode 适配  
> **日期：** 2026-09-03

## 结论

| 问题 | 答案 |
|------|------|
| fx 能否用 OpenCode Zen（OpenAI 兼容 URL + key）？ | **上游不支持**；wasm 走 **Vercel AI Gateway** `v3/ai/language-model`，不是 `/v1/chat/completions` |
| cellp 是否做 Gateway → Zen 的 shim？ | **否**（刻意 out of scope；改造成本中～大，见对话/专题分析） |
| cellp 上 fx 验收怎么做？ | **WS 通路**：`dev/scripts/fx-websocket-smoke.sh`（可无 key）；**完整 agent**：用户提供 **`AI_GATEWAY_API_KEY`** |

## OpenCode 用在哪里

- **Pi（A02）**：`OPENAI_BASE_URL=https://opencode.ai/zen/v1` + `OPENAI_API_KEY` —— 已支持。

## 用户提供 Vercel Gateway key 后

1. 写入 **不提交** 的本地配置（`.dev.vars` 或 `dev/.env` 由 deploy 脚本注入 version env —— 以当前 `deploy-support-app.sh` / overlay 为准）。
2. `./dev/scripts/deploy-support-app.sh A04`
3. `bash dev/scripts/fx-websocket-smoke.sh` —— 期望除 error 外有 **binary** 或 `command` 事件。

## 若未来要 OpenCode + 终端形态

- 依赖 **vercel-labs/fx / libfx** 官方支持自定义 LLM endpoint，或单独 fork；**不在 cellp 主线**。
