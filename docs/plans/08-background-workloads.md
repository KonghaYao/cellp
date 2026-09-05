# 08 Background Workloads

> 状态：Draft；AD-11 prod-only Cron 为现行事实；Q8/Q9/Q11 已确认为 fail-closed 设计边界，多 replica 唯一执行能力仍须 SP-E3 证明。

## 目的
防止 HTTP scale-to-zero 丢失 Cron、Queue、Workflow、alarm 或破坏 WebSocket。
## 范围
SURGE §15；workload capability matrix、policy guard、Cron arm/disarm、live connection drain exclusion。
## 非目标
不设计 durable Event Activator，不声称所有 workload 可归零，不修改 celld bindings 协议。
## 术语
`resident_required` 表示 `min>=1`；Event Activator 是未来独立设计；live WebSocket 是普通 scale-in 抑制信号。
## 输入/输出
输入：部署时解析的静态 workload manifest/bindings、prod pointer、live workload state。输出：policy validation、resident reason、Cron arm/disarm intent。
## 接口/数据模型
**Draft**：`background_mode={none,resident_required,unknown}`；以静态 manifest/bindings 为主要判定依据。Queue consumer、Workflow、alarm-capable Durable Object、已武装 Cron 均为 `resident_required`；无法完整解析、未知 schema/binding 或不能证明 HTTP-only 时 fail-closed；capabilities 不含 secret。
## 状态/不变量
只有 prod arm Cron（AD-11）；resident workload `min>=1`；非 prod 默认不 arm Cron；live WebSocket replica 不普通 scale-in。celld fleet 未分别证明全局唯一协调、故障接管和重复语义前，含 Cron/Queue/Workflow 的 Version 必须 `max=1`；unknown 同样适用。
## 错误/降级
检测失败视为 resident；Cron reconcile 失败保持旧 prod 安全状态并告警；无 durable wake source 时拒绝 `min=0`，而非静默接受；不得用瞬时 runtime idle 推断未来无事件。
## 依赖和并行边界
依赖 01 policy、06 validation、07 SP-E3、09 promote。WP-BG 独占 background policy/Cron integration；不改 celld frozen contracts。
## 未来实现 WP
`WP-BG` capability detector/guard；未来 Event Activator 必须新任务、新决策；多 replica 解禁由 WP-CELLD 证据和独立 AD 驱动。
## 验证
unit：matrix/validation/unknown；contract：capability schema；component：prod pointer change；e2e：TP-E9/E10、扩展 TP-V17；stress：多 replica duplicate detector；chaos：resident node loss恢复。
## 证据产物
`docs/evidence/surge/e4/background/<run-id>/` 与 `sp-e3`：每 workload 触发计数、owner、接管、重复语义和恢复时间。
## 阻塞 spike
SP-E3 分别证明 Queue/Workflow/Cron/alarm；SP-E4 证明 shutdown；持久 wake 源不存在是现行限制，在独立 Event Activator 获批前不得放宽 resident guard。
## 回滚/兼容注意事项
flag=0 维持 AD-11；Elastic 回滚前所有 resident prod 保证一个 legacy replica；不得把 cold 当 archived。
