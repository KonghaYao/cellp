#!/usr/bin/env bash
set -euo pipefail
VITE="${1:?vite.config.ts}"
FILE="$VITE" node <<'NODE'
const fs = require('fs');
const p = process.env.FILE;
let s = fs.readFileSync(p, 'utf8');
if (s.includes('CELLP_VITE_BASE')) process.exit(0);
const needle = 'export default defineConfig({';
if (!s.includes(needle)) throw new Error('defineConfig not found');
s = s.replace(needle, `${needle}\n  base: './', /* CELLP_VITE_BASE */`);
fs.writeFileSync(p, s);
console.log('patched vite base ./', p);
NODE
