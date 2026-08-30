# 可观测性（cellp v1）

> **状态：** 一期（Prometheus + 日志文件）已可用；**请求 trace / tail / Dashboard 日志 UI** 未排期（三期）。  
> **AD-10：** 不做 SaaS 式 Analytics 产品；运维方自建 Grafana / Loki。

---

## 1. 今天有什么

| 层 | 信号 | 入口 |
|----|------|------|
| **cellpd** | HTTP metrics、registry 状态 | `:8790/metrics`（Prometheus） |
| **Gateway** | 路由、upstream 健康 | 同上 + Gateway 访问日志（cellpd stdout） |
| **celld** | Worker 日志、binding operator | 每 version 进程 stdout / `CELPD_DATA` 下日志文件 |
| **RustFS / S3** | 对象存储 | RustFS 自带 metrics（外层运维） |
| **SQLite registry** | project / version / route | 无内置 UI；`sqlite3 cellp-registry.sqlite` 只读查询 |

**没有什么：**

- `wrangler tail` 等价物
- Dashboard 请求日志浏览器
- 分布式 trace（OpenTelemetry）— 未接入
- 全球边缘 POP 级 RTT 报表

---

## 2. Prometheus 抓取

```yaml
# prometheus.yml 片段
scrape_configs:
  - job_name: cellpd
    static_configs:
      - targets: ["127.0.0.1:8790"]
    metrics_path: /metrics
```

本地 dev：`./dev/scripts/up.sh` 后 `curl -s localhost:8790/metrics | head`.

常用指标（名称以实际 export 为准，见 `cellp/internal/metrics`）：

- HTTP 请求计数 / 延迟 histogram
- Orchestrator job 队列深度
- Gateway upstream 状态

压测基线见 [test-plan-phase2.md](./test-plan-phase2.md) TP2-L4（500 RPS Gateway）。

---

## 3. 日志

| 组件 | 位置 | 内容 |
|------|------|------|
| cellpd | systemd / docker logs / `screen` | API、orchestrator、saga、archive reaper |
| celld | 每 version 子进程 | Worker `console.log`、binding CLI stderr |
| e2e / stress | `docs/evidence/*.log` | 验收与压测（gitignore，本地生成） |

**Archive reaper** 日志关键字：`orch: archive reaper`。

禁用 reaper：`CELLP_ARCHIVE_REAPER=0`。

---

## 4. 运维检查清单

```bash
# 栈健康
./dev/scripts/health.sh

# M1 烟雾
./e2e/scripts/health-all.sh

# Promote 后 5xx（压测门禁）
# 见 stress/scripts/gateway-load.sh · test-plan-phase2 TP2-L5
```

生产 promote 窗口：对照 [runbooks/rollback.md](./runbooks/rollback.md)，监控 Gateway 5xx 率 &lt; 1%（TP2-L5）。

---

## 5. 与 Cloudflare / Vercel 对照

| 能力 | CF / Vercel | cellp v1 |
|------|-------------|----------|
| 实时请求 tail | `wrangler tail` / Logs | **无** — 查 celld stdout 或外层 log agent |
| Analytics UI | 内置 | **无** — Prometheus + Grafana |
| Worker 异常告警 | 内置集成 | 自建 Alertmanager |
| 版本级隔离日志 | 账号维度 | 每 version 独立 celld 进程 → 按 PID/目录分流 |

迁移用户预期管理：[cloudflare-migration.md](./cloudflare-migration.md) · [vercel-migration.md](./vercel-migration.md)。

---

## 6. 路线图（非承诺）

| 阶段 | 范围 |
|------|------|
| **一期（当前）** | `/metrics`、文件日志、证据目录 |
| **二期** | 结构化 JSON 日志、统一 log 目录约定 |
| **三期** | 请求 ID 贯穿 Gateway→celld、Dashboard 只读日志视图 |

三期 **未** 列入 AD-10 核心范畴；issue 不得静默越界为产品必做。
