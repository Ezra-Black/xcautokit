package server

import (
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

session_set_defaults stores project/scheme/udid under ~/.xcautokit for convenience across tool calls. Prefer explicit args when unsure.

## Optional tool filtering

Set XCAUTOKIT_WORKFLOWS=core to omit live Xcode tools, or a comma list such as device,ui,input,build.
`

func (a *App) checkInterrupts(udid string) (has bool, interrupts []sim.Interrupt) {
	raw, err := sim.DescribeUI(udid)
	if err != nil {
		return false, nil
	}
	els, err := sim.ParseDescribeUI(raw)
	if err != nil {
		return false, nil
	}
	interrupts = sim.DetectInterrupts(els)
	return len(interrupts) > 0, interrupts
}

// attachInterrupts adds interrupt snapshot fields to a tool result map.
func (a *App) attachInterrupts(udid string, out map[string]any) {
	if out == nil {
		return
	}
	// Brief settle so system dialogs can appear after launch.
	time.Sleep(350 * time.Millisecond)
	has, interrupts := a.checkInterrupts(udid)
	out["hasInterrupt"] = has
	out["interrupts"] = interrupts
	if has {
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
	if interruptStrictOff() {
		return nil
	}
	has, interrupts := a.checkInterrupts(udid)
	if !has {
		return nil
	}
	blocking := blockingInterrupts(interrupts)
	if len(blocking) == 0 {
		// SpringBoard-only: warn but allow (tapping icons is valid).
		return nil
	}
	return map[string]any{
		"success":      false,
		"blocked":      true,
		"reason":       "blocking_interrupt",
		"hasInterrupt": true,
		"interrupts":   blocking,
		"device":       udid,
		"message":      "UI input blocked by alert/sheet/permission. Call ui_dismiss_interrupt with action=accept|decline|dismiss|button, then retry. Set XCAUTOKIT_INTERRUPT_GUARD=off to disable.",
	}
}
