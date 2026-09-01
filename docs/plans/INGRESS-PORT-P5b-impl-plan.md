# Ingress Port P5b 实现计划（Orchestrator + 项目配置）

> **状态：** 实施计划 · 只读产出（本文件不写产品代码）  
> **权威设计：** [INGRESS-PORT-DEPLOYMENT.md](./INGRESS-PORT-DEPLOYMENT.md) §4–§6、§8–§9  
> **前置：** [INGRESS-PORT-P5-impl-plan.md](./INGRESS-PORT-P5-impl-plan.md) P5a（`port_allocations` + `AttachIngressListenPort` / `DetachIngressListenPort` 已落地）  
> **代码入口：** `cellp/internal/orch/ingress.go` · `cellp/internal/registry/port_ledger.go` · `cellp/internal/config/ingress.go`

---

## 1. 目标与边界

### 1.1 P5b 要交付什么

在 **不改变 Host 默认行为** 的前提下，把 Tier B **Port 模式** 的编排闭环接到 registry 台账（P5a），使：

| 能力 | 说明 |
|------|------|
| 项目级 tier / 稳定 prod 口 | `projects.ingress_tier_b`、`projects.prod_listen_port` 迁移与解析 |
| 全局 tier 配置 | `CELLP_INGRESS_TIER_B` 校验与 **effective tier**（项目 NULL → 继承全局） |
| Ready / Archive / Promote | 按 tier 调用 **Attach(ephemeral|stable)** / **Detach** / **Reserve**；promote **不改** prod `listen_port`（**R-PROD-PORT-STABLE**） |
| 对外 URL | `preview_url` / API `prod_url` 在 listen 模式下含 `http://127.0.0.1:{port}/` |
| Ready 失败 | 回滚本次 **ephemeral** 台账与 preview binding；**不**释放 stable prod 口 |
| 可测性 | 单元/集成测试 + **可选** e2e 脚本或 env 说明（HTTP 200 依赖 P5c 时可 defer） |

### 1.2 明确不在 P5b（defer）

| 阶段 | 内容 | 原因 |
|------|------|------|
| **P5c** | Gateway `ReconcileListeners`（`Listen 127.0.0.1:port`）、§5.5 orphan 清理、`prod_port` 混合模式完整 e2e | 设计 §9 分期；无 listener 时本机 `curl 127.0.0.1:port` 可能失败 |
| **P5d** | OpenAPI `PATCH` project、`prod_host` 列、Dashboard `format.ts` | 设计 §7 / §6 |
| **P5e** | `celld_upstream` 写入同一台账 | 设计 §3.3 / §9；P5b 仅 **文档约束** ready 仍用现有 `SetRoute` + runtime 内存口 |

**P5b 仍应：** 在 registry/编排层写对 `ingress_listen` 台账；Gateway 解析 `LookupIngressByListenPort` 已存在，**listener 绑定**留给 P5c。

---

## 2. 现状对照（P5a 之后）

| 组件 | 现状 | P5b 缺口 |
|------|------|----------|
| `projects` | 无 `ingress_tier_b` / `prod_listen_port` | 迁移 + `Project` 字段 + `GetProject`/`CreateProject`/`UpdateProject`（最小：Create 可选字段；Update 可仅 registry 层供 orch/API 后续用） |
| `config.IngressConfig` | 读 `CELLP_INGRESS_TIER_B` 字符串，**无**枚举校验 | `ValidateTierB()`；`EffectiveIngressTierB(global, projectOverride)` |
| `orch/ingress.go` | 仅 Host：`UpsertIngressBinding`，`FormatPreviewURL(host, nil)` | tier 分支 + `AttachIngressListenPort` / `DetachIngressListenPort` |
| `runDeploy` | `ensurePreviewIngress` 在 deploy 前；verify 仅 `VerifyGatewayRouteHost(previewHost)` | port 模式用 **preview_url authority**；Attach 时机对齐设计 §5.2 |
| `Archive` | `setPreviewIngressActive(false)`，**无** Detach | ephemeral → `DetachIngressListenPort` |
| `compensateDeploy` | route/offshoot/stop，**无** ingress 回滚 | Detach preview ephemeral |
| `Promote` | `ensureProdIngress` 仅 Host Upsert | prod port 模式 **禁止**改 `listen_port`；仅 Host/upstream 语义 |
| `ProdURL` | 始终 Host @ Gateway | 读 prod binding `listen_port` 或 `prod_listen_port` → `127.0.0.1:port` |
| Registry | `ReserveStablePort`、`ProdPortReserveOwnerID` | stable 预占 → `{project}-prod` binding **owner 迁移**（见 §4.4） |

