# 13 Resilience, Chaos and Runbook

> 状态：Draft；故障行为仍须新 AD/实验批准，真实恢复时间 unknown；Q6/Q8/Q10/Q12 与现行安全/耐久不变量作为测试输入。

## 目的
定义组件失效时 bounded failure、恢复收敛和 operator 操作，不把故障扩散为全站/OOM/split brain。
## 范围
SURGE §17、§25；celld/node/controller/Gateway/Agent/RustFS/network 故障矩阵、chaos、runbook。
## 非目标
不承诺零 `503`，不绕过 fencing，不做外部云自动扩容，不包含 secret 操作值，不以 hard-stop 代替安全 drain。
## 术语
LKG snapshot；fenced；capacity exhausted；wake storm；thrashing；rollback reserve；RTO 是实测而非草案承诺。
## 输入/输出
输入：health/lease/metrics/audit。输出：ejection/replacement/有界 `503`/告警/人工步骤和恢复证据。
## 接口/数据模型
runbook 操作：查 serving gap、manual override/cancel、cordon/drain、诊断 wake/pressure/crash loop、`celld diagnose` 失败处置、flag rollback、previous prod rollback、secret轮换后受影响 fleet重启。Node Agent 故障矩阵包括 HTTP+mTLS identity、证书轮换/吊销/过期、replay；snapshot 包括 revision 回退、SQLite pause、LKG 与首次启动。
## 状态/不变量
E1/E2 单 active controller；controller down 时 warm 基于 LKG 继续、cold 有界失败；首次无 snapshot fail-closed；node/Agent lease 过期不得重回 snapshot；RustFS 无法证明写时不确认。普通 scale-down 不断开 live WebSocket；pinned 不低于 operator floor且不抢占 active/background work；所有动作可审计且无 secret。
## 错误/降级
Gateway burst→有界 `503`；controller→LKG；Agent/node→eject+replace；RustFS→fail write/fence；celld→有界 takeover；无容量→priority shedding。node emergency drain/显式 operator 操作才可受控关闭 WebSocket；floor 后仍有压力时 admission/load shedding，不直接 OOM。
## 依赖和并行边界
消费 01–10；WP-OPS 拥有 runbook/chaos orchestration，组件 owner 提供 fault hooks；chaos 单栈串行或独立硬件。
## 未来实现 WP
`WP-OPS` runbook与chaos driver；`WP-RT/CELLD/GW/REG` 各修自身恢复，不跨写共享文件。
## 验证
unit：状态超时；contract：reason/audit/mTLS；component：dependency failure；e2e：恢复；stress：burst/thrash；chaos：TP-EC1..6 与 TP-ES，记录 RTO/RSS/写确认/WebSocket/floor。
## 证据产物
`docs/evidence/surge/e5/chaos/<case>/<run-id>/`：timeline、fault、expected/actual、recovery、版本，全部脱敏。
## 阻塞 spike
SP-E2/4/5；单 active controller failover 操作；WebSocket emergency drain；SQLite revision/LKG；可控网络分区/RustFS pause fixture。
## 回滚/兼容注意事项
runbook 第一条确认 flag与模式；回滚遵循14，不能 hard-stop 全 fleet；legacy 模式继续使用现有 runbook/门禁；spike 失败按最小能力缩小范围。
