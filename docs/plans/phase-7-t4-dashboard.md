# Phase 7 T4 — Dashboard Bindings hub

> **Track：** **P7-T4** · 仅 `web/`  
> **TP：** TP-UI-6（保持）· **TP-UI-7..12**（新增）  
> **规格：** [DESIGN.md §8.5](../../DESIGN.md) 路由 · [§8.4](../../DESIGN.md) API  
> **决策：** [AD-6 · AD-7](../decisions.md)  
> **前置：** [phase-7-bindings.md](./phase-7-bindings.md) · **T1 OpenAPI 路径已写入** · T2/T3 operator 合同稳定  
> **对照：** [phase-4-dashboard.md](./phase-4-dashboard.md) — **v1 路由保留**，在 Storage 下加 bindings 页

一期 Dashboard **不做复杂图表**（DESIGN §2.2）。侧栏仍叫 **Storage**。R2 / Cron **只徽章、无浏览器**。

## Exit Criteria

- [ ] 路由与 DESIGN §8.5 一致（下表）；v1 路径不删
- [ ] `StoragePage` 成为 Bindings hub：ready version 行上 **d1 / kv / queue / workflow / r2 / cron** 徽章
- [ ] 新页：`KvPage` · `QueuesPage` · `WorkflowsPage`
- [ ] 现有 D1 browser（`/storage/:vid/browser` · Schema · Data · Query · Branches）**行为与路径不变**
- [ ] AD-7 空起步横幅：Preview 的 KV/Queue **不继承** prod
- [ ] R2 / Cron：总览徽章 + tooltip，**无** `/r2` `/cron` 路由
- [ ] `cellp-api.ts` 仅经 `:8790/v1` 增 KV / Queue / Workflow 函数
- [ ] `web/e2e/mock-api-server.mjs` 覆盖新路径 + 隔离 fixture
- [ ] Playwright **TP-UI-7..12** 全绿；既有 D1 describe 仍绿
- [ ] TP-UI-6：`rg ':8792|offshoot' web/src/ web/e2e/` 无匹配
- [ ] `cd web && npm run test:e2e` · `cd web && npm run build` exit 0

## 现状（开工前必读）

| 文件 | 现状 | T4 处置 |
|------|------|---------|
| `web/src/App.tsx` | v1 路由 + **实验性** `path="bindings"` → `BindingsPage` | §8.5 为准：hub 在 Storage；`/bindings` **重定向到** `/storage` |
| `project-sidebar.tsx` | 多了一项 **Bindings** | **去掉**；侧栏仍三件套 Overview · Deployments · Storage |
| `StoragePage.tsx` | 只列出 **有 D1** 的 ready version | 改为 **全部 ready** + 徽章；D1 仍链到 browser |
| `DatabasePage.tsx` | D1 四 tab | **禁止改交互/路径**（switcher 默认仍 `storageBrowserHref`） |
| `BindingsPage.tsx` + `binding-topology.tsx` | 极坐标拓扑；KV/Queue/WF 节点 **无链** | 可抽徽章元数据；**不得**当主入口；不做图表主路径 |
| `lib/cellp-api.ts` | 已有 `getBindings`（flat `bindings[]` + `crons[]`） | 追加 operator 函数；清单形状跟 T1 OpenAPI |
| `lib/routes.ts` | `bindingsHref` → `/projects/:id/bindings` | 改为 Storage 子路径 helper；旧 href 可 redirect |
| `mock-api-server.mjs` | 仅 `GET …/bindings`；无 queue/workflow；无 KV 写 | 补全 §8.4 fixture |
| `e2e/dashboard.spec.ts` | TP-UI-1..5 + D1 | **保留**；另加 `bindings.spec.ts` |

T4 **不得**在 T1 未把 KV/Queue/Workflow 写进 OpenAPI 前写死未合同路径。清单已存在：`GET /v1/projects/{id}/versions/{vid}/bindings`（Go `BindingsManifest` = `worker` + `bindings[]` + `crons[]`）。DESIGN §8.4 文字写 `d1[]/kv[]/…` 是语义分组；**客户端按 OpenAPI / 现有 Go JSON**（flat `type` 字段）渲染。

