# Images

The Cloudflare **Images** binding (`env.IMAGES`) is available in **celld** with **partial** behavior. Transforms run locally in the celld process (Rust `image` crate)—celld does not call Cloudflare’s remote Images API. **cellp** does not add a Dashboard or operator API; declare and deploy like any other wrangler binding.

[Bindings](/concepts/bindings) · [Binding guides](./index) · [Configure bindings](/build/wrangler)

## What works

Typical resize / format pipeline:

```js
const out = await env.IMAGES
  .input(imageBytes)
  .transform({ width: 800, fit: 'scale-down' })
  .output({ format: 'webp', quality: 85 })
return out.response()
```

`input()` accepts `ReadableStream`, `ArrayBuffer`, views, `Blob`, or `Response`. `transform()` supports `width`, `height`, `fit`, `gravity`, `background`, `rotate` (multiples of 90), `flip`, `flop`. `output()` supports jpeg, png, webp, avif with `quality` on jpeg/webp.

## What does not

- No call to Cloudflare’s remote Images API or billing.
- Many transform options (`blur`, `sharpen`, `trim`, `border`, overlays / `draw()`) **fail closed**.
- Inputs larger than **32 MiB** are refused.

Images **do not branch** across preview child versions as a managed dataset (same pattern as Worker script: this version’s runtime only).

Details and method table: [celld cloudflare-compat — Images](https://github.com/KonghaYao/cellp/blob/main/celld/docs/cloudflare-compat.md#images).
