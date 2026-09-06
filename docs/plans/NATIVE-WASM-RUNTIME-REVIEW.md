# Native Wasm Runtime 研究纲领 — R0 对抗审查记录

| 字段 | 值 |
|------|-----|
| **审查对象** | [NATIVE-WASM-RUNTIME-RESEARCH.md](./NATIVE-WASM-RUNTIME-RESEARCH.md) |
| **初审 verdict** | **REVISE** |
| **修订状态** | **VERIFIED — R0 PASS；R1 desk research 已获用户批准并完成仓内验证** |
| **状态边界** | 本文件**仅**记录 **R0** 对抗审查与 NW-R0-F01–F11 closure；**不是** [R1 Market Scan](./NATIVE-WASM-RUNTIME-R1.md) 的原始 verifier findings（NW-R1-V01–V04 及 desk 证据链以 R1 ledger 为准，本 REVIEW 不复述或替代）。R0 合并修订**仅**使研究纲领具备进入 **R1 desk research** 的资格；**R0 与 R1 closure 均不**授权 R2/R3 prototype、**不**授权产品实现、**不**改变 `docs/test-plan.md` / M2 / 公开站点 |
| **日期** | 2026-09-05（R0）；R1 链状态同步见文末 |

---

## 范围与方法

- **范围：** R0 Contract brief — 研究边界、术语、候选中立性、stateless seam、实例 lease、取消/流所有权、capability eligibility、缓存/安全/可观测性、Native/JS 矩阵关系、证据与阶段授权。
- **方法：** 三路只读 Sonnet 对抗审查 + 主管裁决合并；**未**做外部网络调研或 runtime 选型。
- **审查 thread ID：**
  - Architecture：`01a071fd-6d8f-76c3-aa95-bb5718a994ff`
  - Runtime / security：`01a071fd-6d9b-73d3-8367-3ac964b690ad`
  - Compat / method：`01a071fd-6da3-7093-8cf0-7965ba1858ad`
- **独立验证 thread ID：** `01a07207-0ebd-7503-9894-cdd8865e261f`（Sonnet verification；verdict `PASS`）

**AD-15 说明：** 权威状态以 [decisions.md](../decisions.md) **§20（已正式批准）** 为准；`SURGE-PROPOSED-AD.md` 文件名仍为 Proposed，不覆盖 decisions 状态。

---

## Findings 合并表（NW-R0-F01 … F11）

| ID | Severity | 初审问题（摘要） | 修订依据（research doc anchor） | Closure |
|----|----------|------------------|----------------------------------|---------|
| **NW-R0-F01** | MAJOR | AD/lifecycle：须纳入 AD-12/AD-15；同一 Gateway ingress；无引擎分流；embedded adapter 为 R2/R3 基线 | §2.3、§2.4；header AD 引用 | **CLOSED** |
| **NW-R0-F02** | MAJOR | WASI 0.2/0.3 术语；test baseline identifier；「冻结契约」→「定义并审查契约方向」 | §0.1、§1.2、§0 一句话 | **CLOSED** |
| **NW-R0-F03** | MAJOR | Wasmtime/Wasmer R1 同等；移除 R2/R3 偏置；UNKNOWN 与单引擎 replaceability | §6.1、§6.3、§6.4、§11 | **CLOSED** |
| **NW-R0-F04** | BLOCKER | 首期 stateless HTTP only；CellHost/DO 等 out of scope；seam map；generation/drain；R2 vs R3 | §2.2、§2.4、§8 | **CLOSED** |
| **NW-R0-F05** | BLOCKER | 实例 lease profile：单 active request、stream 完成前不归还、fault discard、R3 quarantine | §4.7、§7.1 | **CLOSED** |
| **NW-R0-F06** | MAJOR | RequestContext/HostCallContext；有界取消；stream ownership；cancel fixtures | §4.2、§4.6、§7.1 | **CLOSED** |
| **NW-R0-F07** | MAJOR | HostCapabilities 最低事实；capability 超集 fail-closed；AD-6 CLI vs WIT 分层 | §2.3（AD-6）、§4.3 | **CLOSED** |
| **NW-R0-F08** | MAJOR | cache miss/corrupt discard；威胁模型 honesty；引擎中立稳定语义 + 脱敏内部诊断 | §4.3、§10 | **CLOSED** |
| **NW-R0-F09** | MAJOR | Native matrix 独立于 JS；不继承 Yes/Partial/No；不自动补齐 CF 全语义 | §3.1、§7.1 | **CLOSED** |
| **NW-R0-F10** | MAJOR | 文献 vs 实验 PASS；R1 source log / R2–R3 evidence；rubric 测量前冻结；阶段授权 | §6.3、§8、§13、§14 | **CLOSED** |
| **NW-R0-F11** | MINOR | Native v0 WebSocket 拒绝 vs 现有 JS WebSocket；AD 引用与清单一致 | §2.2、header、§2.3 | **CLOSED** |

