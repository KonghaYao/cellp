# cellp Phase 6 — 千万级扩展验收计划

> **前置：** Phase 0–5 完成（M3 压测 sign-off）  
> **计划：** [plans/phase-6-scale-10m-master.md](./plans/phase-6-scale-10m-master.md) · [plans/v1-v0b-phase6-plan.md](./plans/v1-v0b-phase6-plan.md)  
> **范围（2026-08-29）：** **6A 实现完成**（SQLite scope）。**6B–6F OUT OF SCOPE**（无 PG、无多租户）。**不宣称** 6F / 千万 sign-off。  
> **环境：** 填写 `docs/evidence/scale-env.json`（集群拓扑、节点规格、阈值）

## 目标

在 **100% 私有化** 约束下，分阶段验证四维扩展目标（D1–D4）：

| 维度 | 定义 | 10M 目标 |
|------|------|----------|
| **D1** | 终端用户 Gateway QPS | 峰值 100k–500k RPS |
| **D2** | 活跃 Project 数 | 100 万活跃 + 900 万归档 |
| **D3** | Version 历史行数 | 1 亿（热 1000 万 + 冷 9000 万） |
| **D4** | 控制面 deploy 吞吐 | 1k–5k deploy/min |

**验收原则：** 四个维度 **分别设 Gate**；不允许「Gateway 撑住了但 Registry OOM」的伪通过。

---

## A. 6A — 基线与止血（SQLite + 分页 + GC + seed）

### [x] TP6-A1 — API 游标分页

| 交付 | `GET /v1/projects?limit&cursor` · `GET /v1/projects/{id}/versions?limit&cursor&status` |
| Track | 6A-T1 |
| 通过 | 默认 `limit=50`，最大 200；响应含 `next_cursor`；无全表扫描超时 |

### [x] TP6-A2 — Gateway 路由缓存

| 交付 | Gateway `RouteCache`（Ristretto/arc，写 invalidate） |
| Track | 6A-T2 |
| 通过 | @500 RPS counter fixture，p99 **相对基线 −50%** |

### [x] TP6-A3 — Job/Version GC

| 交付 | `jobs`/`versions` GC cron |
| Track | 6A-T3 |
| 通过 | `jobs` 表 **<100k 行**（7 天滚动） |

### [x] TP6-A4 — Dashboard 虚拟列表

| 交付 | Dashboard cursor fetch + 虚拟滚动 |
| Track | 6A-T4 |
| 通过 | **1 万行** 项目/版本列表流畅（无卡顿、无 OOM） |

### [~] TP6-A5 — 百万 seed 压测与 Registry 基准

> **10k seed ✅** · ListProjects p99 **238–262ms** @10k（gate 200ms **未过** — **SQLite waiver**）· ListVersions p99 1ms ✅  
> **SQLite 结论：** @1k 满足 gate（p99 ~64ms）；@10k 单节点 SQLite 实测上限 ~238–262ms（优化后从 534ms 降至 ~238ms）。完整 200ms gate 需 PG（**6B，OUT OF SCOPE**）。  
> **未跑：** 10×10k versions 全量 bench · 100k Gateway RPS（需 infra；dev 基线见 `gateway-scale.sh`）。

| 命令 | `stress/phase6/seed-projects.sh` · `seed-versions.sh` · `registry-bench.sh` · `list-api-load.sh` · `registry-size-report.sh` |
| Track | 6A-T5 |
| 通过 | 见下表；证据 [scale-report-6A.md](./evidence/scale-report-6A.md) |
| **豁免** | ListProjects @10k p99 **238–262ms** — 接受为 SQLite 天花板；不阻塞 6A 实现 sign-off |

**TP6-A5 规模门禁：**

| 规模 | 通过标准 | 实测 |
|------|----------|------|
| **10k projects** | `ListProjects` cursor **p99 <200ms** | p99 **238–262ms** ❌（**SQLite waiver**） |
| **100k versions/项目**（模拟 10 项目 ×1 万） | 分页 **p99 <100ms** | 部分 seed（10×1k）✅；全量 10×10k 待跑 |
| **100k Gateway RPS**（cached route） | 错误率 **<0.1%** | **未测**（dev 基线 `gateway-scale.sh` ~500–2k RPS） |

---

## B. 6B — 租户模型 + PostgreSQL

