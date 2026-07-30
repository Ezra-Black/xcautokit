# XCAutokit

First-party **MCP server + CLI** for Xcode and iOS Simulator automation.

Install the same way as [XcodeBuildMCP](https://www.xcodebuildmcp.com/#get-started): `npx` for MCP clients, or a global CLI.

npm package name: **`xcautokit`** (available). Brand: **XCAutokit**.

## Install

### MCP server (recommended)

```json
{
  "mcpServers": {
    "xcautokit": {
      "command": "npx",
      "args": ["-y", "xcautokit@latest", "mcp"]
    }
  }
}
```

### Global CLI

```bash
npm install -g xcautokit@latest
xcautokit doctor
xcautokit tools
xcautokit mcp
```

### From this repo

```bash
npm install
npm run build:go
npm link
xcautokit doctor
xcautokit init
```

Local sales / build guide: open [`site/index.html`](site/index.html) in your browser.

## CLI

| Command | Purpose |
|---------|---------|
| `xcautokit mcp` | Start MCP server (stdio) |
| `xcautokit tools` | List tools |
| `xcautokit doctor` | Check axe / Xcode / mcpbridge |
| `xcautokit status` | Simulator + backend JSON |
| `xcautokit init` | Write `AGENTS.md` + MCP snippet |
| `xcautokit version` | Print version |

## Xcode live backend

With **Xcode 26.3+**, open your project and enable **Settings → Intelligence → Allow external agents to use Xcode tools**. XCAutokit uses `xcrun mcpbridge` internally when available; otherwise it falls back to `xcodebuild`.

Env: `XCAUTOKIT_XCODE_BACKEND=auto|on|off` (legacy `AUTOKIT_XCODE_BACKEND` still accepted).

## Modern MCP tickets

Stateful tools return a **ticket** handle (recording, log capture). Pass that ticket to the matching stop tool.

## License

MIT
