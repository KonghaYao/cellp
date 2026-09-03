# 多 Worker / Service Binding 部署

> **状态：不支持（Deferred）** — 2026-09-02 产品决定：cellp **不**编排 `wrangler [[services]]` 多 Worker 栈（如 cloudflarebase 的 AUTH_AGENT 等）。单 version = 单 celld = 单主 Worker + 本 manifest 内 bindings（D1/KV/R2/Queue…）。  
> **验收影响：** S14 等依赖多 service 的项目仅 **partial**（可打开营销/重定向，**无**完整控制台登录链）。  
> 若未来重启，见下文方案 B 草案。

---

## 1. 问题陈述

今天 `deploy-support-app.sh` 假设：

- **一个** `wrangler.jsonc` → **一个** celld 进程（AD-1：一 version 一 bucket 一 celld）
- `[[services]]` 指向 **Cloudflare 账号内其它 Worker 名**；在 cellp 上 **没有** 自动把「依赖 worker」起成独立 version

因此：

- **单页 / 单 worker + ASSETS**（S06、S20、S21）→ 适配良好  
- **多 worker 控制台**（S14）→ `/login` 需要 `AUTH_AGENT` binding → **500**（不是 celld 崩溃）

---

## 2. 可选方案

### A. 文档标注「不支持多 service」

- support 矩阵标 **N/A · multi-worker**  
- **成本：** 低  
- **代价：** cloudflarebase、部分 BaaS 模板永远 partial

### B. **Composite project**（推荐讨论方向）

- 一个 **cellp project** = 逻辑应用；**多个 version 槽** 或 **多子 artifact**：
  - `support-cfbase`（用户可见 ingress）
  - `support-cfbase-auth`（internal，仅 service 绑定）
- cellpd orchestrator：`services` 解析为 **同 project 族** 的 `http://127.0.0.1:<port>` 或 **worker-to-worker fetch** 等价
- **成本：** 中–高（编排 + wrangler 合并 + 部署顺序）  
- **对齐：** 仍是一用户一 prod Host，子 worker 无公网 ingress

### C. **Monolith bundle**（仅适合可合并仓库）

- 把 agent 代码打进主 bundle（esbuild），去掉 `[[services]]`  
- cloudflarebase **不适合**（独立 agent 进程、D1 分库）  
- **成本：** 按项目定制，不可扩展

### D. **E2E 栈外挂 agent**（仅 dev）

- `dev/docker-compose` 或固定端口起 mock agent  
- 不进入 prod cellp 模型  
- **成本：** 低，**与 AD-10 分布式控制面** 不一致

---

## 3. 建议决策点（请你拍板）

| # | 问题 | 选项 |
|---|------|------|
| 1 | S14 是否纳入 M2/M3 门禁？ | 标 partial / 做 B / 跳过 |
| 2 | service binding 语义 | CF 同名 worker vs cellp 内部 service URL |
| 3 | 部署 UX | 一个 `deploy-support-app.sh S14` 自动拉 4 子 worker vs 显式 `S14-auth`… |
| 4 | wrangler | 禁止 overlay **整文件覆盖**；改为 **merge** upstream + cellp strip |

---

## 4. 与 crypto / ingress 的关系

- **crypto**：celld 单 isolate 能力；多 worker 不解决 S14 login  
- **ingress**：S01 类问题用 **promote 清单** 即可；与多 worker 正交

---

## 5. 下一步（若选 B）

1. ADR 冻结 `service_bindings` 解析规则  
2. PoC：`support-cfbase` + `support-cfbase-auth` 两 version，主 wrangler `services` 指向内部 host  
3. 扩展 `deploy-support-app.sh`：`MULTI_WORKER_MANIFEST=dev/examples/support-cfbase/manifest.yaml`

请回复倾向 **A / B / D** 或组合（例如「S14 标 partial，B 进 V0b」）。**当前采纳：A（不支持，S14 partial）。**

---

## 6. 当前产品声明（2026-09-02）

| 能力 | cellp |
|------|--------|
| 单 Worker + D1/KV/R2/Queue/Workflow（单 manifest） | **支持** |
| `[[services]]` 指向其它 Worker（多部署单元） | **不支持** |
| wrangler overlay | 须 **merge** 保留 bindings；禁止删掉 `services` 后声称 full 支持 |
