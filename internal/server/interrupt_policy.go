package server

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/xcautokit/xcautokit/internal/sim"
)

// AgentInstructions is returned to every MCP client on initialize.
// This ships the simulator-ui-interrupts workflow to customers (not Cursor-local rules).
const AgentInstructions = `XCAutokit — iOS Simulator + Xcode automation.

## UI interrupts (required)

After app_launch, build_run_sim, open_url, or any navigation that may show a system dialog:

1. Read hasInterrupt / interrupts from the tool result (auto-attached after launch tools), or call ui_check_interrupt / ui_summary.
2. If blocked, call ui_dismiss_interrupt with an explicit action: accept | decline | dismiss | button (+ buttonLabel).
3. Re-check before continuing.

Do NOT invent raw taps on "Allow". Do NOT silently auto-accept ATT, location, camera, microphone, or notification permissions — choose intentionally.

If kind is springboard: button home → app_launch → ui_check_interrupt again. Do not use ui_dismiss_interrupt for SpringBoard.

Input tools (tap/swipe/type/gesture/…) refuse to run while a blocking interrupt (alert/sheet/permission/banner) is present; dismiss first.

## Tickets (process-local)

record_start / start_sim_log_cap return a ticket string. Pass it to record_stop / stop_sim_log_cap.
Tickets live only in this MCP process — if the server restarts, start a new capture.

## Session defaults

Defaults are isolated to this MCP process. XCAUTOKIT_SESSION_FILE opts into a dedicated persistent file. Prefer explicit simulatorUuid for host subagents.

## Host-owned agents

The host owns all planning, models, and subagents. XCAutokit never starts agents or calls a model.
Use device_claim for one driver per simulator; pass leaseToken to mutations and release when done. Other agents can analyze captured evidence or drive separate UUIDs. Locks coordinate UUID-targeted XCAutokit tools; external simulator tools and live IDE run_some_tests/swift_snippet follow their own destinations and are outside that lock. Do not run their mutations alongside a simulator driver.
Prefer ui_act with a selector and waitFor over find/tap/sleep/check. Use ui_wait for observed conditions.
workflow_run executes only host-supplied steps and stops on failure; workflow_save saves a successful run for replay. Inspect evidence and expected conditions before claiming completion.

## Optional tool filtering

Set XCAUTOKIT_WORKFLOWS=core to omit live Xcode tools, or a comma list such as device,ui,input,build.
`

func (a *App) checkInterrupts(ctx context.Context, udid string) (interrupts []sim.Interrupt, err error) {
	raw, err := sim.DescribeUIContext(ctx, udid)
	if err != nil {
		return nil, err
	}
	els, err := sim.ParseDescribeUI(raw)
	if err != nil {
		return nil, err
	}
	interrupts = sim.DetectInterrupts(els)
	return interrupts, nil
}

// attachInterrupts adds interrupt snapshot fields to a tool result map.
func (a *App) attachInterrupts(ctx context.Context, udid string, out map[string]any) {
	if out == nil {
		return
	}
	// Brief settle so system dialogs can appear after launch.
	timer := time.NewTimer(150 * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		out["interruptCheck"] = "unavailable"
		out["interruptError"] = ctx.Err().Error()
		out["hasInterrupt"] = nil
		return
	case <-timer.C:
	}
	interrupts, err := a.checkInterrupts(ctx, udid)
	if err != nil {
		out["interruptCheck"] = "unavailable"
		out["interruptError"] = err.Error()
		out["hasInterrupt"] = nil
		out["interruptHint"] = "UI inspection failed. Use ui_check_interrupt before UI input; do not assume the screen is clear."
		return
	}
	out["interruptCheck"] = "complete"
	out["hasInterrupt"] = len(interrupts) > 0
	out["interrupts"] = interrupts
	if len(interrupts) > 0 {
		out["interruptHint"] = "Blocking overlay detected. Call ui_dismiss_interrupt with explicit action before UI input. Never auto-accept permissions."
	}
}

// blockingInterrupts returns alerts/sheets/permissions/banners (not springboard-only).
func blockingInterrupts(interrupts []sim.Interrupt) []sim.Interrupt {
	var out []sim.Interrupt
	for _, i := range interrupts {
		switch i.Kind {
		case sim.KindAlert, sim.KindSheet, sim.KindPermission, sim.KindBanner, sim.KindUnknownOverlay:
			out = append(out, i)
		}
	}
	return out
}

func interruptStrictOff() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("XCAUTOKIT_INTERRUPT_GUARD")))
	return v == "off" || v == "0" || v == "false"
}

// guardInputBlocks returns a result map if input should be refused due to overlays.
func (a *App) guardInputBlocks(udid string) map[string]any {
	return a.guardInputBlocksContext(context.Background(), udid)
}

func (a *App) guardInputBlocksContext(ctx context.Context, udid string) map[string]any {
	if interruptStrictOff() {
		return nil
	}
	raw, err := sim.DescribeUIContext(ctx, udid)
	var els []sim.Element
	if err == nil {
		els, err = sim.ParseDescribeUI(raw)
	}
	return inputBlockFromSnapshot(udid, els, err)
}

func inputBlockFromSnapshot(udid string, els []sim.Element, err error) map[string]any {
	if interruptStrictOff() {
		return nil
	}
	if err != nil {
		return map[string]any{"success": false, "performed": false, "blocked": true, "reason": "ui_unavailable", "device": udid, "message": err.Error(), "nextTool": "ui_describe"}
	}
	interrupts := sim.DetectInterrupts(els)
	blocking := blockingInterrupts(interrupts)
	if len(blocking) == 0 {
		// SpringBoard-only: warn but allow (tapping icons is valid).
		return nil
	}
	return map[string]any{
		"success":      false,
		"performed":    false,
		"blocked":      true,
		"reason":       "blocking_interrupt",
		"hasInterrupt": true,
		"interrupts":   blocking,
		"device":       udid,
		"message":      "UI input blocked by alert/sheet/permission. Call ui_dismiss_interrupt with action=accept|decline|dismiss|button, then retry. Set XCAUTOKIT_INTERRUPT_GUARD=off to disable.",
	}
}
