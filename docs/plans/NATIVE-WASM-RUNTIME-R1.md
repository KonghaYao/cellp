# Native Wasm Runtime — R1 Market Scan Ledger

> **状态：** R1 desk research deliverable **VERIFIED — R1 deliverable PASS**（非实验 PASS，非 M2）
> **日期：** 2026-09-05
> **授权：** 用户已批准 R1 Market Scan；本文件**不是** Architecture Decision、runtime 选型、冻结产品 ABI 或实现授权
> **PASS 消歧：** `DESK PASS` 是单项官方资料结论；`R1 deliverable PASS` 只表示本 desk 文档与仓内证据链达到 R1 交付要求；二者都不是 R2/R3 的可复现**实验 PASS**，也不是 [`docs/test-plan.md`](../test-plan.md) 定义的产品完成门禁 **M2**
> **禁止外推：** 不授权 R2/R3 prototype、产品代码、Registry/OpenAPI、`celld deploy`、现有 JS/V8 路径或公开支持声明的任何变更
> **研究权威：** [NATIVE-WASM-RUNTIME-RESEARCH.md](./NATIVE-WASM-RUNTIME-RESEARCH.md) · R0 [REVIEW](./NATIVE-WASM-RUNTIME-REVIEW.md)

---

## 0. Executive verdict

R1 的目标是检查哪些 exact release 值得提交下一次**独立授权**，而不是选 default runtime。

| 候选 | Exact release | R1 eligibility | 决定性结论 |
|------|---------------|----------------|------------|
| **Wasmtime** | `48.0.1` | **ADVANCE CANDIDATE** | G1–G10 desk 资料无决定性 FAIL，但 G7 仍为 `UNKNOWN`；仅可提交 R2 授权，所有 cellp/celld 行为仍待实验 |
| **Wasmer** | `7.4.0` | **HOLD** | 已真实评估；G4 对本 track 要求的标准 Component + WASIp2 `wasi:http` 路径为 **DESK FAIL** |
| **WAMR** | `2.4.5` | **HOLD** | 官方 module/WASIp1 资料不能证明 Component HTTP baseline；G4 **UNKNOWN**，按门禁不得进入计分/性能决赛 |
| **wasmi** | `2.0.0` | **HOLD** | release 为 core interpreter，官方 #897 仍在追踪“添加 Component Model 支持”；G4 **DESK FAIL（eligibility）** |
| **WasmEdge** | `0.17.1` | **HOLD** | 官方 #4236 显示 Component execution 不完整且 `wasi-http` 为 `None`；G4 **DESK FAIL（eligibility）** |
| **Spin** | reference only | 不评分 | 应用 framework/runtime，底层使用 Wasmtime；不是独立低层 engine |

**R1 Gate（final verdict）：** 只建议将 **Wasmtime 48.0.1** 带入一次新的 R2 授权请求；这既不选择 Wasmtime，也不证明 runtime replaceability。Wasmer 和其他候选保留重审入口；不得为凑“双引擎”而降低 Component + `wasi:http` 契约。

独立 verifier 已关闭 NW-R1-V01–V04；该 verdict 只关闭 R1 deliverable，不能自动开始 R2/R3。原始逐项 verifier findings 不在 git；§9 仅保留 closure 摘要，并在仓内以本节 verdict、§1.2 状态定义、§4–§6 gate dossier/matrix、§8 unknown ledger 与 §10 source log 作为结论映射锚点。该映射只说明当前 ledger 中哪些段落承载修订后的结论与限制，**不是**原始 findings、逐项审查文本或其重建。

---

## 1. 范围、方法与证据词汇

### 1.1 调查范围

- Native v0：stateless HTTP `Native Wasm Component + Binding Host`。
- 执行路径不得依赖 JavaScript/V8；现有 JS/workers-rs 路径保持不变。
- 候选必须可 100% 私有化并由 Rust host 嵌入。
- R1 只读官方 release、tag、crate metadata、版本化源码、官方文档、安全政策与治理资料。
- 未运行 component、benchmark、fault injection、celld integration 或 conformance。

### 1.2 状态定义

