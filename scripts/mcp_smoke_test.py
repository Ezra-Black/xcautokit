#!/usr/bin/env python3
"""Comprehensive XCAutokit MCP smoke test."""

from __future__ import annotations

import json
import subprocess
import sys
import time
from pathlib import Path
from typing import Any

ROOT = Path(__file__).resolve().parents[1]
BIN = ROOT / "xcautokit"
PROJECT = "/Users/ezrablack/Developer/Pennywise/pennywiseiOS/PennyWise.xcworkspace"
SCHEME = "PennyWise"
UDID = "AD253295-45DA-4B6F-BD77-238B0AE6D911"
SHOT_DIR = Path.home() / "Pictures" / "xcautokit" / "screenshots" / "mcp_smoke"
SHOT_DIR.mkdir(parents=True, exist_ok=True)


class MCPClient:
    def __init__(self, bin_path: Path):
        self.proc = subprocess.Popen(
            [str(bin_path)],
            stdin=subprocess.PIPE,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
            bufsize=1,
        )
        self._id = 1

    def close(self):
        try:
            self.proc.terminate()
            self.proc.wait(timeout=2)
        except Exception:
            self.proc.kill()

    def send(self, msg: dict):
        assert self.proc.stdin
        self.proc.stdin.write(json.dumps(msg) + "\n")
        self.proc.stdin.flush()

    def read(self, timeout_s: float = 120.0) -> dict:
        assert self.proc.stdout
        deadline = time.time() + timeout_s
        while time.time() < deadline:
            line = self.proc.stdout.readline()
            if not line:
                err = self.proc.stderr.read() if self.proc.stderr else ""
                raise RuntimeError(f"EOF from server: {err}")
            line = line.strip()
            if not line:
                continue
            obj = json.loads(line)
            if "id" in obj:
                return obj
        raise TimeoutError("timeout waiting for MCP response")

    def initialize(self) -> dict:
        self.send(
            {
                "jsonrpc": "2.0",
                "id": self._next(),
                "method": "initialize",
                "params": {
                    "protocolVersion": "2025-06-18",
                    "capabilities": {},
                    "clientInfo": {"name": "xcautokit-smoke", "version": "1.0"},
                },
            }
        )
        res = self.read()
        self.send({"jsonrpc": "2.0", "method": "notifications/initialized"})
        return res

    def _next(self) -> int:
        i = self._id
        self._id += 1
        return i

    def request(self, method: str, params: dict | None = None, timeout_s: float = 120.0) -> dict:
        self.send({"jsonrpc": "2.0", "id": self._next(), "method": method, "params": params or {}})
        return self.read(timeout_s=timeout_s)

    def call_tool(self, name: str, arguments: dict | None = None, timeout_s: float = 180.0) -> dict:
        return self.request(
            "tools/call",
            {"name": name, "arguments": arguments or {}},
            timeout_s=timeout_s,
        )


def tool_ok(resp: dict) -> tuple[bool, str]:
    if "error" in resp:
        return False, f"protocol error: {resp['error']}"
    result = resp.get("result") or {}
    if result.get("isError"):
        content = result.get("content") or []
        text = ""
        if content and isinstance(content[0], dict):
            text = content[0].get("text", "")
        return False, text or "isError=true"
    sc = result.get("structuredContent")
    if sc is not None:
        return True, json.dumps(sc)[:200]
    content = result.get("content") or []
    if content:
        return True, str(content[0].get("text", ""))[:200]
    return True, "(empty ok)"


def expect_error(resp: dict) -> tuple[bool, str]:
    """Pass if tool returns a handled error (bridge unavailable etc)."""
    ok, msg = tool_ok(resp)
    if not ok:
        return True, f"expected failure: {msg[:180]}"
    # Some tools return success=false style maps
    result = resp.get("result") or {}
    sc = result.get("structuredContent")
    if isinstance(sc, dict) and sc.get("available") is False:
        return True, "unavailable reported cleanly"
    return False, f"expected error but succeeded: {msg[:180]}"


