#!/usr/bin/env bash
# Secure download_session cookie on HTTPS ingress; fix download click (no window.open).
set -euo pipefail
ROOT="${1:?corpus root}"
SHARE="${ROOT}/worker/src/routes/share.ts"
VIEW="${ROOT}/frontend/src/views/share/View.vue"

FILE="$SHARE" node <<'NODE'
const fs = require('fs');
const p = process.env.FILE;
let s = fs.readFileSync(p, 'utf8');
if (!s.includes('requestIsSecure')) {
  s = s.replace(
    'function downloadSessionCookie(token: string, path: string, requestUrl: string, maxAge: number): string {',
    `function requestIsSecure(c: Context<{ Bindings: Bindings }>): boolean {
  const xf = c.req.header('X-Forwarded-Proto')?.split(',')[0]?.trim()
  if (xf) return xf === 'https'
  try {
    return new URL(c.req.url).protocol === 'https:'
  } catch {
    return false
  }
}

function downloadSessionCookie(token: string, path: string, secure: boolean, maxAge: number): string {`,
  );
  s = s.replace(
    'const secure = new URL(requestUrl).protocol === \'https:\' ? \'; Secure\' : \'\'',
    'const secureFlag = secure ? \'; Secure\' : \'\'',
  );
  s = s.replace(
    'return `download_session=${encodeURIComponent(token)}; HttpOnly${secure}; SameSite=Strict; Path=${path}; Max-Age=${maxAge}`',
    'return `download_session=${encodeURIComponent(token)}; HttpOnly${secureFlag}; SameSite=Lax; Path=${path}; Max-Age=${maxAge}`',
  );
  s = s.replace(
    /downloadSessionCookie\(token, downloadUrl, c\.req\.url, DOWNLOAD_SESSION_TTL_SECONDS\)/g,
    'downloadSessionCookie(token, downloadUrl, requestIsSecure(c), DOWNLOAD_SESSION_TTL_SECONDS)',
  );
  fs.writeFileSync(p, s);
  console.log('patched download_session cookie (Secure + SameSite=Lax)');
}
NODE

FILE="$VIEW" node <<'NODE'
const fs = require('fs');
const p = process.env.FILE;
let s = fs.readFileSync(p, 'utf8');
if (!s.includes('CELLp_DOWNLOAD_VIA_ANCHOR')) {
  s = s.replace(
    /const downloadFile = \(\) => \{[\s\S]*?showDownloadStarted\(\)\n\}/,
    `const downloadFile = () => {
  // CELLp_DOWNLOAD_VIA_ANCHOR: avoid window.open (popup/COOP); same-tab navigation keeps cookies
  if (!shareCode.value) return

  const url = shareData.value?.download_url
  if (!url) {
    ElMessage.error(t('shareView.networkFailed'))
    return
  }
  const absolute = url.startsWith('http') ? url : \`\${window.location.origin}\${url}\`
  const a = document.createElement('a')
  a.href = withDisposition(absolute, 'attachment')
  if (shareData.value?.file_name) {
    a.download = shareData.value.file_name
  }
  a.rel = 'noopener'
  document.body.appendChild(a)
  a.click()
  a.remove()
  showDownloadStarted()
}`,
  );
  fs.writeFileSync(p, s);
  console.log('patched share View download click');
}
NODE
