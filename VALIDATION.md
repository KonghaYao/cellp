# cellp 验证 TODO（历史索引）

> **执行请用 [docs/test-plan.md](./docs/test-plan.md)** — 本文件保留 VALIDATION 编号便于对照 DESIGN，不再单独维护勾选状态。  
> **架构决策：** [docs/decisions.md](./docs/decisions.md)

| VALIDATION | test-plan |
|------------|-----------|
| V0a–V0d | TP-V0a–V0d |
| V1–V7 | TP-V1–V7 |
| VE | TP-VE-* |

> 设计背景：[DESIGN.md](./DESIGN.md) · 本地环境：[dev/AGENTS.md](./dev/AGENTS.md)

**规则：** 探针或 spike 失败 → **对应能力不得上线**；换存储版本后重跑受影响的项。

**分期对齐：**

| 文档阶段 | 范围 |
|----------|------|
| **验证门禁 + V1–V7 + VE** | **一期** — CD · Branch · 后端 · 端口 E2E |
| **V8 及以后** | **二期** — KV · Queue · Cron · 弹性 |
| **V20 及以后** | **三期占位** — 可观测 · 性能统计（暂不计划） |

---

## 验证门禁（一期前置 · 阻塞存储上线）

### [ ] V0a — RustFS × celld 条件写探针

**目的：** celld fleet Blob 能在 RustFS 上安全 fencing（无双主）。

| 项 | 内容 |
|----|------|
| 前置 | RustFS 已部署；bucket `cellp-celld` 已创建；凭证已配置 |
| 命令 | `celld diagnose --bucket s3://cellp-celld --endpoint "$S3_ENDPOINT" --region "$AWS_REGION"` |
| 通过 | 输出含 `ok bucket conditional write (create, reject-create, update, reject-stale)` |
| 失败 | celld fleet **不得启动**；备选 MinIO 重跑 V0a，或换 RustFS 版本 |
| 记录 | RustFS 镜像 tag · celld 版本 · diagnose 完整日志 |

---

### [ ] V0b — offshoot SQLite branch × RustFS S3（全序列）

**目的：** attach 探针通过 **不等于** fork/export/promote 可用；须在 **目标 RustFS** 上验证 branch 全路径。

| 项 | 内容 |
|----|------|
| 前置 | V0a 建议先过（可并行）；`OFFSHOOT_STORE=s3://cellp-offshoot` + `OFFSHOOT_S3_*` 指向 RustFS |
| 序列 | ① `offshoot init` → ② `create` → ③ 写入 + `checkpoint` → ④ `fork`（≥2） → ⑤ `export` → ⑥ `promote` → ⑦ `destroy` |
| 并发 | 同一 parent **并行 fork ≥2**，无 CAS 冲突、无脏读 |
| 物化路径 | 大库触发 `CopyObject` / multipart 不报错（可选 >100MB） |
| 通过 | 全序列 exit 0；export 可打开；promote 后 head 一致 |
| 失败 | offshoot **不得用 RustFS**；暂用 local 或 MinIO |
| 记录 | offshoot 版本 · RustFS tag · 每步命令与耗时 |

**V0b 完成前：** offshoot → local 或 MinIO；RustFS 仅 celld Blob + 制品。

---

### [ ] V0c — RustFS 多节点条件写（若 prod 多 endpoint）

| 适用 | prod RustFS ≥2 节点且客户端连不同 endpoint |
| 通过 | 并发条件写仅一个成功 |
| 跳过 | 全走 **单一 VIP** 时可跳过并文档注明 |

---

### [ ] V0d — offshoot attach 探针（RustFS）

| 通过 | attach 不因无条件写拒绝 |
| 注意 | **不能替代 V0b** |

---

## 一期 — CD · Branch · 线上稳定

### [ ] V1 — offshoot export → celld D1 seed

| 步骤 | export → `celld d1 execute` → deploy → Worker 读 D1 |
| 通过 | fork 后数据可见；health OK |

---

### [ ] V2 — Quiesce + checkpoint 一致性 fork

| 通过 | drain → checkpoint → fork；子 version 无 stale 写入 |

---

### [ ] V3 — 单 celld fleet + Gateway 双 version 路由

| 通过 | `/{project}/{version}/*` 可区分两版本 |

