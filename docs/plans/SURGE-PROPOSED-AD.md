# Proposed AD-15 — Elastic Serving Fleet 与安全 Scale-to-Zero

> **状态：已批准（AD-15 · 2026-09-05）— 见 [decisions.md §20](../decisions.md#20-ad-15--elastic-serving-fleet-与安全-scale-to-zero)；分阶段实现中**  
> **编号：AD-15（暂定；正式批准时才占用）**  
> **日期：2026-09-05**  
> **决策输入：** [SURGE Decision Brief](./SURGE-DECISION-BRIEF.md)  
> **详细设计：** [SURGE Design Index](./SURGE-DESIGN-INDEX.md)  
> **审查：** [SURGE Proposed AD Adversarial Review](./SURGE-PROPOSED-AD-REVIEW.md)

本文是一份已完成文本对抗审查、等待正式审批的架构决定草案。它不修改当前产品行为，不取代 `DESIGN.md` 或 `docs/decisions.md`，不修改冻结 D1 RPC，不把 TP-E/SP-E 纳入现行 `docs/test-plan.md`，也不授权产品代码、迁移、E0/SP 或上线。

---

## 1. Context

cellp 当前依据 AD-1 为每个 `ready` Version 运行一个独立 celld 进程和独立 bucket；AD-9 通过显式 `archived` 停止进程并保留 RustFS 数据。该模型提供清晰隔离，但所有未封存 preview 均常驻，且单 Version 只有一个 serving endpoint，无法按流量在自有容量内进行 0→1、1→N 或 pressure 回收。

SURGE 方向需要在不弱化下列现行约束的前提下增加弹性 serving：

- RustFS 是唯一持久层，本地 `CELLD_WATCH` 只是可丢弃页缓存；
- 每个 Version 的 bucket 隔离不变，D1 import/branch RPC 继续冻结；
- AD-5 `offshoot_promote` 与 `SetProdVersionCAS` 仍是 promote 硬门禁；
- AD-9 `archived` 仍需显式 wake，不能被 HTTP 请求隐式激活；
- AD-10 产品边界、AD-11 prod-only Cron、AD-12 Host ingress、AD-14 OTLP 与查询门面保持；
- SQLite 仍是第一阶段控制面持久存储，不引入 PostgreSQL、Redis 或外部协调服务；
- celld ownership、epoch/fencing、成功写 durability 与 `celld diagnose` 不能由 best-effort 行为替代。

## 2. Decision

若本 AD 后续正式批准，cellp 将把 **Version 部署生命周期** 与 **Serving Fleet** 分离：每个可部署 Version 拥有 `0..N` 个独立 celld replica，控制面以 policy、desired state、assignment、generation 和 lease 驱动实际进程；Gateway 只从不可变 Route Snapshot 选择 ready endpoint。

该方向分阶段启用，默认 feature flag 关闭。现有 Version 不自动迁移；只有显式进入 elastic adoption 的 Version 才使用新状态和 controller。第一阶段 prod 不允许 scale-to-zero，多节点、多 replica background workload 和多 active controller 均受独立证据门禁。

### 2.1 Version 状态、Serving 状态与生命周期 API

新增 additive Version 状态 **`deploy_ready`**，保留现有 **`ready`** 语义：

- `deploy_ready`：artifact、数据、migration 和 bindings 已准备完成，可安全启动 celld，但当前不保证存在 ready serving endpoint；
- `ready`：至少存在一个 lease/generation 有效、health 通过、非 draining 且已进入有效 Route Snapshot 的 serving endpoint；
- `cold`、`waking`、`starting`、`warm`、`degraded` 是 Serving 层派生状态，不替代 Version 状态；
- `archived` 是显式产品状态，任何 ingress 请求都不得隐式 wake；
- 初次 deploy 在 artifact/data/bindings 完成后进入 `deploy_ready`，qualification 使用的 endpoint 只有完成 health、Gateway route verify 和 snapshot 发布后，Version 才进入对外 CD 终态 `ready`；qualification 中间态不得短暂暴露为 `ready`；
- 当 elastic preview 的最后一个 ready replica 被安全停止且仍可部署时，Version 从 `ready` 回到 `deploy_ready`，Serving 派生为 `cold`；激活成功并将有效 endpoint 发布到 snapshot 后再转回 `ready`；
- deployment/qualification 失败进入现有失败路径，不得以 `deploy_ready` 掩盖 artifact、data、binding、health、route verify 或 snapshot 失败。

`deploy_ready` 的 branch 语义是本 AD 对 AD-8 的明确扩展：父 Version 为 `ready`、`deploy_ready` 或 `archived`，且 Registry 能证明 immutable fork 元数据、父 bucket、LTX/overlay anchor 和 binding identity 完整时，均可作为 D1/KV/R2/Queue branch 父版。branch 直接读取 RustFS 中的父快照，不要求为 cold 父版启动 celld；任何存储证明缺失均 fail-closed。该扩展只改变 cellp 父状态验证，不修改 D1 frozen RPC 或 celld branch 格式。

AD-9 archive 与 elastic cold 使用互斥路径：

| Version 状态/模式 | 普通 ingress | `POST wake` | 显式 archive | 自动 idle 行为 |
|---|---|---|---|---|
| legacy `ready` | 现行路由 | 4xx：非 archived | 维持 AD-9 | 维持 AD-9 grace/45m reaper |
| elastic `ready/warm` | endpoint snapshot | 4xx：非 archived | 允许，须先摘流/drain | 只走 Autoscaler，不进入 AD-9 reaper |
| elastic `deploy_ready/cold` | Activator 分级处理 | 4xx：`version_not_archived` | 允许且不得为 archive 先启动 celld | 保持 cold，不自动改成 archived |
| `archived` | `503 version_archived` | 维持 AD-9 显式 wake | 幂等/按现行 API | 不进入 Activator |

因此，elastic enrollment 必须使该 Version 从 AD-9 自动 reaper 的候选集中排除；operator 仍可显式 archive。`POST wake` 只接受 `archived`，不能作为 cold Activator 的别名。具体 additive API enum、4xx status/reason、polling schema 和 Dashboard 呈现由 E0 contract freeze，但上述状态和操作语义不得改变。

状态转换必须由 Registry 事务和 generation/CAS 保护。E0 必须审计 CLI、CI polling、OpenAPI、Dashboard、promote、destroy、branch、archive/wake 等全部 consumer；未被显式采用的 legacy Version 不发生 `ready → deploy_ready` 转换。

### 2.2 控制面模型与唯一 writer

控制面引入下列逻辑对象；最终表名和 API 字段由 E0 contract freeze 确定：

- `ServingPolicy{revision,min_replicas,max_replicas,priority,background_mode}`；
- `ServingDesire{desired_replicas,generation,reason}`；
- `RuntimeNode{node_id,capacity,compatibility,cordoned,lease}`；
- `RuntimeReplica{replica_id,node_id,generation,state,valid_until}`；
- `Endpoint{replica_id,address,state,valid_until}`；
- `RouteSnapshot{revision,bindings,endpoint_sets,policy_revision}`。

不变量：

1. Autoscaler 是 `desired_replicas` 唯一 writer；Scheduler 是 assignment 唯一 writer；Node Agent 只执行带 generation/lease 的命令并上报 observation。
2. 旧 generation、过期 lease、旧 assignment 和重复使用的 replica ID 均无效。
3. E1/E2 以及本 AD 的初始部署形态只允许一个 active `cellpd` 运行 Scheduler/Reconciler 写循环；其他实例只能承担 standby、API 或 Gateway。
4. active writer 必须持有机器可验证、进程生命周期内持续有效的 singleton guard；对初始单机/同 SQLite 拓扑，参考机制是与权威 SQLite 同宿主的排他 OS lock。无法取得或失去 guard 的实例必须拒绝启动/继续 Scheduler/Reconciler 写循环，拒绝发出新的 Start/Drain/Stop/assignment，产生低基数告警和审计事件；误将两个实例配置为 active 时，至少一方必须 fail-closed。禁止使用可能在暂停进程仍可产生副作用时过期的普通 lease，除非 Node Agent 同时校验 controller term/fencing。
5. 不提供自动 standby promotion。operator 必须确认旧 active 已停止或 guard 已由操作系统释放后，才能显式启动新的 active；SQLite/WAL、普通 CAS 或部署副本数均不等价于 leader election。
6. 多 active controller、自动选主和 controller-wide term/fencing 属于未来 V12 独立 AD，不得从本 AD 推导。E0 必须冻结 singleton guard、失效处置和人工接管协议，E1 必须以误双 active 和进程暂停场景证明 fail-closed。

### 2.3 RuntimeBackend 与 Node Agent

本机和远程 Runtime Node 均通过 **HTTP+mTLS Node Agent** 执行 transport-neutral 生命周期命令：

- `StartReplica`、`ProbeReplica`、`DrainReplica`、`StopReplica`、`ListReplicas`；
- 请求必须绑定 node identity、project/version scope、generation、lease/expiry、nonce 或等价 replay protection；
- command spec 只包含 secret reference，不传输或记录 token、password、private key、bucket credential 或完整连接字符串；
- listener 仅位于内部网络，不是公网入口；本机调用也不能绕过 mTLS 或授权；
- 每个 replica 使用独立 celld listener 和 watch；停止后删除可丢弃 watch；启动前执行 `celld diagnose`；
- Node Agent API 与证书签发、SAN/identity、轮换、吊销、过期和 mixed-version 策略必须先通过 contract/security gate。

E1 必须证明本机 HTTP+mTLS 生命周期与幂等/generation；E3 才扩展到远程 node、网络分区、证书轮换和 mixed-version 场景。

### 2.4 Scheduler、容量与 pressure

Scheduler 将 desired 转换为受容量、兼容性、cordon、反亲和、start budget 和 lease 约束的 assignment。无容量时保留 desired 并返回低基数 `capacity_exhausted`，不能无限创建 pending assignment。

第一阶段不设置 `min_durability_nodes=2`。Serving replica 数只表达 serving demand；持久化正确性继续依赖 RustFS 和 celld guarantees，durability health 必须独立建模。

pressure eviction 与普通 idle scale-down 分离：

- 优先拒绝新 start，再回收低优先级、无 active work 的容量；
- pinned Version 中高于 operator 显式 `min_replicas` 的 idle replica 可被回收；
- 不得突破 operator floor，不得抢占 inflight request、live WebSocket、Queue/Workflow/Cron/alarm 等 active/background work；
- 必须先从 snapshot 摘流，再 drain，最后 stop；任一 fencing/drain 前置失败不得强停；
- floor 保留后压力仍存在时，使用 admission control、load shedding、有界 `503` 和告警，不以 OOM 或静默破坏 pin 收场。

### 2.5 Gateway Route Snapshot

E1/E2 使用 **SQLite revision polling + Gateway 进程内不可变 snapshot 原子替换**：

- 所有影响 Host/listen port、prod pointer 和 endpoint set 的写入必须与全局单调 revision 在同一事务提交；
- Gateway 从一致性读事务构建 `Host/listen port → Version → ready endpoints`；
- 只接受 schema 兼容、内容校验通过且 revision 严格递增的 snapshot；
- 更新失败时保留 last-known-good；进程首次启动没有有效 snapshot 时 fail-closed；
- snapshot 中每个 endpoint 携带 `valid_until` 或等价租约边界。只有持有 singleton guard 的 active writer 可以续期 endpoint/replica lease；即使使用 LKG，Gateway 也不得在该边界后继续把新请求发往过期 endpoint；active writer 失联导致全部 endpoint 过期时，warm 请求也必须有界 fail-closed，而不是无限使用 LKG；
- Gateway 热路径不查询 SQLite，也不能通过 Host 或请求参数选择内部 endpoint、bucket 或 node；
- draining endpoint 不接收新请求，已绑定请求可在 deadline 内完成。

只有多 Gateway 节点无法安全读取同一权威 SQLite，或轮询无法满足经实测定义的传播延迟/负载 SLO 时，才允许启动 V12 snapshot distribution AD。E1/E2 的默认拓扑是 Gateway 与唯一 active controller 读取同一宿主上的同一权威 SQLite；任何共享文件系统复制、异步 SQLite 副本或跨节点文件挂载都不在该假设内。

### 2.6 Activator 与冷请求安全

Activator 只处理 **非 archived 的 `deploy_ready+cold` Version**，并通过幂等 `EnsureCapacity(min=1)` 请求控制面提升 desired。Gateway 本地 singleflight 只能合并单实例请求；跨 Gateway 防风暴依赖控制面对 `ensure desired>=1` 的 generation/CAS 幂等。

请求分级如下：

- GET、HEAD 和满足明确内存上限的小 body 请求，可在 per-Version 与全局 request/body/deadline 预算内等待 endpoint ready；
- 小 body 的非幂等请求只有在尚未发生任何 upstream 尝试时才可驻留内存，endpoint ready 后最多发起一次 forwarding attempt；平台不承诺传输不确定性下的 exactly-once；
- 大 body、chunked/流式请求和 WebSocket 触发激活后快速返回 `503 + Retry-After`，不得无限缓存；
- 一旦请求可能已被 upstream 接收，Gateway 不自动换 endpoint 或重放；
- queue full、wake timeout、control unavailable、capacity exhausted 和 version archived 使用稳定低基数 reason；请求 body、Authorization、Cookie 和敏感 query 不落盘、不进日志/OTLP。

本 AD 不冻结“小 body”、queue count/body bytes、等待 deadline 或 `Retry-After` 的具体数值；这些值必须由 SP-E6 和 E2 在目标硬件上校准，并受配置验证与全局硬上限约束。

### 2.7 Autoscaling policy

第一阶段 policy 边界：

- 本地 dev preview idle timeout 默认 **5 分钟**；生产 preview 默认 **15 分钟**；
- preview 只有在无 inflight、无 queued request、无 live WebSocket、无 resident/unknown background workload、无 active migration/drain 和其他已登记 guard 时才可归零；
- 第一阶段 prod 禁止 `min_replicas=0`，始终至少保留一个 ready replica；
- scale-up 可根据并发、backlog 和 desired-ready gap 快速发生；scale-down 必须使用稳定窗口、cooldown 和 drain；
- concurrency target、panic threshold、queue/body/deadline budget、stabilization 和 pressure 阈值不是本 AD 的已确认数值，必须由 SP-E6 与目标硬件基线校准；
- signal 缺失或异常时保持安全 floor/LKG，不无限扩 desired。

已确认的 5m/15m 与 prod floor 不得被实现或压测静默改写；修改需新的架构决策。

### 2.8 Background workload 与 WebSocket

以部署时解析的静态 workload manifest/bindings 作为 `background_mode` 主要判定依据：

- Queue consumer、Workflow、alarm-capable Durable Object 和已武装 Cron 均为 `resident_required`；非 prod bundle 中仅声明但依 AD-11 未 arm 的 Cron 不单独触发 resident，除非同时存在其他 resident/unknown workload；
- 未知 schema/binding、解析失败或无法证明 HTTP-only 时 fail-closed 为 `resident_required`；
- 没有 durable Event Activator 时，resident/unknown workload 禁止 scale-to-zero；
- celld fleet 尚未分别证明全局协调、故障接管、fencing 和重复投递语义前，含 Cron、Queue consumer 或 Workflow executor 的 Version 必须 `min_replicas>=1,max_replicas=1`；
- AD-11 保持：只有 prod Version arm Cron。

普通 scale-down 不主动断开 live WebSocket；有 live WebSocket 的 replica 不进入普通候选集。为防止长连接无界占用，Gateway 必须支持新连接 admission、per-Version/全局容量保护和 pressure 告警。只有 node emergency drain 或显式 operator 操作可以按冻结的 close reason/deadline 执行受控关闭；透明会话迁移不在本 AD 范围内。

### 2.9 celld fleet 与 durability gate

同 Version 多 celld replica 只有在 SP-E1..E5 对目标 RustFS 拓扑证明以下事项后才能启用：

- 相同 immutable deploy revision/bindings 和独立 node identity/watch；
- 同 Version 只共享自己的 bucket，跨 Version 不共享写；
- owner takeover、epoch/fencing、成功写 durability、partition 和恢复；
- graceful shutdown、mixed-version compatibility 和 fail-closed；
- Cron、Queue、Workflow、alarm 分别定义并证明唯一执行/重复投递语义。

任何 spike 失败都必须形成 `no-go` 或 `scope-reduced`，只移除最小受影响能力，并恢复现行安全基线；禁止以 best-effort、外部云存储、新数据库或 cellp 自研 broker 绕过 celld/RustFS guarantees。

### 2.10 Promote、archive 与 migration

Promote 保留 AD-5 的 forward/compensate 语义，并增加 elastic snapshot 与 AD-11 的完成条件：

1. `validate`：目标 Version 必须为 `ready`，至少一个 endpoint lease 未过期、非 draining 且已出现在当前有效 snapshot；`deploy_ready`-only、cold、archived、failed 均按现行非法状态规则拒绝，保持 TP-API-6 fail-closed。
2. `drain_old`：对旧 prod 启动有界 drain，但保留其进程、bucket 与 rollback reserve；旧 prod preview binding 不因 promote 删除。
3. `deactivate_old_route`：撤销旧 prod 资格，不能撤销其独立 preview binding；发生失败按 AD-5 逆序补偿。
4. `offshoot_promote`：继续作为外部硬门禁；失败时不允许 prod CAS、prod binding 或 snapshot revision 前进，并逆序补偿此前步骤。
5. `CAS_prod` → `activate_prod_route`：在同一个 SQLite 事务中校验 expected prod，更新 `prod_version_id`、新 prod binding/route 投影并递增 Route Snapshot revision；逻辑顺序保持 CAS 后激活，但只有完整事务提交才对 Gateway 可见。CAS、route 或 revision 任一失败都不发布部分状态，并按 AD-5 补偿。
6. Gateway 继续使用旧 LKG，直到接受包含新 prod endpoint 的更高 revision；旧 prod replica 在收敛确认前不得 stop。若新 snapshot 无法发布或在 deadline 内无法被要求的 Gateway 实例确认，promote 不对外宣称完成，保持/恢复旧 prod 指针与可用 snapshot，并进入可审计补偿或人工安全模式。
7. Route/cache 刷新确认后执行 AD-11 Cron reconcile：先确认旧 prod 不再 arm，再允许新 prod arm；不得出现两个 prod Cron owner。reconcile 必须幂等；失败时 promote 不对外宣称完成，记录 degraded/待重试状态，且禁止在旧 owner 未确认解除时武装新 owner。不得因 Cron 失败静默重放 `offshoot_promote`。

AD-9 的 archive/wake 产品语义继续有效，并按 §2.1 的模式表与 elastic cold 隔离。promote 后旧 prod 继承 AD-9 `previous_prod` rollback protection：至少保留一个 ready replica **60 分钟**（或当时生效的更长 operator `rollback_keep`），期间不受 preview 5m/15m idle scale-to-zero；保护期后才进入普通 preview policy。archived 仍停止进程、删除 watch、禁 route、保留 RustFS 数据，且只接受显式 `POST wake`。

Migration 采用 additive schema 和 per-Version enrollment；feature flag 默认关闭，不能一次性自动接管全部现有 Version。legacy reconciler 与 elastic controller 同时至多一个拥有 Start/Stop/assignment 权；enrollment 和 rollback 均须受 singleton guard 与审计保护。

### 2.11 Security、observability 与审计

遵守 AD-10 与 AD-14：

- 不新增账号体系、公共 Agent API、TLS 终止、WAF、自研日志/搜索后端；
- 生命周期命令、policy override、cordon、drain、emergency WebSocket close 和 controller mode 变化必须审计；
- metrics/traces 使用低基数 reason，默认不以 project/version 作为无界 label；
- 不记录请求内容、认证 header、secret、credential、内部 watch path或完整连接串；redaction 失败时宁可丢字段；
- OTLP/查询后端故障不得阻断数据面或改变控制状态，buffer 必须有界。

## 3. Adoption Gates

本 AD 的 **批准** 与各能力的 **启用** 是两个层次。当前文档停留在 Proposed，文本对抗审查已完成。

| Gate | 允许范围 | 必须证明 | 明确禁止 |
|---|---|---|---|
| Proposed | 文档与对抗审查 | Q1–Q12、AD 接缝、风险、owner、回滚完整 | 产品实现、schema migration |
| E0 | contract/lab planning | 状态机、API consumer audit、单 active 运维协议、mTLS contract、SP 计划、baseline | serving 行为变化 |
| AD approval | 允许按 WP 开始 E1 | E0 PASS、审查无未关闭 critical/high、scope 与 rollback 获批 | 跳过 phase gate |
| E1 | 本机声明式 0/1 | Registry/generation、local HTTP+mTLS、single active、legacy snapshot/rollback | HTTP scale-from-zero、远程 node、多 replica |
| E2 | preview HTTP 0→1 | Activator budgets、revision/LKG/expiry、5m/15m、request replay safety | prod min=0、background 归零 |
| E3 | remote node | mTLS rotation/revocation、partition、lease/fencing、mixed-version | 自动 controller failover |
| E4 | 1→N | 对应 SP-E1..E6 PASS、celld/RustFS/后台语义证据 | 由单一 workload 推断全部 background 安全 |
| E5 | hardening/API/docs | chaos、runbook、OpenAPI/UI/site、完整回滚演练、现行 M1/M2 | 未实现能力标 GA |

E2 与 E3 的设计可以并行，但实现不得越过前置 gate。TP-E/EP/EC/ES 与 SP-E1..E6 在 WP-ADOPT 明确纳入前不是现行 `docs/test-plan.md` 门禁。

## 4. Parallel Ownership

采用 `SURGE-DESIGN-INDEX.md` 登记的唯一写范围：WP-CONTRACT 先冻结共享逻辑契约；WP-REG、WP-RT、WP-SCHED、WP-GW-ACT、WP-GW、WP-SCALE、WP-CELLD、WP-BG、WP-ORCH、WP-OPS/WP-SEC、WP-API/CONFIG/WEB/SITE/TEST 按 DAG 消费；WP-WIRE 最后串行接线；WP-ADOPT 独占 `DESIGN.md`、`docs/decisions.md`、`docs/test-plan.md` 和 `VALIDATION.md`。

任何共享 contract、Registry schema、OpenAPI、serve wiring、配置、权威文档或 celld submodule 都只能由登记 owner 修改。新 AD 未批准前，所有产品 WP 的写权限均未开放。

## 5. Rollback

`CELLP_ELASTIC_RUNTIME=0` 是最终 kill switch 名称，但只有完成下述安全收敛后才能切换；它不能通过直接停止 controller 或让 cold Version 失去恢复路径来实现。每次回滚必须：

1. 停止新 scale-up、activation 和 enrollment；冻结新的 desired 变化。
2. 枚举所有已采用 elastic 的非 archived Version，包括 `ready` 和 `deploy_ready+cold`。
3. 对每个 Version 做显式选择：
   - 恢复 legacy：启动并验证一个 celld，转换为现行 `ready`，发布单 endpoint；或
   - 由 operator 使用现有 archive 操作显式封存；不得由回滚流程静默 archive。
4. 对恢复 legacy 的 Version 发布 revision 更高的单 endpoint snapshot，并等待 Gateway 收敛。
5. 摘流并 drain 所有额外 replica；live WebSocket/background work 按已冻结的 emergency/operator 规则处理，不能 hard-stop。
6. 停止 elastic Autoscaler/Scheduler/Reconciler，再启用 legacy reconciler；禁止两个 controller 同时拥有执行权。
7. 保留 additive SQLite 新表为只读，不在应急回滚中删除持久状态。
8. 串行运行现行 Go/M1/M2 与相关 D1/celld 门禁；不满足回滚前置条件时停在安全模式并人工处置。

## 6. Consequences

### Positive

- preview 可按 idle 归零并在首访时有界激活，降低常驻资源；
- 单 Version 可在 celld guarantees 被证明后横向扩展；
- 控制面、执行面和 Gateway snapshot 职责清晰，可按唯一 owner 并发开发；
- generation、lease、LKG、request 分类和 fail-closed background guard 限制故障扩散。

### Negative / Trade-offs

- `ready → deploy_ready` 是 elastic preview 的新可见转换，需要 API/UI/CI consumer 迁移；
- 本机 HTTP+mTLS 增加证书和运维复杂度；
- E1/E2 不提供 controller 自动 HA，故障恢复依赖明确的人工接管协议；
- WebSocket 和 resident workload 可长期阻止 scale-down；
- 自有容量耗尽时平台明确返回有界 `503`，不承诺无限排队；
- SQLite polling 在更大拓扑下可能成为瓶颈，但本 AD 拒绝提前引入未证明必要的分发协议。

## 7. Rejected Alternatives

1. **重定义 `ready` 为“可启动”**：破坏现有客户端语义；采用 additive `deploy_ready`。
2. **第一阶段 prod scale-to-zero**：冷请求、后台事件和可用性证据不足；prod 保持 `min>=1`。
3. **本机 Unix socket、远程 HTTP+mTLS 两套 Agent 传输**：生命周期和认证路径分叉；统一 HTTP+mTLS。
4. **SQLite lease/CAS 直接实现多 active controller**：无法 fence 进程、端口、drain 和 snapshot 等跨资源副作用；初期单 active，V12 另立 AD。
5. **以两个 serving replicas 充当 durability floor**：混淆 serving 与持久化正确性；继续依赖 RustFS/celld proof。
6. **运行时瞬时状态判断 background 可归零**：存在 TOCTOU 和漏事件；静态 manifest 为主，unknown fail-closed。
7. **允许 cellp Scheduler 保证 Cron/Queue/Workflow 唯一执行**：Scheduler generation 不能 fence celld 内部 dispatch/ack/step owner；等待 celld fleet 证据。
8. **普通缩容主动断开 WebSocket**：当前无透明迁移与完整重连契约；普通缩容排除 live connection。
9. **V12 snapshot push 协议立即落地**：需求和 ownership/fencing 未证明；E1/E2 先 SQLite polling。
10. **spike 失败后 best-effort 上线**：会弱化 durability/fencing；必须 scope reduction/no-go。

## 8. Preconditions for Approval

本 Proposed AD 只有满足全部条件后才可提交正式批准：

- 对抗审查的 critical/high finding 全部关闭，且修订可追踪；
- E0 完成 Version 状态机与 legacy consumer audit，明确 `deploy_ready` API/migration；
- 单 active controller 的配置、启动、人工接管和审计协议可执行；
- HTTP+mTLS identity、证书生命周期、授权和 replay contract 冻结；
- Route Snapshot revision、endpoint expiry、bootstrap 和 LKG contract 冻结；
- cold request 方法/body/内存/deadline 与重放边界可测试；
- background manifest schema、unknown 行为和 celld capability matrix 明确；
- SP-E1..E6 的环境、输入、PASS/FAIL、证据和 scope-reduction 规则可复核；
- per-Version enrollment 与完整 rollback rehearsal 计划获批；
- 所有未来产品写入范围有唯一 owner，且未修改 D1 frozen RPC。

## 9. Current Verdict

**PROPOSED — REVIEWED, NOT EFFECTIVE.** 文本对抗审查已以 13/13 finding `CLOSED`、`APPROVE-AS-PROPOSED` 收口；该结论不等于正式架构批准。下一步仅允许正式审批准备与 E0 规划。正式批准、E0/SP 执行、权威 AD 更新、产品开发和上线均需要后续独立动作与门禁。
