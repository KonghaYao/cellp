# Native Wasm E2E（WP-T1/T2）

- 日期：2026-09-06
- 合同：`intent.md` revision 1；`execution.md` revision 1
- Qualification：Q3 PASS at celld `44a3259bf2e50ade2204f978108e89b630de4b2e`
- 当前 celld：`7dae6d9512801ddd3140d003774f9ede7e0de7e1`
- Runtime：Wasmtime-only；experimental `native-http-v1` + `cellp:config@0.1` / `cellp:kv@0.1`
- 结论：目标脚本的 HTTP、真实 KV、未授权恢复、version 隔离、JS smoke **PASS**；完整 TP-NATIVE / G-E2E 仍因 guest trap 与独立 hostcall failure 缺少端口级触发 fixture 而 **BLOCKED**。

## Binary 与 fixture provenance

| 项 | 值 |
|---|---|
| celld binary | `celld/target/lab/celld`（本 worktree） |
| celld SHA-256 | `fe80d6eb0569241edacdc4fc13a2408bb48ed811abaf85fa58d7a79a1d15ef00` |
| Native fixture SHA-256 | `e9f31acd199104f93635d2789b0e52867cc1ef16f30b30de4cca51159a880e91` |
| Gateway/API/celld dev ports | `18787` / `18790` / `18792`（避免复用另一 worktree 的进程） |

`PATH` 首项为本 worktree 的 `celld/target/lab`。未使用 mock；HTTP 成功与错误请求均经 Gateway preview Host。直连 shared celld 只用于附加健康探针，不作为 Native HTTP 的唯一或主要证据。

## 实现与登记

- 新增 `e2e/scripts/v18-native-wasm.sh`。
- 登记 `e2e/scripts/MANIFEST`。
- 在 `docs/test-plan.md` 新增未勾选的 `TP-NATIVE`，避免将部分场景 PASS 冒充完整 gate。
- JSON/log：`docs/evidence/native-wasm-e2e.json`、`docs/evidence/native-wasm-e2e.log`。

## 最终目标运行

```bash
./dev/scripts/up.sh && ./dev/scripts/health.sh
./e2e/scripts/run-all.sh --only v18-native-wasm
```

结果：最终一次 exit 0，68 秒。目标 runner 明确打印 `not TP-VE-ALL`。

已证明：

1. **正式 Gateway Host HTTP**：Native VA 经 `Host: {version}.{project}.lvh.me` 执行 guest `PUT` 后 `GET` 返回确定值。
2. **guest → operator KV**：guest 经 Gateway 写入后，cellpd `:18790` operator API 对同 version/namespace/key 返回相同值。
3. **operator → guest KV**：cellpd operator API 写入第二个 key 后，guest 经 Gateway 读取到相同值。
4. **未授权与恢复**：移除部署中的 KV namespace 授权但保留 guest `cellp:kv@0.1` import；KV 请求返回 engine-neutral HTTP 500（body 含归一化 error code，且不含 Wasmtime/WebAssembly/backtrace/credential 文本）；随后已授权 VA 的 guest 请求继续 HTTP 200，cellpd runtime route 对 VD 报 `celld_health: ok`。
5. **两个 version 隔离**：VA/VB 使用同 namespace/key；VB 初始 operator/guest GET 均为 404；VB 写入后 VA 仍保留原值。
6. **JS smoke**：同一目标脚本部署既有 `dev/examples/counter`，经其 preview Host 返回 HTTP 200 JSON。

## Runtime bugs found and fixed

真实 E2E 首次启动 Native per-version celld 时出现稳定 `instantiate_failed`。本地受保护 startup 诊断确认 fixture 导入 `wasi:cli/environment@0.2.12`，而 adapter 仅注册 `wasi:http/proxy` 的最小 WASI imports。

修复采用 Wasmtime 48.0.1 官方 embedding 组合：

1. bindgen 复用完整 canonical `wasmtime_wasi::p2::bindings` namespace，避免生成不兼容的重复 WASI Host trait；commit `d5b5160`。
2. linker 调用 `wasmtime_wasi::p2::add_to_linker_async`，再调用 `wasmtime_wasi_http::p2::add_only_http_to_linker_async`；保留现有 outbound HTTP deny hook；commit `7dae6d9`。

临时原始 linker 诊断在定位后已撤销，未进入 commit、HTTP response 或本证据。

## 回归验证

| 命令 | 结果 |
|---|---|
| `cargo test -p celld native --locked` | PASS，17/17 |
| `cargo build -p celld --profile lab --locked` | PASS |
| `cargo test -p celld --locked` | PASS，86/86 |
| `cd cellp && go test ./...` | PASS |
| `./dev/scripts/health.sh` | PASS |
| `./e2e/scripts/run-all.sh --only v18-native-wasm` | PASS |

celld 编译仍报告仓库既有 unused/private-interface warnings；本工作包未扩大范围处理。

## 阻塞与诚实边界

当前 hermetic fixture 源码只实现 `/health` 和 KV CRUD，没有 guest panic/`unreachable`、deadline、memory-growth 或可控独立 hostcall failure 路由。WP-T1 的独占范围不包含 `dev/examples/native-wasm/**`，因此本工作包没有越权修改 fixture，也没有把未授权 KV 的 HTTP 500 冒充 guest trap。

所以：

- guest trap 后同进程健康的端口级证据：**BLOCKED**（需 WP-F1 owner 扩展 fixture 后重跑）。
- 独立 hostcall failure 后健康的端口级证据：**BLOCKED**（需可控但不泄密的 fixture/runtime fault trigger）。
- bounded cancellation、deadline、memory、trap discard/recovery：Q3 qualification 已 PASS，但 qualification 不能替代正式 Gateway E2E。
- 未运行完整 `RUN_GATES=1 ./e2e/scripts/run-all.sh`，不声明 TP-VE-ALL/M2/G-REL。