---

## 驳回 / 限定记录

| 建议 | 裁决 | 理由 / 正确 closure |
|------|------|---------------------|
| Native capability 以上游 celld JS Yes/Partial/No 为上限 | **REJECT** | 两条 guest ABI 独立 → **F09** |
| 首期 stateless native 必须新增 `CellHost::Native` | **REJECT** | CellHost 为 stateful 边界，首期 out of scope → **F04** |
| 完全隐藏 engine diagnostics | **QUALIFY** | 允许受限内部 engine id/version；稳定产品语义不依赖且须脱敏 → **F08** |
| R0 冻结完整 pool/state/stream Rust API | **QUALIFY** | R0 只冻结研究不变量与 test profile；公开 ABI 仍不提前冻结 → **F05/F06** |

---

## Exit gate checklist

| Gate | 状态 |
|------|------|
| F01–F11 在 research doc 有可追溯 closure | ☑ PASS |
| 无 Wasmtime 优先 / P0 spike / JS compat ceiling / 独立 daemon 默认等措辞 | ☑ PASS |
| AD-15 与 decisions §20 一致表述 | ☑ PASS |
| REVIEW 与 README 索引链接有效 | ☑ PASS |
| `git diff --check` 通过（含 untracked） | ☑ PASS |

**Final verdict（本记录，R0 时点）：** **R0 PASS — READY FOR R1 APPROVAL**。该 verdict 仅证明研究边界可进入用户 Gate；在 R0 完成当时**不等于**用户已批准 R1，也不授权 R2/R3、产品实现或公开支持声明。

**链状态同步：** 用户已批准 R1 Market Scan；R1 desk deliverable 与仓内独立文档验证 **PASS** — 见 [NATIVE-WASM-RUNTIME-R1.md](./NATIVE-WASM-RUNTIME-R1.md)（**非**本 REVIEW 的 findings 产物）。R1 **PASS** 仅闭合 desk 交付与文档证据链；**不是**实验 PASS、M2 或 runtime 选型，**不**因 R1 完成而授权 R2/R3、产品实现或公开支持声明。

---

## 残余 R1 / R2 unknowns（非阻塞 R0 准入）

- 冻结后的 **test baseline identifier** 具体 WIT package/world 字符串（R2 前）。
- Wasmtime vs Wasmer 对 baseline 的 hard-gate matrix（R1 desk + R2 实验）。
- Tokio 路径与各引擎 async hostcall 组合细节（R2/R3）。
- Pool quarantine / 健康 escalation 具体策略（R3 证明项）。
- Binding WIT package 命名与 R5 Proposed AD 范围。

---

## 相关链接

- 研究纲领（修订后）：[NATIVE-WASM-RUNTIME-RESEARCH.md](./NATIVE-WASM-RUNTIME-RESEARCH.md)
- R1 Market Scan ledger（R1 findings / NW-R1-V01–V04）：[NATIVE-WASM-RUNTIME-R1.md](./NATIVE-WASM-RUNTIME-R1.md)
- 文档索引：[docs/README.md](../README.md)
