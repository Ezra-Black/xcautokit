package server

import (
	"context"
	"encoding/json"

	"github.com/xcautokit/xcautokit/internal/config"
	"github.com/xcautokit/xcautokit/internal/sim"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (a *App) registerResources(srv *mcp.Server) {
	srv.AddResource(&mcp.Resource{
		URI:         "simulator://status",
		Name:        "Simulator Status",
		Description: "Current simulator state and booted device info",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		st, err := sim.GetStatus()
		if err != nil {
			return textResource(req.Params.URI, map[string]any{"error": err.Error()}), nil
		}
		return textResource(req.Params.URI, st), nil
	})

	srv.AddResource(&mcp.Resource{
		URI:         "simulator://devices",
		Name:        "Available Devices",
		Description: "List of all available iOS simulator devices",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		devices, err := sim.ListDevices()
		if err != nil {
			return textResource(req.Params.URI, map[string]any{"error": err.Error()}), nil
		}
		return textResource(req.Params.URI, map[string]any{"count": len(devices), "devices": devices}), nil
	})

	srv.AddResource(&mcp.Resource{
		URI:         "simulator://config",
		Name:        "Server Configuration",
		Description: "Current XCAutokit configuration and session defaults",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return textResource(req.Params.URI, map[string]any{
			"version":          config.Version,
			"deviceUDID":       a.Cfg.DeviceUDID,
			"recordingDir":     a.Cfg.RecordingDir,
			"screenshotDir":    a.Cfg.ScreenshotDir,
			"recordingCodec":   a.Cfg.RecordingCodec,
			"screenshotFormat": a.Cfg.ScreenshotFormat,
			"session":          a.Session.Get(),
		}), nil
	})

	srv.AddResource(&mcp.Resource{
		URI:         "xcode://status",
		Name:        "Xcode Backend Status",
		Description: "mcpbridge availability, connection state, tabIdentifier, workspace path",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		st := a.Bridge.Status(ctx)
		if st.Available && a.Bridge.Enabled() {
			// Best-effort refresh of window info without failing the resource.
			_, _ = a.Bridge.ListWindows(ctx)
			st = a.Bridge.Status(ctx)
		}
		return textResource(req.Params.URI, st), nil
	})

	srv.AddResource(&mcp.Resource{
		URI:         "simulator://interrupts",
		Name:        "Live Interrupts",
		Description: "Current blocking overlays (alerts, sheets, permissions) on the booted simulator",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		udid, err := a.resolveUDID("")
		if err != nil {
			return textResource(req.Params.URI, map[string]any{"error": err.Error()}), nil
		}
		has, interrupts := a.checkInterrupts(udid)
		return textResource(req.Params.URI, map[string]any{
			"device":       udid,
			"hasInterrupt": has,
			"interrupts":   interrupts,
			"blocking":     blockingInterrupts(interrupts),
		}), nil
	})

	srv.AddResource(&mcp.Resource{
		URI:         "xcautokit://agent-guide",
		Name:        "Agent Guide",
		Description: "Customer-facing agent instructions: interrupts, tickets, tool routing (ships with MCP — not Cursor-local)",
		MIMEType:    "text/markdown",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		return &mcp.ReadResourceResult{
			Contents: []*mcp.ResourceContents{{
				URI:      req.Params.URI,
				MIMEType: "text/markdown",
				Text:     AgentInstructions + "\n\n" + agentGuideExtra,
			}},
		}, nil
	})
}

const agentGuideExtra = `## Tool routing (short)

| Task | Tools |
|------|--------|
| Interrupts | ui_check_interrupt, ui_dismiss_interrupt |
| UI inspect | ui_summary, ui_describe, ui_find |
| UI input | gesture, tap, swipe, type_text (blocked while interrupt present) |
| Capture | screenshot, record_start/stop (process-local ticket) |
| Build | build_sim, build_run_sim, test_sim |

## Tickets

Tickets are process-local handles. Pass them back to the matching stop tool. After an MCP restart, start a new capture.

## Workflow filter

Optional env XCAUTOKIT_WORKFLOWS=core or device,ui,input,build to reduce tools exposed to the model.
`

func textResource(uri string, v any) *mcp.ReadResourceResult {
	b, _ := json.MarshalIndent(v, "", "  ")
	return &mcp.ReadResourceResult{
		Contents: []*mcp.ResourceContents{{
			URI:      uri,
			MIMEType: "application/json",
			Text:     string(b),
		}},
	}
}
