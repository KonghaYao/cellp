# cellp 原生 Wasm Runtime + Bindings（权威研究纲领）

> **状态：** Proposed · Research Track · **未成为 AD，不授权产品实现**
> **权威范围：** 本文是原生 Wasm track 的唯一研究范围、术语、比较方法与决策门禁；它**不是**当前产品行为或冻结 RPC
> **当前产品：** `JS / workers-rs shim + V8 + celld bindings` 继续按 [decisions.md](../decisions.md) AD-1 / AD-6 / AD-8 / **AD-12** / **AD-15** 运行（AD-15 已正式批准，见 decisions §20）
> **R0 / R1 状态：** R0 对抗审查与独立验证 **PASS**（2026-09-05）— 见 [NATIVE-WASM-RUNTIME-REVIEW.md](./NATIVE-WASM-RUNTIME-REVIEW.md)；R1 已获用户批准，**R1 desk research deliverable** 与仓内独立文档验证 **PASS** — 见 [NATIVE-WASM-RUNTIME-R1.md](./NATIVE-WASM-RUNTIME-R1.md)；**不授权 R2/R3 或产品实现**
> **相关现状：** [celld Wasm](../../celld/docs/wasm.md) · [Cloudflare compatibility](../../celld/docs/cloudflare-compat.md)
> **最后更新：** 2026-09-05

---

## 0. 一句话

**新增一条 stateless HTTP 的 `Native Wasm Component + Binding Host` 执行路线；先定义并审查 runtime-neutral 契约方向，再用同一 conformance suite 在 R1 同等对待 Wasmtime、Wasmer 等候选；不改、不替换、不侵入现有 `JS + Wasm + bindings` 生态。**

```text
                         cellp Version / Gateway / Promote
                                      │
                              celld lifecycle shell
                                      │
                  ┌───────────────────┴───────────────────┐
                  │                                       │
       Existing JS execution                    Native Wasm execution
       V8 + JS Worker APIs                      Runtime Adapter contract
       workers-rs shim + .wasm                  Wasmtime / Wasmer / ...
                  │                                       │
                  └───────────────────┬───────────────────┘
                                      │
                           Engine-neutral Binding Host
                           KV / D1 / R2 / Queue / config
```

### 0.1 术语表

| 术语 | 含义 |
|------|------|
| **WASI 0.2** | 规范与社区文档中的 **WASIp2** 线；HTTP 等 world 以实际采用的 **WIT package / world 版本** 为准，R2 前写入 **test baseline identifier** |
| **WASI 0.3** | **WASIp3** 线；成熟度与 API 稳定性须逐版本核验，不得与 0.2 混称 |
| **test baseline identifier** | R2 前必须冻结的**单一**研究基线标识（至少含 Component Model / WASI 线、`wasi:http` world、相关 WIT package versions）；**不是**公开 ABI freeze |
| **Native support matrix** | Native guest 能力矩阵；与 JS [cloudflare-compat](../../celld/docs/cloudflare-compat.md) **独立**，每项 capability 单独 contract test 证明 |
| **embedded celld adapter** | R2/R3 唯一默认集成形态：Native runtime 作为 celld 内 adapter；独立 daemon / 第二 upstream / 独立 lifecycle **非**本 track 默认，须另行 Proposed AD |

---

## 1. 已确认事实与尚未决策

### 1.1 已确认事实

1. Cloudflare Workers 没有为现有 `workers-rs` 产物提供独立于 JavaScript 的公开稳定 host ABI。Cloudflare 官方说明 Wasm Worker 由自动生成的 JavaScript entrypoint 调用，平台 API 通过 `wasm-bindgen` 与 JavaScript objects 互操作。
2. celld 当前 `wasm-v1` 是 Wrangler `CompiledWasm` 语义：`.wasm` 作为 `WebAssembly.Module` 被 JS 入口导入；它不是 native Wasm HTTP entrypoint。
3. 因而“请求不经过 JS/V8”与“原样执行现有 workers-rs 二进制”不能同时承诺。
4. Component Model + WIT 适合定义与引擎无关的 guest/host 边界；`wasi:http` 可承载标准 HTTP 部分，D1/R2/Queue/DO 等仍需平台扩展接口。

### 1.2 尚未决策

