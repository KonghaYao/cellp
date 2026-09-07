# Native Wasm Runtime — R2 Qualification HTTP Fixture

> **状态：** Proposed — 用户已批准 R2 adapter-only 范围（2026-09-06）
> **授权：** 仅 `native_qualification` 内 HTTP 适配器与 fixture；**不**授权 celld 生产 dispatch、OpenAPI、e2e、站点或产品支持声明
> **基线：** `cellp-native-baseline-1` · Wasmtime **48.0.1** · world **`cellp:native-http@0.1.0`** · WASI proxy **`wasi:http/proxy@0.2.12`**

---

## 范围

| 在范围内 | 不在范围内 |
|----------|------------|
| `native_qualification` 的 `http_adapter` / `http_bindings` | R3 · celld 集成 · Gateway 路由 |
| 内存 stub：`cellp:config` · `cellp:kv` | 产品 authorization · `decisions.md` 变更 |
| 加载 `dev/examples/native-wasm/component.wasm` 的 HTTP smoke | outbound HTTP（拒绝） |

---

## Runner

```bash
cd celld && cargo test -p native_qualification --locked
```

---

## Exit gate

- HTTP fixture **PASS** on Wasmtime **48.0.1**（`tests/http_fixture.rs`：`GET /health` → 200，body 含 `native-http-v1` 与 `hello-native`）
- 明确 **不是** R3/celld 集成或产品 authorization

---

## 链

- R1：[NATIVE-WASM-RUNTIME-R1.md](./NATIVE-WASM-RUNTIME-R1.md)
- 研究纲领：[NATIVE-WASM-RUNTIME-RESEARCH.md](./NATIVE-WASM-RUNTIME-RESEARCH.md)
