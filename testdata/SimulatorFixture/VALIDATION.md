# Local verification — 2026-09-26

Environment: Apple Silicon macOS, Xcode 26.2 (17C52), iPhone 17 simulator running
iOS 26.2. These results concern this fixture and local tool execution. They do
not measure a host model, model decision time, competitor tools, or arbitrary
application reliability.

## Observed checks

- The updated MCP built the fixture, resolved its matching application artifact,
  installed it, and launched it on the explicitly selected simulator.
- Two additional build-and-run calls in separate MCP processes reused the same
  default build-cache directory. Both installed and launched the fresh artifact;
  the observed cold and warm calls took 12.911 seconds and 3.256 seconds.
  This demonstrates cache reuse across a server restart, not a general build
  speed guarantee.
- Twelve live integration checks passed: cross-process reservations and renewal;
  reads during another process's reservation; ambiguous and disabled target
  refusal without side effects; completed taps with failed postconditions
  without repetition; verified workflow save and replay; immutable workflow
  names; failure stopping later steps; cross-process trace and screenshot reads;
  parameterized typing with delayed UI verification; explicit decline of an
  app-local test alert; actual long press; literal option-like text input;
  release and subsequent reclamation.
- Twenty protocol/read-only checks passed, including native MCP image delivery,
  tool schemas, handled failures, prompts/resources, isolated session defaults,
  and tool-group agreement between CLI and MCP.
- The retained screenshot was visually inspected: the fixture's submitted name,
  saved-result label, counter, duplicate controls, and disabled control appeared
  in the expected layout.
- Full Go race tests and vet passed locally. Linux CI is configured to run these
  checks plus the simulator-independent MCP smoke suite; no hosted CI result is
  claimed here.
- The npm build now atomically replaces its local executable. The packaged
  binary passed protocol smoke after replacement; this avoids overwriting an
  executable inode still in use by an existing MCP process.

## Small local timing comparison

Each trial resets the fixture counter and verifies zero, then increments and
verifies one. All methods use exact selectors and conditions. Three successful
trials per method were run with rotating order. Setup, pre-trial reset, lease
management, and launch were excluded equally; workflow screenshots were disabled.
Response size counts complete JSON-RPC response lines, including both text and
structured fields. These are medians, not statistical guarantees.

| Current execution path | Tool calls | Wall time | Response JSON bytes |
|---|---:|---:|---:|
| Separate gesture and wait calls | 4 | 2,012.652 ms | 13,376 |
| Two `ui_act` calls | 2 | 1,326.025 ms | 13,094 |
| One `workflow_run` call | 1 | 1,255.164 ms | 8,011 |

The workflow used 75% fewer tool calls, about 38% less local execution time, and
about 40% less response data in this fixture. All paths use the updated server;
this is a comparison of its invocation patterns, not an old-release benchmark.
Host reasoning latency and billed tokens were not measured.

Reproduce with `scripts/mcp_benchmark.py --binary PATH --simulator UUID
--repetitions 3 --output RESULTS.json`, after provisioning the fixture on a
dedicated simulator. See the fixture README for setup.

## Limits

Live Xcode IDE bridge tools require Xcode 26.3+ and were not exercised on this
machine. Structural assertions and screenshots establish only the observed
fixture behavior. Real project flows still need their own runtime checks.

During the pre-fix executable replacement check, two old `version` probes entered
an uninterruptible macOS exit state. Termination was requested, but the OS still
listed those probes at the last cleanup check. They run no simulator operations;
the atomically replaced binary passed the subsequent suites.
