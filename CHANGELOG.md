# Changelog

## 2.0.0

XCAutokit executes deterministic operations. All AI planning, interpretation,
vision, and subagents remain with the connected host; the server runs no agents
and calls no models.

### Upgrade notes

- Session defaults are now isolated per MCP process. Set
  `XCAUTOKIT_SESSION_FILE` to an explicit project/agent-specific file to opt into
  persistence; the former shared defaults file is not loaded automatically.
- Ambiguous device names, multiple booted devices without an explicit selection,
  and ambiguous actionable selectors now fail instead of picking a target.
  Supply a simulator UUID and unique accessibility identifiers.
- Input stops when UI inspection fails or a blocking overlay is present.
  Interrupt dismissal requires an explicit action and reports its verification.
- Build, test, and build-and-run use the requested `xcodebuild` project and
  destination. Optional live Xcode IDE tools remain separate.
- Text input rejects characters unsupported by the AXe keyboard path. Verify
  actual field values when application autocorrection or smart punctuation is on.
- Restart the MCP connection after upgrading to load the new server.

### Added and improved

- `ui_act` and `ui_wait`: fresh semantic selectors, bounded polling, explicit
  postconditions, compact observations, and no automatic repetition of taps.
- `workflow_run`, `workflow_save`, and `workflow_list`: bounded explicit steps,
  stop-on-failure behavior, reusable verified workflows, and resource-accessible
  traces and screenshots.
- `device_claim`, `device_release`, and `device_lease_status`: simulator
  reservations across host calls and coordination across XCAutokit processes.
- Native MCP screenshot images, cancellation of child processes, stronger
  process-local capture-ticket cleanup, and more accurate failure outcomes.
- Build-and-run now builds, resolves the matching artifact, installs, and
  launches. Incremental build caches survive MCP restarts, with cancellable
  cross-process locks protecting shared build output.
- Correct long presses, literal option-like text entry, safer selector input,
  bounded UI summaries, and narrower interrupt detection.
- Safe project setup, an embedded host companion skill, protocol and live
  fixture checks, a reproducible local benchmark, and CI configuration.
- npm releases include current Apple Silicon and Intel macOS binaries and
  atomically replace the local executable during a build.