---

### [ ] V4 — Promote 原子 cutover

| 通过 | prod 切换无双写；旧路由已停 |

---

### [ ] V5 — Orchestrator 失败补偿

| 通过 | deploy 失败无孤儿 branch / 泄漏路由 |

---

### [ ] V6 — Schema migration × fork 顺序

| 通过 | preview 上 migrate 完成后再 ready |

---

### [ ] V7 — 外部 CI → cellp 端到端（一期验收 · API/脚本）

| 步骤 | 任意 CI：build → artifact 上传 RustFS → `POST /versions` → preview URL → promote prod |
| 通过 | 全链路无人值守；prod URL 可访问且数据与 branch 一致 |
| 注意 | cellp **不依赖** 特定 Git/CI 产品；示例见 `dev/examples/ci-deploy.example.yml` |

---

### [ ] VE — 端口级 E2E（**后端 P0 完成门禁 · 前端开工前必过**）

**目的：** `cellpd` 替换 mock 后，**仅通过 HTTP/脚本** 验证各端口，不依赖 Dashboard 与浏览器。

| 端口 | 服务 | 检查 |
|------|------|------|
| `:8790` | cellpd API | `GET /v1/health` · `POST/GET /versions` · promote · destroy |
| `:8787` | cellpd Gateway（内置） | `GET /{project}/{version}/` → 200 · 路由切换 |
| `:8792` | celld | `GET /__celld/health` |
| `:9000` | RustFS | S3 探针已由 V0a 覆盖 |

| 场景 | 步骤 |
|------|------|
| **CD 闭环** | API 触发 version → 轮询 `ready` → curl Gateway preview |
| **Promote** | API promote → 旧 prod 路由失效 · 新 prod 200 |
| **失败补偿** | 模拟 deploy 失败 → version `failed` · 无泄漏路由 |

| 项 | 内容 |
|----|------|
| 实现 | `e2e/scripts/`（bash + curl + jq，或 Go testscript） |
| 通过 | 脚本 exit 0；CI 可重复跑 |
| 阻塞 | **VE 不过 → 不开工 `web/` Dashboard** |

---

## 二期 — KV · Queue · Cron · 弹性

> 一期 **不做** 以下项；Worker 内 celld 原生 cron 仍可由 celld 执行，但 cellp 不编排。

### [ ] V8 — scale-to-zero 唤醒（Gateway → celld hibernate）

### [ ] V9 — Valkey KV（prefix · `inherit_kv` · ACL）

### [ ] V10 — Queues（CD 路径接入，方案待定）

### [ ] V11 — Cron 平台化（注册 · 多 version · 可观测）

### [ ] V12 — 多节点 cellpd + Orchestrator 队列

---

## 三期 — 可观测 · 性能/统计（暂不计划 · 仅文档）

> **无排期。** 立项前不实现、不验收。此处占位便于与一期/二期区分。

### [ ] V20 — OTEL / Prometheus 指标（占位）

### [ ] V21 — 集中日志 / 告警（占位）

### [ ] V22 — 部署与运行时性能统计（占位）

### [ ] V23 — Dashboard 用量/图表（占位）

---

## 汇总表

| ID | 验证项 | 阶段 | 阻塞 |
|----|--------|------|------|
| **V0a** | RustFS × celld diagnose | 门禁 | celld fleet |
| **V0b** | offshoot branch × RustFS S3 | 门禁 | offshoot 用 RustFS |
| **V0c** | RustFS 多节点条件写 | 门禁 | 多 endpoint |
| **V0d** | offshoot attach × RustFS | 门禁 | （非充分） |
| **V1–V3** | 数据管道 · fork 一致性 · 路由 | **一期** | CD 闭环 |
| **V4–V7** | cutover · saga · migrate · 外部 CI/API E2E | **一期** | 一期验收 |
| **VE** | 端口级 E2E（无 UI） | **一期 P0** | **前端开工门禁** |
| **V8–V12** | wake · KV · Queue · Cron · 多节点 | **二期** | 二期验收 |
| **V20+** | OTEL · 统计 · 图表 | **三期占位** | 暂不计划 |

---

*最后更新：2026-08-27 · SQLite Registry · cellpd 内置 Gateway · 外部 CI*
