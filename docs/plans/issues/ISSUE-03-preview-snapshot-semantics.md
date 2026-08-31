# ISSUE-03: 产品不变量 — preview 为 fork 时刻快照；promote = 换 prod 桶（非 merge）

**优先级:** P1 · **类型:** 文档 + API 可观测性  
**关联:** AD-8、DESIGN.md §8、子 version skip export

## 问题

用户心智为「当前 prod + PR」；实际为父 celld bucket 在 `fork_txid` 的快照，promote 不合并 fork 后 prod 写入。叙事过度承诺导致 promote 后「丢数据」类事故。

## 验收标准

- [ ] `site/` 用户文档新增或更新一节：preview 数据时间线、promote 语义、与 Git 分支差异
- [ ] `DESIGN.md` 修正与 AD-8 矛盾的 P3（KV 空起步等）
- [ ] API：`GET /v1/projects/{id}/versions/{vid}` 或 bindings 响应中可选字段说明 parent 快照（若已有字段则文档化；若无则评估最小 JSON 字段如 `data_parent_version_id` / `forked_at` 仅当低成本）
- [ ] Dashboard 在 version 详情一处短文案（中/英与 site 一致）
- [ ] 不要求实现 rebase/merge

## 非目标

- D1 compact / 二层 branch
- 改 promote 数据合并语义

## 调研提示

- `sqlite` version 模型、`StoragePage.tsx`
- `docs/plans/D1-BRANCH-RPC.md` fork_txid
