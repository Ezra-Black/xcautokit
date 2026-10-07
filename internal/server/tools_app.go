package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/xcautokit/xcautokit/internal/sim"
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
	addTool(a, srv, toolMeta("app_install", "Install App", "Install .app bundle", annWrite()), func(ctx context.Context, req *mcp.CallToolRequest, in appInstallIn) (*mcp.CallToolResult, map[string]any, error) {
		if in.AppPath == "" {
			return nil, nil, fmtError("appPath parameter required")
		}
		udid, err := a.resolveUDIDContext(ctx, in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		if _, err := sim.RunSimctlContext(ctx, "install", udid, in.AppPath); err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"success": true, "appPath": in.AppPath, "device": udid}, nil
	})

	addTool(a, srv, toolMeta("app_launch", "Launch App", "Launch app by bundle ID. Response includes hasInterrupt/interrupts — dismiss overlays before UI input.", annWrite()), func(ctx context.Context, req *mcp.CallToolRequest, in appBundleIn) (*mcp.CallToolResult, map[string]any, error) {
		if in.BundleId == "" {
			return nil, nil, fmtError("bundleId parameter required")
		}
		udid, err := a.resolveUDIDContext(ctx, in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		if _, err := sim.RunSimctlContext(ctx, "launch", udid, in.BundleId); err != nil {
			return nil, nil, err
		}
		out := map[string]any{"success": true, "bundleId": in.BundleId, "device": udid}
		a.attachInterrupts(ctx, udid, out)
		return nil, out, nil
	})

	addTool(a, srv, toolMeta("app_terminate", "Terminate App", "Terminate running app", annDestructive()), func(ctx context.Context, req *mcp.CallToolRequest, in appBundleIn) (*mcp.CallToolResult, map[string]any, error) {
		if in.BundleId == "" {
			return nil, nil, fmtError("bundleId parameter required")
		}
		udid, err := a.resolveUDIDContext(ctx, in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		if _, err := sim.RunSimctlContext(ctx, "terminate", udid, in.BundleId); err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"success": true, "bundleId": in.BundleId, "device": udid}, nil
	})

	addTool(a, srv, toolMeta("open_url", "Open URL", "Open URL scheme. Response includes hasInterrupt/interrupts when system dialogs appear.", annWrite()), func(ctx context.Context, req *mcp.CallToolRequest, in openURLIn) (*mcp.CallToolResult, map[string]any, error) {
		if in.URL == "" {
			return nil, nil, fmtError("url parameter required")
		}
		udid, err := a.resolveUDIDContext(ctx, in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		if _, err := sim.RunSimctlContext(ctx, "openurl", udid, in.URL); err != nil {
			return nil, nil, err
		}
		out := map[string]any{"success": true, "url": in.URL, "device": udid}
		a.attachInterrupts(ctx, udid, out)
		return nil, out, nil
	})
}
