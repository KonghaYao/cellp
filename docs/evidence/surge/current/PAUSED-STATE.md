# SURGE / AD-15 暂停检查点

## 状态

- **状态：PAUSED / GOAL BLOCKED（不是完成）**
- **记录时间：** `2026-09-07T03:51:03Z`
- **记录时 HEAD：** `3b822e4bf319a4190b0cb822e82234c1f6241d98`
- **工作树：** dirty；scoped snapshot 的 `git status --short --branch` 共 181 行，其中包含用户与并发工作的修改，不能把全部变更归属于本任务。
- **限制：** 未 commit，未 publish；恢复时不得覆盖现有并发工作。

## 已完成

1. SQLite context-aware migration 的 focused 收口与目标包验证。
2. `celld` bind/advertise 地址分离及 standalone Agent production plumbing。
3. 相关 focused tests 与 `-race` 验证。
4. 独立只读审查结论：`HIGH=0`、`ACCEPTANCE-BLOCKER=0`。
5. 当时工作树快照的 `go test -count=1 ./...` 与 `go vet ./...` 通过。
6. 使用真实 RustFS 与真实 `celld 0.4.0` 的 `v0a-celld-diagnose` 通过。
7. 主动终止长门禁后，dev 栈已恢复健康。

## 未完成

1. 真实 standalone Agent 驱动的 `celld` `start → probe → drain/stop` 集成验收。
2. 真正不同节点上的 controller/gateway + 两个 standalone Agent/celld 拓扑。
3. Scheduler `0→N`、`1→N`、跨节点 placement 与 background uniqueness。
4. `SP-E1..E6` 的真实执行和完整证据。
5. 完整 `TP-VE-ALL/M2` 与 `RUN_GATES=1` 的最终 exit 0。
6. 项目定义的生产压测、24h soak 和生产就绪门禁。
7. 根据最终实测结果同步 public site，并进行最终独立评估。

## `RUN_GATES=1` 事实

- 本轮在确认只有当前单机、总目标无法闭合后被主动终止，**不得称为 PASS**。
- 日志：`docs/evidence/surge/current/run-all-gates.log`。
- 已记录通过至 V9；V10 输出了 PASS，但整个套件没有取得最终 exit 0。
- V4 promote cutover 实测 `18.801s`，超过脚本 `2s` warning 阈值，也超过生产门禁 `5s`，必须在恢复后处理。
- 门禁运行期间 HEAD 从 `de39f3a` 变化到 `3b822e4`，因此该轮不是冻结代码快照上的最终证明。

## 精确 blocker

当前只有一台 Mac。权威 SURGE 合同要求：

- SP-E1：同一 Version 在独立 runtime node 上运行双 `celld`；
- SP-E2：在真实节点间验证 ownership/takeover、partition 与 fencing；
- SP-E5：不同 runtime node 访问生产等价 RustFS；
- E4 `1→N`：以 `SP-E1..E6` 的真实证据为前置。

同机 Docker 或同机多进程不能作为最终多节点证据。

解除 blocker 至少需要：

1. 一台 controller/gateway 主机和两台独立 Agent/celld 主机；或能够证明工作负载落在至少三个不同物理/虚拟 Worker Node 上的集群；
2. 所有节点可访问的生产等价 RustFS；
3. controller 与 Node Agent 的 mTLS 身份和证书；
4. 所需 listener 端口；
5. 对节点执行 kill、pause、partition、恢复等故障注入的权限。

## 恢复顺序

1. 冻结待验收 HEAD/工作树快照，先确认并保护用户与并发修改。
2. 准备上述真实多节点资源。
3. 补齐真实 Agent/celld lifecycle 与可移植 SP harness。
4. 依合同运行 `SP-E1..E6` 并保存完整脱敏证据。
5. 在冻结快照上重跑无缓存 Go tests、`go vet`、完整 `RUN_GATES=1` 和 M2。
6. 运行生产压测与 24h soak，处理 V4 cutover 性能偏差。
7. 根据实际通过范围同步 public site，并执行最终独立审查。

恢复时继续既有 SURGE/AD-15 架构，**不要从头重设计，不要 commit，不要 publish**。

## 现有证据

均位于 `docs/evidence/surge/current/`：

- `go-test-uncached.log`
- `go-vet.log`
- `run-all-gates.log`
