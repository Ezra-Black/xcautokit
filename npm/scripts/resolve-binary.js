'use strict';

const fs = require('fs');
const path = require('path');
const os = require('os');

function platformKey() {
  const platform = os.platform();
  const arch = os.arch();
  if (platform !== 'darwin') {
    throw new Error('xcautokit currently supports macOS only (darwin)');
  }
  if (arch === 'arm64') return 'darwin-arm64';
  if (arch === 'x64') return 'darwin-amd64';
  throw new Error(`unsupported arch: ${arch}`);
}

function resolveBinary() {
  // Package root is two levels up from npm/scripts
  const root = path.resolve(__dirname, '../..');
  const key = platformKey();
  const name = `xcautokit-${key}`;
  const candidates = [
    path.join(root, 'dist', name),
    path.join(root, 'npm', 'dist', name),
    path.join(root, 'xcautokit'),
    // Legacy Autokit build name
    path.join(root, 'dist', `autokit-${key}`),
  ];
  for (const c of candidates) {
    if (fs.existsSync(c)) return c;
  }
  return candidates[0];
}

module.exports = { resolveBinary, platformKey };