## 顺序

```
T1 OpenAPI ──> cellp-api.ts + routes.ts
                 │
                 ├──> StoragePage hub + AD-7 banner + 侧栏回退
                 ├──> KvPage / QueuesPage / WorkflowsPage
                 └──> mock-api-server.mjs
                          └──> Playwright TP-UI-7..12
```

T2/T3 未绿时：UI 可对 501/404 显示空态，但 **mock 必须自洽**，使 `npm run test:e2e` 不依赖真 cellpd。

## 路由（DESIGN §8.5 · v1 保留）

| 路径 | 页面 | 来源 |
|------|------|------|
| `/` | Projects | v1 不动 |
| `/projects/:id` | Overview | v1 不动 |
| `/projects/:id/deployments` | Deployments | v1 不动 |
| `/projects/:id/storage` | **Bindings hub**（徽章总览） | v1 路径，**升级内容** |
| `/projects/:id/storage/:vid/browser` | **现有 D1** Schema · Data · Query · Branches | **路径与页禁止改语义** |
| `/projects/:id/storage/:vid/kv` | `KvPage` | **新增** |
| `/projects/:id/storage/:vid/queues` | `QueuesPage` | **新增** |
| `/projects/:id/storage/:vid/workflows` | `WorkflowsPage` | **新增** |
| `/projects/:id/settings` | Settings | v1 不动 |
| `/projects/:id/versions/:vid` | Version 详情 | v1 不动 |
| `/projects/:id/versions/:vid/database` | → `storage/:vid/browser` | v1 重定向保留 |
| `/projects/:id/bindings` | → `/projects/:id/storage` | **兼容重定向**（消掉实验导航） |

**不做路由：** `/storage/:vid/r2` · `/storage/:vid/cron` · `/storage/:vid/do`。

`routes.ts` 新增（名称可微调，路径必须如上）：

```
storageKvHref(projectId, vid)        → /projects/{id}/storage/{vid}/kv
storageQueuesHref(projectId, vid)    → /projects/{id}/storage/{vid}/queues
storageWorkflowsHref(projectId, vid) → /projects/{id}/storage/{vid}/workflows
```

`bindingsHref`：改为指向 `storageHref`，或保留签名但 query `?version=` 落在 Storage hub（不要再当独立 IA）。

`VersionSwitcher` 已有 `versionHref`。D1 页 **继续默认** `storageBrowserHref`。KV/Queue/WF 页传入对应 href，切换 version **留在同类页**。

## Storage hub（`StoragePage`）

**不再** `filter` 成「仅有 D1 的 version」。列出全部 `status === "ready"`。每行：

| 列 | 内容 |
|----|------|
| Deployment | `version.id` · Production badge |
| Bindings | 按 `getBindings` 计数字徽章：`d1` `kv` `queue` `workflow` `r2` `cron` |
| Actions | 有 d1 → `Open D1` → browser；有 kv/queue/workflow → 链到对应页 |

徽章规则：

| type | 可点？ | 目标 |
|------|--------|------|
| d1 | 是 | `storageBrowserHref` |
| kv | 是 | `storageKvHref` |
| queue | 是 | `storageQueuesHref` |
| workflow | 是 | `storageWorkflowsHref` |
| r2 | **否** | tooltip：无 `celld r2`，无对象浏览器 |
| cron | **否** | 展示表达式（`manifest.crons` 或 `type===cron`）；无「触发一次」 |

无任何绑定：空态「Ready deployments have no wrangler bindings」。不要虚构 `database_name: main`（既有 UX 债）。

Overview / Version 详情上的 **D1** 链仍走 browser；可加「Storage」链到 hub，**不要**再加侧栏 Bindings。

## AD-7 空起步横幅

凡 `version.parent_version_id != null` 的 **KV / Queue** 表面（hub 行 + KvPage + QueuesPage）显示固定文案（中英择一，全站一致）：

