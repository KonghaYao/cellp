#!/usr/bin/env node
/** Pin OpenNext stack to S30-proven versions (see dev/support-corpus/support-opennext/.../package.json). */
const fs = require('fs');
const pkgPath = process.argv[2];
if (!pkgPath) {
  console.error('usage: pin-package.mjs <package.json>');
  process.exit(1);
}
const j = JSON.parse(fs.readFileSync(pkgPath, 'utf8'));
j.dependencies = j.dependencies || {};
Object.assign(j.dependencies, {
  '@opennextjs/cloudflare': '1.14.0',
  next: '16.0.7',
  react: '19.2.1',
  'react-dom': '19.2.1',
});
j.devDependencies = j.devDependencies || {};
j.devDependencies.wrangler = '4.123.0';
j.scripts = j.scripts || {};
if (!j.scripts.build) j.scripts.build = 'next build';
j.scripts.deploy = 'opennextjs-cloudflare build && opennextjs-cloudflare deploy';
j.scripts.preview = 'opennextjs-cloudflare build && opennextjs-cloudflare preview';
fs.writeFileSync(pkgPath, JSON.stringify(j, null, 2) + '\n');
console.log('pin-package: pinned next/react/@opennextjs/cloudflare/wrangler (S30 corpus)');
