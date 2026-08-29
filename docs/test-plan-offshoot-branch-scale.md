# 单项目大型 offshoot SQLite branch 压测

> **Harness:** `stress/phase6/offshoot-branch-scale.sh`  
> **V0b-L 入口:** `e2e/scripts/v0b-l-large-fork.sh`  
> **证据:** `docs/evidence/offshoot-branch-scale-report.md` · `docs/evidence/offshoot-branch-metrics.jsonl`

## 范围

一个 offshoot database（对应一个 cellp project）上：大 SQLite + 大量 CoW fork。  
**不测** 50 个 live celld（`CELLP_MAX_READY_VERSIONS` 默认 5）。

## 命令

```bash
# 默认：100MB · 扇出 50 · 链式 20 · 并发 4 · checkout 8
./stress/phase6/offshoot-branch-scale.sh

# 仅大库 fork+export（TP-V0b-L）
./e2e/scripts/v0b-l-large-fork.sh
```

## 门禁

| ID | 通过 |
|----|------|
| TP-OB-1 | seed 文件 ≥ 目标 MB |
| TP-V0b-L | 100MB fork+export 成功，导出色 ≥ seed |
| TP-OB-2 | 扇出 N/N；fork p50/p99 入证据 |
| TP-OB-4 | 每 fork 增量 < 10% seed（CoW） |
| TP-OB-5 | 并发 fork 全成功 |
| TP-OB-3 | 链式 N/N；末次 fork < 10× 首次 |
| TP-OB-7 | destroy + `gc --grace 0s` 两次后无 `storage=shared` |

需要：`offshoot`（`go install github.com/sricola/offshoot/cmd/offshoot@latest`）· `sqlite3` · `python3` · `jq`。
