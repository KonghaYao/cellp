# 内部过程文档（docs/process）

> 贡献者用：**交付摘要、阶段 handoff 合并表**；不替代 [decisions.md](../decisions.md)、冻结契约或 [test-plan.md](../test-plan.md)。

## 文档分层（2026-09-08 整理）

| 层级 | 含义 | 典型路径 |
|------|------|----------|
| **权威 AD / 门禁** | 现行决策与验收口径 | [decisions.md](../decisions.md) · [test-plan.md](../test-plan.md) |
| **冻结契约** | 修改需对抗审查 | [plans/D1-*-RPC.md](../plans/D1-IMPORT-RPC.md) · [openapi.yaml](../../cellp/api/openapi.yaml) |
| **有效设计包** | Draft 但未替代 AD 的成套设计 | [SURGE-DESIGN-INDEX.md](../plans/SURGE-DESIGN-INDEX.md) · `plans/00–14` |
| **过程记录** | 已合并的交付/handoff 摘要 | **本目录**（如 [surge-delivery-log.md](./surge-delivery-log.md)） |
| **活跃计划** | 仍在演进的产品/实验计划 | [plans/](../plans/)（排除 [archive](../archive/) 中副本） |
| **证据** | 本地跑数产物为主 | [evidence-index.md](../evidence-index.md) · `docs/evidence/` |
| **归档** | 历史 Draft / 会话交接全文 | [archive/README.md](../archive/README.md) |

## 本目录文件

| 文件 | 用途 |
|------|------|
| [surge-delivery-log.md](./surge-delivery-log.md) | SURGE AD-15 E2–E5 实现 handoff 摘要（原 `evidence/surge/e*/**/handoff.md` 已合并） |

## SURGE 分阶段自动化

[scripts/surge-phased-delivery.mjs](../../scripts/surge-phased-delivery.mjs)（Workflow）按序交付 E2→E5、ADOPT、ACCEPT：

| 项 | 约定 |
|----|------|
| **仓库根** | `CELLP_REPO` 环境变量，默认 `process.cwd()` |
| **过程记录** | 每阶段 agent **追加一行**到 [surge-delivery-log.md](./surge-delivery-log.md) 的 `## 自动化追加`；**禁止**再写 `docs/evidence/surge/**/handoff.md` |
| **可选** | `docs/evidence/surge/<phase>/delivery-notes.md` 仅当次运行的短 bullet |
| **提交** | 每阶段独立 `git commit`（含 `Co-Authored-By` 行） |

## 约定

- **不要**在 `docs/plans/` 或 `docs/evidence/` 下长期堆会话级 handoff；合并进 `docs/process/` 或归档后删原文件。
- **不要**把归档文档当现行 AD；读 SURGE 弹性以 [SURGE-DESIGN-INDEX](../plans/SURGE-DESIGN-INDEX.md) 与 [SURGE-AD15-HANDOFF](../handoff/SURGE-AD15-HANDOFF.md) 为准。
- 产品/onboarding 仍以 [GitHub Pages](https://konghayao.github.io/cellp/) 与 [docs/README.md](../README.md) 导航为准。
