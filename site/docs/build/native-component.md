# Native Component Worker (experimental)

**Support level:** experimental **0.x** · **Wasmtime-only** · **not** binary-compatible with workers-rs · **not** a hostile multi-tenant security boundary.

cellp can deploy a **WebAssembly Component** that serves **stateless inbound HTTP** without a JavaScript or V8 shim. Traffic still enters through the **Gateway preview/prod Host** → per-version **celld** (same model as [Gateway routing](/concepts/routing)). The default path remains a JS Worker — see [Write a Worker](/build/).

## How this differs from JS and `wasm-v1`

| Path | What you ship | Runtime |
|------|----------------|---------|
| **JS / V8 (default)** | `main` + JS/TS bundle | celld V8 + Workers APIs |
| **`wasm-v1`** | JS shim imports a core `.wasm` module | Still V8; not this page |
| **`native-http-v1`** | Component bytes + explicit wrangler profile | **Wasmtime** Component Model in celld |

A **workers-rs** build normally emits a JS shim plus a core module. That artifact must keep using the JS path. Native requires a component targeting the **cellp WIT world** checked into celld (`wasi:http/proxy@0.2` + `cellp:config@0.1` + `cellp:kv@0.1`).

## Declare and deploy

Execution is **never** inferred from a `.wasm` file name. Set it in wrangler:

```jsonc
{
  "name": "my-native",
  "compatibility_date": "2026-01-01",
  "cellp": {
    "execution": "native-http-v1",
    "component": "component.wasm"
  },
  "vars": {
    "GREETING": "hello"
  },
  "kv_namespaces": [
    {
      "binding": "CACHE",
      "id": "my-native-cache"
    }
  ]
}
```

Stage the artifact directory the same way as a JS Worker (`component.wasm` beside `wrangler.jsonc`, then `POST /versions`). cellpd runs `celld deploy`, which records:

- component bytes and **SHA-256** digest
- baseline **`cellp-native-baseline-1`** (Wasmtime **48.0.1** qualification pin)
- world **`wasi:http/proxy@0.2`**
- required interfaces **`cellp:config@0.1.0`**, **`cellp:kv@0.1.0`**

Unknown execution profiles, worlds, extra imports, digest mismatch, or incompatible wrangler combinations **fail closed** before the version serves traffic. Native deployments **cannot** set `main`, static assets, D1/R2/Queue/Workflow/Cron/DO bindings, migrations, or cron triggers in R1.

**Reference fixture:** [`dev/examples/native-wasm`](https://github.com/KonghaYao/cellp/tree/main/dev/examples/native-wasm) and `scripts/build-component.sh` (target **`wasm32-wasip2`**).

## WIT bindings (experimental)

| Package | Version | Guest use |
|---------|---------|-----------|
| `cellp:config/config` | `0.1.0` | `get(name)` → declared var or absent |
| `cellp:kv/kv` | `0.1.0` | `open(binding)` → `get` / `put` / `delete` on byte values |

- **Config:** only names from the admitted manifest snapshot; no enumeration API.
- **KV:** pass the **Wrangler binding name** (e.g. `CACHE`), not the namespace `id`. Undeclared bindings are **denied** without revealing other namespaces.
- **HTTP:** inbound only via `wasi:http/incoming-handler` inside the proxy world. **No** outbound HTTP, **no** Native WebSocket upgrade (use JS/V8 for WebSocket).

KV uses the **same data plane** as JavaScript: `__KvNamespace` cell, version bucket, LTX, branch rules, and cellp operator KV API. Guest writes are visible to operators; operator writes are visible to the guest. Sibling versions stay isolated per [Versions](/concepts/versions) even when namespace `id` strings match.

## Build your component

1. Copy WIT deps from `celld/crates/celld/native/wit/` (or follow the example guest).
2. Export `wasi:http/incoming-handler@0.2.12` and import the cellp interfaces you need.
3. `cargo build --target wasm32-wasip2 --release`.
4. Ship the **component** artifact (not a core module only).

Internal ABI details: celld [`runtime-bindings.md`](https://github.com/KonghaYao/cellp/blob/main/celld/docs/runtime-bindings.md).

## Errors and resource limits

Each request gets a **fresh** Wasmtime store and component instance with finite:

- request deadline and cancellation grace
- memory, request/response body size, hostcall count

Completion, client cancel, deadline, memory exhaustion, or a guest **trap** discards that instance; the next request starts clean. celld stays up; later healthy requests on the same version succeed.

HTTP failures use stable, engine-neutral categories (examples: `capability_denied`, `hostcall_failed`, `guest_trap`, `deadline_exceeded`, `resource_exhausted`, `cancelled`). Responses must **not** include Wasmtime trap text, backtraces, or secrets. Variable values are redacted from capability debug output; you are still responsible for not echoing secrets in HTTP bodies.

This is **defensive resource containment**, not proof of safe co-tenancy of mutually untrusted code on one machine. Isolate untrusted workloads at a stronger boundary.

## Unsupported in R1 (use JS/V8)

| Capability | Native R1 |
|------------|-----------|
| D1, R2, Queues, Workflows, Cron, DO, service bindings | **No** |
| Static assets / `main` JS | **No** |
| Outbound `fetch` / WASI outbound HTTP | **No** |
| WebSocket upgrade on Native path | **No** (Gateway→celld WebSocket for **JS** remains supported) |
| workers-rs binary as-is | **No** — wrong world and shim model |
| Edge cache semantics on KV | Same celld limits as JS ([KV](/bindings/kv)) |

## Verify locally

With the dev stack up ([Local stack](/get-started/local)):

```bash
./e2e/scripts/run-all.sh --only v18-native-wasm
```

Proof uses **Gateway Host** routing and real KV — not direct celld port access alone.

## Related

| Topic | Page |
|-------|------|
| Default JS Worker | [Write a Worker](/build/) |
| KV data + branch | [KV](/bindings/kv) · [Platform data](/build/data) |
| Product limits | [Limits](/reference/limits) |
| Cloudflare API gaps | [Compatibility](/reference/compatibility) |
