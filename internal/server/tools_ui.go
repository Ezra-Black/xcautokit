package server

import (
	"context"
	"fmt"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/xcautokit/xcautokit/internal/sim"
)

type uiFindIn struct {
	By            string `json:"by" jsonschema:"Selector type: accessibilityId, label, text, value, hint, predicate, role"`
	Query         string `json:"query" jsonschema:"Search query"`
	Index         *int   `json:"index,omitempty" jsonschema:"Optional index if multiple matches"`
	SimulatorUuid string `json:"simulatorUuid,omitempty"`
}

type uiSearchIn struct {
	Query         string `json:"query" jsonschema:"Text to search in the UI hierarchy"`
	SimulatorUuid string `json:"simulatorUuid,omitempty"`
}

type uiPointIn struct {
	X             float64 `json:"x" jsonschema:"X coordinate"`
	Y             float64 `json:"y" jsonschema:"Y coordinate"`
	SimulatorUuid string  `json:"simulatorUuid,omitempty"`
}

type simOnlyIn struct {
	SimulatorUuid string `json:"simulatorUuid,omitempty"`
}

type uiDismissInterruptIn struct {
	Action        string `json:"action" jsonschema:"accept, decline, dismiss, or button"`
	ButtonLabel   string `json:"buttonLabel,omitempty" jsonschema:"Required when action=button; exact or partial button label"`
	Index         *int   `json:"index,omitempty" jsonschema:"Which interrupt to dismiss when multiple (default 0)"`
	SimulatorUuid string `json:"simulatorUuid,omitempty"`
}