> Preview 的 KV / Queue 是空起步，**不会**带上 Production 的 key 或积压。D1 仍走 branch。

Workflow 子 version 实例列表同样为空（只读）；横幅可复用「不继承 prod」。R2 无浏览器，只在徽章 tooltip 提一句隔离。

**禁止** UI 提供 inherit / copy / 「从 prod 同步」按钮。

## 新页面

### `KvPage` — `/storage/:vid/kv`

- `getBindings` / `listKvNamespaces` 选 namespace（`{ns}` = wrangler `kv_namespaces[].id`，verbatim）
- `listKvKeys`（prefix · cursor · limit）
- `getKvKey` 展示 UTF-8 / base64 标记
- `putKvKey` · `deleteKvKey`（确认删除）
- `getKvInfo`（live / bytes / stored，有则显示）
- 404 `version_not_ready` → 与 D1 相同「not ready」空态
- **不做** bulk

### `QueuesPage` — `/storage/:vid/queues`

- 列表来自 bindings `type===queue` 或 `GET …/queues`
- `getQueue` info · `peekQueue`（limit 1–100）
- pause / resume / redrive
- purge：**必须**二次确认 + body `{ force: true }`；无 force 的 400 要可见
- **不做** 手工 attach consumer、pull consumer

### `WorkflowsPage` — `/storage/:vid/workflows`

- `GET …/workflows` + `GET …/workflows/{name}/instances`
- 只读表：id / status / 时间
- 空列表 = 200，不是错误
- **禁止** pause / resume / restart / delete 按钮

D1 仍只在 `DatabasePage`。三新页不要复制 SQL editor。

## `cellp-api.ts`

全部 `request()` → `VITE_CELLP_API_URL`（默认 `http://127.0.0.1:8790`）+ Bearer。**禁止**拼 `:8792`、S3、offshoot。

在现有 `getBindings` 旁追加（路径以 T1 OpenAPI 为准，下表 = DESIGN §8.4）：

| 函数 | 方法 · 路径 |
|------|-------------|
| `listKvNamespaces` | `GET …/kv` |
| `listKvKeys(ns, {prefix,cursor,limit})` | `GET …/kv/{ns}/keys` |
| `getKvKey(ns, key)` | `GET …/kv/{ns}/keys/{key}` |
| `putKvKey(ns, key, body)` | `PUT …/kv/{ns}/keys/{key}` |
| `deleteKvKey(ns, key)` | `DELETE …/kv/{ns}/keys/{key}` |
| `getKvInfo(ns)` | `GET …/kv/{ns}` |
| `listQueues` | `GET …/queues` |
| `getQueue(name)` | `GET …/queues/{name}` |
| `peekQueue(name, limit)` | `GET …/queues/{name}/peek` |
| `pauseQueue` / `resumeQueue` / `redriveQueue` | `POST …/queues/{name}/pause\|resume\|redrive` |
| `purgeQueue(name, {force:true})` | `POST …/queues/{name}/purge` |
| `listWorkflows` | `GET …/workflows` |
| `listWorkflowInstances(name)` | `GET …/workflows/{name}/instances` |

`encodeURIComponent` 所有 id / ns / key / name。PUT CORS：mock 的 `Access-Control-Allow-Methods` 必须含 **PUT**。

## mock-api-server.mjs

沿用现有 `state.projects` · `auth` · `requireReadyVersion`。扩展：

**清单** `bindingsManifest`（`demo-app` 各 ready version）：

- 保留 d1 + kv + r2
- **加上** `queue`（`TASKS` / `tasks` / producer）与 `workflow`（`REPORTS` / `report-builder`）
- `v1`：`crons: ["0 * * * *"]`（已有）；`v2+` 可空 cron，便于徽章有/无对比

**内存 KV（AD-7）：**

| version | ns | 预置 key |
|---------|-----|----------|
| `v1`（prod） | `demo-app-cache` | `greeting=hello-prod` |
| `v2`（parent=`v1`） | 同 ns id | **空** |

