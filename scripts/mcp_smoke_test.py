#!/usr/bin/env python3
"""Bounded MCP contract checks; no Xcode or simulator needed by default.

Build: go build -o xcautokit .
Live observation: python3 scripts/mcp_smoke_test.py --simulator UUID
Live mode only reads UI and captures one temporary screenshot. It never sends
input, boots a device, launches an app, or dismisses a prompt.
"""
from __future__ import annotations

import argparse
import base64
import json
import os
from pathlib import Path
import queue
import re
import subprocess
import sys
import tempfile
import threading
import time
from typing import Any
import uuid

ROOT = Path(__file__).resolve().parents[1]


def require(condition: bool, message: str) -> None:
    if not condition:
        raise AssertionError(message)


class MCPClient:
    """Newline-delimited stdio with real deadlines and bounded teardown."""

    def __init__(self, binary: Path, env: dict[str, str], timeout: float):
        self.timeout, self.next_id = timeout, 0
        self.last_response_bytes = 0
        self.messages: queue.Queue[Any] = queue.Queue()
        self.stderr = tempfile.TemporaryFile(mode="w+b")
        try:
            self.proc = subprocess.Popen(
                [str(binary), "mcp"], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                stderr=self.stderr, text=True, encoding="utf-8", env=env, bufsize=1,
            )
        except BaseException:
            self.stderr.close()
            raise
        self.reader = threading.Thread(target=self._read, daemon=True)
        self.reader.start()

    def _read(self) -> None:
        assert self.proc.stdout is not None
        try:
            for line in self.proc.stdout:
                if line.strip():
                    self.messages.put((json.loads(line), len(line.rstrip("\r\n").encode("utf-8"))))
        except Exception as exc:
            self.messages.put(exc)
        finally:
            self.messages.put(EOFError("MCP server closed stdout"))

    def __enter__(self) -> MCPClient:
        return self

    def __exit__(self, *_: Any) -> None:
        if self.proc.stdin:
            self.proc.stdin.close()
        try:
            self.proc.wait(timeout=2)
        except subprocess.TimeoutExpired:
            self.proc.terminate()
            try:
                self.proc.wait(timeout=2)
            except subprocess.TimeoutExpired:
                self.proc.kill()
                self.proc.wait(timeout=2)
        self.reader.join(timeout=1)
        if self.proc.stdout:
            self.proc.stdout.close()
        self.stderr.close()

    def send(self, message: dict[str, Any]) -> None:
        assert self.proc.stdin is not None
        self.proc.stdin.write(json.dumps(message) + "\n")
        self.proc.stdin.flush()

    def request(self, method: str, params: dict[str, Any] | None = None) -> dict[str, Any]:
        self.next_id += 1
        self.send({"jsonrpc": "2.0", "id": self.next_id, "method": method, "params": params or {}})
        deadline = time.monotonic() + self.timeout
        while True:
            remaining = deadline - time.monotonic()
            if remaining <= 0:
                raise TimeoutError(f"{method} exceeded {self.timeout:g}s")
            try:
                item = self.messages.get(timeout=remaining)
            except queue.Empty as exc:
                raise TimeoutError(f"{method} exceeded {self.timeout:g}s") from exc
            if isinstance(item, Exception):
                raise item
            message, response_bytes = item
            require(isinstance(message, dict), "MCP message must be an object")
            require(message.get("jsonrpc") == "2.0", "Missing JSON-RPC version")
            if "method" in message:
                require("id" not in message, "Server requested an unadvertised client capability")
                continue
            require(message.get("id") == self.next_id, "Mismatched MCP response id")
            self.last_response_bytes = response_bytes
            return message

    def initialize(self) -> dict[str, Any]:
        result = rpc_result(self.request("initialize", {
            "protocolVersion": "2025-06-18", "capabilities": {},
            "clientInfo": {"name": "xcautokit-smoke", "version": "2"},
        }))
        self.send({"jsonrpc": "2.0", "method": "notifications/initialized"})
        return result

    def list_items(self, method: str, key: str) -> list[dict[str, Any]]:
        items: list[dict[str, Any]] = []
        cursor, seen = None, set()
        for _ in range(100):
            result = rpc_result(self.request(method, {"cursor": cursor} if cursor else {}))
            require(isinstance(result.get(key), list), f"{method} missing {key}")
            items.extend(result[key])
            cursor = result.get("nextCursor")
            if not cursor:
                return items
            require(isinstance(cursor, str) and cursor not in seen, "Invalid pagination cursor")
            seen.add(cursor)
        raise AssertionError(f"{method} exceeded 100 pages")

    def call(self, name: str, arguments: dict[str, Any] | None = None) -> dict[str, Any]:
        return rpc_result(self.request("tools/call", {"name": name, "arguments": arguments or {}}))