| 状态 | 本文件含义 |
|------|------------|
| **DESK PASS** | 官方资料对该门槛没有决定性阻塞，并提供所需机制/能力的正面证据；**不是** R2/R3 PASS |
| **DESK FAIL** | exact release 的官方 architecture/API/support evidence 与本 track 已冻结要求不相容；不是永久否定项目未来版本 |
| **UNKNOWN** | 一手资料不能证明；不得猜测、计分或自动推进 |
| **HOLD** | 当前不进入性能决赛/实现 spike；满足明确重审条件后可用新 exact release 重做 R1 |
| **ADVANCE CANDIDATE** | 仅具备提交下一阶段授权请求的资格；不是首选、default、AD 或 shipped support |

动态网页只证明访问日的 desk fact，不能冒充 exact release 的 pinned API。官方网页也不能替代 R2/R3 的可复现实验。

### 1.3 G1–G10 映射

| Gate | 研究纲领 §6.2 硬门槛 |
|------|----------------------|
| **G1** | OSI-compatible license 与 cellp 分发方式可兼容 |
| **G2** | 可完全私有化、离线；核心执行不依赖 vendor control plane/SaaS |
| **G3** | 有受支持或可维护的 Rust embedding 路径 |
| **G4** | 执行选定 Component Model/WASI HTTP baseline，或存在边界清晰、维护成本可接受的 adapter |
| **G5** | 可限制 memory、CPU/执行时间、并发与 host capabilities |
| **G6** | trap/OOM/timeout 不结束 celld 进程 |
| **G7** | 支持 async hostcalls 与请求取消，不长期阻塞 Tokio worker |
| **G8** | 有安全政策、版本与漏洞响应渠道 |
| **G9** | x86_64 Linux、arm64 Linux 可构建并过 conformance；macOS 可开发 |
| **G10** | 可在不暴露引擎类型的情况下实现研究纲领 §4 adapter |

---

## 2. 标准基线

### 2.1 规范状态与实现状态必须分开

| 事实层 | R1 结论 |
|--------|---------|
| **WASI 0.2 / WASIp2 specification** | Stable，现已被 0.3 supersede；基于 Component Model/WIT，HTTP serverless 入口使用 `wasi:http/proxy` |
| **WASI 0.3 / WASIp3 specification** | Stable、current；加入 native `async func`、`stream<T>`、`future<T>`；HTTP world 与 0.2 不可混称 |
| **工具链现实** | WASI 官方页面称多数其他 toolchain 仍目标 0.2；选择 0.3 规范不等于所有候选引擎已有成熟 host implementation |
| **Wasmtime p3 implementation** | 官方 API 明示 `experimental, unstable and incomplete`、不受 semver、未 ready for production，且 p3-only bug/security fix 不保证 patch release |

因此，“WASIp3 规范已发布”与“某个 engine 的 p3 host API 仍实验性”是两个兼容事实。

### 2.2 R1 practical baseline

R1 建议将 **WASIp2 + `wasi:http/proxy`** 作为 R2 test baseline 的 practical direction：

- 与 stateless HTTP proxy/handler 目标直接对齐；
- Wasmtime 有明确 Component linker、incoming/outgoing handler 与 Tokio host 实现；
- 多数其他 toolchain 仍以 WASIp2 为主；
- 不迫使 R2 同时研究两套 world 与异步模型。

这不是 baseline freeze。只有 R2 获独立授权后，且在执行前，才能记录单一 `test baseline identifier`，至少包括 WASI line、`wasi:http` world 与所有 WIT package exact versions。

---

## 3. Exact-version inventory

| 候选 | Tag | Version / notes date | GitHub publication | Commit | 证据强度 |
|------|-----|----------------------|--------------------|--------|----------|
| Wasmtime | `v48.0.1` | 2026-08-24 | 2026-08-24 | `7bac2c2775808aaec5d4aa5627a5e447b51102cf` | release + crates.io provenance pinned |
| Wasmer | `v7.4.0` | 2026-08-31 | 2026-08-31 | `32b50f8b600efa8e2d5f88593c453139bf1ca222` | release + versioned source pinned |
| WAMR | `WAMR-2.4.5` | 2026-06-29 | 2026-06-29 | `25bd7eb63e828e4bd242cc9b38d260b4b31c6605` | release pinned |
| wasmi | `v2.0.0` | 2026-09-01 | 2026-09-01 | `2970aa871cc1001b57b267ccecdcd1e42306199e` | release pinned |
| WasmEdge | `0.17.1` | 2026-07-03 | 2026-07-06 | `0502c560d787d969c66f5676b9e11ac64bba3656` | release/tag commit pinned；区分 notes version date 与 GitHub publication |

