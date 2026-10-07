---
name: xcautokit
description: Build, inspect, drive, and verify Apple simulator apps with XCAutokit, including coordinated simulator work by host-owned agents.
---

# XCAutokit

Use the connected XCAutokit tools for Xcode and iOS Simulator work. XCAutokit executes deterministic operations; planning, interpretation, vision, and any subagents belong to the host. It never runs its own agents or calls AI providers.

## Inspect, act, verify

- Discover the current tools and select an explicit simulator UUID when multiple devices are available. Session defaults are local to an MCP process unless persistence is explicitly configured.
- Prefer `build_run_sim` for a build/install/launch request. A successful build is not runtime verification: inspect the launched app and test the requested behavior.
- Start UI work with `ui_summary`; use `ui_find` for a particular control and `ui_describe` for hierarchy details. Use the actual tool schemas, since clients may expose a filtered tool set.
- Prefer `ui_act` with a selector over guessed coordinates. Review its observed result before choosing the next action. Use `ui_wait` for an expected UI condition instead of fixed sleeps.
- When a result reports an interrupt, choose an explicit `ui_dismiss_interrupt` action consistent with the user's task. Never silently accept a permission. SpringBoard is an app-launch problem, not an alert to dismiss.
- Use `screenshot` for visual framing and states not represented in accessibility data. Report what was directly observed separately from what passed a structural check.
- Screenshot dimensions are pixels; accessibility frames and input coordinates are UI points. Prefer selectors rather than copying image pixel positions into input calls.
- AXe text entry supports printable ASCII, tabs, and line breaks. Treat rejected Unicode as unsupported input, not a reason to keep retrying.
- An action may have executed even if its following observation failed. Inspect current state before retrying an action that could submit, toggle, purchase, or delete anything.

## Deterministic workflows

Use `workflow_run` for explicit steps and assertions whose intent is already known. Keep batches bounded and stop on failed checks or unexpected interrupts. The tool does not infer goals or invent steps. A saved workflow is a reusable execution recipe, not permission to perform its actions in another context.

## Coordination by the host

Use host-owned subagents only when the host supports them and the task benefits. Good parallel assignments include reviewing captured screenshots, analyzing logs, investigating source code, and running independent checks on separate simulators.

One agent controls each simulator at a time. Use `device_claim`, `device_lease_status`, and `device_release` according to their schemas; pass the lease token when mutating a claimed device. Renew before expiry and release when finished. Leases coordinate XCAutokit processes sharing the same state directory; they do not lock other tools or people out. A lease is coordination, not authorization. Other agents should analyze shared captured evidence or use a different UUID. Do not let two agents alternate input on the same live UI.

Default DerivedData is stable per project, scheme, configuration, and destination. XCAutokit serializes operations sharing a build directory; use separate DerivedData paths for independent parallel builds. External Xcode builds do not participate in that lock. Coordinate live IDE `run_some_tests` and `swift_snippet` through the host because their active Xcode context is outside UUID-based simulator leases.

Give reviewers evidence tagged with its simulator, capture time, and relevant action/result so conclusions are tied to an observed state. Workflow trace and screenshot resources can contain visible app data; review before sharing them.

## Captures and recovery

Recording and log-capture start tools return process-local tickets. Pass each ticket to its matching stop tool. Restarting the MCP server invalidates those tickets.

On a stale target, refresh the UI and choose again. On a timeout or failed assertion, preserve the observed state and report the failing step. Do not treat a model's interpretation, an accepted input event, or a successful build as proof of the user's expected outcome.
