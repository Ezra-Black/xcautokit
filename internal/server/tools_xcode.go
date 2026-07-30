package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type issuesIn struct {
	TabIdentifier string `json:"tabIdentifier,omitempty"`
}

type previewIn struct {
	TabIdentifier string `json:"tabIdentifier,omitempty"`
	FilePath      string `json:"filePath" jsonschema:"SwiftUI source file path"`
	PreviewName   string `json:"previewName,omitempty"`
}

type docsIn struct {
	Query string `json:"query" jsonschema:"Search query for Apple docs / WWDC"`
}

type snippetIn struct {
	TabIdentifier string  `json:"tabIdentifier,omitempty"`
	Code          string  `json:"code" jsonschema:"Swift code to execute"`
	Timeout       float64 `json:"timeout,omitempty"`
}

type buildLogIn struct {
	TabIdentifier string `json:"tabIdentifier,omitempty"`
	Severity      string `json:"severity,omitempty" jsonschema:"error, warning, or all"`
}

type runSomeTestsIn struct {
	TabIdentifier string   `json:"tabIdentifier,omitempty"`
	Tests         []string `json:"tests" jsonschema:"Test identifiers to run"`
	Scheme        string   `json:"scheme,omitempty"`
}

func (a *App) registerXcodeTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "xcode_windows",
		Description: "List open Xcode windows/tabs via mcpbridge (requires Xcode 26.3+ with external agents enabled)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in emptyIn) (*mcp.CallToolResult, map[string]any, error) {
		if !a.Bridge.Enabled() {
			st := a.Bridge.Status(ctx)
			return nil, map[string]any{"available": false, "status": st}, fmtError(st.Message + "; " + st.Error)
		}
		text, err := a.Bridge.ListWindows(ctx)
		if err != nil {
			return nil, nil, err
		}
		st := a.Bridge.Status(ctx)
		return nil, map[string]any{
			"available":     true,
			"output":        text,
			"tabIdentifier": st.TabIdentifier,
			"workspacePath": st.WorkspacePath,
			"backend":       "mcpbridge",
		}, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "xcode_issues",
		Description: "Get Issue Navigator diagnostics from live Xcode (mcpbridge)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in issuesIn) (*mcp.CallToolResult, map[string]any, error) {
		if !a.Bridge.Enabled() {
			return nil, nil, fmtError("Xcode mcpbridge unavailable (need Xcode 26.3+)")
		}
		tab := in.TabIdentifier
		if tab == "" {
			tab = a.Session.Get().TabIdentifier
		}
		text, structured, err := a.Bridge.ListNavigatorIssues(ctx, tab)
		if err != nil {
			return nil, nil, err
		}
		out := map[string]any{"output": text, "backend": "mcpbridge"}
		if structured != nil {
			out["issues"] = structured
		}
		return nil, out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "xcode_build_log",
		Description: "Fetch Xcode build log via mcpbridge",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in buildLogIn) (*mcp.CallToolResult, map[string]any, error) {
		if !a.Bridge.Enabled() {
			return nil, nil, fmtError("Xcode mcpbridge unavailable (need Xcode 26.3+)")
		}
		tab := in.TabIdentifier
		if tab == "" {
			tab = a.Session.Get().TabIdentifier
		}
		text, structured, err := a.Bridge.GetBuildLog(ctx, tab, in.Severity)
		if err != nil {
			return nil, nil, err
		}
		out := map[string]any{"output": trimOut(text), "backend": "mcpbridge"}
		if structured != nil {
			out["result"] = structured
		}
		return nil, out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "xcode_preview",
		Description: "Render a SwiftUI preview via Xcode mcpbridge",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in previewIn) (*mcp.CallToolResult, map[string]any, error) {
		if in.FilePath == "" {
			return nil, nil, fmtError("filePath required")
		}
		if !a.Bridge.Enabled() {
			return nil, nil, fmtError("Xcode mcpbridge unavailable (need Xcode 26.3+)")
		}
		tab := in.TabIdentifier
		if tab == "" {
			tab = a.Session.Get().TabIdentifier
		}
		text, structured, err := a.Bridge.RenderPreview(ctx, tab, in.FilePath, in.PreviewName)
		if err != nil {
			return nil, nil, err
		}
		out := map[string]any{"output": text, "backend": "mcpbridge"}
		if structured != nil {
			out["result"] = structured
		}
		return nil, out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "docs_search",
		Description: "Search Apple Developer Documentation / WWDC via Xcode mcpbridge",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in docsIn) (*mcp.CallToolResult, map[string]any, error) {
		if in.Query == "" {
			return nil, nil, fmtError("query required")
		}
		if !a.Bridge.Enabled() {
			return nil, nil, fmtError("Xcode mcpbridge unavailable (need Xcode 26.3+)")
		}
		text, structured, err := a.Bridge.DocumentationSearch(ctx, in.Query)
		if err != nil {
			return nil, nil, err
		}
		out := map[string]any{"output": text, "backend": "mcpbridge"}
		if structured != nil {
			out["results"] = structured
		}
		return nil, out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "swift_snippet",
		Description: "Execute a Swift snippet in Xcode project context via mcpbridge",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in snippetIn) (*mcp.CallToolResult, map[string]any, error) {
		if in.Code == "" {
			return nil, nil, fmtError("code required")
		}
		if !a.Bridge.Enabled() {
			return nil, nil, fmtError("Xcode mcpbridge unavailable (need Xcode 26.3+)")
		}
		tab := in.TabIdentifier
		if tab == "" {
			tab = a.Session.Get().TabIdentifier
		}
		text, structured, err := a.Bridge.ExecuteSnippet(ctx, tab, in.Code, in.Timeout)
		if err != nil {
			return nil, nil, err
		}
		out := map[string]any{"output": text, "backend": "mcpbridge"}
		if structured != nil {
			out["result"] = structured
		}
		return nil, out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "run_some_tests",
		Description: "Run specific tests via Xcode mcpbridge (falls back unavailable message if bridge missing)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in runSomeTestsIn) (*mcp.CallToolResult, map[string]any, error) {
		if len(in.Tests) == 0 {
			return nil, nil, fmtError("tests required")
		}
		if !a.Bridge.Enabled() {
			return nil, nil, fmtError("Xcode mcpbridge unavailable; use test_sim for full suite via xcodebuild")
		}
		tab := in.TabIdentifier
		if tab == "" {
			tab = a.Session.Get().TabIdentifier
		}
		text, structured, err := a.Bridge.RunSomeTests(ctx, tab, in.Tests, in.Scheme)
		if err != nil {
			return nil, nil, err
		}
		out := map[string]any{"output": trimOut(text), "backend": "mcpbridge"}
		if structured != nil {
			out["result"] = structured
		}
		return nil, out, nil
	})

}
