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
const targets = process.argv.includes('--release')
  ? ['darwin-arm64', 'darwin-amd64']
  : [key];

for (const target of targets) {
  const targetPath = path.join(dist, `xcautokit-${target}`);
  const go = spawnSync('go', ['build', '-trimpath', '-o', targetPath, '.'], {
    cwd: root,
    stdio: 'inherit',
    env: { ...process.env, GOOS: 'darwin', GOARCH: target.slice('darwin-'.length), CGO_ENABLED: '0' },
  });
  if (go.status !== 0) {
    process.exit(go.status || 1);
  }
  fs.chmodSync(targetPath, 0o755);
  console.log(`built ${targetPath}`);
}

// Never overwrite the inode of an executable that an MCP process may still be
// running. Apart from corrupting its mapped pages, in-place replacement can
// leave macOS with stale code-signature state. Rename a new file atomically.
const stage = fs.mkdtempSync(path.join(dist, '.alias-'));
try {
  const stagedAlias = path.join(stage, 'xcautokit');
  fs.copyFileSync(out, stagedAlias, fs.constants.COPYFILE_EXCL);
  fs.chmodSync(stagedAlias, 0o755);
  fs.renameSync(stagedAlias, alias);
} finally {
  fs.rmSync(stage, { recursive: true, force: true });
}
console.log(`updated local executable ${alias}`);
