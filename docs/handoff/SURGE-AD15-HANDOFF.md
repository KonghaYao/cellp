# SURGE / AD-15 任务交接

> **状态（2026-09-07）：** 继续开发中，但**总目标仍未完成**。  
> **当前 blocker：** 只有当前单机；权威 `SP-E1/E2/E5` 与 E4 `1→N` 需要真实不同 runtime node，同机 Docker/多进程不能作为最终证据。  
> **暂停检查点：** `docs/evidence/surge/current/PAUSED-STATE.md`（已单独提交 `b36dd2a`）。

---

## 1. 任务目标（给接手人）

在**不破坏现有并发工作、不提交/发布未授权改动**的前提下，继续完成 AD-15/SURGE 剩余交付：

1. remote HTTP+mTLS Node Agent
2. 真实 `celld` `start → probe → drain/stop` lifecycle
3. Scheduler `0→N` / `1→N`、跨节点 placement、background uniqueness
4. `SP-E1..E6` 真实证据
5. 完整 `TP-VE-ALL/M2`、`RUN_GATES=1`
6. 项目定义的生产压测 / 24h soak / 生产就绪门禁
7. 按实测结果同步 public site，且不得过度宣称

**不在范围内：** 重设计 SURGE/AD-15、把同机仿真当作多节点通过、读取/提交 `.env`/证书/secret、未经审查修改冻结 D1 RPC。

---

## 2. 已完成（可直接依赖）

### 2.1 架构与代码

- SQLite context-aware migration focused 收口。
- `celld` bind/advertise 分离：
  - `CELLP_AGENT_CELLD_BIND_HOST`
  - `CELLP_AGENT_CELLD_ADVERTISE_HOST`
- standalone `cellp agent` production plumbing。
- controller remote-control mTLS、registry relay、atomic `GetAuthorizedAgentVersionEnv`。
- controller-only / remote-only 模式与 focused/race 测试。
- 独立只读审查：`HIGH=0`、`ACCEPTANCE-BLOCKER=0`。

### 2.2 已验证

- 当时工作树快照：
  - `go test -count=1 ./...`
  - `go vet ./...`
- 真实 RustFS + 真实 `celld 0.4.0`：
  - `v0a-celld-diagnose`
- 真实本机 `celld` lifecycle（非多节点）：
  - `e2e/surge/real-celld-lifecycle.sh`
  - 证据：`docs/evidence/surge/real-celld-lifecycle/20260907-121127-15027/`
- dev 栈在终止长门禁后已恢复健康。

### 2.3 已提交

- `b36dd2a` — `docs/evidence/surge/current/PAUSED-STATE.md`

**注意：** 根仓库仍大量 dirty，含用户/并发改动；**不要** `git add .`。

---

## 3. 未完成（必须继续）

1. 真实 standalone Agent 驱动的 `celld` lifecycle 集成验收。
   - 本机 `runtime.Manager` 真进程路径已通过
   - standalone Agent harness 已就绪（`e2e/surge/real-agent-celld-lifecycle.sh`）；**尚未取得 PASS 证据**（需健康 dev 栈后执行）
2. 真正不同节点上的 controller/gateway + 两个 standalone Agent/celld。
3. Scheduler `0→N` / `1→N`、跨节点 placement、background uniqueness。
   - 单机 **0→1 / scale-to-zero** harness：`e2e/surge/single-node-scheduler.sh`（待 dev 栈证据）
   - 单机 **1→N 真 celld** 仍受 `runtime.Manager` 每 version 单端口限制；多副本逻辑见 `scheduler` 单测 + `placement_degraded`
4. `SP-E1..E6` 真实执行与完整证据。
5. 完整 `TP-VE-ALL/M2`、`RUN_GATES=1` 最终 exit 0。
6. 生产压测、24h soak、生产就绪门禁。
7. public site 最终同步与独立审查。

---

## 4. 权威验收合同（不要猜）

### 4.1 SP-E1..E6

权威来源：

- `docs/evidence/surge/e0/2026-09-05-e0-01/sp-plan-review.md`
- `docs/plans/SURGE-PROPOSED-AD.md` §14.2
- `docs/plans/07-celld-fleet-consistency.md`
- `docs/plans/08-background-workloads.md`

关键事实：

- `SP-E1`：同一 Version 在**两个独立 runtime node** 上运行双 `celld`。
- `SP-E2`：真实节点间 ownership takeover / partition / fencing。
- `SP-E3`：background uniqueness（Cron/Queue/Workflow）。
- `SP-E4`：0→N / 1→N / downscale / route revision / gateway reachability。
- `SP-E5`：不同 runtime node 访问生产等价 RustFS。
- `SP-E6`：chaos / recovery / rollback。

**E4 的 1→N 只有在 SP-E1..E6 全部真实通过后才可签收。**

### 4.2 M2 / RUN_GATES

- 完整门禁：`RUN_GATES=1 ./e2e/scripts/run-all.sh`
- 证据目录：`docs/evidence/surge/current/`

### 4.3 生产压测 / 生产就绪

