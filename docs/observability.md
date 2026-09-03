# 可观测性（cellp）

> **权威架构（唯一）：** [plans/OTEL-OBSERVABILITY.md](./plans/OTEL-OBSERVABILITY.md) · **AD-14**  
> 本文只记 **一期已落地** 的 Prometheus / 进程日志，以及如何接到 AD-14。

---

## 0. 分层（不要合成）

| 档 | 现在 | AD-14 目标 |
|----|------|------------|
| **平台 metrics** | `:8790/metrics`（本文 §2） | 保留 |
| **Live** | 每 version celld stdout 文件 | 门面 `logs/stream`（SSE） |
| **Investigate / Analyze** | **无** | OTLP → 可换后端；门面 `traces` / `search`；Grafana 深链 |

**AD-10 / AD-14：** 不做 SaaS Analytics；检索引擎外置；Dashboard 只打 `:8790`。

---

## 1. 今天有什么

| 层 | 信号 | 入口 |
|----|------|------|
| **cellpd** | HTTP metrics、registry 状态 | `:8790/metrics` |
| **Gateway** | 路由、upstream 健康 | 同上 + Gateway 访问日志（stdout） |
| **celld** | Worker `console`、binding CLI | `$TMPDIR/celld-{project}-{version}.log`（进程流） |
| **celld OTEL** | Span / Log（默认关） | `CELLD_OTEL=1` → bucket Parquet 或 OTLP（见 [celld telemetry](../celld/docs/telemetry.md)） |
| **RustFS / S3** | 对象存储 | RustFS 自带 metrics |
| **SQLite registry** | project / version / route | `sqlite3` 只读 |

**还没有（以 AD-14 为准，未实现）：** 查询门面、Gateway `traceparent`、Dashboard 调查页、`dev --profile otel`。

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

本地：`./dev/scripts/up.sh` 后 `curl -s localhost:8790/metrics | head`。

指标名以 `cellp/internal/metrics` 为准：HTTP 计数 / 延迟、job 队列、Gateway upstream。

压测基线：[test-plan-phase2.md](./test-plan-phase2.md) TP2-L4。

---

## 3. 进程日志（Live 的现状）

| 组件 | 位置 |
|------|------|
| cellpd | systemd / docker / stdout |
| celld | 每 version 子进程 stdout/stderr（`manager.go` 落到 `$TMPDIR`） |
| e2e / stress | `docs/evidence/*.log`（gitignore） |

Archive reaper：`orch: archive reaper`。禁用：`CELLP_ARCHIVE_REAPER=0`。

---

## 4. 接到 AD-14

1. 发射补齐入站 span + resource `cellp.project` / `cellp.version`  
2. Gateway 写/传 `traceparent`  
3. 门面 + `CELLP_OTEL_BACKEND=memory`（测试）  
4. 生产 `lgtm-prod`；dev `--profile otel`  

**不要**把 DuckDB 扫 `telemetry/*.parquet` 做成 Dashboard 默认搜。bucket 仅冷备。

---

## 5. 运维检查

```bash
./dev/scripts/health.sh
./e2e/scripts/health-all.sh
```

promote 窗口：Gateway 5xx &lt; 1%（TP2-L5）· [runbooks/rollback.md](./runbooks/rollback.md)。

---

## 6. 与 CF / Vercel 对照

| 能力 | CF / Vercel | cellp |
|------|-------------|--------|
| 实时 tail | `wrangler tail` | 一期：进程文件；AD-14：`logs/stream`（未实现） |
| 检索 / 看板 | Workers Logs | AD-14：门面 + 外置 Tempo/Loki/Grafana |
| Analytics 产品 | 内置 | **不做**（AD-10） |

迁移预期：[cloudflare-migration.md](./cloudflare-migration.md) · [vercel-migration.md](./vercel-migration.md)。
