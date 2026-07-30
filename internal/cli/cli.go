package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/xcautokit/xcautokit/internal/config"
	"github.com/xcautokit/xcautokit/internal/server"
	"github.com/xcautokit/xcautokit/internal/sim"
	"github.com/xcautokit/xcautokit/internal/xcodebridge"
)

func Run(args []string) error {
	if len(args) == 0 {
		return cmdHelp()
	}
	switch args[0] {
	case "mcp", "serve":
		return cmdMCP()
	case "version", "--version", "-v":
		fmt.Printf("xcautokit %s\n", config.Version)
		return nil
	case "help", "--help", "-h":
		return cmdHelp()
	case "tools":
		return cmdTools()
	case "doctor":
		return cmdDoctor()
	case "init":
		return cmdInit(args[1:])
	case "status":
		return cmdStatus()
	case "interrupt":
		return cmdInterrupt(args[1:])
	default:
		return fmt.Errorf("unknown command %q\n\nRun: xcautokit help", args[0])
	}
}

func cmdHelp() error {
	fmt.Print(`XCAutokit — first-party MCP + CLI for Xcode and iOS Simulator

Usage:
  xcautokit mcp                 Start the MCP server (stdio)
  xcautokit tools               List MCP tools
  xcautokit doctor              Check local dependencies
  xcautokit status              Show simulator + Xcode backend status
  xcautokit interrupt check [--udid UDID]
  xcautokit interrupt dismiss --action decline|accept|dismiss [--udid UDID]
  xcautokit init [--dir PATH]   Write AGENTS.md + Cursor MCP snippet
  xcautokit version             Print version
  xcautokit help                Show this help

MCP client config (Cursor / Claude Code / Codex):
  {
    "mcpServers": {
      "xcautokit": {
        "command": "npx",
        "args": ["-y", "xcautokit@latest", "mcp"]
      }
    }
  }

Or after global install:
  npm install -g xcautokit@latest
  # then: "command": "xcautokit", "args": ["mcp"]

Env:
  XCAUTOKIT_XCODE_BACKEND=auto|on|off
  MCP_XCODE_PID=<pid>
`)
	return nil
}

func cmdMCP() error {
	app := server.New()
	return app.Run(context.Background())
}

func cmdTools() error {
	tools := server.ToolCatalog()
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "NAME\tCATEGORY\tDESCRIPTION\n")
	for _, t := range tools {
		fmt.Fprintf(w, "%s\t%s\t%s\n", t.Name, t.Category, truncate(t.Description, 72))
	}
	_ = w.Flush()
	fmt.Printf("\n%d tools\n", len(tools))
	return nil
}

func cmdDoctor() error {
	type check struct {
		Name   string `json:"name"`
		OK     bool   `json:"ok"`
		Detail string `json:"detail"`
	}
	var checks []check

	add := func(name string, ok bool, detail string) {
		checks = append(checks, check{Name: name, OK: ok, Detail: detail})
		mark := "ok"
		if !ok {
			mark = "FAIL"
		}
		fmt.Printf("[%s] %s — %s\n", mark, name, detail)
	}

	if _, err := os.Stat("/usr/bin/xcrun"); err == nil {
		add("xcrun", true, "present")
	} else {
		add("xcrun", false, err.Error())
	}

	if path, err := lookPath("axe"); err == nil {
		add("axe", true, path)
	} else {
		add("axe", false, "missing — install AXe for UI automation")
	}

	st, err := sim.GetStatus()
	if err != nil {
		add("simulator", false, err.Error())
	} else if st.HasBooted && st.BootedDevice != nil {
		add("simulator", true, fmt.Sprintf("booted %s (%s)", st.BootedDevice.Name, st.BootedDevice.UDID))
	} else {
		add("simulator", true, "no device booted (ok)")
	}

	bridge := xcodebridge.New(xcodebridge.ModeFromEnv())
	bst := bridge.Status(context.Background())
	if bst.Available {
		add("mcpbridge", true, bst.BridgePath)
	} else {
		add("mcpbridge", false, "needs Xcode 26.3+ — XCAutokit falls back to xcodebuild")
	}

	fail := 0
	for _, c := range checks {
		if !c.OK && c.Name != "mcpbridge" && c.Name != "simulator" {
			fail++
		}
	}
	if fail > 0 {
		return fmt.Errorf("doctor found %d blocking issue(s)", fail)
	}
	fmt.Println("\ndoctor: ready")
	return nil
}

func cmdStatus() error {
	st, err := sim.GetStatus()
	if err != nil {
		return err
	}
	bridge := xcodebridge.New(xcodebridge.ModeFromEnv()).Status(context.Background())
	out := map[string]any{
		"version":      config.Version,
		"simulator":    st,
		"xcodeBackend": bridge,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func cmdInit(args []string) error {
	dir := "."
	for i := 0; i < len(args); i++ {
		if args[i] == "--dir" && i+1 < len(args) {
			dir = args[i+1]
			i++
		}
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	agents := filepath.Join(abs, "AGENTS.md")
	if err := os.WriteFile(agents, []byte(agentsTemplate()), 0644); err != nil {
		return err
	}
	cursorDir := filepath.Join(abs, ".cursor")
	_ = os.MkdirAll(cursorDir, 0755)
	mcpPath := filepath.Join(cursorDir, "mcp.json")
	mcp := `{
  "mcpServers": {
    "xcautokit": {
      "command": "npx",
      "args": ["-y", "xcautokit@latest", "mcp"]
    }
  }
}
`
	if err := os.WriteFile(mcpPath, []byte(mcp), 0644); err != nil {
		return err
	}
	fmt.Printf("Wrote %s\nWrote %s\n", agents, mcpPath)
	fmt.Println("Reload MCP in your client, then run: xcautokit doctor")
	return nil
}

func agentsTemplate() string {
	return `# XCAutokit

Use XCAutokit as the first-party MCP for Xcode + iOS Simulator work.

## Install

` + "```json" + `
{
  "mcpServers": {
    "xcautokit": {
      "command": "npx",
      "args": ["-y", "xcautokit@latest", "mcp"]
    }
  }
}
` + "```" + `

Or: ` + "`npm i -g xcautokit && xcautokit mcp`" + `

## Workflow

1. ` + "`session_set_defaults`" + ` with projectPath + scheme
2. Prefer ` + "`build_sim` / `test_sim`" + ` (mcpbridge when Xcode 26.3+, else xcodebuild)
3. After launch: read ` + "`hasInterrupt`" + ` on the launch result (auto-attached), or call ` + "`ui_check_interrupt`" + `. If blocked, ` + "`ui_dismiss_interrupt`" + ` with explicit action. Never silently auto-accept permissions.
4. Verify with ` + "`ui_summary`" + `, ` + "`screenshot`" + `, ` + "`gesture`" + ` (input tools refuse while overlays block)
5. Stateful ops return a **ticket** — pass it back on stop/follow-up calls

## Interrupts (built into MCP)

MCP initialize instructions + resources ` + "`xcautokit://agent-guide`" + ` and ` + "`simulator://interrupts`" + ` ship this to every client. Cursor rules are not required for customers.

## Xcode live backend

Enable Xcode Settings → Intelligence → Allow external agents to use Xcode tools.
`
}

func lookPath(bin string) (string, error) {
	path := os.Getenv("PATH")
	for _, dir := range strings.Split(path, string(os.PathListSeparator)) {
		p := filepath.Join(dir, bin)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("not found")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