| 项 | 当前状态 |
|----|----------|
| 首选执行引擎 | **未选**；Wasmtime、Wasmer 为 R1 **同等**首批候选；eligibility 仅由 R1 硬门槛与证据决定 |
| WASI / test baseline | R1 调查 WASIp2 vs WASIp3 成本；**R2 前**冻结单一 test baseline identifier（见 §0.1）；非公开 ABI |
| celld 集成形态 | **R2/R3 基线：** embedded celld adapter；独立 daemon / 第二 Gateway upstream **禁止**作为本 track 默认选项 |
| 对外 artifact/config 字段 | 未设计；不改 OpenAPI / Registry / wrangler 契约 |
| Binding WIT | 未冻结；先写语义测试，再冻结 package/version |
| Durable Objects native ABI | 未设计；独立后续 track |

**纪律：** 本文中的倾向、候选和草图均不能作为实现授权。进入实现前须产出对抗审查后的 Proposed AD。

---

## 2. 研究范围与硬边界

### 2.1 本 track 做什么

- 原生 Wasm HTTP component 的加载、实例化、调用、取消和资源限制。
- 可替换 runtime adapter 契约与 conformance suite。
- engine-neutral Binding Host；native WIT adapter 对齐现有 V8 adapter 的可观察 binding 语义和同一数据实现；本研究不要求重构现有 JS 路径。
- 对齐 Cloudflare 的**产品概念、配置名称和可验证行为语义**。
- 评估 Wasmtime、Wasmer 及后续候选的技术、生态、许可、治理和总体拥有成本。
- 保留 cellp 既有 Version / branch / preview / promote / archive / wake / RustFS 模型。

### 2.2 本 track 明确不做

- 不修改或下线当前 V8 / JavaScript Worker backend。
- 不改变当前 `wasm-v1` / `ModuleKind::Wasm` 的 CompiledWasm 含义。
- 不把 workers-rs 的 `shim.mjs + wasm-bindgen` 产物解释成 native component。
- 不在 Wasm runtime 中模拟完整 JavaScript object model、Promise 或任意 wasm-bindgen imports。
- 不承诺任意 Cloudflare Worker bundle 可在 native backend 执行。
- 不引入 Wasmer Edge、Fermyon Cloud 或其他外部托管服务；候选必须可 100% 私有化。
- 不让 runtime 自己拥有第二套 Version、Gateway、Registry、RustFS bucket 或 promote 生命周期。
- 不先做 Durable Objects、Workflow、WebSocket hibernation 或 Queue consumer。
- **首期 scope：** 仅 **stateless HTTP dispatch**；celld `CellHost` / DO actor / alarms / workflow / queue consumer / **现有 JS WebSocket** 路径保持 V8，**不要求** `CellHost::Native`。Native HTTP v0 对 upgrade/WebSocket 的拒绝**不影响**现有 JS WebSocket 行为。

### 2.3 与现行 AD 的关系

