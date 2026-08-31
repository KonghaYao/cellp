# 用户行为闭环（PM 记录 · TP-UI-14）

**受众：** 平台操作者（单 `ADMIN_TOKEN` / Bearer，**无账号体系 · AD-10**）  
**最后更新：** 2026-08-31

## 用户故事

作为 cellp 平台操作者，我希望在本地或私有化栈上：**用 CLI 部署 Worker → 在 Dashboard 只读/有限写入地检查 preview 数据与路由 → 满意后 promote 到 prod → 必要时 rollback**，且文档与 Dashboard 内引导步骤一致，贡献者有一条脚本可跑 Vitest 心流门禁。

## 闭环定义（Done）

| 阶段 | 操作者动作 | 成功标准 |
|------|------------|----------|
| 准备 | `cellp doctor`、栈 `:8790`/`:8787`、Dashboard `VITE_CELLP_*` | API health 200；Dashboard 带 Bearer 可调 API |
| 项目 | （可选）Dashboard **New project** 或首 deploy 自动注册 | Projects 列表可见 project id |
| 部署 | `cellp dev --project <id>` 或 CI `POST …/versions` | 至少一个 version 进入 `ready` |
| 预览 | `curl` gateway `/{project}/{version}/` 或 Dashboard 链接 | HTTP 200 / 预期响应 |
| 巡检 | Overview → Deployments → Version → Storage → Platform | 路由与 binding 可读；preview 分支见快照说明 |
| 发布 | Version 页 **Promote** 或 `POST …/promote` | `prod_version_id` 切换；prod URL 生效 |
| 回滚 | 再次 promote 旧 ready version | prod 指针回到历史 version |
| 贡献者 | `web/scripts/verify-user-loop.sh` | Vitest 全绿；live E2E 文档化（栈未起可 skip） |

## 已交付

- 站点：[site/docs/get-started/operator-journey.md](../site/docs/get-started/operator-journey.md) + **Operator checklist**（可勾选步骤、成功标准、失败提示）
- Quick start 首页链到 checklist
- Dashboard：`OperatorChecklist` on [ProjectOverviewPage](../../web/src/pages/ProjectOverviewPage.tsx)（无 version → deploy 强调；ready 非 prod → Promote 链到 version 页）
- Vitest：`web/src/flows/*.flow.test.ts`（含 `operator-checklist.flow.test.tsx`）
- Playwright：mock `create-project.spec.ts`；live `e2e/live/operator-loop.spec.ts` + `npm run test:e2e:live`
- 门禁脚本：`web/scripts/verify-user-loop.sh` → 日志 `docs/evidence/user-loop-vitest-YYYYMMDD.log`

## 刻意不做（AD-10）

- 登录 / 注册 / Org / RBAC / 多租户 UI
- Dashboard 内上传 Worker 包（部署仅 CLI/CI）
- 内置 DNS/CDN/TLS/WAF、Git 托管、全球边缘

鉴权保持 **`DEPLOY_TOKEN` + `ADMIN_TOKEN`**；管理维度 **Project + Version**。

## 与 TP-UI-14 对齐

| test-plan 检查项 | 证据 |
|------------------|------|
| mock 创建项目 | `web/e2e/create-project.spec.ts` |
| Vitest 心流 | `web/src/flows/*.flow.test.ts` |
| Operator checklist 文档 + Dashboard | operator-journey § Operator checklist；`operator-checklist.tsx` |
| 贡献者 verify | `web/scripts/verify-user-loop.sh` |
| 真栈 live | `./dev/scripts/up.sh` 后 `npm run test:e2e:live` |

见 [docs/test-plan.md](../test-plan.md) **TP-UI-14**。