func (a *App) registerUITools(srv *mcp.Server) {
	mcp.AddTool(srv, toolMeta("ui_describe", "Describe UI", "Full accessibility tree as JSON with frames/bounds and identifiers", annRO()), func(ctx context.Context, req *mcp.CallToolRequest, in simOnlyIn) (*mcp.CallToolResult, map[string]any, error) {
		udid, err := a.resolveUDID(in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		raw, err := sim.DescribeUI(udid)
		if err != nil {
			return nil, nil, err
		}
		els, err := sim.ParseDescribeUI(raw)
		if err != nil {
			return nil, map[string]any{"raw": raw, "device": udid}, nil
		}
		return nil, map[string]any{"elements": els, "device": udid}, nil
	})

	mcp.AddTool(srv, toolMeta("ui_find", "Find UI Element", "Find elements by selector (accessibilityId, label, text, value, hint, predicate, role). Returns ranked matches with frames and identifiers.", annRO()), func(ctx context.Context, req *mcp.CallToolRequest, in uiFindIn) (*mcp.CallToolResult, map[string]any, error) {
		if in.By == "" || in.Query == "" {
			return nil, nil, fmtError("by and query parameters required")
		}
		udid, err := a.resolveUDID(in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		raw, err := sim.DescribeUI(udid)
		if err != nil {
			return nil, nil, err
		}
		els, err := sim.ParseDescribeUI(raw)
		if err != nil {
			return nil, nil, err
		}
		matches := sim.Find(els, in.By, in.Query)
		out := map[string]any{"count": len(matches), "matches": matches, "device": udid}
		if in.Index != nil {
			if *in.Index < 0 || *in.Index >= len(matches) {
				return nil, nil, fmtError("index out of range")
			}
			out["selected"] = matches[*in.Index]
		}
		return nil, out, nil
	})

	mcp.AddTool(srv, toolMeta("ui_search", "Search UI", "Grep-like text search in UI hierarchy", annRO()), func(ctx context.Context, req *mcp.CallToolRequest, in uiSearchIn) (*mcp.CallToolResult, map[string]any, error) {
		if in.Query == "" {
			return nil, nil, fmtError("query parameter required")
		}
		udid, err := a.resolveUDID(in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		raw, err := sim.DescribeUI(udid)
		if err != nil {
			return nil, nil, err
		}
		els, err := sim.ParseDescribeUI(raw)
		if err != nil {
			return nil, nil, err
		}
		matches := sim.Search(els, in.Query)
		return nil, map[string]any{"count": len(matches), "matches": matches, "device": udid}, nil
	})

	mcp.AddTool(srv, toolMeta("ui_summary", "UI Summary", "LLM-optimized compact summary of the visible UI. Includes interrupts preview (alerts/sheets/permissions) when present.", annRO()), func(ctx context.Context, req *mcp.CallToolRequest, in simOnlyIn) (*mcp.CallToolResult, map[string]any, error) {
		udid, err := a.resolveUDID(in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		raw, err := sim.DescribeUI(udid)
		if err != nil {
			return nil, nil, err
		}
		els, err := sim.ParseDescribeUI(raw)
		if err != nil {
			return nil, nil, err
		}
		intr := sim.DetectInterrupts(els)
		out := map[string]any{
			"summary":       sim.Summarize(els),
			"device":        udid,
			"hasInterrupt":  len(intr) > 0,
			"interrupts":    sim.InterruptPreview(intr),
		}
		return nil, out, nil
	})

	mcp.AddTool(srv, toolMeta("ui_point", "UI Point", "Get element at coordinates", annRO()), func(ctx context.Context, req *mcp.CallToolRequest, in uiPointIn) (*mcp.CallToolResult, map[string]any, error) {
		udid, err := a.resolveUDID(in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		raw, err := sim.DescribeUI(udid)
		if err != nil {
			return nil, nil, err
		}
		els, err := sim.ParseDescribeUI(raw)
		if err != nil {
			return nil, nil, err
		}
		el := sim.ElementAtPoint(els, in.X, in.Y)
		if el == nil {
			return nil, map[string]any{"found": false, "x": in.X, "y": in.Y, "device": udid}, nil
		}
		return nil, map[string]any{"found": true, "element": el, "x": in.X, "y": in.Y, "device": udid}, nil
	})

	mcp.AddTool(srv, toolMeta("ui_check_interrupt", "Check Interrupt", "Detect system/app overlays that may block automation (alerts, sheets, permission prompts, banners, SpringBoard). Detection only — never taps. Call after app_launch/navigation before interacting.", annRO()), func(ctx context.Context, req *mcp.CallToolRequest, in simOnlyIn) (*mcp.CallToolResult, map[string]any, error) {
		udid, err := a.resolveUDID(in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		raw, err := sim.DescribeUI(udid)
		if err != nil {
			return nil, nil, err
		}
		els, err := sim.ParseDescribeUI(raw)
		if err != nil {
			return nil, nil, err
		}
		intr := sim.DetectInterrupts(els)
		return nil, map[string]any{
			"hasInterrupt": len(intr) > 0,
			"count":        len(intr),
			"interrupts":   intr,
			"device":       udid,
			"hint":         "If hasInterrupt, call ui_dismiss_interrupt with explicit action=accept|decline|dismiss|button. Never auto-accept permissions.",
		}, nil
	})

	mcp.AddTool(srv, toolMeta("ui_dismiss_interrupt", "Dismiss Interrupt", "Dismiss a detected interrupt with an explicit action (accept/decline/dismiss/button). Never auto-accepts — you must choose. Re-checks after tap.", annWrite()), func(ctx context.Context, req *mcp.CallToolRequest, in uiDismissInterruptIn) (*mcp.CallToolResult, map[string]any, error) {
		action := strings.ToLower(strings.TrimSpace(in.Action))
		if action == "" {
			return nil, nil, fmtError("action required: accept, decline, dismiss, or button")
		}
		udid, err := a.resolveUDID(in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		raw, err := sim.DescribeUI(udid)
		if err != nil {
			return nil, nil, err
		}
		els, err := sim.ParseDescribeUI(raw)
		if err != nil {
			return nil, nil, err
		}
		intr := sim.DetectInterrupts(els)
		if len(intr) == 0 {
			return nil, map[string]any{
				"dismissed": false,
				"remaining": []sim.Interrupt{},
				"device":    udid,
				"message":   "no interrupt detected",
			}, nil
		}
		idx := 0
		if in.Index != nil {
			idx = *in.Index
		}
		if idx < 0 || idx >= len(intr) {
			return nil, nil, fmtError(fmt.Sprintf("index %d out of range (found %d interrupts)", idx, len(intr)))
		}
		target := intr[idx]
		if target.Kind == sim.KindSpringBoard {
			return nil, nil, fmtError("springboard detected — use button home + app_launch instead of ui_dismiss_interrupt")
		}
		btn, err := sim.PickInterruptButton(target, action, in.ButtonLabel)
		if err != nil {
			return nil, nil, fmtError(err.Error())
		}
		if _, err := sim.RunAxe("tap",
			"-x", fmt.Sprintf("%.0f", btn.CenterX),
			"-y", fmt.Sprintf("%.0f", btn.CenterY),
			"--udid", udid,
		); err != nil {
			return nil, nil, err
		}
		// Re-check
		raw2, err := sim.DescribeUI(udid)
		remaining := []sim.Interrupt{}
		if err == nil {
			if els2, err2 := sim.ParseDescribeUI(raw2); err2 == nil {
				remaining = sim.DetectInterrupts(els2)
			}
		}
		return nil, map[string]any{
			"dismissed": true,
			"pressed":   btn,
			"action":    action,
			"kind":      target.Kind,
			"title":     target.Title,
			"remaining": remaining,
			"hasInterrupt": len(remaining) > 0,
			"device":    udid,
		}, nil
	})
}
