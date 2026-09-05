# 02 Runtime Backend and Node Agent

> 状态：Draft；当前 runtime.Manager 本机一 Version 一进程为现行事实；Q5 已确认本机和远程 Node Agent 统一 HTTP+mTLS，具体安全协议仍待 E0/E3 contract freeze。

## 目的
隔离进程执行与全局调度，使本机和远程节点共享生命周期语义。
## 范围
SURGE §13；Start/Health/Drain/Stop/List、端口/watch 管理、heartbeat、cordon、命令认证。
## 非目标
不写 desired/placement，不定义 celld fleet 一致性，不公开内部 listener，不落地证书基础设施。
## 术语
RuntimeBackend 是 transport-neutral 接口；Node Agent 是节点执行者；assignment/generation 是授权范围；cordon 禁新任务，drain 迁出现有任务。
## 输入/输出
输入：经 HTTP+mTLS 认证授权的 assignment 与 generation。输出：幂等 observation、health、endpoint、资源/失败原因，不含 secrets。
## 接口/数据模型
**Draft**：HTTP+mTLS API 承载 transport-neutral 的 `StartReplica(spec,idempotency)`、`Probe(replica)`、`Drain(replica,deadline)`、`Stop(replica,generation)`、`List()`；`ReplicaSpec` 仅含 secret reference，不含 credential；heartbeat 含容量、zone、agent/celld compatibility。证书 identity、SAN、轮换、吊销和 replay protection 由 E3 安全契约冻结。
## 状态/不变量
requested→starting→ready→draining→stopped/failed；旧 generation、过期 lease、越权 project/version 必须拒绝；本机也不得跳过 mTLS、授权、expiry、nonce 或 replay protection；端口唯一；watch 按 replica 隔离且停止删除；启动前 `celld diagnose`；内部 listener 不得公网暴露。
## 错误/降级
Agent partition 标记 endpoint 失联并由 controller 替换；重复命令返回原结果；deadline 后强停须审计且不得突破 active/background work 安全边界；未知 celld version fail-closed；不回显 env、credential 或完整连接串。
## 依赖和并行边界
依赖 00/01 assignment contract；07 提供 celld 行为；10 提供 HTTP+mTLS 认证。WP-RT 独占 runtime；Scheduler 不能 exec，Gateway 不能直连 Agent。
## 未来实现 WP
`WP-RT`：LocalBackend 与 RemoteBackend 均适配同一 HTTP+mTLS Node Agent；`WP-WIRE` 串行切换。逻辑命令不得绑定具体 HTTP 库。
## 验证
unit：idempotency/generation/port；contract：HTTP API、mTLS identity/auth/replay；component：fake process drain；e2e：E1 0/1、E3 node lifecycle；stress：启动预算；chaos：TP-EC2/EC5、TP-ES1/ES2。
## 证据产物
`docs/evidence/surge/e3/agent/<run-id>/`：命令审计、证书轮换结果、端口、进程、redacted config、恢复时间。
## 阻塞 spike
SP-E4 graceful shutdown；mixed-version 行为；远程 secret reference delivery 威胁审查；证书签发、轮换、吊销和 mTLS 失败策略。
## 回滚/兼容注意事项
LocalBackend 初期包裹现有 Manager，但不能以 in-process/Unix socket 绕过已确认的 HTTP+mTLS 边界；flag=0 走原 Start/Stop；远程 Agent 不可成为 legacy 模式必需依赖。