---

## 4. Wasmtime 48.0.1 dossier

### 4.1 Release facts

- `v48.0.1` release 与 crates.io 均指向同一完整 commit。
- crates.io `wasmtime 48.0.1`：`Apache-2.0 WITH LLVM-exception`，Rust `1.95.0`，包含 `component-model`、`async`、`component-model-async` default features。
- patch notes 包含 Component composition context slot 修复，以及 WASIp2 outbound HTTP 默认设置 `Host` header。
- Wasmtime release policy 将 48.0.0 列为 LTS，支持至 2028-08-20；48.0.1 是该 release line 的 patch。

### 4.2 G1–G10

| Gate | Desk status | 官方证据与边界 | 实验 unknown |
|------|-------------|----------------|--------------|
| G1 | **DESK PASS** | tag license 与 crate metadata 均为 Apache-2.0 WITH LLVM-exception | 最终依赖/SBOM 仍须审计 |
| G2 | **DESK PASS** | 本地 Rust crate/CLI，可自托管；执行不要求 vendor control plane | air-gapped build/reproducibility 未测 |
| G3 | **DESK PASS** | Rust embedding API 为官方 Tier 1 | celld build/link/API churn 未测 |
| G4 | **DESK PASS** | Component Model 与 `wasi-http` 为 Tier 1；p2 模块实现 WASIp2 `wasi:http/proxy`，提供 async linker/instantiate 路径 | exact WIT world/package bytes 待 R2 |
| G5 | **DESK PASS** | Store limiter、fuel/epoch、显式 imports/capabilities 提供机制 | cellp limits profile、host-side buffers、并发待 R2/R3 |
| G6 | **DESK PASS** | safe Rust API、sandbox/import boundary 与 trap/error API 提供隔离机制；官方同时指出 host configuration/compiler bug 命中 native guard page 可 abort，因此不能外推 celld 进程安全 | trap/OOM/deadline 后 celld 存活和 instance discard 待故障注入 |
| G7 | **UNKNOWN** | default async/component-model-async、p2 Tokio 与 async host linker 证明 async mechanism 存在；但整个 gate 还要求请求取消 | `call_concurrent` 官方文档称当前只能 drop 整个 Store hard-cancel，drop future 不取消任务；client cancel、hostcall deadline、bounded stop、stream owner 未证 |
| G8 | **DESK PASS** | 有公开 security policy/advisory channel；supported release 保证 security backport | cellp pinning/升级响应流程未设计 |
| G9 | **DESK PASS** | x86_64 Linux/macOS Tier 1；aarch64 Linux/macOS Tier 2（缺 continuous fuzzing） | cellp 两种 Linux 架构 build+conformance 未跑 |
| G10 | **DESK PASS** | Rust host/linker API 可包在 cellp-owned adapter 后，无需向产品接口暴露 engine type | adapter 黑盒与共同 profile replaceability 未证明 |

### 4.3 WASIp3 caveat

Wasmtime 的 `wasmtime_wasi_http::p3` 文档不是 production endorsement：它明确标记 experimental、unstable、incomplete、不受 semver，并提示 p3-only bug/security fixes 不保证 patch release。WASIp3 不应成为本 R1 的默认实验基线。

### 4.4 Recommendation

**ADVANCE CANDIDATE**：允许主管请求 R2 adapter-only fixture 授权。它不表示 Wasmtime 是首选/default，不表示 G4/G7 已在 cellp 上通过，也不允许先写产品 adapter。

---

## 5. Wasmer 7.4.0 dossier

### 5.1 已真实评估的证据链

