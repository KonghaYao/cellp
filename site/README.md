# cellp docs (this folder)

VitePress source for **https://konghayao.github.io/cellp/**

```bash
cd site
pnpm install          # 根目录 .npmrc → https://registry.npmmirror.com
pnpm run docs:dev      # http://localhost:5173/cellp/
pnpm run docs:build
```

GitHub Actions (`.github/workflows/docs.yml`) builds and publishes on push to `main`.
