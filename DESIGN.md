# cellp 设计文档

> **cellp** — 版本化的 Serverless 应用运行时（cell + platform）  
> **唯一设计入口** · 管理维度：**Project + Version**（无用户体系）  
> 决策摘要：[docs/decisions.md](./docs/decisions.md) · 本地 Dev：`dev/scripts/up.sh` · Agent：`AGENTS.md` · `dev/AGENTS.md`

---

## 1. 概述

**cellp** 在每次 CD 时同时 version 化 **App + Data**，通过稳定 Gateway URL 提供完整可访问环境。

**部署约束：100% 私有化。** 不依赖 AWS / Cloudflare / Azure 等任何外部托管云服务；全部组件运行在自有硬件、私有 VPC 或自建 K8s 上。

### 能力分期

| 阶段 | 范围 | 能力 |
|------|------|------|
| **一期** | 核心 Serverless 平台 | **CD 全流程**（外部 CI → artifact → `POST /versions` → preview URL）· **Branch + Version**（App+Data）· **线上稳定**（promote · quiesce · saga · health gate） |
| **二期** | 扩展运行时绑定 | **KV**（Valkey）· **Queues** · **Cron**（平台化）· scale-to-zero 唤醒 · 多节点 cellpd |
| **三期** | 可观测 · 性能/统计 | **OTEL / 日志 / 指标** · 用量与性能统计 · （**暂不计划实施**，仅文档占位） |

| 阶段 | 能力 | 底层 |
|------|------|------|
| **一期** | 部署 App + CD | celld + cellpd |
| **一期** | Branch + Version | offshoot |
| **一期** | Promote / 线上 cutover | cellpd Orchestrator |
| **二期** | KV | Valkey |
| **二期** | Queues | celld / 自研（待定） |
| **二期** | Cron | celld `triggers.crons` + 平台治理 |
| **二期** | Scale to zero | celld cell hibernate + Gateway wake |

**不做：** DNS / CDN / WAF / DDoS / Zero Trust / **Git 托管 / CI 引擎**（外部系统负责；cellp 只收 artifact + API 调用）。

**一期诚实范围：** 单节点（或 VIP 单入口）preview/prod 平面，≤5 concurrent ready versions / project；**不含** KV / Queue / Cron 平台能力。

---

## 2. 总体架构

### 2.1 模块全景

```mermaid
flowchart TB
  subgraph external ["外部（非 cellp 组件）"]
    Dev["开发者"]
    CI["CI/CD<br/>任意 Git + 构建"]
  end

  subgraph cellp ["cellp 控制面"]
    API["API Server<br/>REST · CD webhook"]
    ORCH["Orchestrator<br/>版本状态机 · 补偿"]
    REG["Registry<br/>SQLite"]
    GW["Gateway<br/>cellpd 内置路由"]
    DASH["Dashboard<br/>Project/Version UI"]
  end

  subgraph runtime ["运行时层"]
    CLD["celld<br/>Workers + DO + D1"]
  end

  subgraph data ["数据层"]
    OS["offshoot<br/>SQLite CoW 分支"]
    OBJ["RustFS<br/>artifact · offshoot · celld"]
    KV["Valkey<br/>二期"]
  end

  Dev --> CI
  CI -->|"artifact + POST /versions"| API
  API --> ORCH
  ORCH --> REG
  ORCH --> OS
  ORCH --> CLD
  ORCH --> OBJ
  ORCH --> KV
  ORCH --> GW
  DASH --> API
  Dev -->|"Preview"| GW
  GW --> CLD
  OS -->|"export → D1 seed"| CLD
  CLD --> OBJ
  CLD --> KV
```

### 2.2 确认技术栈（私有化 · 官方依据）

> **部署原则：** 零外部 SaaS。对象存储统一 **自建 RustFS**（S3 API + 条件写）；celld 与 offshoot 均依赖 CAS（`If-Match` / `If-None-Match`）。

#### 统一对象存储：RustFS（自建集群）

