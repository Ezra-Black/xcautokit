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
  const root = path.resolve(__dirname, '../..');
  const key = platformKey();
  const candidates = [
    path.join(root, 'dist', `xcautokit-${key}`),
    path.join(root, 'xcautokit'),
    path.join(root, 'npm', 'dist', `xcautokit-${key}`),
  ];
  for (const c of candidates) {
    if (fs.existsSync(c)) return c;
  }
  return candidates[0];
}

module.exports = { resolveBinary, platformKey };
