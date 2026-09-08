# Native Wasm Runtime — R1 delivery (experimental)

> **Status:** Shipped **experimental** on celld **`main`** @ `f4619c3` (merged from `adlc/native-wasm-runtime-product-r1`; qualification **WP-Q3 PASS** @ `44a3259`).
> **Public docs:** [Native Component Worker](https://konghayao.github.io/cellp/build/native-component.html) · celld [`runtime-bindings.md`](../../celld/docs/runtime-bindings.md)
> **Decision:** [AD-16](../decisions.md#21-ad-16--experimental-native-component-http-native-http-v1)
> **E2E:** `e2e/scripts/v18-native-wasm.sh` · evidence `docs/evidence/native-wasm-e2e.*`

## Product outcome (R1)

| Item | Choice |
|------|--------|
| Engine | **Wasmtime 48.0.1** only (`cellp-native-baseline-1`) |
| Model | WebAssembly **Component** + WASIp2 **`wasi:http/proxy@0.2`** |
| Profile | Explicit **`native-http-v1`** in wrangler `cellp.execution` |
| Bindings | **`cellp:config@0.1`**, **`cellp:kv@0.1`** (vars + KV get/put/delete) |
| Ingress | Gateway **Host** → per-version celld Native adapter |
| Coexistence | JS/V8 default and **`wasm-v1`** unchanged |

## Non-goals / boundaries

- Not workers-rs binary compatible; not Cloudflare Workers API parity.
- No D1/R2/Queue/Workflow/Cron/DO/assets/outbound HTTP/Native WebSocket in R1.
- No second KV store; Native uses existing celld KV semantic path.
- Bounded stop (deadline, cancel, memory, trap discard) mandatory; qualification crate covers engine experiments — port E2E covers Gateway/KV/isolation/smoke.
- **Not** a hostile multi-tenant guarantee.

## Admission (fail-closed)

Manifest records `native_component`: module path, SHA-256, baseline, world, capability list. Mismatch or unknown profile/world/import → deploy/ready error before serving.

Implementation seams: `celld/crates/celld/protocol.rs`, `deploy.rs`, `native/**`, generation dispatch in `runtime.rs` / `fleet.rs`.

## Verification map

| Gate | Artifact |
|------|----------|
| G-Q | `celld/crates/native_qualification/` · Q3 handoff |
| G-C | Product contract review handoff |
| G-E2E | `v18-native-wasm.sh` · `docs/evidence/native-wasm-e2e.json` (`exit: 0`) |
| G-REL | Full `RUN_GATES=1 run-all.sh` + site build — release integrator |

## Research lineage

- [NATIVE-WASM-RUNTIME-RESEARCH.md](./NATIVE-WASM-RUNTIME-RESEARCH.md)
- [NATIVE-WASM-RUNTIME-R1.md](./NATIVE-WASM-RUNTIME-R1.md) (desk scan; Wasmtime advance)
- [NATIVE-WASM-RUNTIME-REVIEW.md](./NATIVE-WASM-RUNTIME-REVIEW.md)

Frozen **D1** RPC contracts were not modified for this delivery.

## Submodule

cellp gitlink **`f4619c3`** tracks **celld `main`** (native-http-v1 runtime). Rebuild after bump:

```bash
cd celld && git checkout main && cargo build -p celld --profile lab
cp target/lab/celld ~/.local/bin/celld
./dev/scripts/up.sh && ./e2e/scripts/run-all.sh --only v18-native-wasm
```

Qualification baseline reference: **`44a3259`** (G-Q). Feature branch `adlc/native-wasm-runtime-product-r1` is merged; use `main` for new work.
