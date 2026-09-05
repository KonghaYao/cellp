# 04 Activator Cold Start

> 状态：Draft；Q1/Q4 已确认为设计输入：只对 `deploy_ready+cold` 提供 HTTP 激活；archived 仍显式 wake，非幂等请求不得猜测重放。

## 目的
安全合并 `deploy_ready+cold` 首访，触发一次幂等容量请求并进行分级有界等待。
## 范围
SURGE §9.2–§9.5；singleflight、request/byte budget、deadline、`503 + Retry-After` reason。
## 非目标
不唤醒 archived，不做 durable event activation，不写 SQLite/启动 celld，不缓存大流式 body。
## 术语
singleflight key=`project,version,desired_generation`；waiter 是等待 endpoint 的请求；global/per-version budget 双重约束；`deploy_ready` 表示可启动，不代表当前有 active replica。
## 输入/输出
输入：Host 已解析 Version、Version 为 `deploy_ready`、snapshot 显示 cold、请求属性。输出：`EnsureCapacity(min=1)`、一次转发许可或低基数 `503 + Retry-After`。
## 接口/数据模型
**Draft**：ActivatorClient `EnsureCapacity` 幂等；GET/HEAD/小 body 请求可在 request count、body bytes 与 deadline 的明确预算内等待；大 body、chunked/流式请求和 WebSocket 触发激活后快速返回 `503 + Retry-After`。第一阶段 prod 因 `min_replicas>=1` 不依赖 scale-from-zero 路径；preview idle 默认由 06 定义。
## 状态/不变量
每 Gateway 本地 singleflight，集群幂等由控制面 `ensure desired>=1` 保证；取消立即释放；body/Authorization/Cookie 不落盘不日志；请求一旦可能被 upstream 接收则零自动重放；恶意 Version 不能耗尽全局预算；archived 永不隐式进入该路径。
## 错误/降级
queue full=`wake_queue_full`；timeout=`wake_timeout`；no capacity=`capacity_exhausted`；control unavailable 有界失败；archived=`version_archived`；不满足等待分类的请求快速 `503 + Retry-After`。
## 依赖和并行边界
依赖 00 EnsureCapacity、01 snapshot、05 proxy；WP-GW-ACT 独占 activator 新文件，不改 Registry/Node Agent。
## 未来实现 WP
`WP-GW-ACT`；EnsureCapacity server 由 WP-API/REG；最终 wiring 由 WP-WIRE。
## 验证
unit：budget/cancel/singleflight/请求分类；contract：reason/idempotency/Retry-After；component：100 并发仅一 ensure；e2e：TP-E2/E3；stress：TP-EP2/EP6、TP-EC6、TP-ES5。
## 证据产物
`docs/evidence/surge/e2/activator/<run-id>/`：assignment count、RSS、pending bytes、响应分类（请求内容脱敏）。
## 阻塞 spike
SP-E6 校准 timeout/budget；验证多 Gateway 下仅依赖控制面幂等仍不会形成启动风暴；大/流式/WebSocket 快速失败的资源上界须证明。
## 回滚/兼容注意事项
flag=0 完全绕过 Activator；不得改变现行 `ready`、archived/wake；关闭时释放全部 waiter并返回有界 `503`。
