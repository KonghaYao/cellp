# 11 API, Config and Public Docs

> 状态：Draft；Q1–Q12 已确认为 public surface 的设计输入，但所有新 API/config 仍须新 AD 批准；公开产品文档只在行为实现后更新。

## 目的
以 cellpd 为唯一管理面暴露 serving 状态/policy/node/replica，并同步安全配置与产品文档。
## 范围
SURGE §20、§28；REST/OpenAPI、CLI/Dashboard consumer、env validation、site 行为说明。
## 非目标
Dashboard 不直连 `:8792`/Agent/RustFS；不公开 credentials/internal endpoint；不提前宣称 E4 或把 Draft 作为 GA。
## 术语
`deploy_ready` 是 additive 的可启动状态；现有 `ready` 语义保持；serving summary 是只读派生；policy revision 用于 CAS；operator API 与 public ingress 分离。
## 输入/输出
输入：00/01 逻辑模型与 decision-003..014。输出：OpenAPI、config validation、Dashboard/API view、site cold/limits/503 说明。
## 接口/数据模型
**Draft**：additive 暴露 `deploy_ready` 与 serving summary；PUT policy 带 revision；list nodes/replicas；scale override/cordon/drain；错误码稳定低基数。配置表达 preview idle 默认本地 5m/生产 15m、第一阶段 prod `min>=1`、background unknown/resident `min>=1` 且能力未证明前 `max=1`。环境配置集中解析，启动日志只打印非敏感值。
## 状态/不变量
旧 `GET Version` 和 `ready` 语义兼容；Host ingress 仍 AD-12；API 不返回 env/credential/watch/internal listener；Node Agent HTTP+mTLS 不成为公开 API；未实现能力不得在 docs 标为 GA。默认值与 Q1–Q12 不得由实现或压测静默改写。
## 错误/降级
未知字段、非法 min/max/background、第一阶段 prod `min=0`、未证明 background `max>1` 返回 validation error；控制面不可用标准 5xx；冷请求按分类返回有界 `503 + Retry-After`；Dashboard 显示 unknown/degraded，不猜测 warm。
## 依赖和并行边界
依赖 00–10 冻结公开 surface。WP-API 独占 OpenAPI/api server；WP-CONFIG 独占 config与 `.env.example`；WP-WEB、WP-SITE 各自唯一路径 owner，按顺序消费。
## 未来实现 WP
`WP-API` 与 `WP-CONFIG` 可在内部逻辑契约冻结后并行；`WP-WEB`、`WP-SITE` 只消费获批且已实现功能；共享 OpenAPI/config/public docs 各自唯一 owner。
## 验证
unit：config/API validation；contract：OpenAPI backward compatibility；component：mock Store；e2e：policy/state/权限；stress：list pagination；web：Vitest/Playwright；docs：site build/link检查。
## 证据产物
`docs/evidence/surge/e5/api/<run-id>/`、`web/`、`site/` 子目录，禁止含 secret。
## 阻塞 spike
API auth 现状边界；`deploy_ready` migration；SP-E6 校准未确认的 timeout/body/concurrency 数值；background capability 与 snapshot revision 契约。
## 回滚/兼容注意事项
新字段 additive；flag=0 可返回 unsupported/legacy summary；撤回前保持旧客户端和 `ready`；站点仅在获批实现后说明 self-host 容量、有界 `503` 和已证实限制。
