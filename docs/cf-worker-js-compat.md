# CF Worker / nodejs_compat — celld 与社区 JS 栈缺口

> **用途：** 记录 **Wrangler + nodejs_compat** 社区 bundle 在 **celld** 上暴露的 **Node 内置模块** 缺口与 **应用侧缓解**（非 celld 实现细节）。  
> **权威 celld 矩阵：** [`celld/docs/cloudflare-compat.md`](../celld/docs/cloudflare-compat.md) · **缺陷排期：** [`platform-defects-log.md`](./platform-defects-log.md)

---

## 1. `node:zlib` — callback `gzip` / `gunzip`（A05 Mastra）

| | |
|--|--|
| **历史现象** | Worker instantiate 失败 → `celld health timeout` |
| **日志** | `promisify(original)` · `Received undefined` ← `import { gzip } from "node:zlib"` |
| **celld 现状** | **已修复**：同步 helper 外，还提供 callback `gzip` / `gunzip` / `deflate` / `inflate`，通过 `process.nextTick` 延后执行，可被 `util.promisify` 包装 |
| **修复** | celld `4b3a3bf`（2026-09-04） |
| **缺陷 ID** | [PD-20260903-01](./platform-defects-log.md#pd-20260903-01--mastra-wrangler-bundleutilpromisifyundefined-导致-worker-无法-instantiatecelld)（`fixed`） |
| **运行证据** | A05 `v14` strict acceptance；bundle 仍含 `promisify(gzip)` 时可正常 instantiate |

### A05 打包结论

`@mastra/core` 的 feature telemetry 静态引入 `posthog-node`；该包在模块顶层执行 `promisify(gzip)`。Mastra 的第一阶段 bundle 已将它内联，因此后续 Wrangler dry-run 的 `alias` 无法替换这段代码。最终 `.cellp-bundle/index.js` 仍含 `promisify(gzip)`，A05 `v14` 能正常 instantiate 证明生效的是 celld runtime 修复，而不是应用 stub。

- `MASTRA_TELEMETRY_DISABLED=1` 仍用于关闭本 demo 的运行时 telemetry，但不会改变依赖图。
- 无效的 `posthog-node` alias/stub 已删除；A05 不再携带运行时缺口的应用侧绕过。
- Mastra upstream 若让 `posthog-node` 完全可选，可继续缩小 bundle，但这不再是 cellp/celld 运行 blocker。

---

## 2. 其它已记录 nodejs_compat 项（索引）

| 主题 | 文档 |
|------|------|
| `cloudflare:workers` 多模块 ESM stub | PD-20260902-01（fixed） |
| Nitro SSR / `node:timers` | [`plans/NITRO-CELLD-COMPAT.md`](./plans/NITRO-CELLD-COMPAT.md) |
| OpenNext / `node_crypto` | [`plans/NEXT-OPENNEXT-CELLP.md`](./plans/NEXT-OPENNEXT-CELLP.md) |
| Qwik 误开 `nodejs_compat` | [`support-framework-user-acceptance.md`](./support-framework-user-acceptance.md) S28 |

---

## 3. 维护约定

- **celld 能力变更** → 同步 [`celld/docs/cloudflare-compat.md`](../celld/docs/cloudflare-compat.md)。
- **新 support 项目踩坑** → 在本文件加小节 + 链到 `platform-defects-log.md` 条目。
- **不要**把长期应用 patch 写进本文件；仅 **可复用的 overlay / alias / env** 模式。
