#!/usr/bin/env bash
# Package the SPM binary into a minimal .app so macOS enables native full screen, menus, and Accessibility prompts.
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
swift build -c debug
BIN="$ROOT/.build/debug/XCAutokitStudio"
APP="$ROOT/.build/XCAutokit Studio.app"
rm -rf "$APP"
mkdir -p "$APP/Contents/MacOS" "$APP/Contents/Resources"
cp "$BIN" "$APP/Contents/MacOS/XCAutokitStudio"
cp "$ROOT/Info.plist" "$APP/Contents/Info.plist"
chmod +x "$APP/Contents/MacOS/XCAutokitStudio"
open "$APP"
