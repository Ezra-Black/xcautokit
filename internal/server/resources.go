package server

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/xcautokit/xcautokit/internal/config"
	"github.com/xcautokit/xcautokit/internal/sim"
)

func (a *App) registerResources(srv *mcp.Server) {
	srv.AddResource(&mcp.Resource{
		URI:         "simulator://status",
		Name:        "Simulator Status",
		Description: "Current simulator state and booted device info",
		MIMEType:    "application/json",
	}, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
		st, err := sim.GetStatusContext(ctx)
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
		devices, err := sim.ListDevicesContext(ctx)
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
		udid, err := a.resolveUDIDContext(ctx, "")
		if err != nil {
			return textResource(req.Params.URI, map[string]any{"error": err.Error()}), nil
		}
		interrupts, err := a.checkInterrupts(ctx, udid)
		if err != nil {
			return textResource(req.Params.URI, map[string]any{"device": udid, "hasInterrupt": nil, "interruptCheck": "unavailable", "error": err.Error()}), nil
		}
		return textResource(req.Params.URI, map[string]any{
			"device":       udid,
			"hasInterrupt": len(interrupts) > 0,
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
	for _, resource := range []*mcp.ResourceTemplate{
		{URITemplate: "xcautokit://runs/{runId}", Name: "Workflow Trace", Description: "Full persisted workflow observations and outcomes; available to host agents sharing XCAUTOKIT_STATE_DIR", MIMEType: "application/json"},
		{URITemplate: "xcautokit://runs/{runId}/screen", Name: "Workflow Screenshot", Description: "Final or failure screenshot for a workflow run, when captured", MIMEType: "image/png"},
	} {
		srv.AddResourceTemplate(resource, func(ctx context.Context, req *mcp.ReadResourceRequest) (*mcp.ReadResourceResult, error) {
			return readRunResource(stateDir(), req.Params.URI)
		})
	}
}

func readRunResource(root, uri string) (*mcp.ReadResourceResult, error) {
	const prefix = "xcautokit://runs/"
	if !strings.HasPrefix(uri, prefix) {
		return nil, fmt.Errorf("invalid run resource")
	}
	parts := strings.Split(strings.TrimPrefix(uri, prefix), "/")
	if !runIDRE.MatchString(parts[0]) || len(parts) > 2 || (len(parts) == 2 && parts[1] != "screen") {
		return nil, fmt.Errorf("invalid run resource")
	}
	name, mime, limit := "trace.json", "application/json", int64(2<<20)
	if len(parts) == 2 {
		name, mime, limit = "screen.png", "image/png", 32<<20
	}
	file, err := os.Open(filepath.Join(root, "runs", parts[0], name))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("run resource exceeds size limit")
	}
	content := &mcp.ResourceContents{URI: uri, MIMEType: mime}
	if mime == "application/json" {
		content.Text = string(data)
	} else {
		content.Blob = data
	}
	return &mcp.ReadResourceResult{Contents: []*mcp.ResourceContents{content}}, nil
}

const agentGuideExtra = `## Tool routing (short)

| Task | Tools |
|------|--------|
| Interrupts | ui_check_interrupt, ui_dismiss_interrupt |
| UI inspect | ui_summary, ui_describe, ui_find |
| UI input | ui_act, ui_wait; gesture, tap, swipe, type_text for lower-level control |
| Host agent coordination | device_claim, device_release, device_lease_status |
| Repeatable workflows | workflow_run, workflow_save, workflow_list |
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
