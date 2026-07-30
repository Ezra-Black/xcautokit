package server

import (
	"context"

	"github.com/xcautokit/xcautokit/internal/sim"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type appInstallIn struct {
	AppPath       string `json:"appPath" jsonschema:"Path to .app bundle"`
	SimulatorUuid string `json:"simulatorUuid,omitempty"`
}

type appBundleIn struct {
	BundleId      string `json:"bundleId" jsonschema:"App bundle identifier"`
	SimulatorUuid string `json:"simulatorUuid,omitempty"`
}

type openURLIn struct {
	URL           string `json:"url" jsonschema:"URL scheme (http://, tel://, custom://)"`
	SimulatorUuid string `json:"simulatorUuid,omitempty"`
}

func (a *App) registerAppTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "app_install",
		Description: "Install .app bundle",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in appInstallIn) (*mcp.CallToolResult, map[string]any, error) {
		if in.AppPath == "" {
			return nil, nil, fmtError("appPath parameter required")
		}
		udid, err := a.resolveUDID(in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		if _, err := sim.RunSimctl("install", udid, in.AppPath); err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"success": true, "appPath": in.AppPath, "device": udid}, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "app_launch",
		Description: "Launch app by bundle ID. Response includes hasInterrupt/interrupts — dismiss overlays before UI input.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in appBundleIn) (*mcp.CallToolResult, map[string]any, error) {
		if in.BundleId == "" {
			return nil, nil, fmtError("bundleId parameter required")
		}
		udid, err := a.resolveUDID(in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		if _, err := sim.RunSimctl("launch", udid, in.BundleId); err != nil {
			return nil, nil, err
		}
		out := map[string]any{"success": true, "bundleId": in.BundleId, "device": udid}
		a.attachInterrupts(udid, out)
		return nil, out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "app_terminate",
		Description: "Terminate running app",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in appBundleIn) (*mcp.CallToolResult, map[string]any, error) {
		if in.BundleId == "" {
			return nil, nil, fmtError("bundleId parameter required")
		}
		udid, err := a.resolveUDID(in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		if _, err := sim.RunSimctl("terminate", udid, in.BundleId); err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"success": true, "bundleId": in.BundleId, "device": udid}, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "open_url",
		Description: "Open URL scheme. Response includes hasInterrupt/interrupts when system dialogs appear.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in openURLIn) (*mcp.CallToolResult, map[string]any, error) {
		if in.URL == "" {
			return nil, nil, fmtError("url parameter required")
		}
		udid, err := a.resolveUDID(in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		if _, err := sim.RunSimctl("openurl", udid, in.URL); err != nil {
			return nil, nil, err
		}
		out := map[string]any{"success": true, "url": in.URL, "device": udid}
		a.attachInterrupts(udid, out)
		return nil, out, nil
	})
}
