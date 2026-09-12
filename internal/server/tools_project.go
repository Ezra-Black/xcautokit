package server

import (
	"context"
	"strings"

	"github.com/xcautokit/xcautokit/internal/session"
	"github.com/xcautokit/xcautokit/internal/sim"
	"github.com/xcautokit/xcautokit/internal/xcodebuild"
	"github.com/modelcontextprotocol/go-sdk/mcp"
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
	mcp.AddTool(srv, toolMeta("discover_projects", "Discover Projects", "Find .xcodeproj and .xcworkspace files recursively", annRO()), func(ctx context.Context, req *mcp.CallToolRequest, in discoverIn) (*mcp.CallToolResult, map[string]any, error) {
		found, err := xcodebuild.DiscoverProjects(in.Root)
		if err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"count": len(found), "projects": found}, nil
	})

	mcp.AddTool(srv, toolMeta("list_schemes", "List Schemes", "List available build schemes for project", annRO()), func(ctx context.Context, req *mcp.CallToolRequest, in projectIn) (*mcp.CallToolResult, map[string]any, error) {
		project := in.Project
		if project == "" {
			project = a.Session.Get().ProjectPath
		}
		if project == "" {
			return nil, nil, fmtError("project path required")
		}
		out, err := xcodebuild.ListSchemes(project)
		if err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"output": out, "project": project, "backend": "xcodebuild"}, nil
	})

	mcp.AddTool(srv, toolMeta("show_build_settings", "Show Build Settings", "Show build settings for project/scheme", annRO()), func(ctx context.Context, req *mcp.CallToolRequest, in projectIn) (*mcp.CallToolResult, map[string]any, error) {
		project, scheme, cfg, _, d, err := a.projectOpts(in.Project, in.Scheme, in.Configuration, in.Destination)
		if err != nil {
			return nil, nil, err
		}
		out, err := xcodebuild.ShowBuildSettings(xcodebuild.Options{
			Project: project, Scheme: scheme, Configuration: cfg, DerivedDataPath: firstNonEmpty(in.DerivedDataPath, d.DerivedDataPath),
		})
		if err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"output": out, "project": project, "scheme": scheme}, nil
	})

	mcp.AddTool(srv, toolMeta("get_app_bundle_id", "Get App Bundle ID", "Extract app bundle ID from project build settings", annRO()), func(ctx context.Context, req *mcp.CallToolRequest, in projectIn) (*mcp.CallToolResult, map[string]any, error) {
		project, scheme, cfg, _, d, err := a.projectOpts(in.Project, in.Scheme, in.Configuration, in.Destination)
		if err != nil {
			return nil, nil, err
		}
		id, err := xcodebuild.BundleID(xcodebuild.Options{
			Project: project, Scheme: scheme, Configuration: cfg, DerivedDataPath: firstNonEmpty(in.DerivedDataPath, d.DerivedDataPath),
		})
		if err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"bundleId": id, "project": project, "scheme": scheme}, nil
	})

	mcp.AddTool(srv, toolMeta("get_sim_app_path", "Get Simulator App Path", "Get installed app path on simulator", annRO()), func(ctx context.Context, req *mcp.CallToolRequest, in getSimAppPathIn) (*mcp.CallToolResult, map[string]any, error) {
		if in.BundleId == "" {
			return nil, nil, fmtError("bundleId parameter required")
		}
		udid, err := a.resolveUDID(in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		out, err := sim.RunSimctl("get_app_container", udid, in.BundleId)
		if err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"path": strings.TrimSpace(string(out)), "bundleId": in.BundleId, "device": udid}, nil
	})

	mcp.AddTool(srv, toolMeta("session_set_defaults", "Set Session Defaults", "Set session defaults for project, scheme, simulator, etc.", annWrite()), func(ctx context.Context, req *mcp.CallToolRequest, in sessionSetIn) (*mcp.CallToolResult, map[string]any, error) {
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

	mcp.AddTool(srv, toolMeta("session_show_defaults", "Show Session Defaults", "Show current session defaults", annRO()), func(ctx context.Context, req *mcp.CallToolRequest, in emptyIn) (*mcp.CallToolResult, map[string]any, error) {
		return nil, map[string]any{"defaults": a.Session.Get()}, nil
	})

	mcp.AddTool(srv, toolMeta("session_clear_defaults", "Clear Session Defaults", "Clear session defaults (all or specific keys)", annDestructive()), func(ctx context.Context, req *mcp.CallToolRequest, in sessionClearIn) (*mcp.CallToolResult, map[string]any, error) {
		cur, n, err := a.Session.Clear(in.Keys)
		if err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"success": true, "cleared": n, "defaults": cur}, nil
	})

	mcp.AddTool(srv, toolMeta("build_sim", "Build Simulator", "Build project for iOS Simulator (uses Xcode mcpbridge when available, else xcodebuild)", annWrite()), a.handleBuildSim)

	mcp.AddTool(srv, toolMeta("build_run_sim", "Build and Run Simulator", "Build and run project on iOS Simulator", annWrite()), a.handleBuildRunSim)

	mcp.AddTool(srv, toolMeta("test_sim", "Test Simulator", "Run tests for project on iOS Simulator (uses Xcode mcpbridge when available, else xcodebuild)", annWrite()), a.handleTestSim)

	mcp.AddTool(srv, toolMeta("clean", "Clean", "Clean build artifacts for project", annDestructive()), func(ctx context.Context, req *mcp.CallToolRequest, in projectIn) (*mcp.CallToolResult, map[string]any, error) {
		project, scheme, cfg, _, d, err := a.projectOpts(in.Project, in.Scheme, in.Configuration, in.Destination)
		if err != nil {
			return nil, nil, err
		}
		out, err := xcodebuild.Clean(xcodebuild.Options{
			Project: project, Scheme: scheme, Configuration: cfg, DerivedDataPath: firstNonEmpty(in.DerivedDataPath, d.DerivedDataPath),
		})
		if err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"success": true, "output": trimOut(out), "backend": "xcodebuild"}, nil
	})

	mcp.AddTool(srv, toolMeta("launch_app_logs_sim", "Launch App with Logs", "Launch app on simulator with streaming logs", annWrite()), func(ctx context.Context, req *mcp.CallToolRequest, in launchLogsIn) (*mcp.CallToolResult, map[string]any, error) {
		bundleID := in.BundleId
		if bundleID == "" {
			project, scheme, cfg, _, d, err := a.projectOpts(in.Project, in.Scheme, "", "")
			if err != nil {
				return nil, nil, err
			}
			bundleID, err = xcodebuild.BundleID(xcodebuild.Options{Project: project, Scheme: scheme, Configuration: cfg, DerivedDataPath: d.DerivedDataPath})
			if err != nil {
				return nil, nil, err
			}
		}
		udid, err := a.resolveUDID(in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		if _, err := sim.RunSimctl("launch", udid, bundleID); err != nil {
			return nil, nil, err
		}
		out := map[string]any{
			"success": true, "bundleId": bundleID, "device": udid,
			"message": "App launched. Use start_sim_log_cap to stream logs.",
		}
		a.attachInterrupts(udid, out)
		return nil, out, nil
	})
}