| 用途 | bucket / prefix | 依据 |
|------|-----------------|------|
| **offshoot** 分支对象 | `s3://cellp-offshoot/{project}/` | S3 + 条件写；**官方仅 MinIO / AWS S3 shipped-and-tested**；RustFS **未验证**（见下） |
| **CI 制品** | `s3://cellp-artifacts/{project}/{version}/` | 外部 CI 上传 · cellpd 校验 digest |
| **celld Blob** | `s3://cellp-celld/{project}/` | celld S3 路径；**须通过 `celld diagnose` 存储探针** |

**为何选 RustFS 而非 MinIO：** Apache 2.0、Rust 实现、S3 兼容；MinIO CE 在 celld 官方文档中亦未 qualified。两者对 cellp 均 **以运行时探针为准**；RustFS 为 cellp 默认，MinIO 仅作探针失败时的备选。

RustFS 当前 **1.0.0-rc.1**（2026-08），尚未 GA — Phase 0 锁定镜像 tag，升级前重跑双探针。

```bash
# RustFS S3 端点（dev / prod 内网 DNS）
export AWS_ACCESS_KEY_ID=rustfsadmin       # prod：独立 access key
export AWS_SECRET_ACCESS_KEY=...
export AWS_REGION=us-east-1
export S3_ENDPOINT=http://rustfs.internal:9000

export CELLD_BUCKET=s3://cellp-celld/demo-app
export OFFSHOOT_STORE=s3://cellp-offshoot
export OFFSHOOT_S3_ENDPOINT=http://rustfs.internal:9000
export OFFSHOOT_S3_PATH_STYLE=1
```

#### 对象存储准入门槛（RustFS / 任意 S3 后端）

