# XCAutokit

XCAutokit is the first-party MCP server for Xcode + iOS Simulator work in this environment. Prefer XCAutokit tools over any third-party Xcode/simulator MCP.

## Setup

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

Local checkout:

1. `npm install && npm run build:go && npm link`
2. MCP: `"command": "xcautokit", "args": ["mcp"]` (or `.cursor/mcp.local.json`)
3. For live Xcode IDE tools:
   - Install **Xcode 26.3+**
   - Open your project/workspace in Xcode
   - **Xcode → Settings → Intelligence → Model Context Protocol** → enable **Allow external agents to use Xcode tools**
4. Optional env: `XCAUTOKIT_XCODE_BACKEND=auto|on|off`, `MCP_XCODE_PID`

XCAutokit uses `xcrun mcpbridge` **internally** when available.

## Tickets (modern MCP)

`record_start` and `start_sim_log_cap` return a `ticket`. Pass that ticket to `record_stop` / `stop_sim_log_cap`.

## Tool routing

| Task | Tools |
|------|--------|
| Simulator device | `status`, `device_list`, `device_boot`, `device_shutdown`, `open_sim` |
| UI inspect | `ui_summary`, `ui_describe`, `ui_find`, `ui_search`, `ui_point` |
| UI interrupts | `ui_check_interrupt`, `ui_dismiss_interrupt` |
| UI input | `gesture`, `tap`, `swipe`, `type_text`, `long_press`, `button`, `key_press` |
| Capture | `screenshot`, `record_start` / `record_stop`, `start_sim_log_cap` / `stop_sim_log_cap` |
| Project | `discover_projects`, `session_set_defaults`, `list_schemes`, `build_sim`, `test_sim`, `clean` |
| Live Xcode (26.3+) | `xcode_windows`, `xcode_issues`, `xcode_build_log`, `xcode_preview`, `docs_search`, `swift_snippet`, `run_some_tests` |

## Popups / interrupts (shipped in MCP)

Customers get this without Cursor rules:

- **Initialize `instructions`** — every MCP client receives the interrupt workflow on connect
- **Resource** `xcautokit://agent-guide` and live `simulator://interrupts`
- **Auto-attach** — `app_launch` / `open_url` / `build_run_sim` / `launch_app_logs_sim` include `hasInterrupt` + `interrupts`
- **Input guard** — `tap` / `swipe` / `type_text` / `gesture` / keys refuse while a blocking overlay is present (disable with `XCAUTOKIT_INTERRUPT_GUARD=off`)

Workflow:

1. Read `hasInterrupt` from launch results, or call `ui_check_interrupt` / `ui_summary`.
2. If blocked, `ui_dismiss_interrupt` with explicit `action`: `accept` | `decline` | `dismiss` | `button`.
3. Re-check. Never silently auto-accept permissions.
4. SpringBoard → `button` home + `app_launch` (not dismiss).

## Prompts

- `build-and-verify`
- `ui-explore`
- `fix-failing-test`

## Resources

- `simulator://status`
- `simulator://devices`
- `simulator://config`
- `xcode://status`
