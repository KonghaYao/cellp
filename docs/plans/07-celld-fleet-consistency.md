# 07 celld Fleet Consistency

> 状态：Draft；celld ownership/fencing 与 RustFS durability guarantees 为现行事实；Q7/Q11 已确认为保守门禁，同 Version 多 node 全矩阵仍 unknown，不得推断。

## 目的
界定 1→N celld fleet 可上线前必须证明的一致性、兼容与停止语义。
## 范围
SURGE §14、SP-E1..E5；共享同一 Version bucket、不同 watch/listener、owner takeover、bindings、mixed-version。
## 非目标
不修改 D1 import/branch frozen RPC；不在 cellp 自造弱复制；不定义 Gateway/Scheduler；不以 replica 数替代持久化证明。
## 术语
fleet node 是 celld replica；cell owner/epoch/fencing 沿用 celld guarantees；durability nodes 与 serving replicas 分离。
## 输入/输出
输入：相同 immutable deploy revision/bindings、独立 node identity/watch。输出：可验证 public/internal listener、ownership/takeover、durability proof与兼容矩阵。
## 接口/数据模型
只消费 celld 既有接口与 guarantees；任何新增 fleet handshake/compatibility 字段均为 **Draft**，需 celld owner 单独审查。第一阶段不新增 `min_durability_nodes=2`；多 replica background workload 仅在 celld fleet 分别证明全局协调、故障接管和重复投递语义后开放。
## 状态/不变量
同 Version replicas 共享且仅共享本 Version bucket；不同 Version 隔离；成功写不丢；旧 owner fenced；版本/vars/bindings 不一致 fail-closed；生产启动先 diagnose。Cron、Queue consumer、Workflow executor 等证据未闭合时 `min_replicas>=1,max_replicas=1`；unknown 同样 fail-closed。
## 错误/降级
partition/RustFS 不可证明 durability 时不确认写；owner kill 有界 takeover；mixed version 不兼容摘除；spike 失败正式缩小最小受影响范围，而非绕过 guarantees；不能用 serving demand 推导 durability health。
## 依赖和并行边界
依赖 02 lifecycle；08 workload；10 security。WP-CELLD 独占 `celld/` submodule，其他 WP 不写；SP 证据先于 E4。
## 未来实现 WP
`WP-CELLD` 仅在 spike 显示缺口且独立授权后实现；`WP-RT` 只适配公开接口。每类 background workload 的多 replica 解禁需独立契约/AD 证据。
## 验证
unit：若新增协议则 Rust tests；contract：deploy/bindings revision；component：双 node；e2e：TP-E7/E10；stress：TP-EP7；chaos：TP-EC1/EC4；SP-E1..E5 全跑，SP-E3 分别覆盖 Cron/Queue/Workflow/alarm。
## 证据产物
`docs/evidence/surge/sp-e1..sp-e5/<run-id>/`：celld/RustFS 版本、拓扑、raw latency/errors、fencing traces（脱敏）。
## 阻塞 spike
SP-E1 双 node、SP-E2 takeover、SP-E3 bindings/background uniqueness、SP-E4 shutdown、SP-E5 V0c；SURGE §14.2 十项均需在这些矩阵中闭合。
## 回滚/兼容注意事项
E4 可整体或按失败能力禁用并回到单 celld；额外 nodes 先 drain；不得改变 frozen D1 RPC；submodule 版本必须可恢复。
