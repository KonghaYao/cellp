import { sha256 } from '@noble/hashes/sha256'

/** cellp dev: HTTP + nip.io/ingress.local 非 secure context，Web Crypto 不可用 */
export async function digestSha256(data: ArrayBuffer | Uint8Array): Promise<Uint8Array> {
  const bytes = data instanceof ArrayBuffer ? new Uint8Array(data) : data
  if (globalThis.crypto?.subtle && globalThis.isSecureContext) {
    const out = await crypto.subtle.digest('SHA-256', bytes as BufferSource)
    return new Uint8Array(out)
  }
  return sha256(bytes)
}