def rpc_result(response: dict[str, Any]) -> dict[str, Any]:
    require("error" not in response, f"JSON-RPC error: {response.get('error')}")
    require(isinstance(response.get("result"), dict), "Missing result object")
    return response["result"]


def structured(result: dict[str, Any]) -> dict[str, Any]:
    require(not result.get("isError"), f"Tool failed: {result.get('content')}")
    output = result.get("structuredContent")
    require(isinstance(output, dict), "Missing structuredContent object")
    require(output.get("success") is not False, "Failure returned without isError=true")
    return output


def cli_catalog(binary: Path, env: dict[str, str], timeout: float) -> dict[str, str]:
    result = subprocess.run([str(binary), "tools"], env=env, capture_output=True,
                            text=True, check=True, timeout=timeout)
    catalog = {}
    for line in result.stdout.splitlines():
        match = re.match(r"^([a-z][a-z0-9_]*)\s+([a-z][a-z0-9_]*)\s+", line)
        if match:
            catalog[match.group(1)] = match.group(2)
    return catalog


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--binary", type=Path, default=ROOT / "xcautokit", help="Built Go binary (default: repo ./xcautokit)")
    parser.add_argument("--simulator", type=uuid.UUID, metavar="UUID", help="Observe this booted simulator; never send input")
    parser.add_argument("--timeout", type=float, default=30, help="Per-request deadline, 1-120 seconds (default: 30)")
    args = parser.parse_args()
    if not 1 <= args.timeout <= 120:
        parser.error("--timeout must be between 1 and 120 seconds")
    binary = args.binary.expanduser().resolve()
    if not binary.is_file():
        parser.error(f"Missing binary: {binary}; build with: go build -o xcautokit .")
    passed = 0

    def ok(label: str) -> None:
        nonlocal passed
        passed += 1
        print(f"[PASS] {label}", flush=True)

    try:
        with tempfile.TemporaryDirectory(prefix="xcautokit-smoke-") as temp:
            workspace = Path(temp)
            # Isolate session state and disable live Xcode discovery; preserve HOME.
            env = dict(os.environ, XCAUTOKIT_SESSION_FILE=str(workspace / "session.json"),
                       XCAUTOKIT_STATE_DIR=str(workspace / "state"),
                       XCAUTOKIT_XCODE_BACKEND="off", XCAUTOKIT_WORKFLOWS="all")
            version = subprocess.run([str(binary), "version"], env=env, capture_output=True,
                                     text=True, check=True, timeout=args.timeout).stdout.strip().removeprefix("xcautokit ")
            catalog = cli_catalog(binary, env, args.timeout)
            require(bool(catalog), "CLI catalog is empty")
            with MCPClient(binary, env, args.timeout) as client:
                initialized = client.initialize()
                info = initialized.get("serverInfo", {})
                require(info.get("name") == "xcautokit" and info.get("version") == version, "MCP/CLI identity mismatch")
                require({"tools", "resources", "prompts"} <= initialized.get("capabilities", {}).keys(), "Missing capabilities")
                require("ui_dismiss_interrupt" in initialized.get("instructions", ""), "Missing interrupt instructions")
                ok("initialize, version, capabilities, instructions")

                definitions = client.list_items("tools/list", "tools")
                tools = {item["name"]: item for item in definitions}
                require(len(tools) == len(definitions), "Duplicate tool names")
                require(set(tools) == set(catalog), f"CLI/MCP catalog mismatch: {sorted(set(tools) ^ set(catalog))}")
                for name, definition in tools.items():
                    require(definition.get("inputSchema", {}).get("type") == "object", f"{name}: missing input object schema")
                    require(bool(definition.get("description")), f"{name}: missing description")
                for name in ("ui_summary", "ui_find", "session_show_defaults", "discover_projects", "screenshot"):
                    require(tools[name].get("annotations", {}).get("readOnlyHint") is True, f"{name}: missing readOnlyHint")
                ok(f"tools/list and schemas ({len(tools)} tools)")

                uris = {item["uri"] for item in client.list_items("resources/list", "resources")}
                for uri in ("simulator://config", "xcautokit://agent-guide", "xcode://status"):
                    require(uri in uris, f"Missing resource {uri}")
                    result = rpc_result(client.request("resources/read", {"uri": uri}))
                    require(bool(result.get("contents")), f"Empty resource {uri}")
                ok("resources/list and simulator-independent resource reads")

                prompts = client.list_items("prompts/list", "prompts")
                require({"build-and-verify", "ui-explore", "fix-failing-test", "handle-interrupt"}
                        <= {item["name"] for item in prompts}, "Missing standard prompt")
                for prompt in prompts:
                    result = rpc_result(client.request("prompts/get", {"name": prompt["name"]}))
                    require(bool(result.get("messages")), f"Empty prompt {prompt['name']}")
                ok("prompts/list and retrieval")

                require(structured(client.call("session_show_defaults")).get("defaults") == {}, "Session is not isolated")
                fixture = workspace / "projects"
                project, nested = fixture / "Example.xcodeproj", fixture / "Nested" / "Example.xcworkspace"
                project.mkdir(parents=True)
                nested.mkdir(parents=True)
                (fixture / "node_modules" / "Ignored.xcodeproj").mkdir(parents=True)
                found = structured(client.call("discover_projects", {"root": str(fixture)}))
                require(found.get("count") == 2 and set(found.get("projects", [])) == {str(project), str(nested)}, "Incorrect discovery results")
                ok("structured isolated session and generic project discovery")

                # Invalid inputs stop before any simulator/input operation.
                for name, arguments in (
                    ("ui_find", {"by": "label", "query": ""}), ("ui_search", {"query": ""}),
                    ("record_stop", {"ticket": "smoke-nonexistent-ticket"}), ("list_schemes", {}),
                ):
                    result = client.call(name, arguments)
                    require(result.get("isError") is True, f"{name}: expected handled failure with isError=true")
                    require(bool(result.get("content")), f"{name}: error has no explanation")
                unknown = client.request("tools/call", {"name": "smoke_nonexistent_tool", "arguments": {}})
                require("error" in unknown or unknown.get("result", {}).get("isError") is True, "Unknown tool returned success")
                ok("validation failures and unknown tool cannot masquerade as success")

                if args.simulator:
                    udid = str(args.simulator).upper()
                    for name in ("ui_summary", "ui_describe", "ui_check_interrupt"):
                        result = structured(client.call(name, {"simulatorUuid": udid}))
                        require(result.get("device") == udid, f"{name}: wrong simulator")
                    screenshot = workspace / "observation.png"
                    result = client.call("screenshot", {"simulatorUuid": udid, "outputPath": str(screenshot)})
                    metadata = structured(result)
                    require(metadata.get("device") == udid and metadata.get("width", 0) > 0
                            and metadata.get("height", 0) > 0, "Missing screenshot dimensions/device")
                    images = [item for item in result.get("content", []) if item.get("type") == "image"]
                    require(len(images) == 1 and images[0].get("mimeType") == "image/png", "Missing native MCP image")
                    image_bytes = base64.b64decode(images[0]["data"], validate=True)
                    require(image_bytes.startswith(b"\x89PNG\r\n\x1a\n") and image_bytes == screenshot.read_bytes(), "Image does not match captured PNG")
                    ok("live read-only UI and native screenshot")

            filters = ["core", "ui,input", *sorted(set(catalog.values()))]
            for workflow_filter in dict.fromkeys(filters):
                filtered_env = dict(env, XCAUTOKIT_WORKFLOWS=workflow_filter)
                expected = set(cli_catalog(binary, filtered_env, args.timeout))
                with MCPClient(binary, filtered_env, args.timeout) as client:
                    client.initialize()
                    actual = {item["name"] for item in client.list_items("tools/list", "tools")}
                require(actual == expected, f"Filter {workflow_filter}: CLI/MCP mismatch {sorted(actual ^ expected)}")
                if workflow_filter in set(catalog.values()):
                    require(any(catalog[name] == workflow_filter for name in actual), f"Empty requested category: {workflow_filter}")
                ok(f"workflow filter agrees with CLI: {workflow_filter}")
    except (AssertionError, OSError, ValueError, TimeoutError, EOFError, subprocess.SubprocessError) as exc:
        print(f"[FAIL] {exc}", file=sys.stderr)
        return 1
    print(f"\n{passed} checks passed ({'read-only simulator + protocol' if args.simulator else 'protocol only'}).")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
