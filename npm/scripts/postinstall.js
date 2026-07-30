'use strict';

const { spawnSync } = require('child_process');
const fs = require('fs');
const path = require('path');
const { resolveBinary } = require('./resolve-binary');

const root = path.resolve(__dirname, '../..');
const existing = resolveBinary();

if (fs.existsSync(existing)) {
  process.exit(0);
}

const goCheck = spawnSync('go', ['version'], { encoding: 'utf8' });
if (goCheck.status !== 0) {
  console.warn(
    '[xcautokit] Go toolchain not found and no prebuilt binary is present.\n' +
      'Install Go 1.25+ (https://go.dev/dl) then run: npm run build:go\n' +
      'Or use a release tarball that includes dist/xcautokit-darwin-*'
  );
  process.exit(0);
}

const build = spawnSync('node', [path.join(__dirname, 'build-go.js')], {
  cwd: root,
  stdio: 'inherit',
});
process.exit(build.status || 0);
