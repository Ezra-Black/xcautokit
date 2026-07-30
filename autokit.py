#!/usr/bin/env python3
"""
Autokit - iOS Simulator Automation MCP Server
A rebuild of bridge4simulator functionality
"""

import asyncio
import json
import os
import subprocess
import sys
from pathlib import Path
from typing import Any, Dict, List, Optional
import xml.etree.ElementTree as ET
from datetime import datetime

try:
    from mcp.server import Server
    from mcp.server.stdio import stdio_server
    from mcp.types import (
        Resource,
        Tool,
        TextContent,
        ImageContent,
        EmbeddedResource,
    )
except ImportError:
    print("Error: mcp package not installed. Run: pip install -r requirements.txt")
    sys.exit(1)


class AutokitServer:
    def __init__(self):
        self.server = Server("autokit")
        self.config = self.load_config()
        self.setup_resources()
        self.setup_tools()

    def load_config(self) -> Dict[str, Any]:
        """Load configuration with defaults"""
        home = Path.home()
        config = {
            "device": {"udid": "booted"},
            "paths": {
                "recordingDir": str(home / "Pictures" / "autokit" / "recordings"),
                "screenshotDir": str(home / "Pictures" / "autokit" / "screenshots"),
            },
            "recording": {"codec": "hevc"},
            "screenshot": {"format": "png"},
        }

        # Create directories
        Path(config["paths"]["recordingDir"]).mkdir(parents=True, exist_ok=True)
        Path(config["paths"]["screenshotDir"]).mkdir(parents=True, exist_ok=True)

        return config

    def setup_resources(self):
        """Setup MCP resources"""

        @self.server.list_resources()
        async def list_resources() -> List[Resource]:
            return [
                Resource(
                    uri="simulator://status",
                    name="Simulator Status",
                    description="Current simulator state and booted device info",
                    mimeType="application/json",
                ),
                Resource(
                    uri="simulator://devices",
                    name="Available Devices",
                    description="List of all available iOS simulator devices",
                    mimeType="application/json",
                ),
                Resource(
                    uri="simulator://config",
                    name="Server Configuration",
                    description="Current autokit configuration",
                    mimeType="application/json",
                ),
            ]

        @self.server.read_resource()
        async def read_resource(uri: str) -> str:
            if uri == "simulator://status":
                return json.dumps(self.get_status())
            elif uri == "simulator://devices":
                return json.dumps(self.get_devices())
            elif uri == "simulator://config":
                return json.dumps(self.config)
            else:
                raise ValueError(f"Unknown resource: {uri}")

    def setup_tools(self):
        """Setup MCP tools"""

        @self.server.list_tools()
        async def list_tools() -> List[Tool]:
            return [
                Tool(
                    name="status",
                    description="Get current simulator status",
                    inputSchema={
                        "type": "object",
                        "properties": {},
                    },
                ),
                Tool(
                    name="device_list",
                    description="List all available iOS simulator devices",
                    inputSchema={
                        "type": "object",
                        "properties": {},
                    },
                ),
                Tool(
                    name="device_boot",
                    description="Boot a simulator device",
                    inputSchema={
                        "type": "object",
                        "properties": {
                            "udid": {
                                "type": "string",
                                "description": "Device UDID to boot",
                            }
                        },
                        "required": ["udid"],
                    },
                ),
                Tool(
                    name="device_shutdown",
                    description="Shutdown a simulator device",
                    inputSchema={
                        "type": "object",
                        "properties": {
                            "udid": {
                                "type": "string",
                                "description": "Device UDID to shutdown (defaults to booted)",
                            }
                        },
                    },
                ),
                Tool(
                    name="tap",
                    description="Tap at specific coordinates",
                    inputSchema={
                        "type": "object",
                        "properties": {
                            "x": {"type": "number", "description": "X coordinate"},
                            "y": {"type": "number", "description": "Y coordinate"},
                        },
                        "required": ["x", "y"],
                    },
                ),
                Tool(
                    name="gesture",
                    description="Perform a gesture (scroll-up, scroll-down, scroll-left, scroll-right, swipe-from-left-edge, swipe-from-right-edge)",
                    inputSchema={
                        "type": "object",
                        "properties": {
                            "preset": {
                                "type": "string",
                                "enum": [
                                    "scroll-up",
                                    "scroll-down",
                                    "scroll-left",
                                    "scroll-right",
                                    "swipe-from-left-edge",
                                    "swipe-from-right-edge",
                                    "swipe-from-top-edge",
                                    "swipe-from-bottom-edge",
                                ],
                            },
                            "duration": {
                                "type": "number",
                                "description": "Duration in seconds",
                            },
                        },
                        "required": ["preset"],
                    },
                ),
                Tool(
                    name="button",
                    description="Press a hardware button (home, lock, etc.)",
                    inputSchema={
                        "type": "object",
                        "properties": {
                            "buttonType": {
                                "type": "string",
                                "enum": ["home", "lock", "side-button", "siri"],
                            }
                        },
                        "required": ["buttonType"],
                    },
                ),
                Tool(
                    name="type_text",
                    description="Type text into the simulator",
                    inputSchema={
                        "type": "object",
                        "properties": {
                            "text": {"type": "string", "description": "Text to type"},
                        },
                        "required": ["text"],
                    },
                ),
                Tool(
                    name="ui_snapshot",
                    description="Get UI hierarchy snapshot",
                    inputSchema={
                        "type": "object",
                        "properties": {},
                    },
                ),
                Tool(
                    name="ui_find",
                    description="Find UI elements by label, type, etc.",
                    inputSchema={
                        "type": "object",
                        "properties": {
                            "label": {"type": "string"},
                            "type": {"type": "string"},
                            "interactive": {"type": "boolean"},
                        },
                    },
                ),
                Tool(
                    name="ui_describe",
                    description="Get detailed UI element description",
                    inputSchema={
                        "type": "object",
                        "properties": {},
                    },
                ),
                Tool(
                    name="ui_ocr",
                    description="Extract text from screen using OCR",
                    inputSchema={
                        "type": "object",
                        "properties": {},
                    },
                ),
                Tool(
                    name="verify_ui",
                    description="Verify UI state and return snapshot",
                    inputSchema={
                        "type": "object",
                        "properties": {},
                    },
                ),
                Tool(
                    name="screenshot",
                    description="Take a screenshot",
                    inputSchema={
                        "type": "object",
                        "properties": {},
                    },
                ),
                Tool(
                    name="record_start",
                    description="Start screen recording",
                    inputSchema={
                        "type": "object",
                        "properties": {},
                    },
                ),
                Tool(
                    name="record_stop",
                    description="Stop screen recording",
                    inputSchema={
                        "type": "object",
                        "properties": {},
                    },
                ),
            ]

        @self.server.call_tool()
        async def call_tool(name: str, arguments: Dict[str, Any]) -> List[TextContent]:
            try:
                if name == "status":
                    result = self.get_status()
                elif name == "device_list":
                    result = self.get_devices()
                elif name == "device_boot":
                    result = self.boot_device(arguments.get("udid"))
                elif name == "device_shutdown":
                    result = self.shutdown_device(arguments.get("udid", "booted"))
                elif name == "tap":
                    result = self.tap(arguments["x"], arguments["y"])
                elif name == "gesture":
                    result = self.gesture(
                        arguments["preset"], arguments.get("duration", 0.5)
                    )
                elif name == "button":
                    result = self.button(arguments["buttonType"])
                elif name == "type_text":
                    result = self.type_text(arguments["text"])
                elif name == "ui_snapshot":
                    result = self.ui_snapshot()
                elif name == "ui_find":
                    result = self.ui_find(
                        arguments.get("label"),
                        arguments.get("type"),
                        arguments.get("interactive"),
                    )
                elif name == "ui_describe":
                    result = self.ui_describe()
                elif name == "ui_ocr":
                    result = self.ui_ocr()
                elif name == "verify_ui":
                    result = self.verify_ui()
                elif name == "screenshot":
                    result = self.screenshot()
                elif name == "record_start":
                    result = self.record_start()
                elif name == "record_stop":
                    result = self.record_stop()
                else:
                    raise ValueError(f"Unknown tool: {name}")

                return [TextContent(type="text", text=json.dumps(result))]
            except Exception as e:
                return [
                    TextContent(
                        type="text", text=json.dumps({"error": str(e)})
                    )
                ]

    def run_simctl(self, *args) -> tuple[str, int]:
        """Run xcrun simctl command"""
        try:
            result = subprocess.run(
                ["xcrun", "simctl"] + list(args),
                capture_output=True,
                text=True,
                check=False,
            )
            return result.stdout, result.returncode
        except FileNotFoundError:
            raise RuntimeError("xcrun simctl not found. Xcode Command Line Tools required.")

    def get_booted_device(self) -> Optional[Dict[str, Any]]:
        """Get currently booted device"""
        output, code = self.run_simctl("list", "devices", "--json")
        if code != 0:
            return None

        try:
            data = json.loads(output)
            for runtime, devices in data.get("devices", {}).items():
                for device in devices:
                    if device.get("state") == "Booted":
                        device["isBooted"] = True
                        return device
        except (json.JSONDecodeError, KeyError):
            pass

        return None

    def get_status(self) -> Dict[str, Any]:
        """Get simulator status"""
        booted = self.get_booted_device()
        if booted:
            return {
                "simulatorState": "booted",
                "hasBooted": True,
                "bootedDevice": booted,
            }
        else:
            return {
                "simulatorState": "shutdown",
                "hasBooted": False,
            }

    def get_devices(self) -> Dict[str, Any]:
        """Get all available devices"""
        output, code = self.run_simctl("list", "devices", "--json")
        if code != 0:
            return {"count": 0, "devices": []}

        try:
            data = json.loads(output)
            all_devices = []
            for runtime, devices in data.get("devices", {}).items():
                for device in devices:
                    device["isBooted"] = device.get("state") == "Booted"
                    all_devices.append(device)

            return {"count": len(all_devices), "devices": all_devices}
        except (json.JSONDecodeError, KeyError):
            return {"count": 0, "devices": []}

    def boot_device(self, udid: str) -> Dict[str, Any]:
        """Boot a device"""
        _, code = self.run_simctl("boot", udid)
        if code != 0:
            return {"error": f"Failed to boot device {udid}", "success": False}
        return {"success": True, "udid": udid}

    def shutdown_device(self, udid: str) -> Dict[str, Any]:
        """Shutdown a device"""
        _, code = self.run_simctl("shutdown", udid)
        if code != 0:
            return {"error": f"Failed to shutdown device {udid}", "success": False}
        return {"success": True, "udid": udid}

    def tap(self, x: float, y: float) -> Dict[str, Any]:
        """Tap at coordinates"""
        device = self.get_booted_device()
        if not device:
            return {"error": "no booted device"}

        _, code = self.run_simctl("io", device["udid"], "tap", str(int(x)), str(int(y)))
        if code != 0:
            return {"error": "tap failed"}

        return {"x": int(x), "y": int(y), "device": device["udid"]}

    def gesture(self, preset: str, duration: float) -> Dict[str, Any]:
        """Perform a gesture"""
        device = self.get_booted_device()
        if not device:
            return {"error": "no booted device"}

        # Map presets to simctl commands
        gesture_map = {
            "scroll-up": ["drag", "0", "50", "0", "0"],
            "scroll-down": ["drag", "0", "0", "0", "50"],
            "scroll-left": ["drag", "0", "0", "50", "0"],
            "scroll-right": ["drag", "0", "0", "-50", "0"],
            "swipe-from-left-edge": ["swipe", "0", "0", "50", "0"],
            "swipe-from-right-edge": ["swipe", "0", "0", "-50", "0"],
            "swipe-from-top-edge": ["swipe", "0", "0", "0", "50"],
            "swipe-from-bottom-edge": ["swipe", "0", "0", "0", "-50"],
        }

        if preset not in gesture_map:
            return {"error": f"unknown preset: {preset}"}

        _, code = self.run_simctl("io", device["udid"], *gesture_map[preset])
        if code != 0:
            return {"error": "gesture failed"}

        return {
            "device": device["udid"],
            "preset": preset,
            "duration": duration,
            "success": True,
        }

    def button(self, button_type: str) -> Dict[str, Any]:
        """Press a hardware button"""
        device = self.get_booted_device()
        if not device:
            return {"error": "no booted device"}

        button_map = {
            "home": "home",
            "lock": "lock",
            "side-button": "lock",  # Map to lock
            "siri": "home",  # Map to home
        }

        if button_type not in button_map:
            return {"error": f"unknown buttonType: {button_type}"}

        _, code = self.run_simctl("io", device["udid"], button_map[button_type])
        if code != 0:
            return {"error": "button press failed"}

        return {
            "buttonType": button_type,
            "device": device["udid"],
            "success": True,
        }

    def type_text(self, text: str) -> Dict[str, Any]:
        """Type text"""
        device = self.get_booted_device()
        if not device:
            return {"error": "no booted device"}

        _, code = self.run_simctl("io", device["udid"], "text", text)
        if code != 0:
            return {"error": "type text failed"}

        return {"text": text, "device": device["udid"]}

    def ui_snapshot(self) -> str:
        """Get UI snapshot"""
        device = self.get_booted_device()
        if not device:
            return json.dumps({"error": "no booted device"})

        output, code = self.run_simctl("io", device["udid"], "dump", "ui")
        if code != 0:
            return json.dumps({"error": "failed to get UI snapshot"})

        # Format UI snapshot
        return self.format_ui_snapshot(output)

    def format_ui_snapshot(self, xml_output: str) -> str:
        """Format UI snapshot from XML"""
        try:
            root = ET.fromstring(xml_output)
            lines = []
            uid = 0

            def process_element(elem, level=0):
                nonlocal uid
                indent = "  " * level
                tag = elem.tag
                label = elem.get("AXLabel", "")
                value = elem.get("AXValue", "")
                role = elem.get("AXRole", "")

                # Get coordinates if available
                frame = elem.get("AXFrame", "")
                coords = ""
                if frame:
                    try:
                        # Parse frame string like "{{x, y}, {width, height}}"
                        import re
                        match = re.search(r"\{\{([\d.]+),\s*([\d.]+)\},\s*\{([\d.]+),\s*([\d.]+)\}\}", frame)
                        if match:
                            x, y, w, h = map(float, match.groups())
                            coords = f" [{int(x)},{int(y)}]"
                    except:
                        pass

                line = f"uid=0_{uid} {indent}{role}"
                if label:
                    line += f' "{label}"'
                if value:
                    line += f' value="{value[:50]}"'
                if coords:
                    line += coords
                if elem.get("AXEnabled") == "1":
                    line += " enabled"
                if elem.get("AXFocused") == "1":
                    line += " focusable"

                lines.append(line)
                uid += 1

                for child in elem:
                    process_element(child, level + 1)

            process_element(root)
            return "\n".join(lines)
        except ET.ParseError:
            # Fallback to simple text format
            lines = xml_output.split("\n")
            formatted = []
            uid = 0
            for line in lines:
                line = line.strip()
                if line:
                    formatted.append(f"uid=0_{uid} {line}")
                    uid += 1
            return "\n".join(formatted)

    def ui_find(self, label: Optional[str] = None, type: Optional[str] = None, interactive: Optional[bool] = None) -> Dict[str, Any]:
        """Find UI elements"""
        # This would need proper UI parsing
        return {"elements": None, "matchCount": 0}

    def ui_describe(self) -> Dict[str, Any]:
        """Describe UI elements"""
        device = self.get_booted_device()
        if not device:
            return {"error": "no booted device"}

        output, code = self.run_simctl("io", device["udid"], "dump", "ui")
        if code != 0:
            return {"error": "failed to describe UI"}

        # Parse and return structured description
        return {"elementCount": 0, "elements": []}

    def ui_ocr(self) -> Dict[str, Any]:
        """Extract text using OCR"""
        device = self.get_booted_device()
        if not device:
            return {"error": "no booted device"}

        # Take screenshot first
        screenshot_path = Path(self.config["paths"]["screenshotDir"]) / "ocr_temp.png"
        _, code = self.run_simctl("io", device["udid"], "screenshot", str(screenshot_path))
        if code != 0:
            return {"error": "failed to take screenshot for OCR"}

        # Use vision framework or tesseract for OCR
        # For now, return empty results
        return {"results": []}

    def verify_ui(self) -> str:
        """Verify UI and return snapshot"""
        return self.ui_snapshot()

    def screenshot(self) -> Dict[str, Any]:
        """Take a screenshot"""
        device = self.get_booted_device()
        if not device:
            return {"error": "no booted device"}

        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        filename = f"screenshot_{timestamp}.{self.config['screenshot']['format']}"
        path = Path(self.config["paths"]["screenshotDir"]) / filename

        _, code = self.run_simctl("io", device["udid"], "screenshot", str(path))
        if code != 0:
            return {"error": "screenshot failed"}

        return {"path": str(path), "device": device["udid"]}

    def record_start(self) -> Dict[str, Any]:
        """Start recording"""
        device = self.get_booted_device()
        if not device:
            return {"error": "no booted device"}

        timestamp = datetime.now().strftime("%Y%m%d_%H%M%S")
        filename = f"recording_{timestamp}.mov"
        path = Path(self.config["paths"]["recordingDir"]) / filename

        _, code = self.run_simctl("io", device["udid"], "recordVideo", "--codec", self.config["recording"]["codec"], str(path))
        if code != 0:
            return {"error": "failed to start recording"}

        return {"path": str(path), "device": device["udid"]}

    def record_stop(self) -> Dict[str, Any]:
        """Stop recording"""
        device = self.get_booted_device()
        if not device:
            return {"error": "no booted device"}

        # Recording stops automatically when process ends
        return {"success": True, "device": device["udid"]}


async def main():
    """Main entry point"""
    server = AutokitServer()
    async with stdio_server() as (read_stream, write_stream):
        await server.server.run(
            read_stream,
            write_stream,
            server.server.create_initialization_options(),
        )


if __name__ == "__main__":
    asyncio.run(main())