- `v7.4.0` release 重点是 WASIX import preparation hook、N-API、Singlepass budget 与 MemoryStyle；未声明 Component Model/WASIp2/`wasi:http` runtime path。
- tag 内 `lib/api/Cargo.toml` 的运行时抽象是 core Wasm `Module`/compiler/VM；features 包括 `experimental-async` 与 `experimental-host-interrupt`，但不含 `component-model` 或 `wasi-http`。
- 官方 WASIX 文档明确：WASIX 是 **WASI preview1 + POSIX extensions**；plain WASI target 为 `wasm32-wasip1`，import namespace 为 `wasi_snapshot_preview1`。
- WASIX/Core module 路径不能替代本 track 的 Component + versioned WIT + `wasi:http` boundary。

### 5.2 G1–G10

| Gate | Desk status | 官方证据与边界 | 未决事项 |
|------|-------------|----------------|----------|
| G1 | **DESK PASS（有条件）** | 主 `wasmer` runtime 是 MIT；默认 `sys-default` 选择 Cranelift | `wasmer-compiler-singlepass` 与 `wasmer-compiler-llvm` 在 tag `deny.toml` 标为 BUSL-1.1；只认可审计后的 OSI feature set，需法律/依赖复核 |
| G2 | **DESK PASS** | CLI/library 可本地嵌入，核心 module 执行不要求 Edge/SaaS | air-gap supply chain 未测 |
| G3 | **DESK PASS** | 官方 Rust `wasmer` crate 与 embedding API | Component host embedding 不存在于本证据链 |
| G4 | **DESK FAIL** | exact release 官方产品/API/features 一致指向 core module + WASIp1/WASIX；没有本 track 必需的标准 Component runtime + WASIp2 `wasi:http` path | 不得用 WASIX 或自定义 core ABI 放宽 baseline |
| G5 | **DESK PASS（机制）** | 官方列 metering、caching；release 有 Singlepass output budget/MemoryStyle | HTTP/component 全组合 limits 未证 |
| G6 | **UNKNOWN** | “secure by default”不足以证明 trap/OOM/timeout 的 celld 进程级隔离与 lease discard | 需故障注入 |
| G7 | **UNKNOWN** | `experimental-async`/`experimental-host-interrupt` 不等于 Tokio HTTP hostcall cancellation contract | streaming、bounded cancel、hostcall deadline 均未证 |
| G8 | **UNKNOWN** | 有 security 邮箱，但 tag 内 support table 仅列 5.x supported、6.x limited，未列 7.x | 7.x 响应/backport policy 不清晰 |
| G9 | **DESK PASS** | 官方 backend matrix 列 Linux/macOS 与 x86_64/arm64 | cellp build+conformance 未跑 |
| G10 | **UNKNOWN** | core embedding 可包装，但标准 Component HTTP adapter 无官方路径 | 依赖未来 G4 重审 |

### 5.3 Recommendation 与重审条件

**HOLD，不是永久 REJECT。** G4 已阻塞当前性能决赛；G6/G7/G8/G10 保持独立 `UNKNOWN`，不得由 G4 推导失败。

后续新 exact release 同时提供下列官方证据时重做 R1：

1. Component **runtime embedding**，不是仅生成 component 的工具；
2. 可 pin 的 WASIp2/`wasi:http` host/world 支持；
3. 明确覆盖该 7.x/后续 release line 的 security support policy。

---

## 6. Third-candidate pre-screen

### 6.1 Gate matrix

| Gate | WAMR 2.4.5 | wasmi 2.0.0 | WasmEdge 0.17.1 |
|------|------------|-------------|-----------------|
| G1 | DESK PASS — Apache-2.0 WITH LLVM exception | DESK PASS — Apache-2.0 OR MIT | DESK PASS — Apache-2.0 |
| G2 | DESK PASS — standalone/embedded | DESK PASS — Rust interpreter library | DESK PASS — local runtime/SDK |
| G3 | DESK PASS — official Rust SDK wrapper | DESK PASS — native Rust crate | DESK PASS — official Rust SDK path |
| **G4** | **UNKNOWN** | **DESK FAIL（eligibility）** | **DESK FAIL（eligibility）** |
| G5 | UNKNOWN | DESK PASS（fuel mechanism）；完整 profile UNKNOWN | UNKNOWN |
| G6 | UNKNOWN | UNKNOWN | UNKNOWN |
| **G7** | **UNKNOWN** | **UNKNOWN** | **UNKNOWN** |
| G8 | DESK PASS（公开报告/披露渠道 + release CVE fixes） | DESK PASS（policy/audits） | UNKNOWN |
| G9 | DESK PASS（Linux/macOS、x86_64/AArch64） | UNKNOWN（跨平台/no_std 不等于目标矩阵 conformance） | DESK PASS（目标矩阵 desk evidence） |
| G10 | UNKNOWN | UNKNOWN | UNKNOWN |
| Result | **HOLD** | **HOLD** | **HOLD** |

