# V0b PASS — offshoot × RustFS 全序列

> **Date:** 2026-08-29  
> **Result:** PASS  
> **Store:** `s3://cellp-offshoot/e2e`  
> **Endpoint:** `http://127.0.0.1:19000` (RustFS)

## 命令

```bash
export PATH="$HOME/go/bin:$PATH"
./e2e/scripts/v0d-offshoot-attach.sh
./e2e/scripts/v0b-offshoot-rustfs.sh
```

## 序列结果

| 步骤 | 状态 | 备注 |
|------|------|------|
| init | OK | store 已存在（CAS conflict 可忽略） |
| create + checkout + seed | OK | |
| checkpoint | OK | txid 2 |
| parallel fork ×2 | OK | fork-a / fork-b，无 CAS 冲突 |
| export | OK | `v0b-export-34955.db` |
| promote | OK | fork-a → main |
| destroy forks | OK | |
| verify export | OK | `SELECT v FROM kv` → `v0b` |

## 证据文件

- `docs/evidence/v0d-20260829-161828.log`
- `docs/evidence/v0b-20260829-161828.log`
- `docs/evidence/v0b-export-34955.db`

## 影响

- **AD-4 prod tier：** offshoot 可使用 RustFS `s3://cellp-offshoot`
- `v0b-deferred.md` 已移除；`v0b-offshoot-rustfs.sh` 失败时不再 soft-pass
