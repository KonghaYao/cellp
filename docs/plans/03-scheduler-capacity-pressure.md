# 03 Scheduler, Capacity and Pressure

> 状态：Draft；Q6、Q7、Q10 已确认为设计输入；自有容量调度仍不是现行 AD 行为，须等待新 AD/E0。

## 目的
把 desired 转换为有界、可解释、可恢复的 node assignment，并在压力下保护 prod。
## 范围
SURGE §12、§17 容量部分；placement、反亲和、start budget、pressure eviction、lease。
## 非目标
不计算 desired，不 fork 进程，不负载均衡请求，不采购云容量，不把 serving replicas 当作 durability nodes。
## 术语
scheduled/ready 是实际能力；surge reserve 是未占稳态的自有容量；pressure 分 soft/hard；priority 为 prod/previous/pinned/preview 策略输入；operator floor 是显式 `min_replicas`。
## 输入/输出
输入：desired、policy、node heartbeat/capacity、replica observation。输出：assignment/cancel 与低基数 reason。
## 接口/数据模型
**Draft**：`Schedule(version,generation,count,constraints)`；node `allocatable/reserved/pressure/zone`；assignment `valid_until`；评分先过滤不兼容/cordoned 节点，再按余量与反亲和排序。第一阶段不增加 `min_durability_nodes=2`；durability health 与 serving placement 分离。
## 状态/不变量
E1/E2 仅单 active `cellpd` 持有 Scheduler/Reconciler 写权，Scheduler 唯一写 assignment；不能超过 policy max/cluster start budget；旧 lease 不复活。pressure 可回收 pinned Version 中高于 operator `min_replicas` 的 idle replica，但不得突破 floor；先摘流、drain，且不得抢占 inflight、live WebSocket、active/background work。floor 后仍有压力时使用 admission/load shedding/有界 `503`。
## 错误/降级
无容量→`capacity_exhausted`，desired 保留；心跳过期摘 endpoint并补位；SQLite busy 有界重试；禁止无限 pending assignment；drain/fencing 失败不得强停受保护 work。
## 依赖和并行边界
依赖 01 Store、02 Agent observation、06 desired。WP-SCHED 只写新 scheduler 包；pressure policy 由 06 定义，执行由 02 完成。
## 未来实现 WP
`WP-SCHED` placement/reconcile；`WP-REG` lease 原语；`WP-WIRE` controller ownership。V12 HA 选主属于独立设计。
## 验证
unit：过滤/评分/优先级/floor；contract：assignment CAS；component：node loss/无容量/pinned pressure；e2e：E1 pressure、E3 failover；stress：TP-EP5/EP8；chaos：TP-EC2/EC5。
## 证据产物
`docs/evidence/surge/e1/scheduler/<run-id>/`、`.../e3/failover/<run-id>/`，含决策原因、容量时间线、floor 与 drain 结果。
## 阻塞 spike
节点 RSS 可用性/校准；SP-E4 drain；单 active writer 启停与失效处置；未来多 active 需要独立 leader/term/fencing chaos 证据。
## 回滚/兼容注意事项
回滚前停止新 assignment、收敛单 replica；不能先停 Scheduler 留下无人管理进程；flag=0 不启动该 controller。
