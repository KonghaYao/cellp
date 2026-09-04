#!/usr/bin/env node
/**
 * Merge Mastra deployer wrangler (.mastra/output/wrangler.json) with cellp overlay.
 * Writes patched config for wrangler dry-run inside .mastra/output/.
 */
import fs from 'node:fs';
import path from 'node:path';

const appDir = process.argv[2];
const overlayPath = process.argv[3];
if (!appDir || !overlayPath) {
  console.error('usage: merge-wrangler.mjs <appDir> <overlay.jsonc>');
  process.exit(1);
}

function readJsonc(file) {
  let raw = fs.readFileSync(file, 'utf8');
  raw = raw.replace(/^\s*\/\/.*$/gm, '').replace(/,\s*([}\]])/g, '$1');
  return JSON.parse(raw);
}

const overlay = readJsonc(overlayPath);
const outputDir = path.join(appDir, '.mastra/output');
const deployerWrangler = path.join(outputDir, 'wrangler.json');
if (!fs.existsSync(deployerWrangler)) {
  console.error(`missing ${deployerWrangler} — run mastra build first`);
  process.exit(1);
}

const generated = readJsonc(deployerWrangler);

const forDryRun = {
  ...generated,
  name: overlay.name ?? generated.name ?? 'support-mastra',
  main: generated.main ?? './index.mjs',
  compatibility_date:
    overlay.compatibility_date ?? generated.compatibility_date ?? '2026-01-15',
  compatibility_flags: [
    ...new Set([
      ...(generated.compatibility_flags ?? []),
      ...(overlay.compatibility_flags ?? ['nodejs_compat']),
    ]),
  ],
  d1_databases: overlay.d1_databases ?? generated.d1_databases,
  r2_buckets: overlay.r2_buckets ?? generated.r2_buckets,
  kv_namespaces: overlay.kv_namespaces ?? generated.kv_namespaces,
  vars: { ...(generated.vars ?? {}), ...(overlay.vars ?? {}) },
  alias: {
    ...(generated.alias ?? {}),
    ...(overlay.alias ?? {}),
  },
};

delete forDryRun.observability;
delete forDryRun.workers_dev;
delete forDryRun.routes;

const dryRunPath = path.join(outputDir, 'wrangler.cellp.json');
fs.writeFileSync(dryRunPath, `${JSON.stringify(forDryRun, null, 2)}\n`);

const rootWrangler = {
  name: forDryRun.name,
  main: '.cellp-bundle/index.js',
  compatibility_date: forDryRun.compatibility_date,
  compatibility_flags: forDryRun.compatibility_flags,
  assets: overlay.assets,
  d1_databases: forDryRun.d1_databases,
  r2_buckets: forDryRun.r2_buckets,
  kv_namespaces: forDryRun.kv_namespaces,
  vars: forDryRun.vars,
};

const rootJson = JSON.stringify(rootWrangler, null, 2).replace(
  /:\/\//g,
  ':\\u002f\\u002f',
);
fs.writeFileSync(path.join(appDir, 'wrangler.jsonc'), `${rootJson}\n`);

console.log(`merge-wrangler: ${dryRunPath} + ${path.join(appDir, 'wrangler.jsonc')}`);