celld [fencing 文档](https://celld.dev/docs/fencing) release 测试在 R2；**MinIO CE 亦标 unqualified**。RustFS 已合并条件写（[PR #409](https://github.com/rustfs/rustfs/pull/409)）；多节点曾有条件写竞态（[#1659](https://github.com/rustfs/rustfs/issues/1659)，后续修复）— **多节点部署须在 Phase 0 做并发探针**。

**cellp 约定 — 双探针，缺一不可：**

| 探针 | 命令 / 行为 | 通过标准 |
|------|-------------|----------|
| **celld** | `celld diagnose --bucket s3://cellp-celld --endpoint "$S3_ENDPOINT"` | `ok bucket conditional write (create, reject-create, update, reject-stale)` |
| **offshoot** | `offshoot init` / 首次 attach store | attach-time CAS probe 不拒绝 |

1. 探针失败 → **不得上线**  
2. Phase 0 **锁定 RustFS 版本**（如 `1.0.0-rc.1`）  
3. 多节点 RustFS：所有 celld/offshoot 客户端指向 **同一 VIP 或单一 endpoint**，避免跨节点条件写竞态（直至集群级原子性经 Phase 0 验证）

celld 官方 qualified 的 R2 / AWS S3 / GCS / Azure / Tigris **不在 cellp 选型范围内**（外部云服务）。

#### offshoot SQLite branch × S3：**attach 探针 ≠ branch 已验证**

offshoot 的 branch（CoW fork · checkpoint · promote · export · GC）在 S3 上**可以**工作，但官方只把下面两个后端标为 **shipped-and-tested**：

| offshoot 后端 | branch / fork 验证 | 说明 |
|---------------|-------------------|------|
| **local directory** | ✅ shipped-and-tested | quickstart 默认 |
| **MinIO** | ✅ shipped-and-tested | CI 每 PR 跑 conformance + `TestS3RealProvider`（含 multipart） |
| **AWS S3** | ✅ shipped-and-tested | 真实 bucket（us-east-1, 2026-08-13） |
| **RustFS** | ⚠️ **未验证** | 条件写已有实现，但 **不在 offshoot CI**；fork 路径依赖 `CopyObject` / `UploadPartCopy` + CAS ref，需单独 spike |
| **Garage 等** | ❌ | 无条件写 → attach 探针直接拒绝 |

S3 上 fork 的两条路径（[offshoot status.md — Reflink/CopyObject fork](https://github.com/sricola/offshoot/blob/main/docs/status.md)）：

1. **共享 fork（默认）** — 写 `base.json` + branch ref（~377B），存储 O(1)  
2. **物化 fork** — S3 `CopyObject`；>5GiB 走 `UploadPartCopy`（仅在 MinIO/AWS 上测过）

**cellp 约定：**

- **attach CAS 探针通过** 只证明「能连上 store」，**不证明** fork/export/promote 在 RustFS 上正确  
- **Phase 0 必做 V0b**：在目标 S3（RustFS）上跑完整 branch 序列 → **[VALIDATION.md](./VALIDATION.md)**
- **V0b 完成前**：prod offshoot 用 **local directory**（单机）或 **MinIO**（若探针通过）；RustFS 仅用于 celld Blob / 制品  
- **V0b 通过后**：offshoot 才迁到 `s3://cellp-offshoot`（RustFS）

#### cellp 自有组件（确认选型）

| 模块 | 确认选型 | 说明 |
|------|----------|------|
| **控制面** | **Go 1.22+**（`cellpd`） | API · Orchestrator · **内置 Gateway** · OpenAPI |
| **Registry** | **SQLite** | **唯一权威**；单文件 `cellp-registry.sqlite`；dev/prod 同选型 |
| **运行时** | **celld** + **offshoot** | celld 源码以 git submodule `celld/` 跟踪（[KonghaYao/celld](https://github.com/KonghaYao/celld)）；集成层仍走 CLI |
| **对象存储** | **RustFS** | celld Blob · offshoot · artifacts（见上） |
| **Dashboard** | **Vite + React SPA** | 一期 · VE 通过后 · 纯静态 `web/dist/` |
| **Dev mock** | Node（`dev/mock-platform`） | 过渡；由 `cellpd` 替换 |

#### 外部边界（**不是 cellp 依赖**）

| 外部 | cellp 接口 | 说明 |
|------|-----------|------|
| **任意 Git + CI** | `POST /v1/projects/{id}/versions` | cellp 不托管 Git、不跑 CI；只消费 artifact |
| **入口 TLS / LB** | 反代到 cellpd Gateway 端口 | Nginx / 云 LB / 自有网关；**不含 Caddy 选型** |

**明确不引入：** Cloudflare R2 · AWS S3 · GCS · Azure 公有云 · 任何第三方托管 PaaS。

#### 技术选型：为何 Go 后端 + Vite SPA 前端

| 层 | 选型 | 理由 |
|----|------|------|
| **后端（P0）** | **Go** `cellpd` | 单二进制部署；与 celld/offshoot **CLI 子进程**集成自然；Orchestrator 长生命周期 + 并发友好；OpenAPI 契约清晰 |
| **前端（M1 后）** | **Vite + React SPA** | 纯静态 `dist/` 可经 cellp 部署；浏览器直连 REST API；无需 Node 运行时 |

**实现顺序（一期硬约束）：**

```
1. Go cellpd（API · SQLite Registry · Orchestrator · 内置 Gateway）  ← P0
2. 端口级 E2E 测试（全后端、无浏览器）                         ← 前端门禁
3. Vite Dashboard（列表 · 部署 · 存储 · Promote/Destroy）      ← M1 后
```

前端 **不得** 先于后端 E2E 开工；一期 Dashboard **不做** 复杂图表、多租户、权限 UI。

### 2.3 模块职责表

| 模块 | 作用 | 技术栈 | 备注 |
|---|---|---|---|
| **API Server** | 接收 CD；Project/Version CRUD；鉴权 | **Go** · chi/echo · OpenAPI | cellp 自研 |
| **Orchestrator** | 驱动 pending→ready 状态机；失败补偿；fork/deploy 编排 | **Go** · 内置 worker pool | cellp 自研 |
| **Registry** | project/version 树、prod 指针、路由表 | **SQLite** | **权威存储**；WAL 模式 |
| **Gateway** | `/{project}/{version}/*` → celld upstream | **Go**（cellpd 内置 reverse proxy） | 非 Caddy；TLS 由外部 LB |
| **Dashboard** | 项目 · 部署 · 存储 · D1 管理 | **Vite + React SPA** | M1 后；仅 API |
| **Branch Manager** | offshoot checkpoint/fork/export/promote | **offshoot** · RustFS / local | V0b 前 offshoot 可 local |
| **Runtime Manager** | celld deploy/start/health | **celld** · RustFS · esbuild | `celld diagnose` 门禁 |
| **Artifact Store** | CI bundle；digest 校验 | **RustFS** `cellp-artifacts` | 外部 CI 写入 |
| **celld State Store** | cell SQLite 复制 | **RustFS** `cellp-celld` | 条件写探针 |
| **offshoot Store** | SQLite CoW 分支对象 | RustFS / **local** | 与 celld 分逻辑卷 |
| **Valkey** | KV prefix 隔离 | **Valkey 8** | **二期** |
| **Dev Harness** | 本地栈 · simulate-cd | RustFS · mock→cellpd | 见 §11 |

### 2.4 CD 时序

```mermaid
sequenceDiagram
  participant Dev as 开发者
  participant CI as 外部 CI
  participant API as cellp API
  participant Orch as Orchestrator
  participant OS as offshoot
  participant OBJ as RustFS
  participant CLD as celld
  participant GW as Gateway

  Dev->>CI: push / trigger build
  CI->>CI: build + upload artifact
  CI->>API: POST /projects/{id}/versions
  API->>Orch: enqueue version job
  API-->>FG: 202 + poll_url

  Orch->>OBJ: fetch artifact + verify digest
  Orch->>OS: drain parent → checkpoint → fork
  OS-->>Orch: data_branch + export.sql
  Orch->>CLD: d1 execute --file seed.sql
  Orch->>CLD: celld deploy bundle
  Orch->>GW: register /{project}/{version} route
  Orch->>CLD: health check /__celld/health
  Orch-->>CI: status=ready, preview_url

  Dev->>GW: GET /{project}/{version}/
  GW->>CLD: proxy
  CLD-->>Dev: response
```

### 2.5 版本状态机

```mermaid
stateDiagram-v2
  [*] --> pending: POST /versions
  pending --> fetching: 拉 artifact
  fetching --> branching: offshoot fork
  branching --> preparing: export + env
  preparing --> deploying: celld deploy
  deploying --> ready: health OK
  ready --> draining: DELETE / TTL
  draining --> destroyed: celld SIGTERM
  destroyed --> [*]

  fetching --> failed: 错误
  branching --> failed: 错误
  preparing --> failed: 错误
  deploying --> failed: 错误
  failed --> pending: retry
```

### 2.6 数据流（App + Data 同版本）

```mermaid
flowchart LR
  subgraph version ["一次 Version"]
    ART["Artifact<br/>wrangler bundle"]
    BR["offshoot branch<br/>checkpoint 快照"]
  end

  ART --> CLD["celld deploy<br/>Workers/DO"]
  BR --> EXP["offshoot export"]
  EXP --> SEED["celld d1 execute"]
  SEED --> D1["D1 / DO SQLite<br/>运行时真相源"]
  CLD --> D1

  D1 --> OBJ["RustFS celld cells/"]
  BR --> OSTORE["offshoot store"]
```

---

## 3. 核心技术难点

cellp 的护城河不在「包一层 celld」，而在下面 **6 项必须自研攻克的集成难题**：

| # | 难点 | 为什么难 | 攻克方向 | 阶段 |
|---|---|---|---|---|
| **T1** | **offshoot → celld D1 数据种子管道** | celld Worker 无法读 host 文件；offshoot 与 celld 各有一套 SQLite 存储，无官方集成 | export SQL → `celld d1 execute`；约定 Worker 只用 D1 binding；禁止 DATABASE_PATH | **一期** |
| **T2** | **Quiesce + checkpoint 一致性 fork** | fork 时 parent 仍在写入 → 子版本继承 stale 数据 | Orchestrator：drain active route → offshoot checkpoint → 再 fork；非 live head fork | **一期** |
| **T3** | **单 celld fleet 多 Version 路由** | celld 限制 1 fleet = 1 deploy；不能每 version 起一个 celld | Gateway 路径路由 + 单 active deploy per project；版本 metadata 与路由解耦 | **一期** |
| **T4** | **Promote 原子 cutover** | 只改 prod 指针会双 prod 并存、双写分叉 | CAS prod_version + 停旧 Gateway 路由 + drain 旧 deploy + offshoot promote； saga 补偿 | **一期** |
| **T5** | **Orchestrator saga / 补偿** | branching 成功 + deploy 失败 → 孤儿 branch、泄漏路由 | 每步可逆操作；failed 自动 release 路由；offshoot branch GC；idempotent retry | **一期** |
| **T6** | **Schema migration × fork 顺序** | 新代码 + 旧 schema fork 或迁移破坏 fork 数据 | 显式 migrate 步骤：fork → migrate on preview → health gate；版本化 migration 记录 | **一期** |

**依赖风险（非自研但阻塞）：**

| 风险 | 影响 | 缓解 |
|---|---|---|
| celld alpha | API/行为变更 | pin 版本；封装 Runtime Manager；跟进 cloudflare-compat |
| offshoot branch × RustFS S3 未验证 | fork/export/promote 可能在 RustFS 上静默失败 | **[V0b](./VALIDATION.md#v0b--offshoot-sqlite-branch--rustfs-s3全序列)**；完成前 offshoot 不走 RustFS |
| offshoot pre-1.0 | 存储 layout 可能变 | preview-only；pin SHA；不用于 prod 数据 |
| RustFS 条件写探针失败 | celld 双主 / fleet 无法启动 | 部署前 `celld diagnose`；Phase 0 锁定 RustFS 版本；探针失败则不上线 |

**二期再攻（一期明确不做）：** KV · Queues · Cron 平台化 · Gateway wake · 多节点 Orchestrator 队列 · offshoot↔celld 运行时双向 sync。

**三期（暂不计划，仅文档）：** OTEL / 指标 / 集中日志 · 性能与用量统计 · Dashboard 图表。

---

## 4. 核心模型

管理维度仅 **Project + Version**：

```mermaid
erDiagram
  PROJECT ||--o{ VERSION : has
  PROJECT {
    string id
    string git_remote
    string prod_version_id
  }
  VERSION {
    string id
    string parent_version_id
    string git_sha
    string data_branch
    string preview_url
    string status
  }
```

| Project 字段 | Version 字段 |
|---|---|
| `id` · `git_remote`（可选元数据）· `prod_version_id` | `id` · `parent_version_id` · `git_ref/sha` |
| | `artifact_uri/digest` · `data_branch` |
| | `preview_url` · `status` · `ttl` |
| | `kv_prefix`（**二期**；一期 Version 模型预留字段可不填） |

---

## 5. 存储布局（自建 RustFS）

```
# 单一 RustFS 集群，三个 bucket（prod/dev 同构）

s3://cellp-artifacts/           # 外部 CI 上传 wrangler bundle
└── {project}/{version}/

s3://cellp-offshoot/            # offshoot SQLite CoW 对象
└── {project}/branches/

s3://cellp-celld/               # celld fleet Blob（cells/ deploy/ nodes/ …）
└── {project}/                  # CELLD_BUCKET=s3://cellp-celld/{project}

# 控制面元数据 — SQLite（权威，非 PostgreSQL）
/var/lib/cellp/cellp-registry.sqlite    # prod
./dev/data/cellp-registry.sqlite        # dev
```

| 环境 | celld Blob | offshoot | artifact |
|---|---|---|---|
| **prod** | **RustFS** `cellp-celld` | **RustFS** `cellp-offshoot` | **RustFS** `cellp-artifacts` |
| **dev** | **RustFS**（compose `:9000`） | **local dir**（默认；S3 branch 待 T0b） | RustFS 或 `./dev/data/artifacts/` |

---

## 6. CD 用户故事

```
外部 CI → POST cellp /versions
  → artifact → offshoot checkpoint/fork/export
  → celld d1 seed → celld deploy → Gateway 路由
  → preview_url: https://cellp/{project}/{version}/
```

**Promote：** prod 指针 + offshoot promote + **cutover**（停旧路由）+ CAS。

**PR preview：** parent ≠ prod；fork from seed/scrubbed branch。

---

## 7. offshoot ↔ celld 集成

```
offshoot: drain → checkpoint → fork → export
celld:    d1 execute --file seed.sql → deploy → Worker 用 D1/DO
```

**禁止：** `DATABASE_PATH`（celld 无 host FS binding）。

---

## 8. KV / Queue / Cron（二期）

一期 **不提供** 平台级 KV、Queue、Cron 绑定与治理；Worker 若使用 celld 原生 **D1 / DO / `triggers.crons`**，由 celld 自身承载，cellp 不额外编排。

### KV（Valkey）— 二期

Key：`{project}:{version}:{key}` · prod CD 默认 `inherit_kv: true` · Valkey ACL 按前缀。

### Queues — 二期

celld roadmap 有 DO 形态 Queues；cellp 一期不接 CD 路径。

### Cron — 二期

celld 已支持 `triggers.crons`；二期再做平台侧注册、多 version 隔离与可观测。

---

## 9. API 规范

Base: `https://cellp.internal/v1`  
Token：**DEPLOY_TOKEN**（POST versions）· **ADMIN_TOKEN**（promote/destroy）

| 方法 | 路径 | 作用 |
|---|---|---|
| POST | `/projects/{id}/versions` | CD 入口（202） |
| GET | `/projects/{id}/versions/{vid}` | 轮询 status |
| POST | `.../versions/{vid}/promote` | cutover prod |
| DELETE | `.../versions/{vid}` | destroy |

artifact URL **服务端构造**（防 SSRF）。status：`pending` → … → `ready` | `failed`。

---

## 10. Dashboard（一期）

**启动条件：** [docs/test-plan.md](./docs/test-plan.md) **M1（TP-VE-ALL）** 通过后。

**Vite + React SPA**（`web/`），浏览器直连 cellpd REST API，输出纯静态 `dist/`：

| 页面 | 内容 |
|------|------|
| `/` | Project 列表 |
| `/projects/{id}` | 项目概览 |
| `/projects/{id}/deployments` | Version 列表 + status |
| `/projects/{id}/storage/{vid}/browser` | D1 Schema · Data · Query · Branches |
| `/projects/{id}/versions/{vid}` | Promote · Destroy |

**一期不做：** 实时图表 · 用量统计 · 复杂 SSE 面板（轮询即可）。

可观测与性能统计 → **三期**（暂不实施）。

---

## 11. 本地 Dev（Agent 闭环）

```bash
cp dev/.env.example dev/.env
./dev/scripts/up.sh
./dev/scripts/simulate-cd.sh demo-app v-dev1
./dev/scripts/health.sh
curl http://127.0.0.1:8787/demo-app/v-dev1/
```

| 端口 | 模块 |
|---|---|
| 8787 | cellpd Gateway（内置） |
| 8790 | cellpd API |
| 8792 | celld |
| 6379 | Valkey（**二期**；dev 可启） |
| 9000 | RustFS S3 |
| 9001 | RustFS Console |

详见 `dev/AGENTS.md` · `DESIGN.md` 本节。

---

## 12. 安全要点

- celld bind 127.0.0.1；Gateway 对外
- DEPLOY / ADMIN token 拆分
- PR 禁止 fork prod 数据
- CD env 不可覆盖平台托管键

---

## 13. 代码目录

```
cellp/                          # 控制面 Go（一期 P0）
├── cmd/cellpd/
└── internal/{api,orch,registry,gateway,runtime,branch}/

e2e/                            # 端口级 E2E（后端完成后、前端前）
├── scripts/                    # curl / testscript 打 :8787 :8790 :8792
└── README.md

dev/                            # 本地 harness（已有）
├── docker-compose.yml
├── mock-platform/              # → 由 cellpd 替换
└── scripts/

web/                            # Dashboard（Vite SPA · web/src/）
```

---

## 14. 分期路线图

### 一期 — CD + Branch + 线上稳定

**交付：** 外部 CI → 全自动 CD → preview/prod URL；App+Data branch；promote cutover。

| 顺序 | 模块 | 交付物 |
|------|------|--------|
| **P0** | **后端** | `cellpd`（Go）· **SQLite Registry** · Orchestrator · **内置 Gateway** |
| **P0** | **运行时集成** | celld deploy · offshoot fork/export · D1 seed |
| **P0** | **验证** | [docs/test-plan.md](./docs/test-plan.md) TP-V0–V7 + **TP-VE-ALL（M1）** |
| **P1** | **前端** | Vite Dashboard（**M1 后**） |

| 验证 | 范围 |
|------|------|
| 存储/集成 | V0a–V0d · V1–V7 |
| 后端门禁 | **VE** — 各端口 HTTP 全链路，无 UI |

**一期不做：** Valkey · Queues · Cron 平台化 · scale-to-zero · 多节点 · **复杂 Dashboard** · **PostgreSQL Registry** · **多租户/RBAC**。

### Phase 6 扩展基线（SQLite 止血 · 2026-08-29）

> 全量路线图：[docs/plans/phase-6-scale-10m-master.md](./docs/plans/phase-6-scale-10m-master.md)  
> 验收证据：[docs/evidence/scale-report-6A.md](./docs/evidence/scale-report-6A.md)

| 能力 | 一期 + 6A 后 | 千万级需 |
|------|-------------|----------|
| Registry | SQLite + 游标分页 | PostgreSQL（**明确不做**） |
| Project 规模 | ~10k 实测，List p99 ~260ms | 100万需 PG |
| Gateway | 路由进程内缓存 | Valkey + 水平扩展（二期+） |
| 租户 | 单 token | Org/RBAC（**明确不做**） |

**诚实结论：** 在 **SQLite + 单 token** 约束下，6A 分页/GC/缓存已落地；10k project 列表 p99 无法稳定 <200ms，此为存储引擎上限而非实现 bug。

### 二期 — KV / Queue / Cron / 弹性

| 模块 | 交付物 |
|------|--------|
| KV | Valkey 绑定 · prefix · `inherit_kv` |
| Queues | CD 路径接入（方案待定） |
| Cron | 平台注册 + 多 version 策略 |
| 弹性 | Gateway wake · 多节点 cellpd |

**验证：** [VALIDATION.md](./VALIDATION.md) V8–V12

### 三期 — 可观测 · 性能/统计（暂不计划）

> 仅文档占位，**无排期、无实现**；避免与一期/二期 scope 混淆。

| 方向 | 可能包含 |
|------|----------|
| 可观测 | OTEL · Prometheus · 集中日志 · 告警 |
| 性能/统计 | 部署耗时 · version 资源用量 · cell 激活统计 · Dashboard 图表 |

**验证占位：** VALIDATION.md V20+（待三期立项再细化）

- [x] dev harness
- [ ] **P0** Go `cellpd` 替换 mock
- [ ] **P0** VALIDATION V0–V7 + **VE**
- [ ] **P1** 极简 Dashboard（VE 后）

---

## 附录 A — 外部 CI 调用示例

cellp **不包含** Git 托管或 CI 引擎。任意 CI 在构建完成后：

1. 将 artifact 写入 `s3://cellp-artifacts/{project}/{version}/`
2. `POST $CELLP_URL/v1/projects/$PROJECT_ID/versions`（Bearer `DEPLOY_TOKEN`）

参考 workflow：`dev/examples/ci-deploy.example.yml`（Forgejo Actions 仅为示例格式，非依赖）。

---

*cellp · 私有化部署 · Project + Version · RustFS 统一对象层 · 2026-08-27*
