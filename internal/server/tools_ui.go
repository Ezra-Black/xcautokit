package server

import (
	"context"
	"fmt"
	"strconv"
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

type uiSummaryIn struct {
	Limit         int    `json:"limit,omitempty" jsonschema:"Maximum summary elements, 1-100; default 50. Truncation and total are reported explicitly."`
	SimulatorUuid string `json:"simulatorUuid,omitempty"`
}

type uiDismissInterruptIn struct {
	Action        string `json:"action" jsonschema:"accept, decline, dismiss, or button"`
	ButtonLabel   string `json:"buttonLabel,omitempty" jsonschema:"Required when action=button; exact or partial button label"`
	Index         *int   `json:"index,omitempty" jsonschema:"Which interrupt to dismiss when multiple (default 0)"`
	SimulatorUuid string `json:"simulatorUuid,omitempty"`
}

func (a *App) registerUITools(srv *mcp.Server) {
	addTool(a, srv, toolMeta("ui_describe", "Describe UI", "Full accessibility tree as JSON with frames/bounds and identifiers", annRO()), func(ctx context.Context, req *mcp.CallToolRequest, in simOnlyIn) (*mcp.CallToolResult, map[string]any, error) {
		udid, err := a.resolveUDIDContext(ctx, in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		raw, err := sim.DescribeUIContext(ctx, udid)
		if err != nil {
			return nil, nil, err
		}
		els, err := sim.ParseDescribeUI(raw)
		if err != nil {
			return nil, map[string]any{"success": false, "reason": "invalid_ui_snapshot", "message": err.Error(), "raw": raw, "device": udid}, nil
		}
		return nil, map[string]any{"elements": els, "device": udid}, nil
	})

	addTool(a, srv, toolMeta("ui_find", "Find UI Element", "Find elements by selector (accessibilityId, label, text, value, hint, predicate, role). Returns ranked matches with frames and identifiers.", annRO()), func(ctx context.Context, req *mcp.CallToolRequest, in uiFindIn) (*mcp.CallToolResult, map[string]any, error) {
		if in.By == "" || in.Query == "" {
			return nil, nil, fmtError("by and query parameters required")
		}
		udid, err := a.resolveUDIDContext(ctx, in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		raw, err := sim.DescribeUIContext(ctx, udid)
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

	addTool(a, srv, toolMeta("ui_search", "Search UI", "Grep-like text search in UI hierarchy", annRO()), func(ctx context.Context, req *mcp.CallToolRequest, in uiSearchIn) (*mcp.CallToolResult, map[string]any, error) {
		if in.Query == "" {
			return nil, nil, fmtError("query parameter required")
		}
		udid, err := a.resolveUDIDContext(ctx, in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		raw, err := sim.DescribeUIContext(ctx, udid)
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

	addTool(a, srv, toolMeta("ui_summary", "UI Summary", "Compact summary of visible UI, with bounded text and element count, total, and explicit truncation. Includes interrupts preview (alerts/sheets/permissions) when present. Use ui_describe for the full tree.", annRO()), func(ctx context.Context, req *mcp.CallToolRequest, in uiSummaryIn) (*mcp.CallToolResult, map[string]any, error) {
		if in.Limit < 0 || in.Limit > 100 {
			return nil, nil, fmt.Errorf("limit must be between 1 and 100, or omitted")
		}
		if in.Limit == 0 {
			in.Limit = 50
		}
		udid, err := a.resolveUDIDContext(ctx, in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		raw, err := sim.DescribeUIContext(ctx, udid)
		if err != nil {
			return nil, nil, err
		}
		els, err := sim.ParseDescribeUI(raw)
		if err != nil {
			return nil, nil, err
		}
		intr := sim.DetectInterrupts(els)
		observation := sim.ObserveUI(els, in.Limit)
		out := map[string]any{
			"summary":      observation.Elements,
			"total":        observation.Total,
			"truncated":    observation.Truncated,
			"device":       udid,
			"hasInterrupt": len(intr) > 0,
			"interrupts":   sim.InterruptPreview(intr),
		}
		return nil, out, nil
	})

	addTool(a, srv, toolMeta("ui_point", "UI Point", "Get element at coordinates", annRO()), func(ctx context.Context, req *mcp.CallToolRequest, in uiPointIn) (*mcp.CallToolResult, map[string]any, error) {
		udid, err := a.resolveUDIDContext(ctx, in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		raw, err := sim.DescribeUIContext(ctx, udid)
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

	addTool(a, srv, toolMeta("ui_check_interrupt", "Check Interrupt", "Detect system/app overlays that may block automation (alerts, sheets, permission prompts, banners, SpringBoard). Detection only — never taps. Call after app_launch/navigation before interacting.", annRO()), func(ctx context.Context, req *mcp.CallToolRequest, in simOnlyIn) (*mcp.CallToolResult, map[string]any, error) {
		udid, err := a.resolveUDIDContext(ctx, in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		raw, err := sim.DescribeUIContext(ctx, udid)
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

	addTool(a, srv, toolMeta("ui_dismiss_interrupt", "Dismiss Interrupt", "Dismiss a detected interrupt with an explicit action (accept/decline/dismiss/button). Never auto-accepts — you must choose. Re-checks after tap.", annWrite()), func(ctx context.Context, req *mcp.CallToolRequest, in uiDismissInterruptIn) (*mcp.CallToolResult, map[string]any, error) {
		action := strings.ToLower(strings.TrimSpace(in.Action))
		if action == "" {
			return nil, nil, fmtError("action required: accept, decline, dismiss, or button")
		}
		udid, err := a.resolveUDIDContext(ctx, in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		raw, err := sim.DescribeUIContext(ctx, udid)
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
				"success":      true,
				"performed":    false,
				"dismissed":    false,
				"verified":     true,
				"hasInterrupt": false,
				"remaining":    []sim.Interrupt{},
				"device":       udid,
				"message":      "no interrupt detected",
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
		if _, err := sim.RunAxeContext(ctx, "tap",
			"-x", strconv.FormatFloat(btn.CenterX, 'f', -1, 64),
			"-y", strconv.FormatFloat(btn.CenterY, 'f', -1, 64),
			"--udid", udid,
		); err != nil {
			return nil, map[string]any{"success": false, "performed": nil, "dismissed": nil, "verified": false, "actionOutcome": "unknown", "reason": "action_failed", "device": udid, "message": "Interrupt tap completion is unknown. Re-check before retrying. " + err.Error()}, nil
		}
		// Re-check
		raw2, err := sim.DescribeUIContext(ctx, udid)
		remaining := []sim.Interrupt{}
		if err == nil {
			var els2 []sim.Element
			els2, err = sim.ParseDescribeUI(raw2)
			if err == nil {
				remaining = sim.DetectInterrupts(els2)
			}
		}
		return nil, dismissInterruptResult(udid, action, target, *btn, intr, remaining, err), nil
	})
}

// A successful input command proves only that the tap was sent. Report a
// dismissal only when a fresh observation no longer contains that interrupt.
func dismissInterruptResult(udid, action string, target sim.Interrupt, button sim.InterruptButton, before, remaining []sim.Interrupt, observationErr error) map[string]any {
	out := map[string]any{
		"performed": true, "pressed": button, "action": action, "kind": target.Kind,
		"title": target.Title, "device": udid,
	}
	if observationErr != nil {
		out["success"], out["verified"] = false, false
		out["dismissed"], out["remaining"], out["hasInterrupt"] = nil, nil, nil
		out["lastKnownInterrupts"] = before
		out["reason"] = "post_action_observation_failed"
		out["message"] = "The interrupt tap completed, but dismissal could not be verified. Re-check before another action. " + observationErr.Error()
		return out
	}
	stillPresent := false
	for _, interrupt := range remaining {
		if interrupt.Kind == target.Kind && interrupt.Title == target.Title && interrupt.Message == target.Message {
			stillPresent = true
			break
		}
	}
	out["success"], out["verified"], out["dismissed"] = !stillPresent, true, !stillPresent
	out["remaining"], out["hasInterrupt"] = remaining, len(remaining) > 0
	if stillPresent {
		out["reason"] = "interrupt_still_present"
		out["message"] = "The tap completed, but the original interrupt is still detected. Re-check after the transition before another action."
	}
	return out
}
