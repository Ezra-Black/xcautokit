package server

import (
	"context"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/xcautokit/xcautokit/internal/session"
	"github.com/xcautokit/xcautokit/internal/sim"
	"github.com/xcautokit/xcautokit/internal/xcodebuild"
)

type discoverIn struct {
	Root string `json:"root,omitempty" jsonschema:"Root directory to search for Xcode projects (default: current directory)"`
}

type projectIn struct {
	Project         string `json:"project,omitempty" jsonschema:"Path to .xcodeproj or .xcworkspace"`
	Scheme          string `json:"scheme,omitempty" jsonschema:"Build scheme name"`
	Configuration   string `json:"configuration,omitempty" jsonschema:"Debug or Release"`
	Destination     string `json:"destination,omitempty" jsonschema:"xcodebuild destination"`
	DerivedDataPath string `json:"derivedDataPath,omitempty"`
}

type sessionSetIn struct {
	ProjectPath     string `json:"projectPath,omitempty"`
	Scheme          string `json:"scheme,omitempty"`
	Configuration   string `json:"configuration,omitempty"`
	SimulatorUdid   string `json:"simulatorUdid,omitempty"`
	SimulatorName   string `json:"simulatorName,omitempty"`
	DerivedDataPath string `json:"derivedDataPath,omitempty"`
	UseLatestOS     *bool  `json:"useLatestOS,omitempty"`
	TabIdentifier   string `json:"tabIdentifier,omitempty"`
}

type sessionClearIn struct {
	Keys []string `json:"keys,omitempty" jsonschema:"Specific keys to clear; omit to clear all"`
}

type getSimAppPathIn struct {
	BundleId      string `json:"bundleId" jsonschema:"App bundle identifier"`
	SimulatorUuid string `json:"simulatorUuid,omitempty"`
}

type launchLogsIn struct {
	Project       string `json:"project,omitempty"`
	Scheme        string `json:"scheme,omitempty"`
	BundleId      string `json:"bundleId,omitempty"`
	SimulatorUuid string `json:"simulatorUuid,omitempty"`
}

