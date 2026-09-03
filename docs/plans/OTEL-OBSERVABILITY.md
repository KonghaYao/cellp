# cellp 可观测性（权威设计）

> **状态：** 架构冻结（AD-14）· **实现未排期**  
> **决策摘要：** [decisions.md §19 AD-14](../decisions.md#19-ad-14--otel-发射查询门面可换后端)  
> **顶层入口：** [DESIGN.md](../../DESIGN.md) §2.1 / §2.3  
> **celld 发射：** [celld/docs/telemetry.md](../../celld/docs/telemetry.md)（sink / 采样 / shed；**不是**本文件的检索面）  
> **一期运维（Prometheus / 进程日志）：** [observability.md](../observability.md)  
> **最后更新：** 2026-09-03

本文是 cellp **唯一**可观测架构。实现、OpenAPI、Dashboard、`dev/` profile、issue 与本文冲突时以本文为准。修改须对抗审查（与 D1 RPC 同级纪律）。

---

## 0. 一句话

**OTEL 是写入契约；cellp 查询门面是读取契约；检索引擎可换；分析产品不进 cellp。**

```
接口 = Span · console = Log · 一次调用 = 同一 trace_id
Gateway → celld 用 traceparent 贯穿
查询只按 Project + Version 切
```

---

## 1. 问题与否决

| 否决 | 原因 |
|------|------|
| 自研搜索 / Query Builder / 内嵌 ClickHouse | AD-10：不做 SaaS Analytics |
| Dashboard 任意 SQL / TraceQL / 直连 S3·celld | `web/AGENTS.md`；绑定后端方言 |
| DuckDB 扫 `telemetry/*.parquet` 当默认检索 | 小文件、全文、高基数会崩；无倒排 |
| `node_log` / `log_tier` 当可观测 | 那是 fleet LTX 耐久，不是 Worker 日志 |
| 账号级全局检索、按 Host 混 version | AD-10 无账号；AD-12 Host 在 promote 后切 upstream |
| OTEL「查询协议」当换仓依据 | **不存在**稳定的 OTEL query 标准；**OTLP 只 ingest** |

**保留：** 一期 Prometheus `:8790/metrics` 与进程 stdout（与本设计并行，不替代）。

---

## 2. 两层协议

| 层 | 标准 | 锁定 | 不锁定 |
|----|------|------|--------|
| **Emit** | OTLP/HTTP `traces` + `logs` | 字段、resource、`traceparent`、采样/shed 纪律 | 后端品牌 |
| **Query** | **cellp Query API**（本文 §5） | JSON 模板 + `project`+`version` 必选 | Tempo / Loki / Jaeger / memory 实现 |

Dashboard / CLI **只**打 cellpd `:8790`。引擎方言（TraceQL / LogQL / Jaeger HTTP）不得出现在 `web/`。

---

## 3. 全生命周期（一条 trace）

```
[gw]     ingress          Host → version · method · status · duration
           │  traceparent（生成或透传）
[celld]  celld.fetch      入站 Worker（必须：url · http.method · http.status_code）
           ├─ console.*   Log（同一 trace_id / span_id）
           ├─ fetch       出站 CLIENT
           ├─ celld.queue / cell_* / alarm / websocket
           └─ exception
```

promote：Host 不变，**新请求换 `cellp.version` 标签**；旧树按旧 version 查。  
archive：live stream **410**；引擎内数据跟其后端 retention，不跟 `$TMPDIR`。  
destroyed：门面 404；后端删除属运维 retention，不是 Registry 事务。

---

## 4. 发射契约（不可随后端更换）

### 4.1 Resource（每条信号必带）

| Key | 值 |
|-----|-----|
| `service.name` | `celld` 或 `cellp-gateway` |
| `cellp.project` | project id |
| `cellp.version` | version id（**不是** `CARGO_PKG_VERSION`） |
| `celld.node` | celld node 字符串（若有） |
| `deployment.environment` | `preview` \| `prod` |

Loki / 等价日志引擎：**只**把上表当索引标签。`url`、`trace_id`、`body` **禁止**当标签。

### 4.2 入站 Span（`celld.fetch` + Gateway `ingress`）

至少：`url` · `http.method` · `http.status_code` · `duration` · `trace_id` / `span_id` / `parent_span_id`。  
今日 celld 入站 span **缺** url/status —— **实现本设计前必须补列**（否则门面「按路径 / 5xx」无法成立）。

### 4.3 Log

`time` · `body`（沿用 celld `cap_body`）· `trace_id` · `span_id` · 宜有 `severity`。

### 4.4 热路径

未采样请求 **零事件**。队列满 **shed + 计数**，不堵 Worker。shed 必须能被运维看见（metrics 或门面 `context`）。  
采样在 **celld 请求入口**拍板；Gateway 不二次采样成两套真相。

### 4.5 开关

version 进程由 cellpd 注入，例如：

```
CELLD_OTEL=1
CELLD_OTEL_SINK=otlp          # 热路径默认；bucket 仅冷备
OTEL_EXPORTER_OTLP_ENDPOINT=…
```

`CELLD_OTEL=0`（默认）：门面 `context.enabled=false`；live tail 仍可存在。

---

## 5. 查询门面（冻结形状）

鉴权：**仅 `ADMIN_TOKEN`**。`DEPLOY_TOKEN` **禁止**读观测。  
路径均在 `/v1/projects/{project}/versions/{version}/telemetry/`。  
`{version}` 由 Registry 解析；client **不得**传 store URL / TraceQL / bucket glob。

| 方法 | 语义 | 一切后端必须实现 |
|------|------|------------------|
| `GET context` | `enabled` · `backend` · `flush_hint` · `shed` · `deep_link?` | 是 |
| `GET traces/{trace_id}` | 一棵树 + 同 id 的 logs | 是 |
| `POST search` | **模板** + 时间窗 + `limit` | 是 |
| `GET logs/stream` | live tail（**进程流**，非 OTEL） | 独立通道；archived → 410 |

### 5.1 `search` 模板（仅此集合）

`slow` · `error` · `status` · `body`（窄窗 + 扫描上限）· `request_id`（有则）。

强制：时间窗、`limit`（默认 1000，硬顶 10000）。  
**拒绝：** 任意 SQL、任意 TraceQL/LogQL、无时间窗、跨 project、跨 version（对比模式若未来加，须另开配额与决策修订）。

### 5.2 驱动接口（逻辑，非 Go API 冻结）

```
QueryBackend {
  Context(project, version) Context
  GetTrace(project, version, traceID) Tree
  Search(project, version, Template) []Hit
  DeepLink(project, version, view) string   // 可空
}
```

e2e **只**打门面，不打 Tempo/Loki。

---

## 6. 后端档位（`CELLP_OTEL_BACKEND`）

同一套 OTLP 发射；换仓只换驱动 + collector exporter。

| 值 | 组成 | 重量 | 用途 |
|----|------|------|------|
| `none` | 不 ingest | 0 | **默认** |
| `memory` | cellpd 进程内 ring（时间 / 条数双帽） | 极轻 | 单测 · `cellp dev` |
| `otlp-file` | file exporter 或 version bucket `telemetry/` | 轻 | fixture / 冷备；**不是**默认检索 |
| `jaeger` | Jaeger all-in-one（OTLP in） | 一容器 | 集成测 **trace**；log 可仍走 `memory` |
| `lgtm` | [`grafana/otel-lgtm`](https://github.com/grafana/docker-otel-lgtm) 单容器 | 中 | 本地「像生产」；**官方仅 dev/test** |
| `lgtm-prod` | Collector + Tempo + Loki + Grafana **分进程** | 重 | 生产推荐 |

生产检索：**Tempo（TraceQL）+ Loki（LogQL）+ Grafana（看板）**。  
不把 ClickHouse / SigNoz 列为必选。DuckDB **只**作为 `otlp-file` 的一种读法。

`dev/scripts/up.sh`：默认 `core` **不含**观测容器；`--profile otel` 拉 `lgtm` 或 `jaeger`。

---

## 7. 三档体验（禁止合成一个「Logs」）

| 档 | 机制 | 延迟（承诺上限） | UI |
|----|------|------------------|-----|
| **Live** | owner 节点进程流 / SSE | ~1s | Dashboard / `cellp tail` |
| **Investigate** | 门面 → 当前 backend | memory：秒内；LGTM：约 1–5s | Trace 树 + 模板搜 |
| **Analyze** | Grafana 深链（`context.deep_link`） | 看板 5–15s | **iframe / 外链**；cellp 不托管查询语言 |

非 owner cellpd：live **反代或 302 到持有该 version 进程的节点**（与 route/lease 同寻址）。  
Investigate / Analyze **不**跟进程走，跟标签 / 引擎走。

---

## 8. 分布式不变量

- 每个 celld **只推 OTLP**（或关）；不写邻居盘；不写 Registry 正文。
- 一座观测集群，**标签切 version**（与 AD-1「一 version 一 bucket」是不同层）。
- 多副本 fan-in 在 Collector / 引擎内完成；cellpd **不做**全局时间排序。
- 查询入参禁止 `s3://*` / 无 version 的通配。
- 热路径禁止同步 compaction、禁止同步超大 `PutObject`。
- Collector 可做尾采样（错/慢全留）；与 celld head 采样叠加时须在 `context` 声明「不完整」。

---

## 9. Dashboard / CLI

| 允许 | 禁止 |
|------|------|
| Live 面板；`GET traces/{id}` 表+缩进树；模板搜；可选 Grafana iframe | SQL 编辑器、保存查询社交化、火焰图（v0）、直连 `:8792` / Tempo / Loki / S3 |
| `cellp tail <project> [--version vN\|prod]` | 宣称 `wrangler tail` 协议兼容 |

v0 实现顺序：门面 + `memory` + live → 发射补列 → `lgtm` profile → Dashboard 调查页。  
**先 API 契约，后 UI。** 无门面不得做 Query Builder。

---

## 10. 超大体量

写路径：`cap_body` · shed · 采样。  
引擎路径：后端 retention / compaction（Tempo/Loki 自己的）。  
`otlp-file` / bucket：单文件 ≤ 默认 5MB；小时压实在 **maintenance** 节点，不在 celld/cellpd。  
门面：超扫描 → `413` / `429`（`scan_too_large`），不把 DuckDB/S3 错误回给浏览器。

---

## 11. 与 AD-* 对齐

| AD | 关系 |
|----|------|
| **AD-1** | 发射点 = 该 version 的 celld；live 黏进程；OTLP 不破坏一进程一 bucket |
| **AD-9** | archived：无 live；history 在引擎 retention 内 |
| **AD-10** | 引擎外置；无账号级 Analytics；无 RBAC（ADMIN 即全观测读） |
| **AD-12** | Gateway 根 span + Host→version；搜 version 不搜 Host |
| **AD-13** | 正交（框架选择不影响发射） |

---

## 12. 分期（实现，不改契约）

| 步 | 交付 | 刻意不做 |
|----|------|----------|
| **S0** | 本文 + AD-14 + OpenAPI 草稿 | 容器、UI |
| **S1** | 门面 + `memory` + live SSE；Gateway `traceparent` | 检索引擎 |
| **S2** | celld 入站列补齐 + `CELLD_OTEL_SINK=otlp` 注入 | Dashboard 调查页 |
| **S3** | `dev --profile otel`（`lgtm` 或 `jaeger`）+ 门面驱动 | 默认 `run-all.sh` 拉 LGTM |
| **S4** | Dashboard Live + 调查页 + 可选深链 | SQL、跨 version、告警产品 |

默认 `e2e/scripts/run-all.sh` **不**依赖观测容器。otel 契约测 opt-in。

---

## 13. 否定清单（issue 不得静默越界）

1. cellp 二进制内嵌分析引擎或列存  
2. 托管 Tempo/Loki/CH 为 **必选** 运行时依赖（与 RustFS/celld 同级）  
3. Dashboard 直连引擎或 S3  
4. `DEPLOY_TOKEN` 读 telemetry  
5. 无时间窗 / 无 limit 的全集群搜  
6. 用 live tail 冒充 history，或用 DuckDB 冒充 live  
7. 承诺 schema 在 celld `v0-unstable` 补列完成前对 UI 稳定  
8. 把 `service.version`（celld crate）当成 cellp version
