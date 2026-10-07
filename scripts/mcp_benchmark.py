#!/usr/bin/env python3
"""Measure equivalent deterministic fixture flows; no model or agent benchmark.

Requires com.xcautokit.fixture already installed on a dedicated, booted simulator.
This changes its counter. Every measured flow resets to Count: 0, then increments
and verifies Count: 1. Claiming, launching, and pre-trial reset are excluded for
all methods. No simulator is created, booted, erased, or shut down by this script.
"""
from __future__ import annotations

import argparse
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import statistics
import sys
import tempfile
import time
import uuid

from mcp_smoke_test import MCPClient, require, structured


METHODS = ("gesture_and_wait", "ui_act", "workflow_run")
EXPECTED_CALLS = {"gesture_and_wait": 4, "ui_act": 2, "workflow_run": 1}


def target(identifier: str) -> dict:
    return {"by": "accessibilityId", "query": identifier, "match": "exact"}


def condition(count: int) -> dict:
    # The counter's ID stays constant. Its exact label verifies the numeric
    # outcome; checking the ID alone would only prove the label exists.
    return {"selector": {"by": "label", "query": f"Count: {count}", "match": "exact"}, "state": "present"}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--simulator", type=uuid.UUID, required=True, metavar="UUID", help="Dedicated fixture simulator UUID")
    parser.add_argument("--binary", type=Path, required=True, help="Built XCAutokit binary to measure")
    parser.add_argument("--repetitions", type=int, default=3, help="Runs per method, 1-10 (default: 3)")
    parser.add_argument("--output", type=Path, help="Optional JSON result file")
    args = parser.parse_args()
    if not 1 <= args.repetitions <= 10:
        parser.error("--repetitions must be between 1 and 10")
    binary = args.binary.expanduser().resolve()
    if not binary.is_file():
        parser.error(f"Missing binary: {binary}")
    udid = str(args.simulator).upper()
    report = {
        "startedAt": datetime.now(timezone.utc).isoformat(), "simulator": udid,
        "bundleId": "com.xcautokit.fixture", "binary": str(binary),
        "repetitions": args.repetitions, "samples": [], "medians": {},
        "metrics": {
            "wallMs": "Client wall time from the first measured tools/call through final verified response",
            "responseJsonBytes": "UTF-8 bytes of measured JSON-RPC response lines, excluding newline delimiters",
            "toolCalls": "Measured tools/call requests; excludes startup, claim/renew/release, launch, and pre-trial reset",
            "medianPopulation": "Successful samples only; all failures are included in samples and failure counts",
        },
        "scope": "Local deterministic tool execution only; no host model, model latency, token billing, or visual-quality measurement",
        "order": "Rotate method order each repetition; no claimed statistical significance",
    }
    failure = None
    try:
        with tempfile.TemporaryDirectory(prefix="xcautokit-benchmark-") as temp:
            env = dict(os.environ, XCAUTOKIT_XCODE_BACKEND="off", XCAUTOKIT_WORKFLOWS="all",
                       XCAUTOKIT_SESSION_FILE=str(Path(temp) / "session.json"),
                       XCAUTOKIT_STATE_DIR=str(Path(temp) / "state"))
            with MCPClient(binary, env, 60) as client:
                report["serverInfo"] = client.initialize().get("serverInfo")
                claim = structured(client.call("device_claim", {
                    "simulatorUuid": udid, "owner": "fixture-benchmark", "ttlSeconds": 900,
                }))
                mutation = {"simulatorUuid": udid, "leaseToken": claim["leaseToken"]}
                try:
                    structured(client.call("app_launch", dict(mutation, bundleId="com.xcautokit.fixture")))
                    for repetition in range(args.repetitions):
                        offset = repetition % len(METHODS)
                        order = METHODS[offset:] + METHODS[:offset]
                        for method in order:
                            # Identical untimed preparation for every sample.
                            structured(client.call("device_claim", dict(mutation, owner="fixture-benchmark", ttlSeconds=900)))
                            structured(client.call("ui_act", dict(mutation, selector=target("counter.reset"),
                                                                   waitFor=condition(0), timeoutMs=10000)))
                            sample = {"method": method, "repetition": repetition + 1, "toolCalls": 0,
                                      "responseJsonBytes": 0, "success": False}
                            started = time.perf_counter()

                            def measured_call(name: str, arguments: dict) -> dict:
                                sample["toolCalls"] += 1
                                try:
                                    result = client.call(name, arguments)
                                finally:
                                    # A timed-out call has no complete response.
                                    # Reset below prevents counting its predecessor.
                                    sample["responseJsonBytes"] += client.last_response_bytes
                                    client.last_response_bytes = 0
                                return structured(result)

                            client.last_response_bytes = 0
                            try:
                                if method == "gesture_and_wait":
                                    for identifier, count in (("counter.reset", 0), ("counter.increment", 1)):
                                        measured_call("gesture", dict(mutation, gesture="tap", target=target(identifier)))
                                        result = measured_call("ui_wait", {"simulatorUuid": udid, **condition(count), "timeoutMs": 10000})
                                        require(result.get("success") is True, "Expected count was not verified")
                                elif method == "ui_act":
                                    for identifier, count in (("counter.reset", 0), ("counter.increment", 1)):
                                        result = measured_call("ui_act", dict(mutation, selector=target(identifier),
                                                                             waitFor=condition(count), timeoutMs=10000))
                                        require(result.get("postconditionMet") is True, "Expected count was not verified")
                                else:
                                    result = measured_call("workflow_run", dict(mutation, steps=[
                                        {"action": "tap", "selector": target("counter.reset"), "waitFor": condition(0)},
                                        {"action": "tap", "selector": target("counter.increment"), "waitFor": condition(1)},
                                    ], screenshot=False, timeoutMs=30000))
                                    require(result.get("verified") is True, "Workflow final count was not verified")
                                require(sample["toolCalls"] == EXPECTED_CALLS[method], "Unexpected measured call count")
                                sample["success"] = True
                            except (AssertionError, OSError, ValueError, TimeoutError, EOFError) as exc:
                                sample["error"] = str(exc)[:1000]
                            finally:
                                sample["wallMs"] = round((time.perf_counter() - started) * 1000, 3)
                                report["samples"].append(sample)
                            mark = "PASS" if sample["success"] else "FAIL"
                            print(f"[{mark}] {method} run {repetition + 1}: {sample['toolCalls']} calls, "
                                  f"{sample['wallMs']:.1f} ms, {sample['responseJsonBytes']} response bytes", flush=True)
                            if not sample["success"]:
                                # A protocol/deadline failure may leave a response
                                # pending. End this run rather than reuse that stream.
                                failure = sample["error"]
                                raise AssertionError(f"Benchmark stopped after failed {method}: {failure}")
                finally:
                    try:
                        structured(client.call("device_release", mutation))
                    except Exception as exc:
                        report["cleanupError"] = str(exc)[:1000]
                        raise
    except (AssertionError, OSError, ValueError, TimeoutError, EOFError, KeyError, TypeError) as exc:
        failure = str(exc)
        report["error"] = failure
        print(f"[FAIL] {failure}", file=sys.stderr)
    finally:
        for method in METHODS:
            all_samples = [sample for sample in report["samples"] if sample["method"] == method]
            successes = [sample for sample in all_samples if sample["success"]]
            report["medians"][method] = {
                "successfulSamples": len(successes), "failedSamples": len(all_samples) - len(successes),
                **{key: statistics.median(sample[key] for sample in successes) if successes else None
                   for key in ("wallMs", "toolCalls", "responseJsonBytes")},
            }
        report["finishedAt"] = datetime.now(timezone.utc).isoformat()
        if args.output:
            path = args.output.expanduser().resolve()
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(json.dumps(report, indent=2) + "\n")
            print(f"Results: {path}")
        print("\nMedians of successful local tool runs:")
        for method, median in report["medians"].items():
            print(f"  {method}: {median['wallMs']} ms, {median['toolCalls']} calls, "
                  f"{median['responseJsonBytes']} response bytes; "
                  f"{median['successfulSamples']} passed / {median['failedSamples']} failed")
        print("No host model or agent speed was measured.")
    return 1 if failure else 0


if __name__ == "__main__":
    raise SystemExit(main())