Native 请求仍经 **同一 Gateway Host**（[AD-12](../decisions.md#17-ad-12--hostname--port-ingress废弃-path-选-version)）：`ingress_bindings` → celld upstream endpoint；**不得**恢复 path selector 或按 **执行引擎** 分流 Gateway。

| 现行决策 | 本 track 必须保持 |
|----------|------------------|
| AD-1 | 保持 serving 进程隔离与每 version 单一权威 bucket；AD-15 可 additive 扩展为 `0..N` 独立 replica；研究不得静默增加第二套持久层 |
| AD-6 | 当前 JS bindings 仍由 celld 提供；**operator CLI** 与 **新 guest WIT** 分层；native adapter 不能破坏 CLI/operator 行为；Binding WIT 需 **R5 Proposed AD**，R0–R4 **不改** operator API |
| AD-8 | D1/KV/R2/Queue branch 的数据语义保持；guest ABI 不决定 branch 实现 |
| AD-9 | archive/wake 语义对执行引擎一致 |
| AD-10 | 100% 私有化；不扩成通用 PaaS、账号或全球边缘产品 |
| AD-12 | Host / port ingress → celld；无引擎专属路由面 |
| AD-14 | runtime 发射 OTLP；查询仍走 cellp 门面，不出现引擎专属产品 API |
| **AD-15** | **已正式批准**（decisions §20）：additive `deploy_ready`、`0..N` replica、`RouteSnapshot`、archive 与 cold **互斥**；`CELLP_ELASTIC_RUNTIME` 默认关闭；**不得**假设 ready 永远等于单进程常驻 |

**Lifecycle：** Native 与 JS 共用 Version / promote / archive / wake / elastic 视图；draining generation 不收新请求，在途请求在旧 generation 完成（见 §2.4）。

### 2.4 Stateless 研究 seam 与 generation

首期 Native 只接 **stateless HTTP**；下列 seam 用于 R2/R3 fixture 与门禁，**不**冻结正式 Rust/WIT API：

```text
Gateway ingress          Host/port → upstream（AD-12）
        │
celld shell / admission  Version 资格、generation、limits、fail-closed deploy
        │
generation snapshot      deployment generation + capability snapshot；drain 边界
        │
native runtime adapter   embedded adapter-only（R2）或 shell+adapter fixture（R3）
        │
binding semantic backend 共享 KV/D1/R2/Queue 数据语义（AD-6/8）；非第二持久层
```

| Seam | R2 | R3 |
|------|----|----|
| Adapter 黑盒 + component corpus | **必须**（adapter-only harness） | 复用并扩展 |
| celld shell admission / generation / drain | 规格与测试向量 | **必须**（shell+adapter 集成 fixture） |

实例化必须绑定 **deployment generation/version** 与 **capability snapshot**；不得在无 generation 语义下混跑不同 binding 授权集。

---

## 3. 兼容性声明

“Cloudflare 生态兼容”必须按层声明，禁止只写“兼容 Cloudflare Workers”。**Native 不自动补齐 Cloudflare 全语义**；未证明 capability 一律 fail-closed 或 matrix **No/Partial**，不得从 JS 矩阵推断。

### 3.1 Native 与 JS support matrix 关系

| 原则 | 说明 |
|------|------|
| **独立矩阵** | Native support matrix 与 JS compat matrix **独立**；JS 的 Yes/Partial/No **不**自动成为 Native 的任何状态 |
| **证明方式** | Native 每个 capability 须 **contract tests**（及必要时 V8/native 对照 binding **数据语义**）；共享 binding 实现时 **不得**回退 AD-6/8 |
| **公开差异** | 引擎或 ABI 差异进入 **Native** support matrix；不要求与 JS 表面逐字相等 |

| 层 | Native Wasm 目标 | 声明方式 |
|----|------------------|----------|
| 产品概念 | Worker、fetch、binding、version、preview | 可对齐 |
| 配置输入 | 复用或转换 `wrangler.jsonc` binding declarations | 逐字段列支持矩阵 |
| 行为语义 | HTTP、KV、D1、R2、Queue 的可观察结果 | differential / contract tests |
| Rust 源码体验 | 提供 WIT bindings 和 SDK；API 可借鉴 workers-rs | 迁移指南，不声称源码零改动 |
| Wasm ABI | Component Model + versioned WIT | cellp 原生 ABI |
| workers-rs binary ABI | `wasm-bindgen + JS entrypoint` | **不兼容；继续走 V8 backend** |

native artifact 必须显式声明 ABI；禁止按文件扩展名猜测：

```text
*.wasm  ≠ 自动代表 native HTTP component
```

部署遇到未知 world、未知 WIT version 或 backend 不支持的 capability 时必须 fail-closed。

---

## 4. 稳定边界：Runtime Adapter Contract

### 4.1 设计原则

1. **契约拥有者是 cellp/celld，不是某个 runtime crate。**
2. 公共类型不得暴露 `wasmtime::*`、`wasmer::*`、V8 handles 或引擎 trap 类型。
3. guest ABI（WIT）与 host adapter API 分层；更换引擎不改变应用 artifact 语义。
4. capability 在实例化时显式授予；guest 不得按任意字符串打开未声明资源。
5. 编译缓存是可丢弃加速层，不是持久事实；RustFS 中原始 artifact + digest 才是事实。
6. runtime 错误必须归一化；日志可附引擎诊断，但 API/metrics 不绑定引擎方言。

### 4.2 概念接口（非冻结 Rust API）

```rust
trait NativeRuntime: Send + Sync {
    fn engine_id(&self) -> EngineId;
    fn capabilities(&self) -> RuntimeCapabilities;

    async fn compile(
        &self,
        artifact: ComponentArtifact,
        policy: CompilePolicy,
    ) -> Result<CompiledComponent, RuntimeError>;

    async fn instantiate(
        &self,
        compiled: &CompiledComponent,
        host: HostCapabilities,
        limits: InstanceLimits,
        ctx: RequestContext, // 概念类型；见 §4.6
    ) -> Result<Box<dyn NativeInstance>, RuntimeError>;
}

trait NativeInstance: Send {
    async fn handle_http(
        &mut self,
        request: RuntimeRequest,
        ctx: RequestContext,
    ) -> Result<RuntimeResponse, RuntimeError>;

    async fn shutdown(&mut self, reason: ShutdownReason) -> Result<(), RuntimeError>;
}
```

这是职责草图，不冻结 trait 名称、对象模型或实例池策略。所有候选 adapter 必须满足相同的黑盒测试，而不是让公共契约追随某个 SDK。

### 4.3 输入 artifact 最低事实

| 字段 | 含义 |
|------|------|
| exact component bytes | 外部 CI 产生的不可变 bytes |
| SHA-256 digest | 编译缓存与部署身份校验 |
| ABI world + version | 例如经调研后选定的 `wasi:http/proxy` 版本 |
| required capabilities | HTTP outbound、config、KV 等显式声明 |
| limits profile | memory、CPU/deadline、并发、body 等 |

**HostCapabilities（最低事实，非完整 WIT）：** 须含 **generation/version identity**、**binding 授权 handles**（非 guest 任意字符串寻址）、**branch/scope identity**、**trace context**。可替换 runtime 当且仅当 **同一 digest + 同一 declared required capability set**；候选 `RuntimeCapabilities` 必须是该集合的**超集**，否则 deploy/qualification **fail-closed**。

禁止把 runtime 私有的 serialized/AOT executable 当作可跨版本、跨节点的权威 artifact。若使用预编译缓存，其 cache key 至少包含：原始 digest、runtime id/version、target、compiler/backend、feature set 和 host ABI version；**cache miss / corrupt / version mismatch 必须丢弃并从 exact source bytes 重编译**；失败则 deploy fail-closed；**不得**加载错引擎 blob 或静默切换语义。

### 4.4 HTTP 中立类型

celld 当前请求路径使用 `crate::js::HttpResponse`、`RequestBody` 等类型。native track 必须先定义 engine-neutral：

```text
RuntimeRequest
RuntimeRequestBody
RuntimeResponse
RuntimeResponseBody
RuntimeRequestId
RuntimeError
```

至少保持：method、scheme/authority/path/query、重复 headers、流式 body/backpressure、取消、deadline、trailers（是否支持待调研）和 upgrade 明确拒绝。V8 与 Native adapter 分别转换，不让 `js` 类型成为公共内核。

### 4.5 错误分类

| 类别 | 例子 | 必须行为 |
|------|------|----------|
| `artifact_invalid` | 非 component、digest 错、未知 world | deploy fail-closed |
| `abi_unsupported` | WIT/WASI version 不支持 | deploy fail-closed，列缺失 capability |
| `compile_failed` | validation/compiler error | deploy fail-closed |
| `instantiate_failed` | imports/link/resource initialization | ready 前失败 |
| `guest_trap` | unreachable、guest panic | 单请求失败，不退出 celld |
| `deadline_exceeded` | CPU/epoch/fuel deadline | 可确定地中断并回收实例 |
| `resource_exhausted` | memory/table/body/concurrency | 明确限流或请求失败 |
| `host_denied` | 未授予 filesystem/network/binding | 不泄露资源存在性或 secret |
| `host_failed` | KV/D1/R2 后端失败 | 统一可重试分类；不得透出凭据 |
| `runtime_internal` | engine invariant / adapter bug | 标记引擎版本，实例 discard + pool quarantine/健康 escalation（R3 证明）；稳定错误码仍引擎中立 |

### 4.6 请求 / Hostcall 上下文与流所有权（概念 contract）

R0 **不**冻结正式 Rust/WIT API；R2/R3 profile **必须**实现等价语义：

**RequestContext / HostCallContext（或等价非冻结类型）** 至少包含：request id、deployment generation、trace、**guest deadline**、**独立 hostcall deadline**、budget/quota、**cancel signal**。取消须 **有界**；drop future **不是**证明。

**Ingress/egress stream ownership：**

| 阶段 | Owner |
|------|--------|
| dispatch 前 | celld shell |
| 成功安装 guest request context 后 | guest 执行上下文（adapter 托管） |
| 失败 / cancel / mid-stream 异常 | **唯一** owner 回收；backpressure **有界** |

Fixture 须覆盖：client cancel while guest CPU、while hostcall、partial ingress、partial egress；断言 **bounded stop** 且资源释放。

### 4.7 R2/R3 实例 lease 研究 profile（不冻结 pool 策略）

R2/R3 每个 adapter **必须声明并统一**下列 profile（除非未来共同修订并证明）：

| 项 | 默认 research profile |
|----|------------------------|
| 并发 | 每 `NativeInstance` 同时最多 **一个** active request |
| Lease | response stream **完成前** lease 不归还 |
| Guest 可变状态 | 须声明跨请求可见性；正常完成是否复用同一 instance |
| trap / deadline / OOM / `runtime_internal` | 实例 **discard**，**不得**回池 |
| `runtime_internal` | 额外要求 pool **quarantine** 与健康 escalation 策略（**R3** 证明） |

测试须含：sequential state、concurrency、fault 后新请求。

---

## 5. Binding Host：第二套 guest 接口，同一套数据语义

### 5.1 分层目标

```text
                     Binding semantic service
                  capability + identity + errors
                 /                              \
       JS/V8 binding adapter              WIT binding adapter
       env.KV / env.DB / ...              resource handles
```

“第二套替换方案”指第二套 **guest adapter/ABI**，不是第二套 KV/D1/R2/Queue 数据实现。Branch、bucket layout、operator API 仍是共享平台事实。

### 5.2 Binding API 设计纪律

- 优先使用 Component Model `resource` handle 表达已授权 binding。
- WIT package 必须 semver；不复用 Cloudflare 名称冒充其标准 ABI。
- 先写行为矩阵和 golden tests，再写 WIT。
- CF JavaScript 中依赖动态 object/prototype/exception 的表面，不逐字翻译；转换成类型化、可移植语义。
- 未成熟 WASI proposals 可作为研究输入，不能因其存在就自动成为产品依赖。
- Hostcall 必须支持异步、取消、deadline、配额和 trace context。

### 5.3 建议研究顺序

| 顺序 | Capability | 原因 |
|------|------------|------|
| B0 | config / vars | 最小只读 capability；验证授权和 secret redaction |
| B1 | KV | 小接口，适合验证 resource handle、bytes、TTL 和 branch |
| B2 | D1 | 需要冻结 SQL value、integer、BLOB、NULL、batch/error 语义 |
| B3 | R2 | streaming、metadata、range、conditional operation 更复杂 |
| B4 | Queue producer | 消息编码、delay、retry 错误分类 |
| B5 | Queue consumer / Cron | 新 event world 与 lifecycle，不只是 binding call |
| 独立 track | Durable Objects | ownership、residency、alarm、migration、WebSocket hibernation |

WASI 当前提案成熟度必须逐次核验。例如截至本文核验时，Key-Value 和 Messaging 位于 Phase 2，Blob Store 位于 Phase 1，SQL 位于 Phase 1；这些状态可能变化，正式选用前重新核验 [WASI releases](https://wasi.dev/releases)。

---

## 6. Runtime 市场调研框架

### 6.1 首批候选

| 候选 | 纳入理由 | 当前证据状态 |
|------|----------|--------------|
| **Wasmtime** | Rust 嵌入；官方 Component Model / `wasi:http` 文档与 support tier 可 desk 引用 | R1 填硬门槛；**R1 结束前**不得假定可进 R2/R3 |
| **Wasmer** | Rust SDK；MIT；私有化；caching/metering 等官方说明 | R1 填硬门槛；Component/`wasi:http` 须 **PASS/FAIL/UNKNOWN** 证据，**与 Wasmtime 同等** eligibility |
| **后续候选** | WAMR、wasmi、Spin runtime libraries 等 | 只有满足 §6.2 准入条件才加入 |

Spin 可作为 DX、WIT 和 component lifecycle 的参考实现，但不得直接引入其 manifest、cloud、KV/SQLite 生命周期替代 cellp。

### 6.2 候选准入硬门槛

任一项不满足即不进入性能决赛：

1. OSI-compatible license，可在 cellp 的分发方式下合规使用。
2. 可完全私有化、离线运行；核心执行不依赖 vendor control plane / registry / SaaS。
3. 有受支持或可维护的 Rust embedding 路径。
4. 能执行选定的 Component Model/WASI HTTP baseline，或有边界清晰、维护成本可接受的 adapter。
5. 可限制 guest memory、CPU/执行时间、并发和 host capabilities。
6. guest trap/OOM/timeout 不得结束 celld 进程。
7. 支持 async hostcalls 与请求取消；不能长期阻塞 Tokio worker。
8. 上游有安全政策、版本与漏洞响应渠道。
9. x86_64 Linux 与 arm64 Linux 至少能构建并通过 conformance；macOS 用于开发循环。
10. 能在不暴露引擎类型的情况下实现 §4 adapter。

### 6.3 评分维度（通过硬门槛后）

**门禁：** 硬门槛项在 R1 可为 **PASS / FAIL / UNKNOWN**；**UNKNOWN 不得进入 §6.3 计分**。每项硬门槛须经 **R2/R3 变为 PASS** 才可参与性能决赛。**完整 0–100 rubric 与 tie-break 须在任意测量前冻结**；未冻结不得评分或选 default。

总分 100；每项必须附命令、版本、硬件和**原始 evidence 路径**，不接受厂商 benchmark 直接计分。**官方网页**仅支持 desk fact，**不能**使硬门槛 PASS。

| 维度 | 权重 | 测量内容 |
|------|-----:|----------|
| ABI / standards fit | 20 | Component Model、目标 WASI version、async/stream、WIT tooling |
| Isolation / limits | 20 | memory、deadline、fuel/metering、trap containment、capability deny |
| Embedding quality | 15 | Rust API、Tokio async、cancellation、Send/Sync、升级破坏度 |
| Performance | 15 | compile、cold instantiate、warm RPS/p50/p99、streaming、memory/density |
| Operations | 10 | cache、AOT、diagnostics、profiling、OTEL、cross-node reproducibility |
| Portability | 8 | x86_64/arm64 Linux、macOS dev、compiler backend 差异 |
| Security / governance | 7 | fuzzing、安全公告、维护者响应、release cadence |
| License / supply chain | 5 | license、依赖树、binary size、audit/签名/SBOM 可行性 |

### 6.4 每个候选必须产出的档案

```text
runtime / exact version / commit
license + distribution notes
supported targets
component + WASI matrix（PASS / FAIL / UNKNOWN）
embedding prototype location
resource-control matrix
security model and advisories
build time + binary size delta
benchmark methodology and raw evidence
API churn / upgrade notes
known blockers
recommendation: advance / hold / reject
```

`UNKNOWN` 是 R1 合法结果；禁止用“项目很流行”“官网说支持 WASI”替代目标 world 的可复现实验。**若仅一候选通过硬门槛**，仍可继续单引擎 feasibility / 产品决策，但**不得**宣称 runtime replaceability（§9）。

---

## 7. Conformance 优先于引擎

### 7.1 Engine-neutral fixture corpus

同一 component bytes 必须用于所有候选：

- HTTP echo：method/path/query/重复 headers/body。
- streaming + backpressure + client cancel。
- outbound HTTP allow/deny。
- clocks/random/config capability。
- infinite loop deadline。
- memory growth limit。
- trap/panic 后下一请求可继续。
- undeclared import/binding fail-closed。
- compile cache hit/miss/corruption/**version mismatch**（须 discard + 从 source bytes 重编译或 deploy fail-closed）。
- deploy cutover 时旧请求完成、新请求进入新 generation。
- client cancel（guest CPU / hostcall / partial ingress / partial egress）— §4.6。

Bindings 每增加一个 capability，就在两个 runtime 共同声明支持的 capability profile 内做三方行为对照：

```text
expected semantic contract
        ├── JS/V8 adapter
        ├── Wasm runtime adapter A
        └── Wasm runtime adapter B
```

不是所有 JS 细节都要相等；差异必须进入 **Native** support matrix（§3.1），不得假设 JS Yes 上限 Native。

### 7.2 性能测试纪律

至少报告：

- 硬件、OS/kernel、CPU governor、runtime exact version；
- component digest、语言/toolchain、优化参数；
- compile/JIT/AOT/cache 状态；
- cold compile、cold instantiate、warm request 分开；
- p50/p95/p99、吞吐、RSS、每实例增量、错误率；
- 单实例和目标密度；
- HTTP body 大小和并发；
- 至少一次故障注入，而非只测 hello-world。

不得拿 Wasmer Edge 或其他托管平台数字与本地 embedded runtime 数字直接比较。

---

## 8. 分阶段研究与决策门禁

| Phase | 目标 | 产物 | Exit gate | 授权依赖 |
|-------|------|------|-----------|----------|
| **R0 — Contract brief** | 审查本文边界 | 对抗审查 + 合并修订 | 用户批准 → **仅准入 R1** | 本文 + [REVIEW](./NATIVE-WASM-RUNTIME-REVIEW.md) |
| **R1 — Market scan** | Wasmtime/Wasmer/其他 **同等** desk research | 候选档案 + **exact version/date/source log** | 硬门槛均为 PASS/FAIL/UNKNOWN | **R0 用户批准后** |
| **R2 — ABI fixture** | 冻结 test baseline identifier；**adapter-only** corpus | guest fixtures + runner spec | ≥1 候选运行 HTTP fixture，或记录 blocker | **独立授权**（非 R0 隐含） |
| **R3 — Dual spike** | **shell+adapter** 集成 + 第二 adapter | 可复现命令 + **raw evidence 路径** | HTTP、取消、deadline、memory、trap、§4.6/§4.7 | **独立授权** |
| **R4 — Binding spike** | config + KV 共享语义 | V8/native 对照测试 | branch 隔离不回退 | R3 后 + 授权 |
| **R5 — Decision** | default 与 fallback | Proposed AD + 对抗审查 | AD 通过 | **R5 AD 通过前不改产品代码** |
| **R6 — Product plan** | artifact/API/site/E2E | 实施 DAG/test plan | AD 批准 | R5 后 |

**现行不变：** [test-plan.md](../test-plan.md)、M2 门禁、公开站点 **不因 R0–R5 研究**自动改变。

不得提前在 Registry/OpenAPI 中加入 `runtime_kind`，也不得提前改 `celld deploy` manifest。R2/R3 可使用实验目录或 test-only harness，但不能伪装成 shipped feature。

---

## 9. 可替换性的验收定义

只有同时满足以下条件，才可宣称 runtime 可替换：

1. 同一原始 component artifact 在两个 runtime 共同支持的 capability profile 内，无需重编译即可由两个 adapter 执行。
2. 同一 conformance suite 的共同 profile 对两个 adapter 通过；引擎专属或未支持能力均显式记录。
3. cellp Gateway、Version、route、promote、archive/wake 不含引擎分支。
4. Binding semantic service 不引用任何候选 runtime 类型。
5. 切换不改变 RustFS 权威对象和 branch 语义；引擎缓存可直接丢弃重建。
6. OTEL resource/attribute 与标准错误码不随引擎改变。
7. 配置切换 fail-closed，可回退到前一引擎；不允许同一 version 静默混跑不同语义。
8. 候选升级必须重跑 ABI、limits、security、performance 门禁。

**允许的现实边界：** default runtime 与备用 runtime 可以性能不同，也可以有明确 capability 子集；可替换不等于所有内部能力完全相同。

---

## 10. 安全基线

**威胁模型 honesty：** 现有 celld 威胁模型面向 **私有化部署、非 hostile multi-tenant**；Wasm sandbox **不扩大**现有租户隔离承诺。

- 默认不授予 filesystem、raw socket、environment 或任意 outbound network。
- Secret 只作为 capability 提供；禁止进入日志、trap、error response、cache key 或 evidence fixture。
- 每请求必须有可强制执行的 deadline/CPU policy；“future 被 drop”不等于 guest 执行已停止，必须实测。
- 每实例限制 linear memory、table、instances、component resources 和 host-side buffers。
- Hostcall 必须有独立 timeout、取消和配额，不能借 guest deadline 绕过后端超时。
- AOT/serialized artifact 若使用 unsafe deserialize，只能加载本机受信构建产生且完整校验的缓存；不得反序列化用户上传的预编译对象。
- Outbound HTTP 必须阻止 loopback、link-local、metadata endpoint 与未授权内网目标；详细 egress policy 另行设计。
- Trap、OOM、timeout、host failure 必须隔离到请求或实例；进程级故障要有 crash-loop/reconcile 门禁。
- Wasm sandbox 不能替代进程、OS/cgroup、凭据和 bucket 权限边界。

**可观测性：** 引擎 id/version 等诊断可进入 **受限内部日志/health**；**稳定** API 错误码、metric 语义须 **引擎中立**，且不得泄漏 secret、host path、指针或 raw sensitive data。

---

## 11. 当前候选事实快照（不是选型结论）

### Wasmtime

截至本文核验，官方 support tier 将 Component Model、`wasi:http` 和 Rust embedding API 列为 Tier 1；`wasmtime_wasi_http` 提供 WASIp2，WASIp3 标为 experimental/unstable/incomplete。**Desk fact only**— R1 硬门槛与 R2/R3 证据与 Wasmer **同等**对待；不得因官方文档更完整而跳过 eligibility。

### Wasmer

截至本文核验，Wasmer 官方说明其可作为 library/CLI 使用，支持 Rust SDK，默认不开放 filesystem/network/environment，并提供 Singlepass/Cranelift/LLVM backend、caching、metering、profiling；仓库使用 MIT license。以上使其值得进入市场调研。

**尚未从已核验官方资料证明：** 它对本 track 最终选定的 Component Model + `wasi:http` world、async hostcall/cancellation 和 streaming 组合达到何种成熟度。因此这些项保持 `UNKNOWN`，由 R1/R2 的 exact-version spike 决定。

---

## 12. 待研究问题

1. baseline 选 WASI 0.2 还是 0.3；多语言 toolchain 的最低共同集是什么？
2. Wasmer 对目标 Component/WIT/HTTP world 的 exact support matrix 是什么？
3. Wasmtime WASIp2 的同步/异步模型与 celld Tokio request path 如何组合？
4. 一个 instance 多请求复用还是请求级 instance pool；guest 全局可变状态语义是什么？
5. streaming body 如何在 WIT stream、Hyper body 与 Gateway cancel 之间传递？
6. CPU 策略使用 epoch/fuel/metering 中哪一种；公平性与可中断性如何证明？
7. 编译缓存放本地 watch/cache 还是 RustFS 派生对象；如何避免 unsafe deserialize 风险？
8. WIT binding package 是否复用成熟 WASI proposal，何时 fork 为 `cellp:*`？
9. KV/D1/R2 error 与 retry 语义如何映射而不复制 JS exceptions？
10. native backend 是否仅支持 stateless Worker；何时值得设计 DO component lifecycle？
11. celld binary 同时链接 V8 与两个 Wasm runtime 的体积/构建/攻击面是否可接受？
12. fallback runtime 是编译时 feature、进程启动配置，还是部署资格能力；禁止请求级随机切换。

---

## 13. 权威来源与证据纪律

本 track 使用 **文献证据** 与 **实验 PASS** 分级：**官方网页** 仅支持 desk fact，**不能**单独使硬门槛 PASS。R1 页眉与 [R1 ledger](./NATIVE-WASM-RUNTIME-R1.md) 中的 **R1 PASS** 指 **desk deliverable / 独立文档验证**，与 §6.2 **实验** PASS、M2 及 runtime 选型无关。R1 产出 **exact version/date/source log**；R2/R3 产出 **raw evidence 路径**（可归档于 `docs/evidence/`，本地 gitignore 规则不变）。

本 track 优先使用标准组织、runtime 官方文档/仓库和本地可复现实验。市场材料只用于发现候选，不作为兼容性证据。

已核验入口：

- Cloudflare [Rust language support](https://developers.cloudflare.com/workers/languages/rust)
- Cloudflare [WebAssembly](https://developers.cloudflare.com/workers/runtime-apis/webassembly)
- Cloudflare [Wasm in JavaScript](https://developers.cloudflare.com/workers/runtime-apis/webassembly/javascript)
- WASI [Releases](https://wasi.dev/releases)
- Component Model [Running Components](https://component-model.bytecodealliance.org/running-components.html)
- Wasmtime [Tiers of Support](https://docs.wasmtime.dev/stability-tiers.html)
- Wasmtime [`wasmtime_wasi_http`](https://docs.wasmtime.dev/api/wasmtime_wasi_http/index.html)
- Wasmer [Runtime Introduction](https://docs.wasmer.io/runtime)
- Wasmer [Runtime Features](https://docs.wasmer.io/runtime/features)
- Wasmer [repository](https://github.com/wasmerio/wasmer)

后续每轮调研必须记录 exact version/date；网页内容变化时，以可复现实验和归档 evidence 为准。

---

## 14. 下一步（只做研究）

| # | 动作 | 阶段授权 |
|---|------|----------|
| 1 | 独立验证 [REVIEW](./NATIVE-WASM-RUNTIME-REVIEW.md) closure → 用户批准 **R1** | R0 完成后 |
| 2 | Wasmtime 与 Wasmer **同等**候选档案：硬门槛 + `UNKNOWN` + source log | **R1 only** |
| 3 | 冻结 **test baseline identifier** + R2 adapter-only HTTP fixture（不冻结公开 ABI） | **R2 授权后** |
| 4 | 相同 component bytes；R2 adapter-only → R3 shell+adapter | R2/R3 各需授权 |
| 5 | 取消、deadline、memory、trap、§4.6/§4.7 证据 → R5 讨论 default | R3+ → R5 AD |

在 **R5 Proposed AD 获批前**，**不改产品代码、不改公开 Native support matrix、不在站点宣称原生 Wasm 已可用。**
