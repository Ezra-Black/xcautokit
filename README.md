# XCAutokit

MCP server and CLI for Xcode and iOS Simulator work. Build and launch apps, inspect their UI, perform explicit actions, verify expected states, and return screenshots to your AI host.

**The host owns the AI.** XCAutokit runs deterministic local operations. It does not create agents, call models, request MCP sampling, or require provider keys. Codex, Claude, Cursor, or another host can plan the work and delegate analysis using their own capabilities.

npm: **`xcautokit`** · [Website](https://ezra-black.github.io/xcautokit/)

## Install

Runtime requires macOS and Xcode with the relevant simulator runtime. Install [AXe](https://www.axe-cli.com/docs) for accessibility inspection and UI input. Screenshots can fall back to `simctl`.

Add this server to your MCP client's configuration:

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

Or install the CLI:

```bash
npm install -g xcautokit@latest
xcautokit doctor
xcautokit tools
```

For live IDE tools, use **Xcode 26.3+**, open the project, and enable **Settings → Intelligence → Allow external agents to use Xcode tools**. XCAutokit uses `xcrun mcpbridge` for that separate IDE surface. `build_sim`, `test_sim`, and `build_run_sim` use `xcodebuild` with the requested project, configuration, and destination.

## Set up a project safely

```bash
xcautokit init --dir /path/to/project
```

This adds an XCAutokit entry to `.cursor/mcp.json`, preserving other servers and top-level settings. An existing XCAutokit entry is preserved too. Invalid configuration is reported before setup files are written.

It creates `AGENTS.md` and `.agents/skills/xcautokit/SKILL.md` only when those files are absent. Existing project guidance and customized skills remain intact. It refuses symlinked configuration paths within the destination. Reload MCP and skills in your host after setup. For clients using a different MCP configuration location, copy the server entry above into that client's configuration.

The [companion skill](skills/xcautokit/SKILL.md) guides the **host** through inspection, explicit actions, verification, and delegation. A host without skills can use the server's initialize instructions, prompts, and `xcautokit://agent-guide` resource.

## Inspect, act, verify

1. Choose a simulator with `device_list`. Supply `simulatorUuid` explicitly when more than one device is booted.
2. Build/install/launch with `build_run_sim`, or launch an already installed app with `app_launch`.
3. Read `ui_summary`, then locate a specific control with `ui_find`. Inspect `ui_describe` when you need hierarchy details.
4. Use `ui_act` to tap a semantic selector and optionally wait for an expected result. Use `ui_wait` for an expected condition without sending input.
5. Inspect `screenshot` for visual framing and states that accessibility data cannot establish.

An example `ui_act` call:

```json
{
  "simulatorUuid": "SIMULATOR-UUID",
  "selector": {"by": "accessibilityId", "query": "settingsButton"},
  "waitFor": {
    "selector": {"by": "accessibilityId", "query": "settingsTitle"},
    "state": "present"
  },
  "timeoutMs": 10000
}
```

Selectors use `by`, `query`, optional `match` (`auto`, `exact`, `contains`), and optional `role`. Prefer accessibility identifiers. An ambiguous target should be refined rather than resolved by guessing. Wait states are `present`, `absent`, and `enabled`; timeouts are bounded at 60 seconds.

A returned input event is not proof of the intended outcome. Read the observation and assertion result. App autocorrection and smart punctuation can transform typed text; verify the resulting field or submitted value. If input was performed but the following check failed, inspect the current state before retrying: a second tap might toggle or submit again.

Text entry currently supports printable ASCII, tabs, and line breaks through AXe. Unsupported characters are rejected before text is entered; Unicode typing is not supported by this input path.

### Interrupts and screenshots

Launch results and UI observations report interrupts. Input tools refuse blocked interactions. Use `ui_dismiss_interrupt` with an explicit `action` (`accept`, `decline`, `dismiss`, or `button`), then inspect again. Never silently accept permissions. SpringBoard requires launching the app, not dismissing an alert.

`screenshot` returns native MCP image content plus structured path, dimensions, MIME type, and simulator metadata. Use `image: false` for metadata and a saved path only. Visual interpretation stays with the host.

Screenshot dimensions are **pixels**. Accessibility frames and input coordinates use simulator **UI points**. Do not feed image pixel coordinates directly into a tap without accounting for display scale and orientation; prefer semantic selectors.

## Replay explicit workflows

`workflow_run` executes a supplied list of **1–30 steps**, or a previously saved workflow name. Supported steps are `tap`, `wait`, `type_text`, `swipe`, `button`, and `launch`. It stops at the first failure and does not invent recovery steps or retry mutations.

```json
{
  "simulatorUuid": "SIMULATOR-UUID",
  "steps": [
    {
      "action": "tap",
      "selector": {"by": "accessibilityId", "query": "settingsButton"},
      "waitFor": {
        "selector": {"by": "accessibilityId", "query": "settingsTitle"},
        "state": "present"
      }
    }
  ],
  "timeoutMs": 30000
}
```

The default total timeout is 60 seconds, with a maximum of 120 seconds. Each step has its own bounded timeout. Results include the run ID, per-step outcomes, a trace path, and a screenshot path when capture succeeds. `traceURI` and `screenshotURI` expose that evidence through MCP resources at `xcautokit://runs/{runId}` and `xcautokit://runs/{runId}/screen`. `verified` means the declared final UI condition passed; it is not a visual quality verdict or proof of unstated requirements.

After a successful run ending in a `wait` or a tap with `waitFor`, use `workflow_save` with `runId`, a new `name`, and optional `description`. It never overwrites an existing name. Inspect saved recipes with `workflow_list`, then replay with `workflow_run` using `workflow: "name"`.

For variable text, use `textFrom: "fieldName"` in a `type_text` step and supply `inputs: {"fieldName": "value"}` at execution. Inputs are not saved into workflow definitions. **Traces, observations, and screenshots may still contain visible app data, including values typed during the run.** Inspect artifacts before sharing them.

## Coordinate host-owned agents

Parallelize independent work: source investigation, log analysis, review of captured screenshots, or separate simulators. Keep one driver per live simulator.

- `device_claim` takes `simulatorUuid`, an `owner` label, and optional `ttlSeconds` (default 300; range 10–900). It returns a `leaseToken`.
- Pass `leaseToken` to simulator mutations while the device is claimed. Renew with `device_claim` using the same token before expiry.
- `device_lease_status` reports ownership without exposing the token.
- `device_release` takes the simulator UUID and token when the driver is finished.

Leases coordinate XCAutokit processes using the same local state directory. They cannot prevent input from other automation tools or a person. They are coordination, not permission to perform an action. Live IDE operations such as `run_some_tests` and `swift_snippet` use Xcode's active context and are outside UUID-based simulator leases; coordinate those through the host.

Builds reuse a stable DerivedData directory keyed by project, scheme, configuration, and destination, including after an MCP restart. A cancellable cross-process lock serializes XCAutokit access to a shared build directory and spans the full build/install/launch sequence. External Xcode and `xcodebuild` processes do not participate in this lock. Use separate `derivedDataPath` values when independent jobs need to build in parallel.

## Tool groups, state, and captures

| Task | Tools |
|---|---|
| Device | `status`, `device_list`, `device_boot`, `device_shutdown`, `open_sim` |
| UI observation | `ui_summary`, `ui_describe`, `ui_find`, `ui_search`, `ui_point`, `ui_wait` |
| UI actions | `ui_act`, `tap`, `swipe`, `type_text`, `gesture`, `long_press`, `button`, `key_press` |
| Interrupts | `ui_check_interrupt`, `ui_dismiss_interrupt` |
| Capture | `screenshot`, `record_start` / `record_stop`, `start_sim_log_cap` / `stop_sim_log_cap` |
| Project and build | `discover_projects`, `list_schemes`, `build_sim`, `build_run_sim`, `test_sim`, `clean` |
| Coordination | `device_claim`, `device_lease_status`, `device_release` |
| Recipes | `workflow_run`, `workflow_save`, `workflow_list` |
| Live Xcode | `xcode_windows`, `xcode_issues`, `xcode_build_log`, `xcode_preview`, `docs_search`, `swift_snippet`, `run_some_tests` |

`xcautokit tools` lists the currently enabled catalog. Reduce the tools exposed to the host with `XCAUTOKIT_WORKFLOWS=core` (omit live Xcode tools), or select groups such as `device,ui,input,capture`. The CLI and MCP expose the same filtered catalog.

Session defaults are **process-local by default**. `session_set_defaults`, `session_show_defaults`, and `session_clear_defaults` manage project/scheme/device conveniences. To opt into persistence, set `XCAUTOKIT_SESSION_FILE` to an explicit project/agent-specific path. Older automatic use of `~/.xcautokit/session_defaults.json` is no longer the default.

Recording and log-capture start tools return **process-local tickets**. Pass each ticket to its matching stop tool. A server restart invalidates old tickets; start a fresh capture.

| Environment variable | Purpose |
|---|---|
| `XCAUTOKIT_XCODE_BACKEND=auto\|on\|off` | Live Xcode bridge mode |
| `MCP_XCODE_PID` | Select the live Xcode process |
| `XCAUTOKIT_WORKFLOWS` | Filter tool groups |
| `XCAUTOKIT_SESSION_FILE` | Opt-in session-default persistence |
| `XCAUTOKIT_STATE_DIR` | Lease, trace, workflow, and default DerivedData storage; defaults to `~/.xcautokit` |
| `XCAUTOKIT_INTERRUPT_GUARD=off` | Explicitly disable the input interrupt guard |

## Develop and verify

```bash
npm install
npm run build:go
npm link
```

Go **1.25+** builds the server. Python **3.10+** runs the portable smoke test with no additional packages. Linux CI checks the deterministic core and protocol; simulator operations require macOS.

```bash
go test -race ./...
go vet ./...
go build -o xcautokit .
python3 scripts/mcp_smoke_test.py
```

The default smoke run checks MCP initialization, tool schemas, resources, prompts, isolated session state, fixture project discovery, handled errors, and CLI/MCP filter agreement. It does not need Xcode and does not mutate a simulator.

```bash
python3 scripts/mcp_smoke_test.py --binary ./xcautokit --simulator SIMULATOR-UUID
```

The optional live mode observes an already booted device and verifies one temporary screenshot's native MCP image delivery. It never sends input or dismisses prompts. This checks protocol and observation plumbing; meaningful application changes still need direct runtime and visual verification.

For input and workflow integration tests, follow [testdata/SimulatorFixture/README.md](testdata/SimulatorFixture/README.md) to install the `com.xcautokit.fixture` test app on a **dedicated simulator**, then run:

```bash
python3 scripts/mcp_live_test.py --simulator SIMULATOR-UUID --artifacts-dir /tmp/xcautokit-evidence
```

This separate suite changes the fixture UI. It checks two-process ownership, ambiguous selectors, postcondition failures without repeated taps, saved workflow replay, evidence resources, delayed text submission, explicit dismissal of the fixture's harmless alert, and an actual long press. It uses isolated local state and never creates, erases, boots, or shuts down a simulator. Retained traces and screenshots are evidence for host review, not an automated visual-quality verdict.

Measure equivalent counter flows on that dedicated fixture:

```bash
python3 scripts/mcp_benchmark.py --binary ./xcautokit --simulator SIMULATOR-UUID --repetitions 3 --output /tmp/xcautokit-benchmark.json
```

The benchmark compares two legacy gesture-and-wait pairs (4 calls), two `ui_act` calls (2), and one `workflow_run` with screenshots disabled (1). Each verifies the same reset/increment outcomes. It records elapsed wall time, response JSON bytes, success, call counts, and medians; setup and pre-trial reset are excluded consistently. Repetitions are bounded at 10 and method order rotates. These are local tool measurements, with no model latency, billed-token, or agent-speed claim.

## License

MIT
