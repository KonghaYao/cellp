# 10 Security, Observability and Operations

> 状态：Draft；AD-10/AD-14 为现行事实；Q5 已确认 Node Agent 统一 HTTP+mTLS，具体 identity/证书与 telemetry schema 仍待新 AD/E3/E5 冻结；不得记录 secrets。

## 目的
为跨节点生命周期、过载与故障提供最小权限、低基数 telemetry 和可操作诊断。
## 范围
SURGE §18–§19、§25；Agent auth、listener 边界、audit、OTLP metrics/traces/log redaction、操作面。
## 非目标
不做账号/RBAC、TLS终止/WAF、自研搜索引擎，不在 Dashboard 直连 Agent/celld/RustFS，不记录 credential。
## 术语
command envelope 含 principal/scope/generation/expiry/nonce；elastic reason 是低基数枚举；trace 串联 ingress→wake→assignment→ready→proxy。
## 输入/输出
输入：组件状态/事件、HTTP+mTLS 认证上下文。输出：OTLP、查询门面、审计事件、redacted diagnostics。
## 接口/数据模型
**Draft**：本机与远程 Node Agent 均使用内部 HTTP+mTLS；命令 scope 精确到 node/project/version/action，并校验 identity、generation、lease、expiry、nonce/replay。metrics 包括 desired/ready/pending/rejection/start/drain/pressure；audit 不含 token、env、bucket credential、watch path或完整连接串；仅传 secret reference。证书签发、SAN、轮换、吊销和 mixed-version 策略须单独冻结。
## 状态/不变量
内部 listener 不公网；mTLS/auth fail-closed；命令重放、过期、旧 generation 拒绝；label 禁止 project/version 无界默认暴露；请求 body/header/query 不记录；redaction 失败宁可丢字段；AD-14 可换后端。
## 错误/降级
OTLP 不可达不能阻断数据面且有有界 buffer；证书过期/吊销/身份不匹配拒绝命令；查询后端故障不改变控制状态；任何错误不得回显 secret 或内部 credential。
## 依赖和并行边界
横切 01–09；WP-OPS 拥有 telemetry schema，WP-SEC 拥有 Agent HTTP+mTLS auth；组件只发登记事件；API 公开面归 11。
## 未来实现 WP
`WP-OPS` metrics/traces/audit；`WP-SEC` Agent mTLS/auth/certificate lifecycle；共享 telemetry 名称和 security middleware 各自单 owner。
## 验证
unit：redaction/cardinality/auth/replay；contract：HTTP+mTLS identity、OTLP 属性与 reason；component：certificate rotation/collector unavailable；e2e：TP-ES1..ES4；stress：telemetry overhead；chaos：backend outage/证书失效。
## 证据产物
`docs/evidence/surge/e5/security/<run-id>/`、`observability/<run-id>/`：扫描报告、证书轮换结果、trace 样本（脱敏）、cardinality/overhead。
## 阻塞 spike
内部网络拓扑；certificate authority/identity/SAN/轮换/吊销；secret reference 交付；mixed-version telemetry 与 mTLS compatibility。
## 回滚/兼容注意事项
新增 telemetry 可独立关闭但安全校验不可绕过；flag=0 不泄露新内部状态；保留现有 AD-14 查询门面契约；禁止以本机特殊路径绕过 HTTP+mTLS。