- Phase 5：`stress/scripts/run-all.sh`
- Phase 6：`stress/phase6/run-all.sh`
- 阈值来源：`docs/test-plan-phase2.md`、`docs/test-plan-phase6.md`、`docs/evidence/stress-env.json`

---

## 5. 当前门禁事实（不得过度宣称）

### 5.1 部分 `RUN_GATES=1`

日志：`docs/evidence/surge/current/run-all-gates.log`

已记录通过：

- Phase 0 storage gates
- health
- VE-2..VE-5
- D1 seed / D1 branch
- V2 / V3 / V4 / V4b / V5 / V5B / V6 / V7 / V9
- V10 输出 PASS，但整个套件**没有**最终 exit 0

### 5.2 已知偏差

- V4 promote cutover：`18.801s`
  - 超过脚本 warning `2s`
  - 超过生产门禁 `5s`
- 门禁运行中 HEAD 从 `de39f3a` 变为 `3b822e4`
  - 该轮不是冻结代码快照上的最终证明

### 5.3 结论

- **不能**宣称 M2 / TP-VE-ALL / RUN_GATES 全绿
- **不能**宣称生产就绪
- **不能**宣称多节点通过

---

## 6. 精确 blocker

当前只有一台 Mac。

权威合同要求真实不同 runtime node；同机 Docker 或同机多进程**不算**最终多节点证据。

解除 blocker 至少需要：

1. 1 台 controller/gateway + 2 台独立 Agent/celld 主机；或能证明调度到至少 3 个不同物理/虚拟 Worker Node 的集群
2. 所有节点可访问的生产等价 RustFS
3. controller 与 Node Agent 的 mTLS 身份/证书
4. 所需 listener 端口
5. kill / pause / partition / recovery 等故障注入权限

---

## 7. 关键实现面（接手后先读）

| 区域 | 文件 |
|------|------|
| bind/advertise 配置 | `cellp/internal/config/celld_upstream.go` |
| standalone Agent 接线 | `cellp/internal/agentrun/deps.go` |
| replica runtime | `cellp/internal/runtime/replica_manager.go` |
| replica network | `cellp/internal/runtime/replica_network.go` |
| controller remote control | `cellp/internal/serve/remote_control_wire.go` |
| registry atomic env | `cellp/internal/registry/agent_version_env_sqlite.go` |
| SQLite migration | `cellp/internal/registry/sqlite.go` |

默认行为：

- bind/listen 默认 loopback
- advertise 未设时等于 bind
- 真实 `celld --listen` 与 health 使用 bind
- controller/gateway 可见 host 使用 advertise

---

## 8. 下一步（严格顺序）

1. **冻结待验收 HEAD/工作树快照**，先保护并发改动。
2. **补齐本机可验证的真实 lifecycle 验收**（standalone Agent 路径已加 harness，待 dev 栈健康后跑证据）：
   - opt-in Go integration test：`cellp/internal/agentrun/real_agent_celld_lifecycle_test.go`（`CELLP_REAL_AGENT_CELLD_TEST=1`）
   - 薄 shell 入口：`e2e/surge/real-agent-celld-lifecycle.sh`
   - 证据目录：`docs/evidence/surge/real-agent-celld-lifecycle/<run-id>/`
   - 已有 `runtime.Manager` 路径：`e2e/surge/real-celld-lifecycle.sh`
3. **准备可移植 SP harness**：
   - `e2e/surge/README.md`
   - `e2e/surge/sp-e1-dual-celld.sh` 等骨架
   - 明确单机只能 `BLOCKED`，不能冒充 PASS
4. **获得真实多节点资源后**，按合同执行 `SP-E1..E6`。
5. 在冻结快照上重跑：
   - `go test -count=1 ./...`
   - `go vet ./...`
   - `RUN_GATES=1 ./e2e/scripts/run-all.sh`
6. 运行生产压测 / 24h soak，处理 V4 cutover 性能偏差。
7. 按实测结果同步 public site，并完成最终独立审查。

---

## 9. 证据索引

| 路径 | 用途 |
|------|------|
| `docs/evidence/surge/current/PAUSED-STATE.md` | 暂停检查点 |
| `docs/evidence/surge/current/go-test-uncached.log` | 无缓存 Go 全量 |
| `docs/evidence/surge/current/go-vet.log` | go vet |
| `docs/evidence/surge/current/run-all-gates.log` | 部分 RUN_GATES |
| `docs/evidence/surge/real-celld-lifecycle/20260907-121127-15027/` | 本机真实 celld lifecycle PASS |
| `docs/evidence/surge/final/ACCEPTANCE-RUN.md` | 旧最终验收尝试（FAIL） |

---

## 10. 接手禁忌

- 不要 `git add .`
- 不要读取/打印 `.env`、证书、token
- 不要修改冻结 D1 RPC
- 不要把同机仿真写成多节点通过
- 不要在没有 SP-E1..E6 真实证据时宣称 1→N / 生产就绪
- 不要未经用户明确要求就 commit / publish