---

## 3. 部署模式与 effective tier（对齐设计 §2、§4）

解析函数（建议 `config` 或 `orch` 包内纯函数 + 单测）：

```
effective = project.ingress_tier_b ?? global CELLP_INGRESS_TIER_B  // trim, lower
```

| effective | Preview ready | Prod binding |
|-----------|---------------|--------------|
| `host` | Host binding，无 `listen_port` | Host binding，无 `listen_port` |
| `dedicated_port` | **Attach ephemeral** + 默认 **host+port 双写**（Dashboard 仍可见 Host） | **Attach stable**（无 `prod_listen_port` 时池内 allocate stable）+ `listen_port` |
| `prod_port` | **仅 Host**（与 host 模式相同） | **Attach stable** 或 adopt `prod_listen_port` 预占 |
| `external_map` | P5b **不**写 listener 台账（与 host 同或 no-op port）；文档注明外层映射 | 同左 |

**配置校验（P5b）：** `CELLP_INGRESS_TIER_B` 与项目 override 仅允许：`host` | `dedicated_port` | `prod_port` | `external_map`；非法值 → cellpd 启动失败或降级策略（**建议启动 fail-fast**，与 Gateway `IngressTierB` 一致）。

---

## 4. P5b 范围（详细）

### 4.1 Schema：`projects` 迁移