PUT/DELETE 写入该 version 的 map；list/get 不得读兄弟 version。

**Queue：** `v1` peek 可返回 1 条 base64 body；`pause` 设标志；`purge` 无 `force` → **400**。`v2` peek 空数组。

**Workflow：** `v1` 可返回 1 条 instance；`v2` `instances: []` 且 **200**。任何 POST pause → **404**（合同外）。

未 ready version → 与 D1 相同 `version_not_ready` 404。

## Playwright（TP-UI-7..12）

新文件 `web/e2e/bindings.spec.ts`（或等价）。**禁止**改坏 `dashboard.spec.ts` 里 D1 用例（TP-UI 既有 + Neon-like）。

| ID | 检查 | 断言要点 |
|----|------|----------|
| **TP-UI-7** | Storage hub 徽章 | `/projects/demo-app/storage` 可见 d1/kv/queue/workflow/r2/cron（v1 有 cron） |
| **TP-UI-8** | KV browser | 打开 v1 `/kv`；见 `greeting`；PUT 新 key 后 list 出现 |
| **TP-UI-9** | Queue 控制台 | v1 `/queues` 见 `tasks`；peek 区渲染；purge 无确认不得静默清空 |
| **TP-UI-10** | Workflow 只读 | v1 `/workflows` 有实例行；**无** Pause/Resume/Restart |
| **TP-UI-11** | AD-7 横幅 | 打开 **v2** KV 或 Queue；横幅可见；v2 KV **没有** `greeting` |
| **TP-UI-12** | R2/Cron 无浏览器 | hub 上 R2/Cron **不是**链到 `/r2` 或 `/cron`；直接 goto 这些路径 404/回 hub |

回归：

| ID | 命令 / 检查 |
|----|-------------|
| **TP-UI-5** | `cd web && npm run test:e2e` |
| **TP-UI-6** | `rg ':8792\|offshoot' web/src/ web/e2e/` 无匹配（文档 `AGENTS.md` 不计入） |
| D1 路径 | 既有 `storageBrowser("v1")` 用例仍绿；legacy `/versions/v1/database` 仍重定向 |

## 验证

```bash
cd web && npm run test:e2e
cd web && npm run build
rg ':8792|offshoot' web/src/ web/e2e/   # 无匹配
```

可选：`cd web && npm run lint`。

## 禁止

- Next.js / SSR / `web/app/`
- 直连 celld `:8792`、S3、offshoot CLI
- 改 `cellp/` Go（T1–T3 的活）
- 改冻结 `D1-*-RPC.md` · `celld/` submodule
- R2 对象 list/get/put UI；Cron「跑一次」
- Workflow 控制动作；见 AD-6 / AD-7
- inherit / copy KV·Queue·R2
- 把 Storage 从侧栏改名为 Bindings（文案：页内可写 Bindings，**nav label = Storage**）
- 以极坐标拓扑替代徽章表作为唯一总览（一期不做复杂图表）

## 验收对照 VALIDATION

Dashboard 绿 **不能**代替 V9–V11 端口脚本（那是 **P7-T5**）。T4 只证明 UI 经 mock 合同消费 API。

## Subagent prompt

```
Track P7-T4 Dashboard Bindings hub. Work only under web/.
Vite SPA only — no Next.js. No :8792, no S3, no offshoot.
Read DESIGN.md §8.4–8.5, web/AGENTS.md, docs/plans/phase-4-dashboard.md,
docs/plans/phase-7-t4-dashboard.md.
v1 routes stay. Storage is the hub (badges). New pages: Kv / Queues / Workflows
under /storage/:vid/*. D1 /storage/:vid/browser UNCHANGED.
Redirect experimental /bindings → /storage; remove Bindings from sidebar.
AD-7 banner on preview KV/Queue. R2/Cron badge only.
Extend cellp-api.ts + mock-api-server.mjs. Playwright TP-UI-7..12.
Verify: cd web && npm run test:e2e && npm run build
TP-UI-6: rg ':8792|offshoot' web/src/ web/e2e/  (no matches)
```