func (a *App) registerProjectTools(srv *mcp.Server) {
	addTool(a, srv, toolMeta("discover_projects", "Discover Projects", "Find .xcodeproj and .xcworkspace files recursively", annRO()), func(ctx context.Context, req *mcp.CallToolRequest, in discoverIn) (*mcp.CallToolResult, map[string]any, error) {
		found, err := xcodebuild.DiscoverProjectsContext(ctx, in.Root)
		if err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"count": len(found), "projects": found}, nil
	})

	addTool(a, srv, toolMeta("list_schemes", "List Schemes", "List available build schemes for project", annRO()), func(ctx context.Context, req *mcp.CallToolRequest, in projectIn) (*mcp.CallToolResult, map[string]any, error) {
		project := in.Project
		if project == "" {
			project = a.Session.Get().ProjectPath
		}
		if project == "" {
			return nil, nil, fmtError("project path required")
		}
		out, err := xcodebuild.ListSchemesContext(ctx, project)
		if err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"output": out, "project": project, "backend": "xcodebuild"}, nil
	})

	addTool(a, srv, toolMeta("show_build_settings", "Show Build Settings", "Show build settings for project/scheme", annRO()), func(ctx context.Context, req *mcp.CallToolRequest, in projectIn) (*mcp.CallToolResult, map[string]any, error) {
		project, scheme, cfg, dest, d, err := a.projectOpts(in.Project, in.Scheme, in.Configuration, in.Destination)
		if err != nil {
			return nil, nil, err
		}
		out, err := xcodebuild.ShowBuildSettings(xcodebuild.Options{
			Context: ctx, Project: project, Scheme: scheme, Configuration: cfg, Destination: dest,
			DerivedDataPath: firstNonEmpty(in.DerivedDataPath, d.DerivedDataPath), SimulatorName: d.SimulatorName, SimulatorUdid: d.SimulatorUdid,
		})
		if err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"output": out, "project": project, "scheme": scheme}, nil
	})

	addTool(a, srv, toolMeta("get_app_bundle_id", "Get App Bundle ID", "Extract app bundle ID from project build settings", annRO()), func(ctx context.Context, req *mcp.CallToolRequest, in projectIn) (*mcp.CallToolResult, map[string]any, error) {
		project, scheme, cfg, dest, d, err := a.projectOpts(in.Project, in.Scheme, in.Configuration, in.Destination)
		if err != nil {
			return nil, nil, err
		}
		id, err := xcodebuild.BundleID(xcodebuild.Options{
			Context: ctx, Project: project, Scheme: scheme, Configuration: cfg, Destination: dest,
			DerivedDataPath: firstNonEmpty(in.DerivedDataPath, d.DerivedDataPath), SimulatorName: d.SimulatorName, SimulatorUdid: d.SimulatorUdid,
		})
		if err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"bundleId": id, "project": project, "scheme": scheme}, nil
	})

	addTool(a, srv, toolMeta("get_sim_app_path", "Get Simulator App Path", "Get installed app path on simulator", annRO()), func(ctx context.Context, req *mcp.CallToolRequest, in getSimAppPathIn) (*mcp.CallToolResult, map[string]any, error) {
		if in.BundleId == "" {
			return nil, nil, fmtError("bundleId parameter required")
		}
		udid, err := a.resolveUDIDContext(ctx, in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		out, err := sim.RunSimctlContext(ctx, "get_app_container", udid, in.BundleId)
		if err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"path": strings.TrimSpace(string(out)), "bundleId": in.BundleId, "device": udid}, nil
	})

	addTool(a, srv, toolMeta("session_set_defaults", "Set Session Defaults", "Set session defaults for project, scheme, simulator, etc.", annWrite()), func(ctx context.Context, req *mcp.CallToolRequest, in sessionSetIn) (*mcp.CallToolResult, map[string]any, error) {
		cur, err := a.Session.Set(session.Defaults{
			ProjectPath: in.ProjectPath, Scheme: in.Scheme, Configuration: in.Configuration,
			SimulatorUdid: in.SimulatorUdid, SimulatorName: in.SimulatorName,
			DerivedDataPath: in.DerivedDataPath, UseLatestOS: in.UseLatestOS, TabIdentifier: in.TabIdentifier,
		})
		if err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"success": true, "defaults": cur}, nil
	})

	addTool(a, srv, toolMeta("session_show_defaults", "Show Session Defaults", "Show current session defaults", annRO()), func(ctx context.Context, req *mcp.CallToolRequest, in emptyIn) (*mcp.CallToolResult, map[string]any, error) {
		return nil, map[string]any{"defaults": a.Session.Get()}, nil
	})

	addTool(a, srv, toolMeta("session_clear_defaults", "Clear Session Defaults", "Clear session defaults (all or specific keys)", annDestructive()), func(ctx context.Context, req *mcp.CallToolRequest, in sessionClearIn) (*mcp.CallToolResult, map[string]any, error) {
		cur, n, err := a.Session.Clear(in.Keys)
		if err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"success": true, "cleared": n, "defaults": cur}, nil
	})

	addTool(a, srv, toolMeta("build_sim", "Build Simulator", "Build the specified project, scheme, configuration, and simulator destination with xcodebuild", annWrite()), a.handleBuildSim)

	addTool(a, srv, toolMeta("build_run_sim", "Build and Run Simulator", "Build, resolve the produced app, install it, and launch on the selected simulator; reports each completed stage", annWrite()), a.handleBuildRunSim)

	addTool(a, srv, toolMeta("test_sim", "Test Simulator", "Run project tests on the selected iOS Simulator with xcodebuild", annWrite()), a.handleTestSim)

	addTool(a, srv, toolMeta("clean", "Clean", "Clean build artifacts for project", annDestructive()), func(ctx context.Context, req *mcp.CallToolRequest, in projectIn) (*mcp.CallToolResult, map[string]any, error) {
		project, scheme, cfg, dest, d, err := a.projectOpts(in.Project, in.Scheme, in.Configuration, in.Destination)
		if err != nil {
			return nil, nil, err
		}
		out, err := xcodebuild.Clean(xcodebuild.Options{
			Context: ctx, Project: project, Scheme: scheme, Configuration: cfg, Destination: dest,
			DerivedDataPath: firstNonEmpty(in.DerivedDataPath, d.DerivedDataPath), SimulatorName: d.SimulatorName, SimulatorUdid: d.SimulatorUdid,
		})
		if err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"success": true, "output": trimOut(out), "backend": "xcodebuild"}, nil
	})

	addTool(a, srv, toolMeta("launch_app_logs_sim", "Launch App with Logs", "Launch app on simulator with streaming logs", annWrite()), func(ctx context.Context, req *mcp.CallToolRequest, in launchLogsIn) (*mcp.CallToolResult, map[string]any, error) {
		udid, err := a.resolveUDIDContext(ctx, in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		bundleID := in.BundleId
		if bundleID == "" {
			project, scheme, cfg, _, d, err := a.projectOpts(in.Project, in.Scheme, "", "")
			if err != nil {
				return nil, nil, err
			}
			bundleID, err = xcodebuild.BundleID(xcodebuild.Options{Context: ctx, Project: project, Scheme: scheme, Configuration: cfg, DerivedDataPath: d.DerivedDataPath, SimulatorUdid: udid})
			if err != nil {
				return nil, nil, err
			}
		}
		if _, err := sim.RunSimctlContext(ctx, "launch", udid, bundleID); err != nil {
			return nil, nil, err
		}
		out := map[string]any{
			"success": true, "bundleId": bundleID, "device": udid,
			"message": "App launched. Use start_sim_log_cap to stream logs.",
		}
		a.attachInterrupts(ctx, udid, out)
		return nil, out, nil
	})
}

