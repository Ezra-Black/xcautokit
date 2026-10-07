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
			Description: "XCAutokit build → diagnose → UI verify loop",
			Messages: []*mcp.PromptMessage{
				{Role: "user", Content: &mcp.TextContent{Text: `Use XCAutokit only (no competitor Xcode MCPs).

1. Inspect session defaults; discover_projects and list_schemes when project/scheme are missing. Defaults are isolated per MCP process.
2. Select an explicit simulator from device_list. For host-agent coordination, device_claim it and pass leaseToken to mutations; other agents can review source/evidence or use separate devices.
3. build_run_sim builds the requested project/scheme/configuration using xcodebuild, installs its matching artifact, and launches it on the selected simulator. Inspect built/installed/launched and stage on failure. Use build_sim when only compilation is requested.
4. Diagnose the returned output and fix code through the host. Live xcode_issues/xcode_build_log are optional separate IDE evidence; verify that their project matches.
5. Read launch interrupts. If blocked, ui_dismiss_interrupt with an explicit authorized action; never silently auto-accept permissions.
6. Verify actual behavior using ui_act with a selector and waitFor, ui_wait for known conditions, and screenshot for visual framing. A successful build is not runtime verification. Report evidence and release the device claim.`}},
			},
		}, nil
	})

	srv.AddPrompt(&mcp.Prompt{
		Name:        "ui-explore",
		Description: "Explore and interact with the booted simulator UI",
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{
			Description: "Simulator UI exploration with XCAutokit",
			Messages: []*mcp.PromptMessage{
				{Role: "user", Content: &mcp.TextContent{Text: `Use XCAutokit simulator tools:

1. status/device_list — choose an explicit simulator UUID (device_boot / open_sim if needed). Use device_claim for a host-owned driver across calls and pass leaseToken to mutations.
2. After app_launch or navigation: ui_check_interrupt. If hasInterrupt, call ui_dismiss_interrupt with an explicit action (accept|decline|dismiss|button). Never invent Allow taps; never auto-accept ATT/location/camera without choosing intentionally. SpringBoard → button home + app_launch.
3. ui_summary for a compact tree (includes interrupts preview), or ui_describe for full detail.
4. ui_find with by+query (accessibilityId/label/text/role) to locate controls.
5. Prefer ui_act with a unique selector and an explicit waitFor condition. Use ui_wait instead of fixed sleeps. Use gesture/swipe/type_text for other input. If performed is true but verification fails, inspect current state before retrying.
6. Use workflow_run for a bounded sequence of already-known actions and assertions; stop on failure. The host owns all interpretation and recovery decisions.
7. screenshot after meaningful steps for visual review. Share timestamped evidence with host-owned reviewers, and release the device when done.`}},
			},
		}, nil
	})

	srv.AddPrompt(&mcp.Prompt{
		Name:        "fix-failing-test",
		Description: "Run tests and iterate on failures using XCAutokit + Xcode backend",
	}, func(ctx context.Context, req *mcp.GetPromptRequest) (*mcp.GetPromptResult, error) {
		return &mcp.GetPromptResult{
			Description: "Test-fix loop",
			Messages: []*mcp.PromptMessage{
				{Role: "user", Content: &mcp.TextContent{Text: `Use XCAutokit:

1. Confirm the explicit project, scheme, and simulator UUID. test_sim uses xcodebuild on that target and coordinates with its device lease.
2. Run test_sim and inspect the actual failure output. With the optional live IDE bridge, run_some_tests follows the IDE's own project and destination; verify both and do not run it alongside another simulator driver.
3. Edit code through the host and rerun the relevant tests. Do not treat missing output, a timeout, or an unavailable backend as success.
4. For UI behavior, verify the fixed flow in the simulator and inspect screenshots. The host may delegate independent source/log/evidence analysis; XCAutokit never starts agents.`}},
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
