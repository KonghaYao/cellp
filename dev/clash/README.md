# Clash / Clash Verge Rev — cellp dev（lvh.me 直连）

系统代理会把 `lvh.me` 送到远端节点，访问本机 Gateway **8787** 时出现 **HTTP 502**；终端 `curl` 不走代理则正常。

## 系统代理绕过（浏览器 502 时必做）

Clash Verge → **设置** → **系统代理** → **绕过域名 / Bypass**，增加一行：

```text
*.lvh.me,127.0.0.1,localhost
```

保存并**重新加载配置**。终端 `curl` 不走代理所以 200，Chrome 走代理时未绕过会得到 **502**。

## Clash Verge Rev

将仓库模板合并进当前 profile 后 **重新加载配置**：

| 仓库文件 | 用途 |
|----------|------|
| [cellp-verge-rules-prepend.yaml](./cellp-verge-rules-prepend.yaml) | `prepend:` 直连规则 |
| [cellp-verge-merge-dns.yaml](./cellp-verge-merge-dns.yaml) | `fake-ip-filter` 用真实 A 记录 |

规则组名须与订阅一致（模板为 `🎯 直连`）。

## 自测

```bash
curl -x http://127.0.0.1:7897 -w '%{http_code}\n' -o /dev/null \
  'http://support-flaremo.lvh.me:8787/'
# 期望 200
```