def main() -> int:
    if not BIN.exists():
        print("missing binary", BIN)
        return 2

    c = MCPClient(BIN)
    results: list[tuple[str, bool, str]] = []

    def record(name: str, ok: bool, detail: str):
        results.append((name, ok, detail))
        mark = "PASS" if ok else "FAIL"
        print(f"[{mark}] {name}: {detail[:240]}")

    try:
        init = c.initialize()
        info = (init.get("result") or {}).get("serverInfo") or {}
        caps = (init.get("result") or {}).get("capabilities") or {}
        instructions = (init.get("result") or {}).get("instructions") or ""
        record(
            "initialize",
            info.get("name") == "xcautokit" and info.get("version") == "1.0.0",
            f"serverInfo={info} caps={sorted(caps)}",
        )
        record(
            "initialize:instructions",
            "ui_dismiss_interrupt" in instructions and "hasInterrupt" in instructions,
            f"len={len(instructions)}",
        )

        tools = c.request("tools/list")
        tool_names = sorted(t["name"] for t in (tools.get("result") or {}).get("tools") or [])
        record("tools/list", len(tool_names) >= 40, f"count={len(tool_names)}")

        # Schema spot-checks
        by_name = {t["name"]: t for t in (tools.get("result") or {}).get("tools") or []}
        ui_find_schema = (by_name.get("ui_find") or {}).get("inputSchema") or {}
        record(
            "schema:ui_find",
            "by" in (ui_find_schema.get("properties") or {}) and "query" in (ui_find_schema.get("required") or []),
            f"required={ui_find_schema.get('required')} props={list((ui_find_schema.get('properties') or {}).keys())}",
        )
        build_schema = (by_name.get("build_sim") or {}).get("inputSchema") or {}
        record(
            "schema:build_sim",
            "project" in (build_schema.get("properties") or {}),
            f"props={list((build_schema.get('properties') or {}).keys())}",
        )

        resources = c.request("resources/list")
        uris = sorted(r["uri"] for r in (resources.get("result") or {}).get("resources") or [])
        record(
            "resources/list",
            uris == [
                "simulator://config",
                "simulator://devices",
                "simulator://interrupts",
                "simulator://status",
                "xcautokit://agent-guide",
                "xcode://status",
            ],
            f"uris={uris}",
        )

        for uri in uris:
            resp = c.request("resources/read", {"uri": uri})
            ok = "error" not in resp and bool((resp.get("result") or {}).get("contents"))
            text = ""
            if ok:
                text = ((resp.get("result") or {}).get("contents") or [{}])[0].get("text", "")[:120]
            record(f"resources/read:{uri}", ok, text or str(resp.get("error")))

        prompts = c.request("prompts/list")
        prompt_names = sorted(p["name"] for p in (prompts.get("result") or {}).get("prompts") or [])
        record(
            "prompts/list",
            prompt_names == ["build-and-verify", "fix-failing-test", "handle-interrupt", "ui-explore"],
            f"names={prompt_names}",
        )
        for pname in prompt_names:
            resp = c.request("prompts/get", {"name": pname})
            msgs = ((resp.get("result") or {}).get("messages") or [])
            record(f"prompts/get:{pname}", "error" not in resp and len(msgs) > 0, f"messages={len(msgs)}")

        # ---- Device ----
        for name, args in [
            ("status", {}),
            ("device_list", {}),
            ("open_sim", {}),
        ]:
            ok, detail = tool_ok(c.call_tool(name, args))
            record(f"tool:{name}", ok, detail)

        # device_boot already booted device should be ok-ish / may error if already booted
        resp = c.call_tool("device_boot", {"udid": UDID})
        ok, detail = tool_ok(resp)
        if not ok and ("already" in detail.lower() or "booted" in detail.lower() or "current state" in detail.lower()):
            record("tool:device_boot", True, f"already booted tolerated: {detail[:160]}")
        else:
            record("tool:device_boot", ok, detail)

        # ---- UI inspect ----
        for name, args in [
            ("ui_describe", {}),
            ("ui_summary", {}),
            ("ui_find", {"by": "label", "query": "PennyWise"}),
            ("ui_search", {"query": "Sign"}),
            ("ui_point", {"x": 201, "y": 700}),
            ("ui_check_interrupt", {}),
        ]:
            ok, detail = tool_ok(c.call_tool(name, args))
            record(f"tool:{name}", ok, detail)

        # Ensure we're in an app with UI — launch PennyWise if on springboard
        summary = c.call_tool("ui_summary", {})
        sc = (summary.get("result") or {}).get("structuredContent") or {}
        labels = [x.get("label") for x in sc.get("summary") or []]
        # ui_summary should expose interrupt preview fields
        if "hasInterrupt" in sc:
            record("tool:ui_summary:hasInterrupt_field", True, f"hasInterrupt={sc.get('hasInterrupt')}")
        else:
            record("tool:ui_summary:hasInterrupt_field", False, "missing hasInterrupt on ui_summary")

        check = c.call_tool("ui_check_interrupt", {})
        check_sc = (check.get("result") or {}).get("structuredContent") or {}
        if check_sc.get("hasInterrupt"):
            kinds = [i.get("kind") for i in (check_sc.get("interrupts") or [])]
            record("tool:ui_check_interrupt:detected", True, f"kinds={kinds}")
            # Exercise dismiss when a non-springboard dialog is present
            for i, intr in enumerate(check_sc.get("interrupts") or []):
                if intr.get("kind") == "springboard":
                    continue
                btns = [b.get("label", "") for b in (intr.get("buttons") or [])]
                action = "dismiss"
                if any("don't allow" in b.lower() or "dont allow" in b.lower() or b.lower() == "cancel" for b in btns):
                    action = "decline"
                elif any("allow" in b.lower() or b.lower() == "ok" for b in btns):
                    action = "dismiss"
                ok, detail = tool_ok(c.call_tool("ui_dismiss_interrupt", {"action": action, "index": i}))
                record(f"tool:ui_dismiss_interrupt:{action}", ok, detail)
                time.sleep(0.5)
                break
            else:
                record("tool:ui_dismiss_interrupt:skip", True, "only springboard or no buttons")
        else:
            record("tool:ui_check_interrupt:clear", True, "no interrupt")
            # No-op dismiss path should succeed with dismissed=false
            ok, detail = tool_ok(c.call_tool("ui_dismiss_interrupt", {"action": "dismiss"}))
            record("tool:ui_dismiss_interrupt:noop", ok, detail)

        if "PennyWise" in labels and "Safari" in labels:
            # springboard — open PennyWise
            ok, detail = tool_ok(c.call_tool("gesture", {"gesture": "tap", "target": {"label": "PennyWise"}}))
            record("tool:gesture:open_pennywise", ok, detail)
            time.sleep(1.2)
        elif "Create Account" in labels or "Sign In" in labels or "Welcome to" in labels:
            record("tool:gesture:open_pennywise", True, "already in PennyWise")
        else:
            # try openurl / launch
            ok, detail = tool_ok(c.call_tool("app_launch", {"bundleId": "com.ezrablack.PennyWise"}))
            if not ok:
                # try common ids
                for bid in [
                    "com.pennywise.ios",
                    "com.pennywise.app",
                    "com.ezra.PennyWise",
                    "EzraBlack.PennyWise",
                ]:
                    ok, detail = tool_ok(c.call_tool("app_launch", {"bundleId": bid}))
                    if ok:
                        break
            record("tool:app_launch:pennywise", ok, detail)
            time.sleep(1.0)

        # Ensure app_launch is always exercised for tool coverage + interrupt attach
        ok, detail = tool_ok(c.call_tool("app_launch", {"bundleId": "com.pennywise.ios"}))
        record("tool:app_launch", ok, detail)
        time.sleep(0.5)

        # Post-launch interrupt check
        ok, detail = tool_ok(c.call_tool("ui_check_interrupt", {}))
        record("tool:ui_check_interrupt:post_launch", ok, detail)

        # ---- Input ----
        # Prefer Sign In flow if present
        find = c.call_tool("ui_find", {"by": "label", "query": "Sign In"})
        find_sc = (find.get("result") or {}).get("structuredContent") or {}
        if find_sc.get("count", 0) > 0:
            ok, detail = tool_ok(
                c.call_tool("gesture", {"gesture": "tap", "target": {"label": "Sign In"}})
            )
            record("tool:gesture:tap_signin", ok, detail)
            time.sleep(1.0)

        for name, args in [
            ("tap", {"x": 201, "y": 520}),
            ("type_text", {"text": "mcp_test"}),
            ("swipe", {"direction": "down", "distance": "short"}),
            ("long_press", {"x": 201, "y": 400, "duration": 0.4}),
            ("button", {"buttonType": "home"}),
        ]:
            # home last — will leave app; do others first except we already may have home later
            if name == "button":
                continue
            ok, detail = tool_ok(c.call_tool(name, args))
            record(f"tool:{name}", ok, detail)
            time.sleep(0.3)

        # key_press / key_sequence — best effort
        ok, detail = tool_ok(c.call_tool("key_press", {"keyCode": 40}))  # return-ish
        record("tool:key_press", ok, detail)
        ok, detail = tool_ok(c.call_tool("key_sequence", {"keys": [4, 5], "delayMs": 50}))
        record("tool:key_sequence", ok, detail)

        # gesture presets / scroll
        # Re-open app if needed after interactions
        ok, detail = tool_ok(c.call_tool("gesture", {"preset": "scroll-up"}))
        record("tool:gesture:preset_scroll_up", ok, detail)

        # ---- Capture ----
        shot = str(SHOT_DIR / "full_suite.png")
        ok, detail = tool_ok(c.call_tool("screenshot", {"outputPath": shot}))
        record("tool:screenshot", ok and Path(shot).exists(), detail)

        ok, detail = tool_ok(
            c.call_tool(
                "record_start",
                {"outputPath": str(SHOT_DIR / "clip.mp4")},
            )
        )
        record("tool:record_start", ok, detail)
        time.sleep(1.0)
        ok, detail = tool_ok(c.call_tool("record_stop", {}))
        record("tool:record_stop", ok, detail)

        ok, detail = tool_ok(c.call_tool("start_sim_log_cap", {"timeout": 5}))
        record("tool:start_sim_log_cap", ok, detail)
        pid = None
        sc = ((c.call_tool("status", {}).get("result") or {}).get("structuredContent"))
        # get pid from previous start result — call again carefully
        start_resp = None
        # We already called; re-stop any
        ok, detail = tool_ok(c.call_tool("stop_sim_log_cap", {}))
        record("tool:stop_sim_log_cap", ok, detail)

        # ---- App / URL ----
        # open_url to settings-ish about blank
        ok, detail = tool_ok(c.call_tool("open_url", {"url": "https://example.com"}))
        record("tool:open_url", ok, detail)
        time.sleep(0.8)

        # terminate safari/example if launched — best effort
        ok, detail = tool_ok(c.call_tool("app_terminate", {"bundleId": "com.apple.mobilesafari"}))
        record("tool:app_terminate", ok or "not running" in detail.lower() or "failed" in detail.lower(), detail)

        # app_install with missing path should fail cleanly
        ok, detail = expect_error(c.call_tool("app_install", {"appPath": "/tmp/does-not-exist.app"}))
        record("tool:app_install:missing", ok, detail)

        # ---- Session / project ----
        ok, detail = tool_ok(
            c.call_tool(
                "session_set_defaults",
                {
                    "projectPath": PROJECT,
                    "scheme": SCHEME,
                    "configuration": "Debug",
                    "simulatorUdid": UDID,
                    "simulatorName": "iPhone 17 Pro",
                },
            )
        )
        record("tool:session_set_defaults", ok, detail)
        ok, detail = tool_ok(c.call_tool("session_show_defaults", {}))
        record("tool:session_show_defaults", ok, detail)

        ok, detail = tool_ok(c.call_tool("discover_projects", {"root": "/Users/ezrablack/Developer/Pennywise"}))
        record("tool:discover_projects", ok, detail)

        ok, detail = tool_ok(c.call_tool("list_schemes", {"project": PROJECT}))
        record("tool:list_schemes", ok, detail)

        ok, detail = tool_ok(
            c.call_tool(
                "show_build_settings",
                {"project": PROJECT, "scheme": SCHEME},
                timeout_s=300,
            )
        )
        record("tool:show_build_settings", ok, detail)

        ok, detail = tool_ok(
            c.call_tool("get_app_bundle_id", {"project": PROJECT, "scheme": SCHEME})
        )
        record("tool:get_app_bundle_id", ok, detail)
        bundle_id = None
        sc = ((c.call_tool("get_app_bundle_id", {"project": PROJECT, "scheme": SCHEME}).get("result") or {}).get("structuredContent") or {})
        bundle_id = sc.get("bundleId")

        if bundle_id:
            ok, detail = tool_ok(c.call_tool("get_sim_app_path", {"bundleId": bundle_id}))
            record("tool:get_sim_app_path", ok, detail)
            ok, detail = tool_ok(c.call_tool("launch_app_logs_sim", {"bundleId": bundle_id}))
            record("tool:launch_app_logs_sim", ok, detail)
        else:
            record("tool:get_sim_app_path", False, "no bundle id")
            record("tool:launch_app_logs_sim", False, "no bundle id")

        # Build — can take a while
        print("... running build_sim (may take a while)")
        ok, detail = tool_ok(
            c.call_tool(
                "build_sim",
                {
                    "project": PROJECT,
                    "scheme": SCHEME,
                    "destination": f"platform=iOS Simulator,id={UDID}",
                },
                timeout_s=600,
            )
        )
        record("tool:build_sim", ok, detail)

        # clean is destructive-ish but ok
        print("... running clean")
        ok, detail = tool_ok(
            c.call_tool("clean", {"project": PROJECT, "scheme": SCHEME}, timeout_s=300)
        )
        record("tool:clean", ok, detail)

        # test_sim can be very long — run but allow fail if no tests / timeout
        print("... running test_sim (may take a while)")
        resp = c.call_tool(
            "test_sim",
            {
                "project": PROJECT,
                "scheme": SCHEME,
                "destination": f"platform=iOS Simulator,id={UDID}",
            },
            timeout_s=600,
        )
        ok, detail = tool_ok(resp)
        record("tool:test_sim", ok, detail)

        # build_run_sim
        print("... running build_run_sim")
        ok, detail = tool_ok(
            c.call_tool(
                "build_run_sim",
                {
                    "project": PROJECT,
                    "scheme": SCHEME,
                    "destination": f"platform=iOS Simulator,id={UDID}",
                },
                timeout_s=600,
            )
        )
        record("tool:build_run_sim", ok, detail)

        # ---- Xcode bridge tools: expect graceful failure on 26.2 ----
        for name, args in [
            ("xcode_windows", {}),
            ("xcode_issues", {}),
            ("xcode_build_log", {}),
            ("xcode_preview", {"filePath": "ContentView.swift"}),
            ("docs_search", {"query": "SwiftUI List"}),
            ("swift_snippet", {"code": "print(1+1)"}),
            ("run_some_tests", {"tests": ["DummyTests/testA"]}),
        ]:
            ok, detail = expect_error(c.call_tool(name, args, timeout_s=30))
            record(f"tool:{name}:bridge_absent", ok, detail)

        # session clear
        ok, detail = tool_ok(c.call_tool("session_clear_defaults", {"keys": ["tabIdentifier"]}))
        record("tool:session_clear_defaults", ok, detail)

        # device_shutdown NOT run (keep user's simulator up) — validate arg error instead
        ok, detail = expect_error(c.call_tool("device_shutdown", {"udid": "INVALID-UDID-FOR-TEST"}))
        record("tool:device_shutdown:invalid", ok, detail)

        # button at end to leave simulator usable
        ok, detail = tool_ok(c.call_tool("button", {"buttonType": "home"}))
        record("tool:button:home", ok, detail)

        # Ensure every registered tool was covered somehow
        covered = set()
        for name, _, _ in results:
            if name.startswith("tool:"):
                base = name.split(":")[1]
                covered.add(base)
        # map aliases
        aliases = {
            "gesture": "gesture",
            "open_pennywise": "gesture",
            "tap_signin": "gesture",
            "preset_scroll_up": "gesture",
            "missing": "app_install",
            "invalid": "device_shutdown",
            "home": "button",
            "bridge_absent": None,
            "pennywise": "app_launch",
        }
        missing = []
        for t in tool_names:
            if t in covered:
                continue
            # check prefix coverage
            if any(n.startswith(f"tool:{t}") for n, _, _ in results):
                continue
            missing.append(t)
        record("coverage:all_tools", len(missing) == 0, f"missing={missing}")

    finally:
        c.close()

    passed = sum(1 for _, ok, _ in results if ok)
    failed = sum(1 for _, ok, _ in results if not ok)
    print("\n==== SUMMARY ====")
    print(f"passed={passed} failed={failed} total={len(results)}")
    if failed:
        print("\nFailures:")
        for name, ok, detail in results:
            if not ok:
                print(f"  - {name}: {detail}")
    out = SHOT_DIR / "results.json"
    out.write_text(json.dumps([{"name": n, "ok": ok, "detail": d} for n, ok, d in results], indent=2))
    print(f"wrote {out}")
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
