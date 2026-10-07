#!/usr/bin/env python3
"""Exercise the installed XCAutokit fixture on an explicitly supplied simulator.

This MUTATES only the fixture UI: resets/increments its counter, types sample
text, declines its harmless app alert, and tests long press. Provision the
com.xcautokit.fixture app on a dedicated simulator before running this script.
The server never starts agents or calls models. Two local MCP clients exercise
coordination. No simulator is created, booted, erased, or shut down here.
"""
from __future__ import annotations

import argparse
import base64
import json
import os
from pathlib import Path
import shutil
import sys
import tempfile
import uuid

from mcp_smoke_test import MCPClient, ROOT, require, rpc_result, structured


def selector(identifier: str) -> dict:
    return {"by": "accessibilityId", "query": identifier, "match": "exact"}


def condition(label: str) -> dict:
    return {"selector": {"by": "label", "query": label, "match": "exact"}, "state": "present"}


def handled_error(result: dict, reason: str | None = None) -> dict:
    require(result.get("isError") is True, "Expected isError=true, got a successful response")
    require(bool(result.get("content")), "Failure has no explanation")
    output = result.get("structuredContent") or {}
    if reason:
        require(output.get("reason") == reason, f"Expected {reason}, got {output.get('reason')}")
    return output


def verify_evidence(client: MCPClient, result: dict) -> None:
    run_id = result.get("runId")
    require(bool(run_id), "Missing workflow run ID")
    require(result.get("traceURI") == f"xcautokit://runs/{run_id}", "Missing trace resource URI")
    contents = rpc_result(client.request("resources/read", {"uri": result["traceURI"]})).get("contents", [])
    require(len(contents) == 1 and bool(contents[0].get("text")), "Missing trace resource content")
    trace = json.loads(contents[0]["text"])
    require(trace.get("runId") == run_id and trace.get("device") == result.get("device"), "Trace identity mismatch")
    require(trace.get("success") == result.get("success"), "Trace success differs from tool result")
    require(Path(result["tracePath"]).is_file(), "Trace path does not exist")
    require(result.get("screenshotURI") == f"xcautokit://runs/{run_id}/screen", "Missing screenshot resource URI")
    contents = rpc_result(client.request("resources/read", {"uri": result["screenshotURI"]})).get("contents", [])
    require(len(contents) == 1 and contents[0].get("mimeType") == "image/png", "Missing PNG resource")
    image = base64.b64decode(contents[0].get("blob", ""), validate=True)
    require(image.startswith(b"\x89PNG\r\n\x1a\n"), "Screenshot resource is not PNG")
    require(image == Path(result["screenshotPath"]).read_bytes(), "Screenshot resource differs from file")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--simulator", type=uuid.UUID, required=True, metavar="UUID", help="Dedicated fixture simulator UUID")
    parser.add_argument("--binary", type=Path, default=ROOT / "xcautokit", help="Built server binary")
    parser.add_argument("--artifacts-dir", type=Path, help="Optional directory to retain traces/screenshots after this run")
    args = parser.parse_args()
    binary = args.binary.expanduser().resolve()
    if not binary.is_file():
        parser.error(f"Missing server binary: {binary}")
    udid = str(args.simulator).upper()
    passed = 0

    def ok(label: str) -> None:
        nonlocal passed
        passed += 1
        print(f"[PASS] {label}", flush=True)

    try:
        with tempfile.TemporaryDirectory(prefix="xcautokit-live-") as temp:
            workspace = Path(temp)
            # A dedicated fixture device is required: these two clients use an
            # isolated shared lease directory, separate from normal host work.
            env = dict(os.environ, XCAUTOKIT_XCODE_BACKEND="off", XCAUTOKIT_WORKFLOWS="all",
                       XCAUTOKIT_SESSION_FILE=str(workspace / "session.json"),
                       XCAUTOKIT_STATE_DIR=str(workspace / "state"))
            try:
                with MCPClient(binary, env, 60) as driver, MCPClient(binary, env, 60) as peer:
                    driver.initialize()
                    peer.initialize()
                    token = None
                    try:
                        claim = structured(driver.call("device_claim", {
                            "simulatorUuid": udid, "owner": "fixture-driver", "ttlSeconds": 900,
                        }))
                        token = claim.get("leaseToken")
                        require(isinstance(token, str) and bool(token), "Missing lease token")
                        mutation = {"simulatorUuid": udid, "leaseToken": token}
                        read = {"simulatorUuid": udid}

                        def act(identifier: str, label: str | None = None) -> dict:
                            payload = dict(mutation, selector=selector(identifier), timeoutMs=10000)
                            if label:
                                payload["waitFor"] = condition(label)
                            return structured(driver.call("ui_act", payload))

                        def wait(label: str) -> dict:
                            return structured(driver.call("ui_wait", dict(read, **condition(label), timeoutMs=5000)))

                        structured(driver.call("app_launch", dict(mutation, bundleId="com.xcautokit.fixture")))
                        act("counter.reset", "Count: 0")
                        ok("fixture launch and semantic reset")

                        status = structured(peer.call("device_lease_status", read))
                        require(token not in json.dumps(status), "Status exposed the lease token")
                        handled_error(peer.call("device_claim", dict(read, owner="fixture-peer")))
                        handled_error(peer.call("ui_act", dict(read, selector=selector("counter.increment"))), "device_owned")
                        wait("Count: 0")
                        renewed = structured(driver.call("device_claim", dict(mutation, owner="fixture-driver", ttlSeconds=900)))
                        require(renewed.get("leaseToken") == token, "Renewal changed lease token")
                        ok("cross-process ownership, read access, and renewal")

                        ambiguous = handled_error(driver.call("ui_act", dict(
                            mutation, selector={"by": "label", "query": "Duplicate", "match": "exact", "role": "Button"},
                        )), "ambiguous_selector")
                        require(ambiguous.get("performed") is False and ambiguous.get("count") == 2,
                                "Ambiguous selector did not prove no input")
                        wait("Count: 0")
                        result = act("counter.increment", "Count: 1")
                        require(result.get("performed") is True and result.get("postconditionMet") is True,
                                "Tap was not performed and verified")
                        ok("ambiguous tap refused without side effects; unique tap verified")

                        disabled = handled_error(driver.call("ui_act", dict(
                            mutation, selector=selector("disabled"), timeoutMs=1500,
                        )), "timeout")
                        require(disabled.get("performed") is False, "Disabled target received input")
                        wait("Count: 1")
                        ok("disabled target times out without changing fixture state")

                        failed_action = handled_error(driver.call("ui_act", dict(
                            mutation, selector=selector("counter.increment"),
                            waitFor=condition("Impossible fixture state"), timeoutMs=6000,
                        )))
                        require(failed_action.get("performed") is True and failed_action.get("postconditionMet") is False,
                                "Failed postcondition lost the fact that input already happened")
                        wait("Count: 2")
                        ok("failed postcondition preserves performed state and never repeats tap")

                        steps = [
                            {"action": "tap", "selector": selector("counter.reset"), "waitFor": condition("Count: 0")},
                            {"action": "tap", "selector": selector("counter.increment"), "waitFor": condition("Count: 1")},
                        ]
                        run = structured(driver.call("workflow_run", dict(mutation, steps=steps, timeoutMs=30000)))
                        require(run.get("verified") is True and len(run.get("results", [])) == 2, "Workflow not fully verified")
                        verify_evidence(driver, run)
                        saved = structured(driver.call("workflow_save", {
                            "runId": run["runId"], "name": "fixture-counter", "description": "Reset then increment fixture counter",
                        }))
                        saved_path = Path(saved["path"])
                        before = saved_path.read_bytes()
                        handled_error(driver.call("workflow_save", {"runId": run["runId"], "name": "fixture-counter"}))
                        require(saved_path.read_bytes() == before, "Saving duplicate name overwrote workflow")
                        workflows = structured(peer.call("workflow_list"))
                        require(any(item.get("name") == "fixture-counter" for item in workflows.get("workflows", [])),
                                "Second MCP process cannot see saved workflow")
                        replay = structured(driver.call("workflow_run", dict(mutation, workflow="fixture-counter", timeoutMs=30000)))
                        require(replay.get("verified") is True, "Replay not verified")
                        verify_evidence(peer, replay)
                        wait("Count: 1")
                        ok("workflow verification, immutable save, replay, and evidence resources")

                        failed_run = handled_error(driver.call("workflow_run", dict(mutation, steps=[
                            {"action": "wait", "selector": selector("never-present-fixture"), "timeoutMs": 1500},
                            {"action": "tap", "selector": selector("counter.increment")},
                        ], timeoutMs=15000)))
                        require(failed_run.get("verified") is False and len(failed_run.get("results", [])) == 1,
                                "Workflow continued after failed first step")
                        wait("Count: 1")
                        verify_evidence(driver, failed_run)
                        handled_error(driver.call("workflow_save", {"runId": failed_run["runId"], "name": "must-not-save"}))
                        ok("failed workflow stops, captures evidence, and cannot be saved")

                        # Fixture submit dismisses the keyboard and publishes a
                        # delayed result, so this tests a changing UI condition.
                        text_run = structured(driver.call("workflow_run", dict(mutation, steps=[
                            {"action": "tap", "selector": selector("name-field")},
                            {"action": "type_text", "textFrom": "name"},
                            {"action": "tap", "selector": selector("submit"), "waitFor": {
                                "selector": selector("result-ready"), "state": "present",
                            }},
                        ], inputs={"name": "Autokit smoke"}, timeoutMs=30000)))
                        wait("Saved: Autokit smoke")
                        text_saved = structured(driver.call("workflow_save", {"runId": text_run["runId"], "name": "fixture-text"}))
                        definition = Path(text_saved["path"]).read_text()
                        require("Autokit smoke" not in definition and "textFrom" in definition,
                                "Saved definition contains runtime input instead of parameter reference")
                        ok("runtime text input, delayed result, and parameterized workflow save")

                        # This is the fixture's own harmless test alert, never a
                        # system permission prompt. Choose its explicit decline.
                        alert = act("permission-test", "Fixture would like to access your test data")
                        require(alert.get("hasInterrupt") is True, "Fixture alert was not detected")
                        blocked = handled_error(driver.call("ui_act", dict(mutation, selector=selector("counter.increment"))))
                        require(blocked.get("performed") is False and blocked.get("reason") == "blocking_interrupt",
                                "Input was not blocked while the fixture alert was present")
                        structured(driver.call("ui_dismiss_interrupt", dict(mutation, action="decline")))
                        interrupts = structured(driver.call("ui_check_interrupt", read))
                        require(interrupts.get("hasInterrupt") is False, "Fixture alert still present after explicit decline")
                        wait("Count: 1")
                        ok("interrupt blocks input until explicit fixture decline")

                        structured(driver.call("gesture", dict(mutation, gesture="long_press",
                                                               target={"accessibilityId": "hold-target"}, duration=1.0)))
                        wait("Held")
                        ok("actual long press reaches fixture hold result")

                        act("counter.reset", "Count: 0")
                        act("name-field")
                        typed = structured(driver.call("type_text", dict(mutation, text="--help")))
                        require(typed.get("characters") == 6, "Literal option-like text was not fully typed")
                        act("submit", "Saved: --help")
                        ok("option-like text is typed literally without invoking backend flags")

                        structured(driver.call("device_release", mutation))
                        token = None
                        peer_claim = structured(peer.call("device_claim", dict(read, owner="fixture-peer")))
                        structured(peer.call("device_release", dict(read, leaseToken=peer_claim["leaseToken"])))
                        ok("released simulator can be claimed by another process")
                    finally:
                        if token:
                            result = driver.call("device_release", {"simulatorUuid": udid, "leaseToken": token})
                            require(not result.get("isError"), "Could not release fixture lease during cleanup")
            finally:
                if args.artifacts_dir and (workspace / "state" / "runs").is_dir():
                    destination = args.artifacts_dir.expanduser().resolve()
                    destination.mkdir(parents=True, exist_ok=True)
                    shutil.copytree(workspace / "state" / "runs", destination / "runs", dirs_exist_ok=True)
                    print(f"Evidence retained in {destination / 'runs'}", flush=True)
    except (AssertionError, OSError, ValueError, TimeoutError, EOFError, KeyError, TypeError) as exc:
        print(f"[FAIL] {exc}", file=sys.stderr)
        return 1
    print(f"\n{passed} fixture checks passed on {udid}.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
