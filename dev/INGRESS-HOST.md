# Dev Ingress Host（AD-12）

> 设计：[../docs/plans/INGRESS-ROUTING.md](../docs/plans/INGRESS-ROUTING.md)  
> 踩坑：[../docs/evidence/support-validation-lessons.md](../docs/evidence/support-validation-lessons.md)

## 约定

| 项 | 值 |
|----|-----|
| Gateway | `http://<host>:8787/`（**必须带端口**） |
| Preview Host | `{version}.{project}.lvh.me` |
| Prod Host | `{project}.lvh.me` |
| DNS | `*.lvh.me` → `127.0.0.1`（本机，**与 LAN IP 变化无关**） |

## 一次性初始化

```bash
./dev/scripts/ingress-host-init.sh          # 默认 loopback = lvh.me
./dev/scripts/restart-cellpd.sh
./dev/scripts/ingress-repromote-support.sh  # 换 base 后必跑；或 reset+up+seed
```

浏览器示例：

- `http://support-flaremo.lvh.me:8787/`
- `http://v3.support-flaremo.lvh.me:8787/`（preview 若 404 先用 prod）

**不要**在地址栏使用裸 `http://127.0.0.1:8787/`（无匹配 Host → 404）。

## Clash

合并 [clash/cellp-verge-rules-prepend.yaml](./clash/cellp-verge-rules-prepend.yaml) 与 [cellp-verge-merge-dns.yaml](./clash/cellp-verge-merge-dns.yaml)（仅 `lvh.me`），重载配置。见 [clash/README.md](./clash/README.md)。

## `dev/.env`

```bash
CELLP_INGRESS_BASE_DOMAIN=lvh.me
GATEWAY_URL=http://127.0.0.1:8787
CELLP_PUBLIC_SCHEME_PREVIEW=http
CELLP_PUBLIC_SCHEME_PROD=http
INGRESS_HOST_ONLY=1
```

`web/.env`：`VITE_CELLP_INGRESS_BASE_DOMAIN=lvh.me`

## 验证

```bash
curl -sS -o /dev/null -w '%{http_code}\n' 'http://support-flaremo.lvh.me:8787/'
./dev/scripts/health.sh
```

## 可选：dev HTTPS（8788）

见原 AD-10 外层 TLS；本地 `ingress-tls-init.sh` + `ingress-tls-enable.sh`。`lvh.me` 需 mkcert 或 `CELLP_TLS_EXTRA_SAN`。

## 局域网同事

`lvh.me` 只解析到**本机**。同事访问你的机器需其浏览器指向你的 **LAN IP:8787** 且 Host 仍为 `{project}.lvh.me`（通常需在你机器上 hosts 或自建 DNS），或另行约定；**不再维护 nip/sslip 脚本**。
