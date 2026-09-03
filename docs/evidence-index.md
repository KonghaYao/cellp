# 证据目录（`docs/evidence/`）

> 本目录默认 **gitignore**（仅 `.gitkeep` 入库）。本地跑验收后在此落 `.md` 报告与 `.log`；结构化结论可择要提交时改 `.gitignore` 或使用 `docs/` 下活文档。

- **Support 索引：** [support/README.md](./support/README.md)
- **验收计划：** [test-plan.md](./test-plan.md)

## 报告命名（本地）

| 主题 | 典型文件 |
|------|----------|
| D1 | `d1-*-report.md` · `d1-*-metrics.jsonl` |
| 存储 / 6A | `v0b-pass-report.md` · `scale-report-6A.md` · `offshoot-branch-scale-report*.md` |
| Ingress / WS | `ingress-port-p5c.md` · `websocket-ingress-h1h2.md` |
| Support | `support-S*.log` · `verify-full-*.log` |
| Dashboard TP-UI-14 | `user-loop-*.log` · 定义 [plans/user-behavior-closed-loop.md](./plans/user-behavior-closed-loop.md) |

**Prod / curl 明细（入库）：** [support-curl-user-acceptance.md](./support-curl-user-acceptance.md) · [support-framework-user-acceptance.md](./support-framework-user-acceptance.md)