### 6.2 G4 限定依据

`DESK FAIL` 只用于官方 evidence 能直接证明 exact release 与冻结 baseline 不相容的情形；单纯没有文档必须为 `UNKNOWN`：

- **WAMR 2.4.5 — UNKNOWN/HOLD：** release/README/Rust SDK 公开执行模型是 `.wasm`/`.aot` core `Module`，Rust SDK 示例明确 `wasm32-wasip1`；但这些资料只是未证明 Component HTTP，不能排除未文档化能力或可维护 adapter。WASI 官方对其 0.2 支持也只给出 varying-level 口径，不足以 PASS。按 R1 规则，G4 `UNKNOWN` 不得计分或进入性能决赛。
- **wasmi 2.0.0 — DESK FAIL（eligibility）/HOLD：** release 与 embedding matrix 描述 core interpreter；官方 open issue [#897](https://github.com/wasmi-labs/wasmi/issues/897) 的目标就是“添加 Component Model proposal 支持”。该动态 tracking evidence 在访问日直接表明 required Component runtime capability 尚待实现；不表示未来版本永远不支持。
- **WasmEdge 0.17.1 — DESK FAIL（eligibility）/HOLD：** 官方 open issue [#4236](https://github.com/WasmEdge/WasmEdge/issues/4236) 的当前矩阵显示多项 Component instantiate/execute 为 Partial/None，WASI Preview 2 表中 `wasi-http` 为 None；这直接排除访问日用该 release 满足冻结 baseline。issue 是动态 tracking evidence，后续状态变化必须触发新 exact-release 重审。

三者的 G7 均保持 `UNKNOWN`，不得从 G4 推导。

### 6.3 Spin 定位

Spin 是面向 component 应用的 serverless framework/runtime，可参考 WIT、DX、manifest 与 component lifecycle。其底层使用 Wasmtime，因此不能作为与 Wasmtime、Wasmer、WAMR 或 wasmi 同层的独立 engine 评分；也不得把 Spin Cloud、manifest 或 binding lifecycle 引入 cellp 控制面。

---

## 7. License、security、governance、platform 与 supply chain

| 候选 | License/distribution | Security/governance | Platform | Supply-chain R1 notes |
|------|----------------------|---------------------|----------|-----------------------|
| Wasmtime 48.0.1 | Apache-2.0 WITH LLVM-exception | Bytecode Alliance policy；月度 major；48 为 LTS；supported line 安全 backport | x86_64 Linux/macOS Tier 1；arm64 Linux/macOS Tier 2 | crate checksum + GitHub provenance SHA；仍需 SBOM、dependency/advisory audit |
| Wasmer 7.4.0 | runtime MIT；Cranelift-only 可作为候选 feature set；Singlepass/LLVM compiler crates 标 BUSL-1.1 | 有报告邮箱；tag policy 未列 7.x，治理/支持承诺 UNKNOWN | 官方 matrix 覆盖 Linux/macOS、x86_64/arm64 | 必须冻结 feature set，禁止无审计启用 BUSL backend；full-source tarball 是官方 packaging 建议 |
| WAMR 2.4.5 | Apache-2.0 WITH LLVM-exception | TSC；2.4.5 release 修复三项 CVE | Linux/macOS、x86_64/AArch64 | C/C++ core + Rust FFI 增加 bindgen/native build 审计面 |
| wasmi 2.0.0 | Apache-2.0 OR MIT | security policy；历史第三方审计；维护集中度需评估 | 主打 cross-platform/no_std；目标 Linux 双架构仍待实测 | pure Rust 较简单；v2 新解释器核心意味着升级/回归审计重要 |
| WasmEdge 0.17.1 | Apache-2.0 | CNCF project；本轮未取得足够 exact security policy 证据 | 官方项目支持多平台；目标组合仍待实测 | C++ runtime/Rust SDK/native packaging 增加构建与 ABI 面 |

共同要求：正式依赖前必须 pin exact crate/source digest，生成 SBOM，运行 license/advisory audit，记录 native toolchain，并验证离线可重建。R1 未测 binary size、build time、依赖树或签名验证，不能在 0–100 rubric 中计分。

---

## 8. R2/R3 unknown ledger

| ID | 必须通过实验回答 | Phase |
|----|------------------|-------|
| U01 | 冻结单一 WASIp2 `wasi:http/proxy` test baseline identifier 与 exact WIT dependency graph | R2 前 |
| U02 | 同一 component bytes 的 validate/compile/instantiate/HTTP dispatch | R2 |
| U03 | method/path/query、重复 headers、streaming body、backpressure 与 trailers policy | R2/R3 |
| U04 | client cancel during guest CPU、hostcall、partial ingress、partial egress 的 bounded stop | R3 |
| U05 | memory/table/component resources、host buffers、fuel/epoch/deadline 的联合限制 | R2/R3 |
| U06 | trap、OOM、timeout、runtime internal fault 后 celld 存活、instance discard/pool quarantine | R3 |
| U07 | Tokio async hostcall 不长期占 worker；guest deadline 与 hostcall deadline 独立 | R3 |
| U08 | stream 完成前 lease 不归还；sequential state/concurrency/fault 后新请求 | R3 |
| U09 | cache key/provenance、corrupt/version mismatch discard 与从 source bytes 重编译 | R2/R3 |
| U10 | x86_64 Linux 与 arm64 Linux build/conformance；macOS developer loop | R2/R3 |
| U11 | engine-neutral error/telemetry 映射不泄漏 engine type、secret 或内部连接信息 | R3 |
| U12 | embedded celld adapter 与 generation/capability snapshot/drain seam | R3 |
| U13 | Wasmtime 48.0.1 dependency tree、binary delta、license/SBOM/advisory baseline | R2 |
| U14 | 第二个 exact release 何时满足同一 baseline；在此之前禁止 replaceability claim | 后续 R1/R3 |

---

## 9. R1 Gate checklist

| 检查 | Final status |
|------|--------------|
| exact version/tag/date/commit 已记录 | PASS（WasmEdge notes date 与 GitHub publication date 已分列，完整 commit 已记录） |
| Wasmtime 与 Wasmer 均按 G1–G10 评估 | PASS |
| Wasmer 被真实评估且不是 omission | PASS |
| WASIp3 spec 与 engine implementation maturity 分层 | PASS |
| alternatives 的 G4 按证据区分 FAIL/UNKNOWN；G7 未伪造 FAIL | PASS |
| 动态文档与 pinned release evidence 区分 | PASS |
| desk fact 未冒充 conformance/performance | PASS |
| 未选择 default，未宣称 replaceability | PASS |
| 下一阶段仍需用户独立授权 | PASS |
| 独立证据验证 | **R1 deliverable PASS — NW-R1-V01–V04 CLOSED** |

**NW-R1-V01–V04 provenance 与仓内锚点：** 原始逐项 verifier findings 不在 git，因此本文件不复述、推测或声称重建其审查内容。可审查的仓内 provenance 仅为：首轮/修订后 verifier run ID 的 closure 摘要，以及下表对当前 ledger 段落的映射；映射表示修订后结论落点，**不是**原始 findings。

| Closure ID | 当前 ledger 锚点（修订后结论/限制） |
|------------|--------------------------------------|
| NW-R1-V01 | §3 exact-version inventory；§9 exact version/tag/date/commit checklist；§10 pinned/dynamic source log |
| NW-R1-V02 | §1.2 `DESK PASS` / `DESK FAIL` / `UNKNOWN` 定义；§6.2 alternatives G4 限定依据 |
| NW-R1-V03 | §2.1 WASIp2/WASIp3 分层；§4.2 Wasmtime G7 `UNKNOWN`；§8 R2/R3 unknown ledger |
| NW-R1-V04 | §0 candidate verdict；§4.4 / §5.3 recommendation 边界；§9 no-default/no-replaceability/no-R2-R3 checklist |

**Final Gate verdict：R1 deliverable PASS — DESK RESEARCH VERIFIED。** 这里的 PASS 仅裁决 R1 desk 交付物；不是任何 gate 的实验 PASS，也不是产品 M2。

**独立验证记录：** 首轮 verifier `01a0724c-94aa-7e00-9bb8-8aaecfc47edd` 返回 `FAIL` 并提出 NW-R1-V01–V04；修订后 verifier `01a07257-dcac-7c62-8545-bc8b1bba3d4e` 与最终 Sonnet verification `01a0725e-fece-73d0-9175-49decc81718f` 均返回 `PASS`。

验证通过后的允许动作只有：向用户提交 R1 结论并请求是否授权 R2。未经明确授权，不得创建 harness、添加 runtime dependency、冻结产品 ABI、修改 Registry/OpenAPI/deploy manifest 或变更 JS/V8 执行路径。

---

## 10. Source log

所有来源访问于 **2026-09-05**。`pinned` 指 URL 或内容绑定 exact tag/release；`dynamic` 指会随上游更新，不能单独证明 exact release。

### 10.1 Standards

| Title | Official URL | Evidence |
|-------|--------------|----------|
| WASI Releases | https://wasi.dev/releases | dynamic；0.2/0.3 status、toolchain caveat |
| WASI 0.2 | https://wasi.dev/releases/wasi-p2 | dynamic；WASIp2 worlds |
| WASI 0.3.0 release | https://github.com/WebAssembly/WASI/releases/tag/v0.3.0 | pinned；0.3 specification release |

### 10.2 Wasmtime

| Title | Official URL | Evidence |
|-------|--------------|----------|
| Release Wasmtime 48.0.1 | https://github.com/bytecodealliance/wasmtime/releases/tag/v48.0.1 | pinned；date/commit/patch notes |
| crates.io API: wasmtime 48.0.1 | https://crates.io/api/v1/crates/wasmtime/48.0.1 | pinned；license/features/checksum/provenance |
| Wasmtime Tiers of Support | https://docs.wasmtime.dev/stability-tiers.html | dynamic；embedding/component/wasi-http/targets |
| Wasmtime Release Process | https://docs.wasmtime.dev/stability-release.html | dynamic；LTS/cadence/security backports |
| Wasmtime Security | https://docs.wasmtime.dev/security.html | dynamic；sandbox/embedder obligations |
| `wasmtime::Store` resource/interruption API | https://docs.wasmtime.dev/api/wasmtime/struct.Store.html | dynamic；limiter/fuel/epoch/async yield/hostcall fuel |
| `wasmtime::component::Func` | https://docs.wasmtime.dev/api/wasmtime/component/struct.Func.html | dynamic；async call 与 cancellation 限制（drop future 不取消；hard cancel 需 drop Store） |
| `wasmtime_wasi_http::p2` | https://docs.wasmtime.dev/api/wasmtime_wasi_http/p2/index.html | dynamic；WASIp2 proxy/Tokio/linker |
| `wasmtime_wasi_http::p3` | https://docs.wasmtime.dev/api/wasmtime_wasi_http/p3/index.html | dynamic；experimental warning |
| LICENSE at v48.0.1 | https://github.com/bytecodealliance/wasmtime/blob/v48.0.1/LICENSE | pinned；license text |
| SECURITY at v48.0.1 | https://github.com/bytecodealliance/wasmtime/blob/v48.0.1/SECURITY.md | pinned；reporting policy pointer |

### 10.3 Wasmer

| Title | Official URL | Evidence |
|-------|--------------|----------|
| Release v7.4.0 | https://github.com/wasmerio/wasmer/releases/tag/v7.4.0 | pinned；date/commit/release scope |
| `wasmer` Cargo.toml at v7.4.0 | https://raw.githubusercontent.com/wasmerio/wasmer/v7.4.0/lib/api/Cargo.toml | pinned；core/async/backend features |
| Wasmer LICENSE at v7.4.0 | https://raw.githubusercontent.com/wasmerio/wasmer/v7.4.0/LICENSE | pinned；MIT runtime license |
| Wasmer deny.toml at v7.4.0 | https://raw.githubusercontent.com/wasmerio/wasmer/v7.4.0/deny.toml | pinned；BUSL compiler exceptions |
| Wasmer SECURITY at v7.4.0 | https://raw.githubusercontent.com/wasmerio/wasmer/v7.4.0/docs/SECURITY.md | pinned；support table/report email |
| Runtime Introduction | https://docs.wasmer.io/runtime | dynamic；module runtime/backends |
| WASIX | https://docs.wasmer.io/runtime/wasix | dynamic；WASIp1/WASIX relationship |
| Runtime Features | https://docs.wasmer.io/runtime/features | dynamic；metering/platform matrix |

### 10.4 Other candidates and reference

| Title | Official URL | Evidence |
|-------|--------------|----------|
| WAMR 2.4.5 release | https://github.com/wasm-micro-runtime/wasm-micro-runtime/releases/tag/WAMR-2.4.5 | pinned；date/commit/CVE fixes |
| WAMR Security | https://github.com/wasm-micro-runtime/wasm-micro-runtime/security | dynamic；private reporting、披露流程、security update 渠道 |
| WAMR README | https://github.com/wasm-micro-runtime/wasm-micro-runtime/blob/main/README.md | dynamic；module architecture/platform/TSC/license |
| WAMR Rust SDK | https://github.com/wasm-micro-runtime/wamr-rust-sdk | dynamic；Rust wrapper/module/WASIp1 |
| wasmi v2.0.0 release | https://github.com/wasmi-labs/wasmi/releases/tag/v2.0.0 | pinned；date/commit/fuel/core rewrite |
| wasmi README | https://github.com/wasmi-labs/wasmi | dynamic；license/core/WASIp1/security audits |
| wasmi Component Model tracking #897 | https://github.com/wasmi-labs/wasmi/issues/897 | dynamic open issue；目标是添加 Component Model 支持 |
| WasmEdge 0.17.1 release | https://github.com/WasmEdge/WasmEdge/releases/tag/0.17.1 | pinned；notes date、publication date、full commit |
| WasmEdge Component/WASIp2 tracking #4236 | https://github.com/WasmEdge/WasmEdge/issues/4236 | dynamic open issue；Component execution incomplete、`wasi-http` None |
| WasmEdge Component Model | https://wasmedge.org/docs/start/wasmedge/component_model | dynamic；implementation maturity |
| Spin v2 Introduction | https://developer.fermyon.com/spin/v2/index | dynamic；component application framework |
| Why We Chose Rust for Spin | https://www.akamai.com/blog/developers/why-we-chose-rust-for-spin | dynamic；官方说明 Spin 使用 Wasmtime |

---

## 11. Evidence uncertainties and recheck triggers

1. Wasmtime API/tier pages是 dynamic；R2 必须 pin `wasmtime`, `wasmtime-wasi`, `wasmtime-wasi-http` exact crate versions 与 WIT bytes。G7 仍是 `UNKNOWN`：async mechanism 已有证据，但官方 component API 当前只能通过 drop Store hard-cancel，drop future 不取消任务。
2. Wasmer G4 FAIL 依赖 release/API/product scope 的一致证据，而非单纯关键词缺失；若上游发布 Component host path，立即以新版本重审。
3. WAMR G4 是 `UNKNOWN`：公开 module/WASIp1 路径与 WASI 官方 varying-level 口径既不能证明 baseline，也不能证明不存在可维护 adapter；因此保持 HOLD。
4. wasmi #897 与 WasmEdge #4236 是访问日的动态 open tracking evidence；它们支持当前 eligibility FAIL，但不绑定未来版本，状态变化须以新 exact release 重审。
5. WasmEdge `0.17.1` notes/version date 是 2026-07-03，GitHub publication 是 2026-07-06；完整 commit 为 `0502c560d787d969c66f5676b9e11ac64bba3656`。
6. 未对 release assets、签名、dependency lock、SBOM、CVE applicability 或 transitive licenses 做本地验证。
7. 未运行任何代码、dev stack 或 benchmark；本文件不能作为性能、稳定性或安全隔离证明。
8. NW-R1-V01–V04 的原始逐项 findings 不在 git；离线只能核对 §9 closure、run ID 与上表锚点映射，不能重放独立审查全文。
