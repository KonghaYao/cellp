# 14 Adoption Gates and Rollback

> 状态：Draft；方案 A、Q1–Q12 与条件性未来开发门槛已确认；Proposed AD-15 已起草并通过文本对抗审查，E0 启动包已就绪，但 AD 尚未正式批准或生效；E0/SP 尚未执行，当前产品开发仍未授权。

## 目的
把组件设计转成可停止、可缩小范围、可回滚的 E0–E5 路线。
## 范围
SURGE §22、§28–§29；decision closure、feature flags、entry/exit、产品文件 owner、rollback。
## 非目标
不在本任务修改权威 AD/产品、不承诺日期、不把用户工作树改动视为本方案实现、不启动 E0/SP。
## 术语
adoption gate 是进入下一阶段的证据条件；scope reduction 是 spike 失败后的正式删减；kill switch=`CELLP_ELASTIC_RUNTIME=0`，但使用前需安全收敛。
## 输入/输出
输入：decision brief、Proposed AD/review、`SURGE-E0-START-PACK.md`、SP/TP 证据、各组件兼容报告。输出：go/no-go/scope-reduced 决定、正式批准准备及阶段发布清单。
## 接口/数据模型
Proposed AD 已固化 Q1–Q12、baseline、SP 计划、ownership 和兼容边界，并完成文本对抗审查；E0：冻结状态/API consumer audit、单 active 运维协议、mTLS/snapshot contract 与可执行 SP 计划；E1：本机声明式0/1/pressure/legacy snapshot，单 active `cellpd` 写权；E2：preview HTTP 0→1；E3：HTTP+mTLS remote node/ownership；E4：1→N 且相关 SP-E1..E6 PASS；E5：hardening/API/docs/runbook。
## 状态/不变量
每 gate 未满足不得进入；E2/E3 设计可并行，实现不得越前置；AD-1/9/10/12/14、D1 frozen、M1/M2 保持至新 AD 批准生效。产品权威文件由 WP-ADOPT 单 owner。只有新 AD 生效、E0及对应 phase gate通过、WP/DAG ownership与回滚证据齐备后，才可按 WP 开发；当前不得开发。
## 错误/降级
spike FAIL→形成 no-go/scope-reduced 并移除最小受影响能力；测试 flaky→不得宣称 PASS；rollback precondition 不满足→停在安全模式并人工修复；禁止用新依赖或 best-effort 绕过 RustFS、celld guarantees、D1 frozen contracts。
## 依赖和并行边界
依赖 00–13。WP-ADOPT 串行拥有 `DESIGN.md`,`docs/decisions.md`,`docs/test-plan.md`,`VALIDATION.md`；WP-WIRE 串行拥有 serve/main wiring；只有后续独立任务满足授权门槛后才能执行。
## 未来实现 WP
E0 `PASS` 且正式 AD approval 后，先串行开放 WP-CONTRACT，handoff 后开放 WP-REG；E1 限定范围再按 DAG 开放 RT/SCHED/SEC/GW/OPS。WP-GW-ACT/SCALE 等待 E1 后的 E2 gate；remote RT/SEC 等待 E3；CELLD/1→N 受对应 SP 与 E4 门禁；ORCH/API/CONFIG/WIRE 等待上游 handoff，WP-WIRE 最后串行；WEB/SITE/TEST 等待实现与 phase gate。唯一写范围和完整 blocked 清单见 `SURGE-DESIGN-INDEX.md` 与 `SURGE-E0-START-PACK.md`。
## 验证
unit/contract/component随WP；每阶段隔离e2e/stress/chaos；最终 `go test ./...` 与现有 `run-all.sh` 串行；celld变更另 build和专项；D1回归保持。本设计任务仅做静态文档检查。
## 证据产物
`docs/evidence/surge/adoption/<phase>/<run-id>/`：decision refs、required evidence hashes、scope、go/no-go、rollback rehearsal。
## 阻塞 spike
SP-E1..E6 与 SURGE §14.2 十项仍是证据 blocker；Proposed AD 起草和文本对抗审查已完成，`SURGE-E0-START-PACK.md` 已把 contract freeze、consumer audit、SP 计划和 WP 顺序收敛为可执行清单。剩余 blocker 为 E0 尚未执行、正式 approval 尚未发生及对应 phase gate 未通过；Q1–Q12 已闭合并已固化到 Proposed AD。
## 回滚/兼容注意事项
顺序：停新 scale→每个现行 ready Version 收敛1 legacy replica→单 endpoint snapshot→drain额外replica→停elastic controllers→启legacy reconciler→串行M1/M2。保留 SQLite 新表可读但不删，待单独安全 downgrade；失败项不得以绕过 guarantees 方式保留。