> **OUT OF SCOPE** — 产品决策：不做 PostgreSQL、不做多租户/RBAC。

### [-] TP6-B1 — 百万 Project @ PG

| 交付 | PG schema（orgs · members · projects · versions_hot · routes · jobs） |
| Track | 6B-T1–T5 |
| 通过 | **100 万** project seed；`ListProjects` cursor **p99 <100ms** @PG 主库 |

---

## C. 6C — 控制面水平扩展

### [ ] TP6-C1 — 多 cell 并发入队

| 交付 | Orchestrator worker pool · cellpd API ×N · Cell 分片 |
| Track | 6C-T1–T4 |
| 通过 | **3 cell × 1000** 并发 POST → **95%** 入队 **<5s** |

---

## D. 6D — 数据面与归档

### [ ] TP6-D1 — 冷热分离与热表稳定

| 交付 | Version 冷归档 worker · 路由缓存 · RustFS 多节点 |
| Track | 6D-T1–T4 |
| 通过 | **1 亿** version manifest；热表 stable **<10GB** |

---

## E. 6E — 运行时联邦（D1 核心）

### [ ] TP6-E1 — Gateway 50k RPS 单项目 prod

| 交付 | celld fleet 调度 · Gateway 水平扩展 · scale-to-zero preview |
| Track | 6E-T1–T4 |
| 通过 | **50k RPS** 单项目 prod；**p99 <100ms**；**5xx <0.01%** |

---

## F. 6F — 千万验收（Full Sign-off）

### [ ] TP6-F1 — 终端洪峰

| 场景 | **500k RPS** 混合读写 |
| 通过 | **5xx <0.01%**，**p99 <200ms** |

### [ ] TP6-F2 — 租户规模

| 场景 | **100 万** project 随机查询 |
| 通过 | **p99 <100ms** |

### [ ] TP6-F3 — 部署风暴

| 场景 | **5k deploy/min ×10min** |
| 通过 | 队列无丢；**95%** ready **<10min** |

### [ ] TP6-F4 — 混沌

| 场景 | 随机 kill cellpd/celld/RustFS 节点 |
| 通过 | 自愈 **<5min** |

### [ ] TP6-F5 — 24h Soak

| 场景 | F1 的 **10%** 负载持续 24h |
| 通过 | 内存无泄漏；PG 连接稳定 |

---

## 压测脚本（stress/phase6/）

| 脚本 | 维度 | 目标 |
|------|------|------|
| `seed-projects.sh` | D2 | N projects via API（默认 1000） |
| `seed-versions.sh` | D3 | M versions/project（SQLite metadata bulk） |
| `registry-bench.sh` | D2/D3 | ListProjects/Versions p50/p95/p99 |
| `list-api-load.sh` | D2/D3 | vegeta/curl cursor pagination load |
| `seed-orgs.sh` | D2 | 10k org × 100 project（**6B+，OUT OF SCOPE**） |
| `gateway-scale.sh` | D1 | dev 基线 ~500–2k RPS（**非** 100k gate；6E+ OUT OF SCOPE） |
| `deploy-storm.sh` | D4 | 5k/min POST（**6C+，OUT OF SCOPE**） |
| `registry-size-report.sh` | — | SQLite registry 体量 + 行数 + 可选 `EXPLAIN` |

## 证据文件

| 路径 | 内容 |
|------|------|
| `docs/evidence/scale-env.json` | 集群拓扑、节点规格、阈值 |
| `docs/evidence/scale-metrics.jsonl` | 每阶段 append |
| `docs/evidence/scale-report-{6A..6F}.md` | 人读报告 |

---

## 里程碑

| 里程碑 | 含义 | 维度 | 状态 |
|--------|------|------|------|
| **M4** | 6A 实现 + SQLite 基线 | 分页 + cache + GC + Dashboard | **实现完成**；TP6-A5 ListProjects @10k **SQLite waiver**（p99 238–262ms vs 200ms gate） |
| **M5** | 6B PG 切换完成 | D2 100 万 project | **OUT OF SCOPE** |
| **M6** | 6E Gateway 50k RPS | D1 核心 | **OUT OF SCOPE** |
| **M7** | 6F 千万 sign-off | D1–D4 全绿 | **未达成**（6F OUT OF SCOPE） |

---

*Phase 6 Test Plan v2 · 2026-08-29 · 6A SQLite scope · TP6-A5 waiver documented*
