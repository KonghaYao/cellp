# Support 应用生产 Ingress 用户验收（curl）

> **日期：** 2026-09-02  
> **环境：** 本地生产 ingress 模拟 · Gateway `http://127.0.0.1:8787` · `Host: support-<name>.lvh.me`  
> **栈健康：** `./dev/scripts/health.sh` → **OK**（gateway/platform/celld/rustfs 均通过）

## 验收标准

| 结论 | 条件 |
|------|------|
| **fail** | 连接失败、502、503、`ingress_unknown`、空 body |
| **warn** | `/` 为 404 但文档/已知备用入口可用 |
| **pass** | 200 HTML/JSON，或预期的 302/307（登录/仪表盘等） |

---

## 执行摘要

| ID | Host | GET `/` | `-L` 最终 | 结论 | 用户说明（中文） |
|----|------|---------|-----------|------|------------------|
| S01 | support-relay.lvh.me | 404 | 404 | **fail** | 网关返回 `ingress_unknown`，ingress 未注册或未部署（**2026-09-02 后已修**：repromote + seed → 200，请重跑本表） |
| S05 | support-flaremo.lvh.me | 200 | 200 | **pass** | 首屏 HTML 正常 |
| S06 | support-memos.lvh.me | 200 | 200 | **pass** | 首屏 HTML 正常 |
| S07 | support-monolith.lvh.me | 200 | 200 | **pass** | 首屏 HTML 正常 |
| S08 | support-edgeever.lvh.me | 200 | 200 | **pass** | 首屏 HTML 正常 |
| S09 | support-sonicjs.lvh.me | 302 | 200 | **pass** | `/` 重定向至 `/auth/login`，跟随重定向后登录页 HTML |
| S10 | support-nodewarden.lvh.me | 200 | 200 | **pass** | 首屏 HTML 正常 |
| S14 | support-cfbase.lvh.me | 307 | 500 | **fail** | `/` → `/dashboard` → `/login` 链最终在登录页 **500** |
| S15 | support-workflows.lvh.me | 200 | 200 | **pass** | SPA HTML 正常 |
| S17 | support-r2filebox.lvh.me | 200 | 200 | **pass** | 首屏 HTML 正常 |
| S18 | support-webhookflare.lvh.me | 200 | 200 | **pass** | 首屏 HTML 正常 |
| S19 | support-requestbin.lvh.me | 404 | 404 | **warn** | 根路径无路由；入口 **`/new`** → 302 → 200 JSON `[]` |
| S20 | support-r2explorer.lvh.me | 200 | 200 | **pass** | 首屏 HTML 正常 |
| S21 | support-fileworker.lvh.me | 200 | 200 | **pass** | 首屏 HTML 正常 |

**合计：11 pass · 1 warn · 2 fail**（共 14 个应用）

---

## 逐应用详情

### S01 support-relay

```bash
curl -sS -D - -o /tmp/body.txt -w "\nTIME:%{time_total}\n" \
  -H "Host: support-relay.lvh.me" \
  "http://127.0.0.1:8787/"
```

```
HTTP/1.1 404 Not Found
Content-Length: 16
```

Body: `ingress_unknown`

---

### S05 support-flaremo

```
HTTP/1.1 200 OK
Content-Length: 1576
TIME:0.000957
```

Body（前 200 字符）: `<!doctype html>\n<html lang="zh-CN">...`

---

### S06 support-memos

```
HTTP/1.1 200 OK
Content-Length: 280966
```

Body（前 200 字符）: `<!DOCTYPE html>\n<html lang='en'>...`

---

### S07 support-monolith

```
HTTP/1.1 200 OK
Content-Length: 2552
```

---

### S08 support-edgeever

```
HTTP/1.1 200 OK
Content-Length: 2119
```

---

### S09 support-sonicjs

```
HTTP/1.1 302 Found
Location: /auth/login
```

`-L` 最终: `HTTP/1.1 200 OK`, body 15783 字节登录页 HTML

---

### S10 support-nodewarden

```
HTTP/1.1 200 OK
```

---

### S14 support-cfbase

GET `/`: `307` → `Location: /dashboard`

`-L` 链: `307 /` → `303 /dashboard` → `500 /login`

GET `/login`: `500 Internal Server Error`, `Content-Length: 8337`, `X-Sveltekit-Page: true`

---

### S15 support-workflows

```
HTTP/1.1 200 OK
Content-Length: 1422
```

---

### S17 support-r2filebox

```
HTTP/1.1 200 OK
Content-Length: 1354
```

---

### S18 support-webhookflare

```
HTTP/1.1 200 OK
Content-Length: 25860
```

---

### S19 support-requestbin

GET `/`: `404`, body `404 Not Found`

GET `/new` with `-L`: 最终 `200 OK`, `Content-Type: application/json`, body `[]`

---

### S20 support-r2explorer

```
HTTP/1.1 200 OK
Content-Length: 2309
```

---

### S21 support-fileworker

```
HTTP/1.1 200 OK
Content-Length: 390
```

---

## 验证基线

### Check: 开发栈健康

**Command run:** `./dev/scripts/health.sh`

**Output observed:** 全部 `OK`（gateway 8787、platform 8790、celld、rustfs、registry file 等）

**Result: PASS**

---

## 总体结论

- **11 pass / 1 warn / 2 fail**
- 阻塞：S01（ingress_unknown）、S14（登录 500）
- S19 用户入口为 `/new`
