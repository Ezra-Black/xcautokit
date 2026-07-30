package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func (a *App) registerPrompts(srv *mcp.Server) {
	srv.AddPrompt(&mcp.Prompt{
		Name:        "build-and-verify",
		Description: "Build the iOS project, inspect issues, then verify UI on simulator",
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{
			Description: "Autokit build → diagnose → UI verify loop",
			Messages: []*mcp.PromptMessage{
				{Role: "user", Content: &mcp.TextContent{Text: `Use Autokit only (no competitor Xcode MCPs).

1. Read resource xcode://status. If mcpbridge is available, call xcode_windows and store tabIdentifier via session_set_defaults.
2. session_show_defaults — if project/scheme missing, discover_projects then session_set_defaults.
3. build_sim — Autokit routes to mcpbridge when available, else xcodebuild.
4. On failure: xcode_issues or xcode_build_log (bridge) / inspect build_sim output (fallback).
5. Fix code in the editor, rebuild.
6. build_run_sim or app_launch, then ui_check_interrupt. If blocked, ui_dismiss_interrupt with an explicit action (accept/decline/dismiss) — never silently auto-accept permissions. Then ui_summary / screenshot to verify.`}},
			},
		}, nil
	})

	srv.AddPrompt(&mcp.Prompt{
		Name:        "ui-explore",
		Description: "Explore and interact with the booted simulator UI",
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{
			Description: "Simulator UI exploration with Autokit",
			Messages: []*mcp.PromptMessage{
				{Role: "user", Content: &mcp.TextContent{Text: `Use Autokit / XCAutokit simulator tools:

1. status — confirm a booted simulator (device_boot / open_sim if needed).
2. After app_launch or navigation: ui_check_interrupt. If hasInterrupt, call ui_dismiss_interrupt with an explicit action (accept|decline|dismiss|button). Never invent Allow taps; never auto-accept ATT/location/camera without choosing intentionally. SpringBoard → button home + app_launch.
3. ui_summary for a compact tree (includes interrupts preview), or ui_describe for full detail.
4. ui_find with by+query (accessibilityId/label/text/role) to locate controls.
5. gesture or tap/swipe/type_text to interact — prefer element selectors over raw coordinates when possible.
6. screenshot after meaningful steps; re-check interrupts if a tap seems to do nothing.`}},
			},
		}, nil
	})

	srv.AddPrompt(&mcp.Prompt{
		Name:        "fix-failing-test",
		Description: "Run tests and iterate on failures using Autokit + Xcode backend",
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{
			Description: "Test-fix loop",
			Messages: []*mcp.PromptMessage{
				{Role: "user", Content: &mcp.TextContent{Text: `Use Autokit:

1. Ensure session defaults (project/scheme) and xcode_windows if bridge is up.
2. test_sim for the full suite, or run_some_tests for specific identifiers when mcpbridge is available.
3. Read failures from the tool output / xcode_issues.
4. Edit code, re-run the failing tests until green.`}},
			},
		}, nil
	})

	srv.AddPrompt(&mcp.Prompt{
		Name:        "handle-interrupt",
		Description: "Resolve simulator alerts, sheets, and permission prompts before continuing UI work",
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{
			Description: "Interrupt handling (customer workflow)",
			Messages: []*mcp.PromptMessage{
				{Role: "user", Content: &mcp.TextContent{Text: AgentInstructions}},
			},
		}, nil
	})
}