func (a *App) buildOptions(ctx context.Context, in projectIn, pinDevice bool) (xcodebuild.Options, error) {
	project, scheme, cfg, dest, d, err := a.projectOpts(in.Project, in.Scheme, in.Configuration, in.Destination)
	if err != nil {
		return xcodebuild.Options{}, err
	}
	opts := xcodebuild.Options{
		Context: ctx, Project: project, Scheme: scheme, Configuration: cfg, Destination: dest,
		DerivedDataPath: firstNonEmpty(in.DerivedDataPath, d.DerivedDataPath),
		SimulatorName:   d.SimulatorName, SimulatorUdid: d.SimulatorUdid,
	}
	if pinDevice {
		selector, err := deviceFromDestination(dest, "")
		if err != nil {
			return opts, err
		}
		udid, err := a.resolveUDIDContext(ctx, selector)
		if err != nil {
			return opts, err
		}
		opts.SimulatorUdid = udid
		opts.Destination = "platform=iOS Simulator,id=" + udid
	}
	opts.DerivedDataPath = xcodebuild.DerivedDataPath(opts)
	return opts, nil
}

func (a *App) handleBuildSim(ctx context.Context, req *mcp.CallToolRequest, in projectIn) (*mcp.CallToolResult, map[string]any, error) {
	opts, err := a.buildOptions(ctx, in, false)
	if err != nil {
		return nil, nil, err
	}
	log, err := xcodebuild.Build(opts)
	out := map[string]any{"success": err == nil, "built": err == nil, "backend": "xcodebuild", "output": trimOut(log), "project": opts.Project, "scheme": opts.Scheme, "derivedDataPath": opts.DerivedDataPath}
	if err != nil {
		out["stage"] = "build"
		out["message"] = err.Error()
		return &mcp.CallToolResult{IsError: true}, out, nil
	}
	return nil, out, nil
}

func (a *App) handleTestSim(ctx context.Context, req *mcp.CallToolRequest, in projectIn) (*mcp.CallToolResult, map[string]any, error) {
	opts, err := a.buildOptions(ctx, in, true)
	if err != nil {
		return nil, nil, err
	}
	log, err := xcodebuild.Test(opts)
	out := map[string]any{"success": err == nil, "tested": err == nil, "backend": "xcodebuild", "output": trimOut(log), "device": opts.SimulatorUdid, "derivedDataPath": opts.DerivedDataPath}
	if err != nil {
		out["stage"] = "test"
		out["message"] = err.Error()
		return &mcp.CallToolResult{IsError: true}, out, nil
	}
	return nil, out, nil
}

func (a *App) handleBuildRunSim(ctx context.Context, req *mcp.CallToolRequest, in projectIn) (*mcp.CallToolResult, map[string]any, error) {
	opts, err := a.buildOptions(ctx, in, true)
	if err != nil {
		return nil, nil, err
	}
	result, err := xcodebuild.BuildRun(opts, "")
	out := map[string]any{
		"success": err == nil, "backend": "xcodebuild", "stage": result.Stage,
		"built": result.Built, "installed": result.Installed, "launched": result.Launched,
		"project": opts.Project, "scheme": opts.Scheme, "device": opts.SimulatorUdid,
		"derivedDataPath": opts.DerivedDataPath, "output": trimOut(result.Output),
	}
	if result.App.Path != "" {
		out["appPath"] = result.App.Path
		out["bundleId"] = result.App.BundleID
		out["target"] = result.App.Target
	}
	if err != nil {
		out["message"] = err.Error()
		return &mcp.CallToolResult{IsError: true}, out, nil
	}
	a.attachInterrupts(ctx, opts.SimulatorUdid, out)
	return nil, out, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func trimOut(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 8000 {
		return s[len(s)-8000:]
	}
	return s
}
