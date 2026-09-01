# Clash / Clash Verge Rev — cellp dev 直连规则

系统代理（如 mixed-port **7897**）会把 `nip.io` 等域名送到远端节点，访问本机 Gateway **8787** 时出现 **HTTP 502**；终端 `curl` 不走代理则正常。

## Clash Verge Rev（推荐）

当前订阅已挂 **规则增强** / **Merge** 时，把仓库模板拷进对应 profile 文件后 **重新加载配置**：

| 仓库文件 | 拷到 Verge |
|----------|------------|
| [cellp-verge-rules-prepend.yaml](./cellp-verge-rules-prepend.yaml) | `profiles/rdJemN9Rlqs6.yaml` 的 `prepend:` |
| [cellp-verge-merge-dns.yaml](./cellp-verge-merge-dns.yaml) | `profiles/mgOdLjTomcVb.yaml`（或你的 merge 文件） |

Verge 配置目录（macOS 示例）：

`~/Library/Application Support/io.github.clash-verge-rev.clash-verge-rev/profiles/`

**规则组名**须与订阅一致；模板使用 `🎯 直连`。若你的是 `DIRECT`，全局替换即可。

## 规则含义

- `DOMAIN-SUFFIX,nip.io` / `sslip.io` / `ingress.local` → **直连**（走 LAN / 本机 Gateway）
- DNS `fake-ip-filter` 对 `+.nip.io` 等用 **真实 A 记录**（避免 198.18.x.x 假 IP）

## 自测

```bash
curl -x http://127.0.0.1:7897 -w '%{http_code}\n' -o /dev/null \
  'http://demo-app.192-168-12-36.nip.io:8787/'
# 期望 200（把 host 换成你当前的 magic / local 域名）
```

## 不想改 Clash

- 用 **模式 A** `ingress.local`（Clash 常已 bypass `*.local`），或
- 临时关闭系统代理后再开浏览器 URL。
