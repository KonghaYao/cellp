# 12 Test Concurrency and Evidence

> 状态：Draft；现行 M1/M2 `run-all.sh` 串行是既有门禁；以下弹性分片尚未进入 `docs/test-plan.md`，本轮不运行测试。

## 目的
把每个弹性要求映射为独立、可并发且无环境污染的验证与证据。
## 范围
SURGE §23–§24；unit/contract/component/e2e/stress/chaos 分层；所有 TP-E1..10、TP-EP1..8、TP-EC1..6、TP-ES1..5、SP-E1..6。
## 非目标
本轮不运行测试、不修改 MANIFEST/test-plan、不以并发 shard 替代 M1/M2、不将未执行 spike 标记 PASS。
## 术语
shard 是独立 stack；run-id 是全局唯一证据键；gate owner 决定串行纳入；lab spike 非现行产品门禁。
## 输入/输出
输入：各设计契约、Q1–Q12、phase gate。输出：PASS/FAIL/INFRA_FAIL、raw metrics、环境版本、覆盖映射；详细 owner/前置/路径见 `SURGE-DESIGN-INDEX.md`。
## 接口/数据模型
**Draft**：project=`surge-<shard>-<run>`；version=`v-surge-<case>-<run>`；bucket=`surge/<shard>/<run>/...`；独立 SQLite/watch/temp；端口块=`base+100*shard`；evidence=`docs/evidence/surge/<gate-or-spike>/<run-id>/`。每份 manifest 记录 decision refs、版本、fixture、预期和实际结果，不记录 secret。
## 状态/不变量
unit/contract/component 并发；e2e 仅独立栈并发；stress/chaos 独占硬件或串行；失败保留证据；cleanup只删本 run 前缀；不得记录 secret；最终 `run-all.sh` 串行。Q1–Q12 必须转成明确断言：`deploy_ready` 兼容、5m/15m、prod min、请求分类、HTTP+mTLS、单 active writer、无 durability floor、WebSocket guard、静态 background、pinned floor、background max=1、revision/LKG。
## 错误/降级
端口占用/fixture冲突标 INFRA_FAIL 不伪装产品失败；缺 V0c 不可将 SP-E5 标 PASS；重试使用新 run-id 并链接原失败；spike FAIL 触发 scope-reduced/no-go，禁止 best-effort 绕过 guarantees。
## 依赖和并行边界
依赖所有设计。WP-TEST 独占未来 `e2e/surge/`,`stress/elastic/`；MANIFEST、`docs/test-plan.md` 归 WP-ADOPT/gate owner。
## 未来实现 WP
分片：GW(TP-E2/E3,EP1/2/6,EC6,ES5)、REG(E1/5/6/7,EP5,EC3)、RT(E4,EC2/5)、CELLD(E7/10,EP7,EC1/4,SP-E1..5)、BG(E9/10)、ORCH(E8)、SEC(ES1..4)、SP6(GW/stress)。各 shard 使用独立资源和 evidence 目录。
## 验证
unit/contract/component/e2e/stress/chaos 六层均按上述 owner；每个 case 须断言资源清理、隔离、证据 manifest。M1/M2 与 Go tests 在标准栈最后串行；celld 变更按仓库门禁额外验证。
## 证据产物
每目录含 `manifest.json`、脱敏 env/versions、raw log、metrics、summary；硬件/RustFS/celld/policy 必记，不只 PASS/FAIL。
## 阻塞 spike
SP-E1..E6 自身为 E0 lab；测试框架接入前需端口多栈可行性小试；E4 gate 要求相关 spike PASS，失败能力必须从范围移除。
## 回滚/兼容注意事项
弹性测试先不进 MANIFEST；flag=0 全量一期回归；失败不得污染默认 dev project、bucket、SQLite或 evidence；并发分片不能替代标准串行 M1/M2。
