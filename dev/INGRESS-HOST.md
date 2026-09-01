# Dev Ingress Host（AD-12 本机 / 局域网）

> 权威路由设计：[../docs/plans/INGRESS-ROUTING.md](../docs/plans/INGRESS-ROUTING.md)

## 统一约定

| 项 | 值 |
|----|-----|
| Gateway | `http://<host>:8787/`（**必须带端口**，dev 无 TLS） |
| Preview Host | `{version}.{project}.{baseDomain}` |
| Prod Host | `{project}.{baseDomain}` |
| 选路 | **Host 头** → `ingress_bindings` → celld upstream |

**两种 base domain 模式（二选一，改后需 `reset.sh` + `up.sh` + seed）：**

| 模式 | `CELLP_INGRESS_BASE_DOMAIN` | 本机 | 局域网同事 | Clash |
|------|----------------------------|------|------------|-------|
| **A. local（默认）** | `ingress.local` | `/etc/hosts` → `127.0.0.1` | 每人 hosts → 你的 LAN IP | `*.local` 通常已直连 |
| **B. magic DNS** | `{ip-dashes}.nip.io` 或 `{ip}.sslip.io` | 免 hosts | 免 hosts，开 URL 即可 | 需 [clash 直连规则](./clash/README.md) |

初始化（同步 `dev/.env` + `web/.env`）：

```bash
./dev/scripts/ingress-host-init.sh local    # 默认
./dev/scripts/ingress-host-init.sh magic    # 自动 sslip / nip + 打印 Clash 说明
./dev/scripts/reset.sh && ./dev/scripts/up.sh && ./dev/scripts/seed-demo.sh
```

## 模式 A：`ingress.local`（推荐本机 + 已用 Clash 的开发者）

`/etc/hosts` 一行（示例）：

```text
127.0.0.1 demo-app.ingress.local v1.demo-app.ingress.local commerce-store.ingress.local v1.commerce-store.ingress.local
```

浏览器：`http://demo-app.ingress.local:8787/`

局域网：把 `127.0.0.1` 换成你的 **LAN IP**，同事各自加同样 hosts 行。

## 模式 B：magic DNS（局域网免 hosts）

```bash
./dev/scripts/ingress-host-init.sh magic
# 或 ./dev/scripts/ingress-magic-dns-enable.sh --nip
```

示例 URL：`http://demo-app.192-168-12-36.nip.io:8787/`

**Clash / 系统代理：** 未直连时浏览器会 **502**，curl 仍 200。见 [dev/clash/README.md](./clash/README.md)。

## 环境变量（`dev/.env`）

```bash
CELLP_INGRESS_BASE_DOMAIN=ingress.local
GATEWAY_URL=http://127.0.0.1:8787
CELLP_PUBLIC_SCHEME_PREVIEW=http
CELLP_PUBLIC_SCHEME_PROD=http   # 非 443 端口时 API 勿生成 https://…:8787
INGRESS_HOST_ONLY=1
```

Dashboard 同步：

```bash
VITE_CELLP_INGRESS_BASE_DOMAIN=ingress.local   # 与 cellpd 一致
VITE_CELLP_GATEWAY_URL=http://127.0.0.1:8787
```

## Dashboard 与 Worker

- **API-only**（如 `demo-app`）：Production 链到 Dashboard，不打开 gateway 根 JSON。
- **HTML  storefront**（`commerce-store`）：开发态走 Vite `/__gateway?__cellp_host=…`。
- 面板 **「局域网 Preview」** 块：可复制 URL、hosts 行、Clash 提示。

## 验证

```bash
./dev/scripts/health.sh
curl -sS -H "Host: demo-app.ingress.local" http://127.0.0.1:8787/health
# magic 模式把 Host 换成 API 返回的 prod/preview 主机名
```
