'use strict';

const { spawnSync } = require('child_process');
const fs = require('fs');
const path = require('path');
const { platformKey } = require('./resolve-binary');

const root = path.resolve(__dirname, '../..');
const dist = path.join(root, 'dist');
fs.mkdirSync(dist, { recursive: true });

const key = platformKey();
const out = path.join(dist, `xcautokit-${key}`);
const alias = path.join(root, 'xcautokit');

const go = spawnSync('go', ['build', '-o', out, '.'], {
  cwd: root,
  stdio: 'inherit',
  env: process.env,
});

if (go.status !== 0) {
  process.exit(go.status || 1);
}

fs.copyFileSync(out, alias);
fs.chmodSync(out, 0o755);
fs.chmodSync(alias, 0o755);
console.log(`built ${out}`);
