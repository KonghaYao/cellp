# cellp docs (this folder)

VitePress source for **https://konghayao.github.io/cellp/**

```bash
cd site
npm install          # .npmrc → https://registry.npmmirror.com
npm run docs:dev      # http://localhost:5173/cellp/
npm run docs:build
```

GitHub Actions (`.github/workflows/docs.yml`) builds and publishes on push to `main`.
