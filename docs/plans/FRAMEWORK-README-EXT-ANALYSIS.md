# README 框架扩展 S27–S30 阻塞综合分析

> **日期：** 2026-09-03  
> **口径：** [support-matrix.md](../support-matrix.md) §README 框架扩展（S26–S30）  
> **关联：** [NEXT-OPENNEXT-S30-ROOT-CAUSE.md](./NEXT-OPENNEXT-S30-ROOT-CAUSE.md) · PD-20260903-08

---

## 1. 总览

| ID | 框架 | 矩阵 | 当前阻塞阶段 | 一句话 |
|----|------|:----:|--------------|--------|
| S27 | SolidStart | 不支持 | **artifact 生成** | 非交互 C3/`create-solid` 无法产出 SolidStart 工程 |
| S28 | Qwik City | 不支持 | **celld 启动 / ready** | slim artifact 可构建（v3），`celld health timeout` |
| S29 | Waku | 不支持 | **celld 启动 / ready** | dry-run 多模块 ~848 KiB；**no_bundle 仅上传 main + 扁平 wasm**，sibling ESM 未收录 |
| S30 | OpenNext | 不支持 | **HTTP 语义** | **PD-08 wasm 已修**（celld `54957b2`）· v5 **ready** · prod `GET /` → **308** `Location: ?` |

S26（Hono）已 **支持**。

---

## 2. S27 — SolidStart

- C3 `solid` 委派 **`create-solid -s`**，非交互下常 **TTY / hello-world stub**。
- **绕过：** rsync `workers-sdk` `packages/create-cloudflare/templates/solid/`（同 S29 waku overlay）；或 `create-solid -s` + `CI=1` 试验。
- `deploy-support-app.sh` S27 当前 `WORKDIR_SUB=hello-world-with-assets/ts`，与意图不符。

---

## 3. S28 — Qwik

- v3 artifact：**~333 KiB** 单 bundle，`no_bundle`，无 wasm → **非 PD-08**。
- **health timeout**（~60s）：需 `${TMPDIR}/celld-support-qwik-v3.log` 区分 **load 失败** vs **compile 慢**（`stateless Worker failed to load` / `node:*` compat）。

---

## 4. S29 — Waku

- wrangler dry-run **14 ESM 模块**；`index.js` **静态 import** `./assets/server-entry-*.js`。
- **celld `no_bundle`** 只上传 **main** + 同目录 `*.wasm` → **sibling `.js` 未进 manifest** → 高概率 load 失败。
- **修复方向：** celld 收录 outdir 全部模块 **或** Waku 侧单文件再 bundle。

---

## 5. S30 — OpenNext

| 阶段 | 结果 |
|------|------|
| v1–v3 | health timeout（wasm 缺失） |
| **v5**（PD-08） | **ready** · deploy ~8623 KiB 含 wasm |
| prod | **308** `Location: ?` · `X-Opennext: 1` |

**剩余：** Host / `global_fetch_strictly_public` / OpenNext URL 矩阵（非 wasm）。

---

## 6. 环境与 AD-1

- 每 version 独立 celld + S3 bucket；日志 `$TMPDIR/celld-{project}-{version}.log`。
- 失败 version 可能残留进程：`pgrep -lf celld`；`PATH` 上 celld 须含 PD-08 构建。

---

## 7. 修复优先级

| P | 项 | 责任方 |
|---|-----|--------|
| **P0** | S29 多模块 no_bundle | **celld** + artifact |
| **P0** | S30 `308 Location: ?` | **celld** + **cellp** + artifact vars |
| **P0** | S27 SolidStart 模板 scaffold | **artifact** |
| **P1** | S28 log 定性 + compat | **celld** |
| **P1** | 重跑 S28/S29 在 PD-08 celld 上 | **验证** |

---

## 8. 给修复 subagent 的执行清单

1. S27：`templates/solid` rsync scaffold（仿 waku）。
2. S28：读 qwik v3 celld log；修 compat 或重部署验证。
3. S29：celld `no_bundle` 扩展收录 wrangler outdir 模块 **或** prepare 单文件化；重部署至 `grep` 验收。
4. S30：查 308（`PUBLIC_URL`、Host、OpenNext config）；目标 prod **200** HTML。
5. 全局：确认 `~/.local/bin/celld` = submodule `54957b2+`；清理僵死 celld。

---

## 9. 2026-09-03 更新（wasm 修复后）

- **celld** `54957b2`：`read_wasm_modules_from_dir` · 已 push。
- **cellp** `fae65b7`：submodule + PD-08 **fixed**（wasm）；S30 矩阵仍 **不支持**（308）。
- 完整 explorer 长文见会话记录；本文为仓库内可维护摘要。

---

## 10. 复验结果（2026-09-03 · P0 实施）

| ID | 变更 | 复验 |
|----|------|------|
| **S30** | 同上 + celld `445569a` `collapse_request_path_and_query` | **v10** promote 后 prod 仍 **308** `Location: ?` · **`docs/evidence/verify-full-20260903.log`** |
| **S29** | **celld** `no_bundle` 递归收录 `.cellp-bundle/**/*.js` | deploy **v6** health timeout **8834** · load `export named 'r'` · **FAIL** |
| **S28** | deploy **v4** | health timeout **8833** · load `process.stdin` unenv · **FAIL** |
| **S27** | — | 未做（余力外） |

**celld 提交：** `no_bundle` sibling ESM + `request_url` 路径 `//` 折叠（防御性）。
