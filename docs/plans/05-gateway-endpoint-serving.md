# 05 Gateway Endpoint Serving

> 状态：Draft；Q8/Q12 已确认为设计输入；当前 Host→单 upstream 为现行事实，不可变 endpoint set、LB 与 LKG 仍待新 AD/E0。

## 目的
定义 warm 数据面、多 endpoint 选择、准入、health/ejection 和 drain 可见性。
## 范围
SURGE §4.3、§9.1、§10；RouteSnapshot 读取、流式 proxy、hard concurrency、warm backlog。
## 非目标
不写 desired/schema，不实现 promote saga，不让 Dashboard/Gateway 直连 operator API/RustFS，不设计独立 snapshot 分发协议。
## 术语
Endpoint state：starting/ready/suspect/ejected/draining/removed；LKG 是最后校验通过快照；inflight 是 Gateway 本地负载信号。
## 输入/输出
输入：Host binding、snapshot、请求、endpoint health。输出：选中 endpoint、demand metrics、低基数 `503`。
## 接口/数据模型
**Draft**：E1/E2 以 SQLite revision 轮询发现变化，从一致性读事务构建 `Host/listen port → Version → ready endpoints` 不可变 snapshot，并在 Gateway 进程内原子替换；Po2+least inflight，tie 用 EWMA；连接前明确失败可换端点，可能接收后不重试；body 默认流式。
## 状态/不变量
热路径不查 SQLite；只接受严格递增且校验通过的 snapshot；revision 回退、非法地址或撕裂视图拒绝应用并保留 LKG；首次无有效 snapshot 时 fail-closed。draining 不接新请求，已绑定请求可完成；存在 live WebSocket 的 replica 不进入普通 scale-down；普通 4xx 不 eject；Host 不能指定 endpoint/bucket。
## 错误/降级
无 ready endpoint 交 04；全部 hard limit 时短 backlog 或 `503`；SQLite/控制面异常保留 LKG；应用 5xx 与连接/runtime 错误分类；只有 node emergency drain 或显式 operator 操作可受控关闭 live WebSocket。
## 依赖和并行边界
依赖 00/01 snapshot、04 cold path、10 metrics/security。WP-GW 独占 warm/snapshot 文件；`gateway.go` 最终接线归 WP-WIRE。
## 未来实现 WP
`WP-GW` snapshot/LB/proxy；`WP-GW-ACT` 独立；`WP-WIRE` 串行 feature flag 接线。V12 独立分发协议只在轮询 SLO/拓扑证据表明必要时另立设计。
## 验证
unit：LB/inflight/ejection/revision；contract：snapshot compatibility；component：poll/atomic swap/LKG/drain/stream/non-retry；e2e：TP-E3/E4、TP-ES4；stress：TP-EP1/EP7；chaos：TP-EC3/EC6。
## 证据产物
`docs/evidence/surge/e2/gateway/<run-id>/` 与 E4 throughput 曲线，含 revision/endpoint 状态且不含内部 credential。
## 阻塞 spike
SP-E1 水平收益；SP-E4 WebSocket/drain；SQLite polling 的传播延迟与读取负载；多 Gateway 收敛和首次启动 fail-closed 证据。
## 回滚/兼容注意事项
legacy Route 投影为单 endpoint；flag=0 使用现有 RouteCache；切回前额外 endpoint drain，保留 Host/AD-12 行为。
