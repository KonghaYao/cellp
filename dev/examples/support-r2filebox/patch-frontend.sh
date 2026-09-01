#!/usr/bin/env bash
set -euo pipefail
F="${1:?request.ts}"
FILE="$F" node <<'NODE'
const fs = require('fs');
const p = process.env.FILE;
let s = fs.readFileSync(p, 'utf8');
if (s.includes('CELLp_API_PREFIX')) {
  if (!s.includes('support-r2filebox')) {
    s = s.replace(
      /const CELLp_API_PREFIX = \(\(\) => \{[\s\S]*?\}\)\(\);/,
      `const CELLp_API_PREFIX = (() => {
  if (/\\.support-r2filebox\\./.test(location.hostname)) return '';
  const m = location.pathname.match(/^(\\/support-r2filebox\\/v\\d+)/);
  return m ? m[1] : '';
})();`,
    );
    fs.writeFileSync(p, s);
    console.log('updated host ingress prefix', p);
  }
  process.exit(0);
}
const needle = 'const instance: AxiosInstance = axios.create({';
const ins = `const CELLp_API_PREFIX = (() => {
  if (/\\.support-r2filebox\\./.test(location.hostname)) return '';
  const m = location.pathname.match(/^(\\/support-r2filebox\\/v\\d+)/);
  return m ? m[1] : '';
})();

${needle}`;
if (!s.includes(needle)) throw new Error('needle not found');
s = s.replace(needle, ins);
s = s.replace("baseURL: '',", 'baseURL: CELLp_API_PREFIX,');
fs.writeFileSync(p, s);
console.log('patched', p);
NODE
