# 06 Autoscaler Policy

> 状态：Draft；Q2/Q3/Q7/Q8/Q10 已确认为政策边界；未确认的算法阈值仍须 E0/压力基线校准。

## 目的
把需求与 policy 转换为 desired，而不越过自有容量和 workload 安全边界。
## 范围
SURGE §8、§11；min/max、concurrency/backlog/latency 信号、快扩慢缩、stabilization、scale-to-zero。
## 非目标
不 placement、不 Start/Stop、不承诺无限容量、不为后台事件造新队列、不以 serving replica 代替 durability proof。
## 术语
target/hard concurrency；panic threshold；stable window；cooldown；manual override；desired-ready gap；operator floor 是显式 `min_replicas`。
## 输入/输出
输入：Gateway demand、ready count、resource pressure、policy/background guard。输出：单调 generation 的 `desired_replicas`、reason、expiry。
## 接口/数据模型
**Draft**：desired=max(min,ceil(observed_concurrency/target),backlog term)，受 max/cluster budget 约束；scale-up 快、scale-down 取稳定窗口。preview 满足全部 guard 后允许归零，本地 dev idle timeout 默认 5 分钟、生产 preview 默认 15 分钟；第一阶段 prod 禁止 `min_replicas=0`。不增加 `min_durability_nodes=2`，durability 单独建模。
## 状态/不变量
Autoscaler 唯一写 desired；Scheduler 不能反写需求；manual override 明确优先级/TTL；resident-required 强制 `min>=1`，未证明 background 多 replica 唯一执行前 `max=1`；live WebSocket replica 不进入普通 scale-down。pressure 可回收 pinned 中超过 operator floor 的 idle replica，但不得低于 floor或抢占 active/background work；desired 不等于 capacity。
## 错误/降级
信号缺失保持安全 min/LKG；unknown workload fail-closed 为 resident；异常高基数丢弃；持续 gap 产生 overload reason而非无限扩 desired；thrash 触发 cooldown；floor 后仍有压力时准入限制并有界 `503`。
## 依赖和并行边界
依赖 01 policy/Store、05 metrics、08 guard、03 capacity feedback。WP-SCALE 独占算法包，不改 schema/Gateway。
## 未来实现 WP
`WP-SCALE` policy validation/autoscaler；`WP-CONFIG` 落地已确认默认值；`WP-API` override surface。任何默认值变更须新决策，不得由压测静默改写。
## 验证
unit：公式/window/bounds/guards；contract：policy validation；component：signal loss/override；e2e：preview idle-to-zero、prod min 拒绝；stress：TP-EP3/EP4/EP8；chaos：controller restart、pressure floor。
## 证据产物
`docs/evidence/surge/e2/autoscaler/<run-id>/`、`.../e4/scale/<run-id>/`：signal/desired/ready 时间序列、默认值、floor 和硬件。
## 阻塞 spike
SP-E6 冷成本、SP-E1 scaling curve、目标硬件基线；算法阈值、等待预算、hysteresis 与 pressure 校准仍待证据。
## 回滚/兼容注意事项
flag=0 不写 desire；回滚冻结 scale operation并收敛单 replica；保留已确认的产品边界，算法参数只有新 AD 获批后才能落地。
