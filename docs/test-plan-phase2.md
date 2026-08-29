# cellp 生产压测验收计划

> **前置：** [test-plan.md](./test-plan.md) **M2** 全绿。  
> **计划：** [plans/phase-5-stress.md](./plans/phase-5-stress.md)  
> **环境：** 填写 `docs/evidence/stress-env.json`（含 **offshoot tier**: local | rustfs）

## 目标

单节点生产拓扑下验证：并发 CD · promote under load · 24h soak · 混沌恢复 · 数据一致性。

**不测：** 多 cellpd 节点 · KV/Queue · 全球边缘。

---

## 环境基线

| 项 | 记录 |
|----|------|
| 硬件 | CPU/RAM/磁盘 → `stress-env.json` |
| RustFS tag | 同 Phase 0 |
| offshoot tier | local 或 rustfs（报告须注明） |
| 阈值基线 | 首次 L1 跑完写入 `stress-env.json` 的 `baseline` 段 |

---

## A. 负载基准

### [x] TP2-L1 — 顺序 CD 基线

| 命令 | `stress/scripts/sequential-cd.sh` |
| 通过 | 10/10 终态 `ready`/`failed`；p95 ≤ `STRESS_P95_DEPLOY_SEC`（默认 600，写入 env.json） |

### [x] TP2-L2 — 并发 CD（同 project · 3 路）

| 命令 | `stress/scripts/concurrent-cd.sh` |
| 通过 | 3/3 终态 ≤ **900s**；`sqlite3 … "SELECT count(*) FROM routes WHERE active=1"` 预期值 |

### [x] TP2-L3 — 多 project 并发

| 通过 | 3×2 版本；`curl` body 含 project 标识，无串扰 |

### [x] TP2-L4 — Gateway RPS

| 命令 | `stress/scripts/gateway-load.sh` |
| 通过 | 500 RPS × 5min；错误率 < **0.1%**；p99 < **500ms**（counter fixture） |

### [x] TP2-L5 — Promote under load

| 通过 | cutover 窗口 ≤ **5s**；窗口内 5xx ≤ **1%**；切后 60s 内 5xx = **0** |

---

## B. Soak

### [x] TP2-S1 — 24h Soak

| 命令 | `stress/scripts/soak-24h.sh` |
| 通过 | RSS(T24)/RSS(T0) < **1.10**；`cellp-registry.sqlite` < **500MB** |

### [x] TP2-S2 — Version 上限

| 通过 | 第 6 个 ready POST → **429**（或文档化 queue 行为） |

### [x] TP2-S3 — TTL 回收

| 命令 | `stress/scripts/ttl-gc.sh` |
| 通过 | TTL 到期 → `destroyed`；route 行删除 ≤ **300s** |

---

## C. 混沌

### [x] TP2-C1 — celld SIGKILL mid-deploy

| 命令 | `stress/scripts/chaos-celld-kill.sh` |
| 通过 | ≤ **120s** 内 `failed`；重试可 `ready` |

### [x] TP2-C2 — RustFS pause 30s

| 通过 | 无 celld 双主；恢复后可重试 |

### [x] TP2-C3 — cellpd restart mid-orchestrate

| 命令 | `stress/scripts/chaos-cellpd-restart.sh` |
| 通过 | job 从 SQLite `jobs` 恢复或 ≤ **300s** → `failed` |

### [x] TP2-C4 — offshoot fork 失败

| 命令 | `stress/scripts/chaos-offshoot-fail.sh` |
| 通过 | saga GC；无 active route |

### [x] TP2-C5 — SQLite 争用

| 命令 | `stress/scripts/chaos-sqlite-contention.sh` |
| 通过 | 并发 promote+route 更新；无 hung > **60s** |

---

## D. 数据正确性

### [x] TP2-D1 — 并发 counter

| 命令 | `stress/scripts/data-counter-load.sh` |
| 通过 | 100 并发 × 5min；最终 count = **预期公式**（脚本内文档化） |

### [x] TP2-D2 — Promote 读一致

| 通过 | promote 后 1000 读全部新 prod body |

---

## E. 报告

### [x] TP2-R1 — stress-report.md

| 路径 | `docs/evidence/stress-report.md` |
| 必含 | 每项 p50/p95/p99 · 错误率 · RSS@T0/T24 · sqlite 大小 |

### [x] TP2-R2 — 可重复 harness

| 命令 | `./stress/scripts/run-all.sh`（不含 24h 时可 `-short`） |

### [x] TP2-MET-1 — 指标归档

| 路径 | `docs/evidence/stress-metrics.jsonl` |

---

**test-plan-phase2 完成 = 全部 `[x]` + tier 与阈值记录在 stress-env.json**

---

*test-plan-phase2 v2 · 2026-08-27*