在 `sqlite.go` 迁移链增加（对齐 [INGRESS-PORT-DEPLOYMENT §4.1](./INGRESS-PORT-DEPLOYMENT.md#41-projects-扩展列)）：

```sql
ALTER TABLE projects ADD COLUMN ingress_tier_b TEXT;        -- NULL | host | dedicated_port | prod_port | external_map
ALTER TABLE projects ADD COLUMN prod_listen_port INTEGER;   -- optional stable prod
```

| 列 | 行为 |
|----|------|
| `ingress_tier_b` | NULL = 继承全局 `CELLP_INGRESS_TIER_B` |
| `prod_listen_port` | 须在 `[INGRESS_PORT_MIN, INGRESS_PORT_MAX]`；设置时 **ReserveStablePort**（`owner_id = ProdPortReserveOwnerID(project)`）；冲突 → **R-STABLE-RESERVE** / 409（API 层 P5d，P5b 至少在 registry Update 返回 `ErrPortConflict`） |

**P5b 最小 API 面：** 扩展 `CreateProjectInput`（可选 tier + port）；若暂无 HTTP PATCH，orch 测试通过 store 直接写列或 `ExecTestSQL`。

**不在 P5b 必须：** `prod_host` 列（设计 §4.1 标注 P5 落地，可与 P5d 一并）。

### 4.2 Config：`CELLP_INGRESS_TIER_B`

文件：`cellp/internal/config/ingress.go`（与 `cellp/internal/gateway/config.go` 枚举 **保持一致**）。

- 常量或 `var ValidIngressTierB = [...]`
- `func (IngressConfig) TierBOrDefault() string`
- `func ValidateIngressTierB(s string) error`
- `func EffectiveIngressTierB(global, project *string) string`

池边界已存在：`IngressPortMin/Max`（env `INGRESS_PORT_MIN/MAX`）；orch Attach 时 `GatewayID` 使用 **`CELLPD_INSTANCE_ID`**（与 gateway `GatewayID` 同源）。

### 4.3 Registry：项目字段 + stable 预占 → prod binding

| 任务 | 说明 |
|------|------|
| `Project` 结构体 + Scan/Insert/Update | `IngressTierB *string`, `ProdListenPort *int` |
| `CreateProject` | 若 `prod_listen_port` 非空 → 校验池 → `ReserveStablePort{ OwnerID: ProdPortReserveOwnerID(id), Stability: stable, ... }` |
| `UpdateProject`（或专用方法） | 改 `prod_listen_port`：P5b **建议**「未实现 admin 迁移则禁止改口」（设计 §4.2）；仅 Create 时指定 |
| **Adopt stable for prod binding** | 新增 registry 事务辅助（名称待定，如 `AdoptStableIngressPortForBinding`）：将 `owner_id` 从 `{project}-prod-reserve` **UPDATE** 为 `{project}-prod`（**推荐** 单 port 同行迁移，避免 release 窗口）；若无 reserve 且 binding 尚无口 → `AttachIngressListenPort` + `Stability: stable` 或 `ReserveStablePort` + upsert binding |

**禁止：** promote / rollback 路径调用 `ReleasePort` 作用于 prod binding 的 **stable** 台账。

### 4.4 Orchestrator：`ingress.go` 重构

建议新增（保持 `previewBindingID` / `prodBindingID` 不变）：

| 函数 | 职责 |
|------|------|
| `effectiveTier(ctx, projectID)` | 读 project + `o.cfg.Ingress.TierB` |
| `ensurePreviewIngress` | 按 tier：host/prod_port → 现逻辑；dedicated_port → 构建 binding 后 **`AttachIngressListenPort(..., ephemeral)`**；`FormatPreviewURL(host, listenPort)` |
| `ensureProdIngress` | host/prod_port(preview 无关) → Host upsert；dedicated_port/prod_port prod 部分 → **若已有 prod binding 且 `listen_port` 非空则 no-op 改口**（R-PROD-PORT-STABLE）；否则 adopt reserve 或 stable Attach |
| `teardownPreviewIngress` | archive/destroy/compensate：`DetachIngressListenPort(previewBindingID, reason)`；host-only 时等价于 `setPreviewIngressActive(false)` |

**Ready 事务顺序（对齐 [INGRESS-PORT-DEPLOYMENT §5.2](./INGRESS-PORT-DEPLOYMENT.md#52-version-readypreview)）：**

1. 现有：deploy → start → health → D1/bindings → `SetRoute(active=true)`  
2. **若 preview 需 port：** 在 route 之后、status ready 之前（或与 binding 同事务边界）：`ensurePreviewIngress` 内 Attach（若 Attach 已含 Upsert binding，避免重复 Upsert 无台账写 `listen_port`）  
3. `SetVersionPreviewURL` + `PUBLIC_BASE_URL`（已有）  
4. **Gateway verify：**  
   - Host 模式：`VerifyGatewayRouteHost(gatewayURL, previewHost)`（现状）  
   - Port 模式：新增 `VerifyGatewayRouteURL(ctx, previewURL)` 或 `VerifyGatewayRouteHostPort(host, port)` — 请求 authority 来自 **API 形 preview_url**（`127.0.0.1:port`），仍带 **synthetic Host** 头（R-UPSTREAM-HOST）  
5. **P5c 前：** verify 可能因无 dedicated listener 失败 — 测试用 `CELPD_SKIP_GATEWAY_VERIFY=1` 或 tier=host 回归；port e2e 文档标注依赖 P5c  

**Ready 失败 rollback（设计 §5.2 末段）：**

- `processOne` 失败 → `compensateDeploy` 增加：`DetachIngressListenPort(preview, "deploy_failed")`（仅 ephemeral；Detach 内 Release + 清 listen_port）  
- 若 Attach 发生在 deploy 早期且后续失败，同一 Detach  
- **不得** Release `{project}-prod` / reserve stable  

**可选 P5b：** 分配前 `bind(127.0.0.1:port)` 探针（设计 §3.3）— 放在 `AllocateIngressListenPort` 调用前或 registry 内（与 P5a「DB only」衔接）；失败换口或返回 `ErrPortConflict`。

### 4.5 Archive / Wake

`Archive`（`archive.go`）在 `setPreviewIngressActive` 之前或替换为：

1. `DetachIngressListenPort(previewBindingID, "archive")`（ephemeral 释放；host-only binding Detach 行为见 `port_ledger.go`：有 Host 则清 `listen_port`）  
2. `SetRouteActive(false)`、`Stop`、status archived（现状）

`Wake`：若 archived preview 在 dedicated_port 项目下需再次 reachable — 重新 `ensurePreviewIngress`（新 ephemeral 口或幂等 Attach）；与 Host wake 路径一致地激活 route。

### 4.6 Promote（R-PROD-PORT-STABLE）

文件：`orchestrator.go` `Promote`。

| 规则 | 实现要点 |
|------|----------|
| **禁止**改 prod `listen_port` | `ensureProdIngress` 内：若 prod binding 已存在且 `ListenPort != nil`，**跳过** Attach/Allocate，仅保证 Host/synthetic/active（port 模式 Host 可为 NULL 或保留，与设计「prod_port preview 仅 host」一致） |
| CAS / route | 不变 |
| `mergeProdPublicBaseURL` | 使用 §4.7 `ProdURL`（含 port） |

**审查点：** 今日 `ensureProdIngress` 全量 Upsert 可能 **覆盖** `listen_port` — P5b 必须改为 **merge** 或 read-modify-write 保留既有 port。

### 4.7 对外 URL：`preview_url` / `prod_url`

| 函数 | 变更 |
|------|------|
| `FormatPreviewURL` | 已支持 `listenPort`；dedicated_port 默认 **host 非空 + port 非空** 时优先级见现有实现（模板 > host URL > 纯 port）— 确认 dedicated 默认走 **host+port** 或 **纯 127.0.0.1**（设计 §5.2：默认 host+port 双写便于 Dashboard；对外 curl 以 API `preview_url` 为准） |
| `ProdURL(projectID)` | 签名可扩展为 `ProdURL(projectID string, listenPort *int)` 或 orch/API 从 **GetIngressBinding(prod)** 读 port；port 模式返回 `http://127.0.0.1:{port}/`（scheme 可 env `CELLP_PUBLIC_SCHEME_PROD`，dev 下 http） |
| `server.go` `prod_url` | 读 prod binding 或 project `prod_listen_port` + effective tier，**勿**仅 `cfg.ProdURL(projectID)` Host 形态 |

单元测试：`config/ingress_test.go` 增加 port 形态断言。

### 4.8 项目创建 / 首次 prod

对齐 [INGRESS-PORT-DEPLOYMENT §5.1](./INGRESS-PORT-DEPLOYMENT.md#51-项目创建)：

1. `CreateProject`：effective tier + optional `prod_listen_port` → Reserve  
2. 首次 ready 设 prod CAS 时（`runDeploy` 末 `SetProdVersionCAS`）：可 lazy `ensureProdIngress` — **P5b** 在 **Promote** 与 **首次 deploy 设 prod** 路径均调用 `ensureProdIngress`（现状已有 promote；首次 deploy 若设 prod 需补 prod ingress port）

---

## 5. 实现顺序（建议 PR commit 顺序）

1. `projects` 迁移 + `Project` 字段 + `GetProject`/`CreateProject`（Reserve 钩子在 Create）  
2. `config`：TierB 校验 + `EffectiveIngressTierB` + 测试  
3. Registry：`AdoptStableIngressPortForBinding`（或等价 UPDATE owner）+ 可选 `UpdateProject` 只读 port 校验  
4. `config.ProdURL` / preview URL 行为 + 测试  
5. `orch/ingress.go`：tier 分支 + Attach/Detach；**ensureProdIngress 保留 listen_port**  
6. `orchestrator.runDeploy`：preview 时机、verify authority、失败路径  
7. `compensateDeploy` + `Archive`/`Wake` Detach  
8. `Promote` 审查（no-op 改 prod port）  
9. `orch` 单元测试（表驱动 tier × ready/archive/promote）  
10. （可选）`e2e/scripts/v1-ingress-port-preview.sh` + `lib-ingress.sh`  helpers  

---

## 6. 测试与 e2e

### 6.1 单元 / 包测试（P5b 必做）

| ID | 场景 | 位置建议 |
|----|------|----------|
| T1 | effective tier：NULL 继承 / override | `config/ingress_test.go` |
| T2 | dedicated_port ready：Attach 后 binding + ledger 一致（R-PORT-LEDGER） | `orch/*_test.go` + registry 已有 Attach 测试 |
| T3 | deploy 失败：compensate Detach，port 可再分配 | `orch/process_one_test.go` 或新文件 |
| T4 | Archive：preview ephemeral Release | `orch/archive_test.go` |
| T5 | Promote 两次：prod `listen_port` 不变 | `orch/promote_*_test.go` |
| T6 | CreateProject + `prod_listen_port`：reserve owner；ensureProd adopt 同 port | `registry` + `orch` |
| T7 | `FormatPreviewURL` / `ProdURL` 含 `127.0.0.1:` | `config/ingress_test.go` |

### 6.2 e2e（最小脚本或 env）

**目标（设计 §9 P5b）：** `CELLP_INGRESS_TIER_B=dedicated_port` 下 preview `127.0.0.1:<port>` 返回 200。

**建议脚本：** `e2e/scripts/v1-ingress-port-preview.sh`（**不**默认加入 `MANIFEST`，避免 P5c 前 CI 红）

```bash
# 前置：./dev/scripts/health.sh
export CELLP_INGRESS_TIER_B=dedicated_port
# 可选：与 cellpd/gateway 同实例
export CELLPD_INSTANCE_ID="${CELLPD_INSTANCE_ID:-...}"

# 1. 创建项目/版本（沿用 ve-cd-loop 或 API）
# 2. GET version → jq .preview_url → 解析 port
# 3. curl -sf "http://127.0.0.1:${port}/" -H "Host: synthetic...."
# 4. 期望 200（**P5c 前可能失败** → 脚本 SKIP 或文档 defer）
```

**lib-ingress.sh 增量：** `curl_preview_url()`、`wait_http_200_preview_url()`（解析 API URL，非 Host @ 8787）。

**Defer 策略：** 若实现周期内无 P5c，在脚本内 `[[ "${INGRESS_PORT_E2E:-0}" == "1" ]]` 才跑；否则 `docs/evidence/ingress-port-p5b.md` 记录 registry/orch 单测 + 手动 env。

**与现有 e2e 关系：** 默认 `run-all.sh` **不**改；回归仍跑 Host 路径（`ve-cd-loop.sh`、`ve-promote.sh`、`v15-archive.sh` 等）。

---

## 7. 验收命令

```bash
# 1. Go 全量（P5b 门禁）
cd cellp && go test ./...

# 2. Registry port 回归
cd cellp && go test ./internal/registry/... -count=1

# 3. Orchestrator 重点
cd cellp && go test ./internal/orch/... -count=1

# 4. Config ingress
cd cellp && go test ./internal/config/... -run Ingress -count=1
```

**e2e（有点名，非默认门禁）：**

| 脚本 | 用途 |
|------|------|
| `./e2e/scripts/run-all.sh` | Host 模式回归（**不应**因 P5b 退化） |
| `./e2e/scripts/ve-cd-loop.sh` | deploy → preview 可达（Host） |
| `./e2e/scripts/ve-promote.sh` | promote 主路径 |
| `./e2e/scripts/v15-archive.sh` | archive 后 preview 不可达 |
| `./e2e/scripts/v1-ingress-port-preview.sh` | **新增（可选）** dedicated_port + `INGRESS_PORT_E2E=1` |

**本地栈（手动 port e2e 前）：**

```bash
./dev/scripts/up.sh && ./dev/scripts/health.sh
```

---

## 8. 风险与决策点（P5b PR 前确认）

| 项 | 建议 |
|----|------|
| stable reserve → prod binding | **UPDATE `port_allocations.owner_id`** 同事务 upsert prod binding（避免 release 竞态） |
| dedicated_port preview URL 默认 | **host+port 双写** binding；`preview_url` 优先 host@gateway 或纯 loopback — 与设计 §5.2 一致：**verify 用 API preview_url authority** |
| `ensureProdIngress` Upsert | **必须** merge 保留 `listen_port` / 台账 owner（promote 回归） |
| Gateway verify vs P5c | port e2e 可 skip；Host `run-all` 必须绿 |
| bind 探针 | P5b 实现或显式 defer 并写 TODO（设计 §3.3） |
| `external_map` | P5b no-op 台账；不报错即可 |
| PATCH `prod_listen_port` | P5b 可仅 Create；改口禁止直至 admin 迁移设计 |

---

## 9. 强制规则自检（P5b 完成后）

| ID | P5b 责任 |
|----|----------|
| R-PORT-LEDGER | 生产路径仅 Attach/Detach 写 `listen_port` |
| R-PROD-PORT-STABLE | promote/rollback 不改 prod 口 |
| R-PORT-UNIQUE | 依赖 P5a registry |
| R-STABLE-RESERVE | CreateProject Reserve 冲突 |
| R-ARCHIVE-TEARDOWN | Archive Detach ephemeral |
| R-BIND-LOOPBACK | listen 127.0.0.1（Gateway P5c） |

---

## 10. 参考索引

- 生命周期 normative：[INGRESS-PORT-DEPLOYMENT.md §5](./INGRESS-PORT-DEPLOYMENT.md#5-生命周期orchestrator--gateway)  
- P5a 台账 API：[INGRESS-PORT-P5-impl-plan.md §3–§5](./INGRESS-PORT-P5-impl-plan.md)  
- 现有 orch ingress：[cellp/internal/orch/ingress.go](../../cellp/internal/orch/ingress.go)  
- Attach/Detach：[cellp/internal/registry/port_ledger.go](../../cellp/internal/registry/port_ledger.go)  
- Host e2e 辅助：[e2e/scripts/lib-ingress.sh](../../e2e/scripts/lib-ingress.sh)

---

*文档版本：2026-09-01 · 对应仓库 P5a 已落地、orch Host-only 快照。*

---

## 实施记录

**日期：** 2026-09-01 · **范围：** P5b（registry 项目列 + config tier + orch Attach/Detach + prod URL）

### 交付摘要

| 项 | 实现 |
|----|------|
| `projects.ingress_tier_b` / `prod_listen_port` | `migrateIngressProjectColumns` + `Project` / `CreateProject`（Create 时 `ReserveStablePort`） |
| Tier 校验 | `config/ingress_tier.go`；`cellpd` 启动 `cfg.Ingress.Validate()` |
| Stable 预占 → prod binding | `AdoptStableIngressPortForBinding`（UPDATE `owner_id`） |
| Preview ready | `dedicated_port` → `AttachIngressListenPort` ephemeral；`preview_url` 为 `127.0.0.1:port` |
| Prod | `ensureProdIngress` 保留既有 `listen_port`；stable adopt / Attach |
| Archive / compensate | `teardownPreviewIngress` → `DetachIngressListenPort` |
| Deploy 顺序 | `SetRoute` 后 `ensurePreviewIngress`；Host verify 不变；port verify 仅 `CELLP_INGRESS_PORT_GATEWAY_VERIFY=1` |
| API | `GET /v1/projects/{id}` 含 `ingress_tier_b`、`prod_listen_port`、`prod_url`（读 prod binding）；Create 可选字段 |
| 未做（按计划 defer） | P5c `ReconcileListeners`；P5d Dashboard/PATCH；默认 e2e port 脚本 |

### 测试

```text
cd cellp && go test ./... -count=1   # 全绿（2026-09-01）
```

新增：`config/ingress_tier_test.go`、`ingress_port_test.go`；`registry/store_project_ingress_test.go`；`orch/ingress_test.go`；`runtime/gateway_verify_port.go`。

### 备注

- 生产路径 `listen_port` 仅经 `AttachIngressListenPort` / `AdoptStableIngressPortForBinding` / `DetachIngressListenPort`。
- Promote 路径 `ensureProdIngress` 对已存在 prod 口 no-op 改口（R-PROD-PORT-STABLE）。