func (a *App) handleBuildSim(ctx context.Context, req *mcp.CallToolRequest, in projectIn) (*mcp.CallToolResult, map[string]any, error) {
	project, scheme, cfg, dest, d, err := a.projectOpts(in.Project, in.Scheme, in.Configuration, in.Destination)
	if err != nil {
		return nil, nil, err
	}
	if a.Bridge.Enabled() {
		text, structured, berr := a.Bridge.BuildProject(ctx, d.TabIdentifier, scheme, cfg)
		if berr == nil {
			out := map[string]any{"success": true, "backend": "mcpbridge", "output": trimOut(text)}
			if structured != nil {
				out["result"] = structured
			}
			if project != "" {
				out["project"] = project
			}
			return nil, out, nil
		}
		// fall through to xcodebuild
		_ = berr
	}
	out, err := xcodebuild.Build(xcodebuild.Options{
		Project: project, Scheme: scheme, Configuration: cfg, Destination: dest,
		DerivedDataPath: firstNonEmpty(in.DerivedDataPath, d.DerivedDataPath),
		SimulatorName:   d.SimulatorName, SimulatorUdid: d.SimulatorUdid,
	})
	if err != nil {
		return nil, nil, err
	}
	return nil, map[string]any{"success": true, "backend": "xcodebuild", "output": trimOut(out), "project": project, "scheme": scheme}, nil
}

func (a *App) handleTestSim(ctx context.Context, req *mcp.CallToolRequest, in projectIn) (*mcp.CallToolResult, map[string]any, error) {
	project, scheme, cfg, dest, d, err := a.projectOpts(in.Project, in.Scheme, in.Configuration, in.Destination)
	if err != nil {
		return nil, nil, err
	}
	if a.Bridge.Enabled() {
		text, structured, berr := a.Bridge.RunAllTests(ctx, d.TabIdentifier, scheme)
		if berr == nil {
			out := map[string]any{"success": true, "backend": "mcpbridge", "output": trimOut(text)}
			if structured != nil {
				out["result"] = structured
			}
			return nil, out, nil
		}
	}
	out, err := xcodebuild.Test(xcodebuild.Options{
		Project: project, Scheme: scheme, Configuration: cfg, Destination: dest,
		DerivedDataPath: firstNonEmpty(in.DerivedDataPath, d.DerivedDataPath),
		SimulatorName:   d.SimulatorName, SimulatorUdid: d.SimulatorUdid,
	})
	if err != nil {
		return nil, nil, err
	}
	return nil, map[string]any{"success": true, "backend": "xcodebuild", "output": trimOut(out)}, nil
}

func (a *App) handleBuildRunSim(ctx context.Context, req *mcp.CallToolRequest, in projectIn) (*mcp.CallToolResult, map[string]any, error) {
	_, out, err := a.handleBuildSim(ctx, req, in)
	if err != nil {
		return nil, nil, err
	}
	project, scheme, cfg, _, d, err := a.projectOpts(in.Project, in.Scheme, in.Configuration, in.Destination)
	if err != nil {
		return nil, out, nil
	}
	bundleID, err := xcodebuild.BundleID(xcodebuild.Options{
		Project: project, Scheme: scheme, Configuration: cfg, DerivedDataPath: firstNonEmpty(in.DerivedDataPath, d.DerivedDataPath),
	})
	if err != nil {
		out["warning"] = "built but could not resolve bundle ID: " + err.Error()
		return nil, out, nil
	}
	udid := d.SimulatorUdid
	if udid == "" {
		udid = "booted"
	}
	// Try install from common DerivedData locations is complex; launch if already installed.
	if _, lerr := sim.RunSimctl("launch", udid, bundleID); lerr != nil {
		out["bundleId"] = bundleID
		out["warning"] = "build ok; launch failed (install .app first): " + lerr.Error()
		return nil, out, nil
	}
	out["bundleId"] = bundleID
	out["launched"] = true
	resolved, _ := a.resolveUDID(udid)
	if resolved == "" {
		resolved = udid
	}
	a.attachInterrupts(resolved, out)
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
