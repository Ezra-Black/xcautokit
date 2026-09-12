# XCAutokit

First-party **MCP server + CLI** for Xcode and iOS Simulator automation.

Install the same way as [XcodeBuildMCP](https://www.xcodebuildmcp.com/#get-started): `npx` for MCP clients, or a global CLI.

npm package: **`xcautokit`** · Brand: **XCAutokit** · Site: [ezra-black.github.io/xcautokit](https://ezra-black.github.io/xcautokit/)

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

## CLI

| Command | Purpose |
|---------|---------|
| `xcautokit mcp` | Start MCP server (stdio) |
| `xcautokit tools` | List tools |
| `xcautokit doctor` | Check axe / Xcode / mcpbridge |
| `xcautokit status` | Simulator + backend JSON |
| `xcautokit interrupt check\|dismiss` | CLI interrupt helpers |
| `xcautokit init` | Write `AGENTS.md` + MCP snippet |
| `xcautokit version` | Print version |

## Modern MCP surface

XCAutokit is a full MCP server (official Go SDK):

- **Tools** with JSON Schema inputs, structured results, and annotations (`readOnlyHint` / `destructiveHint`)
- **Resources** (`simulator://…`, `xcode://status`, `xcautokit://agent-guide`)
- **Prompts** (`build-and-verify`, `ui-explore`, `fix-failing-test`, `handle-interrupt`)
- **Initialize instructions** so interrupt handling ships to every client

### Tickets (process-local)

`record_start` / `start_sim_log_cap` return a **ticket** string. Pass it to the matching stop tool.

Tickets live in the MCP process only. If the server restarts, start a new capture. Project/scheme convenience still uses `session_set_defaults` (`~/.xcautokit/session_defaults.json`).

### Optional workflow filter

Reduce tool catalog context:

```bash
# Omit live Xcode tools
XCAUTOKIT_WORKFLOWS=core

# Or pick groups: device,ui,input,app,capture,project,build,xcode
XCAUTOKIT_WORKFLOWS=device,ui,input,build
```

## Xcode live backend

With **Xcode 26.3+**, open your project and enable **Settings → Intelligence → Allow external agents to use Xcode tools**. XCAutokit uses `xcrun mcpbridge` internally when available; otherwise it falls back to `xcodebuild`.

Env: `XCAUTOKIT_XCODE_BACKEND=auto|on|off` (legacy `AUTOKIT_XCODE_BACKEND` still accepted).

## License

MIT
