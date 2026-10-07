package server

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/xcautokit/xcautokit/internal/sim"
)

type uiWaitCondition struct {
	Selector sim.Selector `json:"selector"`
	State    string       `json:"state,omitempty" jsonschema:"present (default), absent, or enabled (enabled with a usable on-screen frame)"`
}

type uiWaitIn struct {
	Selector       sim.Selector `json:"selector"`
	State          string       `json:"state,omitempty" jsonschema:"present (default), absent, or enabled (enabled with a usable on-screen frame)"`
	TimeoutMs      int          `json:"timeoutMs,omitempty" jsonschema:"Total deadline in milliseconds, 1-60000; default 5000"`
	PollIntervalMs int          `json:"pollIntervalMs,omitempty" jsonschema:"Time between fresh observations, 50-2000 milliseconds; default 250"`
	SimulatorUuid  string       `json:"simulatorUuid,omitempty"`
}

type uiActIn struct {
	Selector         sim.Selector     `json:"selector"`
	Action           string           `json:"action,omitempty" jsonschema:"tap (default); resolves a fresh, unique, enabled on-screen target"`
	TimeoutMs        int              `json:"timeoutMs,omitempty" jsonschema:"Total deadline for locating, tapping, optional wait, and observation; 1-60000 milliseconds, default 10000"`
	PollIntervalMs   int              `json:"pollIntervalMs,omitempty" jsonschema:"Time between fresh observations, 50-2000 milliseconds; default 250"`
	WaitFor          *uiWaitCondition `json:"waitFor,omitempty" jsonschema:"Optional condition to verify after the tap; no tap is repeated while waiting"`
	ObservationLimit int              `json:"observationLimit,omitempty" jsonschema:"Maximum elements in the fresh post-action observation, 1-100; default 20"`
	SimulatorUuid    string           `json:"simulatorUuid,omitempty"`
}

// The narrow IO boundary makes polling and action safety testable without a
// simulator. Production always reads a new hierarchy on each attempt.
type uiWorkflowIO struct {
	describe func(context.Context, string) ([]sim.Element, error)
	tap      func(context.Context, string, sim.SelectorMatch) error
	block    func(string, []sim.Element, error) map[string]any
}

func (a *App) uiWorkflowIO() uiWorkflowIO {
	return uiWorkflowIO{
		describe: func(ctx context.Context, udid string) ([]sim.Element, error) {
			raw, err := sim.DescribeUIContext(ctx, udid)
			if err != nil {
				return nil, err
			}
			return sim.ParseDescribeUI(raw)
		},
		tap: func(ctx context.Context, udid string, target sim.SelectorMatch) error {
			_, err := sim.RunAxeContext(ctx, "tap", "-x", strconv.FormatFloat(target.CenterX, 'f', -1, 64), "-y", strconv.FormatFloat(target.CenterY, 'f', -1, 64), "--udid", udid)
			return err
		},
		block: inputBlockFromSnapshot,
	}
}

func workflowTiming(timeoutMs, pollMs, defaultTimeout int) (time.Duration, time.Duration, error) {
	if timeoutMs == 0 {
		timeoutMs = defaultTimeout
	}
	if pollMs == 0 {
		pollMs = 250
	}
	if timeoutMs < 1 || timeoutMs > 60000 {
		return 0, 0, fmt.Errorf("timeoutMs must be between 1 and 60000")
	}
	if pollMs < 50 || pollMs > 2000 {
		return 0, 0, fmt.Errorf("pollIntervalMs must be between 50 and 2000")
	}
	return time.Duration(timeoutMs) * time.Millisecond, time.Duration(pollMs) * time.Millisecond, nil
}

func (condition uiWaitCondition) validate() error {
	if err := condition.Selector.Validate(); err != nil {
		return err
	}
	switch strings.ToLower(strings.TrimSpace(condition.State)) {
	case "", "present", "absent", "enabled":
		return nil
	default:
		return fmt.Errorf("state must be present, absent, or enabled")
	}
}

func (in uiActIn) validate() error {
	if err := in.Selector.Validate(); err != nil {
		return err
	}
	if in.Action != "" && in.Action != "tap" {
		return fmt.Errorf("action must be tap")
	}
	if in.ObservationLimit < 0 || in.ObservationLimit > 100 {
		return fmt.Errorf("observationLimit must be between 1 and 100, or omitted")
	}
	if in.WaitFor != nil {
		if err := in.WaitFor.validate(); err != nil {
			return fmt.Errorf("waitFor: %w", err)
		}
	}
	_, _, err := workflowTiming(in.TimeoutMs, in.PollIntervalMs, 10000)
	return err
}

type uiPollResult struct {
	elements []sim.Element
	matches  []sim.SelectorMatch
	attempts int
	reason   string
	err      error
	blocked  map[string]any
}

func contextReason(err error) string {
	if errors.Is(err, context.Canceled) {
		return "cancelled"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timeout"
	}
	return "observation_failed"
}

func waitForUI(ctx context.Context, io uiWorkflowIO, udid string, condition uiWaitCondition, interval time.Duration, forAction bool) uiPollResult {
	result := uiPollResult{}
	for {
		if err := ctx.Err(); err != nil {
			result.reason, result.err = contextReason(err), err
			return result
		}
		result.attempts++
		// Never describe an older snapshot as the outcome of a failed new read.
		result.elements, result.matches = nil, nil
		elements, err := io.describe(ctx, udid)
		if err != nil {
			result.reason, result.err = contextReason(err), err
			return result
		}
		result.elements = elements
		if err := ctx.Err(); err != nil {
			result.reason, result.err = contextReason(err), err
			return result
		}
		if forAction {
			if blocked := io.block(udid, elements, nil); blocked != nil {
				result.reason, result.blocked = "blocking_interrupt", blocked
				return result
			}
		}
		result.matches, err = sim.Select(elements, condition.Selector)
		if err != nil {
			result.reason, result.err = "invalid_selector", err
			return result
		}
		if forAction {
			if len(result.matches) > 1 {
				result.reason = "ambiguous_selector"
				return result
			}
			if len(result.matches) == 1 && result.matches[0].Actionable {
				return result
			}
		} else {
			switch strings.ToLower(strings.TrimSpace(condition.State)) {
			case "", "present":
				if len(result.matches) > 0 {
					return result
				}
			case "absent":
				if len(result.matches) == 0 {
					return result
				}
			case "enabled":
				for _, match := range result.matches {
					if match.Actionable {
						return result
					}
				}
			}
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			result.reason, result.err = contextReason(ctx.Err()), ctx.Err()
			return result
		case <-timer.C:
		}
	}
}

func attachUIObservation(out map[string]any, elements []sim.Element, limit int) {
	out["observation"] = sim.ObserveUI(elements, limit)
	interrupts := sim.DetectInterrupts(elements)
	out["hasInterrupt"] = len(interrupts) > 0
	out["interrupts"] = sim.InterruptPreview(interrupts)
}

func attachUIMatches(out map[string]any, matches []sim.SelectorMatch) {
	out["count"] = len(matches)
	if matches == nil {
		matches = []sim.SelectorMatch{}
	}
	truncated := len(matches) > 20
	if truncated {
		matches = matches[:20]
	}
	compact := make([]sim.SelectorMatch, len(matches))
	copy(compact, matches)
	for i := range compact {
		for _, value := range []*string{&compact[i].Label, &compact[i].Value, &compact[i].Identifier, &compact[i].Hint, &compact[i].Type, &compact[i].Role, &compact[i].Frame} {
			runes := []rune(*value)
			if len(runes) > 240 {
				*value = string(runes[:240]) + "…"
				truncated = true
			}
		}
	}
	out["matches"], out["matchesTruncated"] = compact, truncated
}

func runUIWait(ctx context.Context, io uiWorkflowIO, udid string, in uiWaitIn) (map[string]any, error) {
	condition := uiWaitCondition{Selector: in.Selector, State: in.State}
	if err := condition.validate(); err != nil {
		return nil, err
	}
	timeout, interval, err := workflowTiming(in.TimeoutMs, in.PollIntervalMs, 5000)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	result := waitForUI(ctx, io, udid, condition, interval, false)
	state := strings.ToLower(strings.TrimSpace(in.State))
	if state == "" {
		state = "present"
	}
	out := map[string]any{"success": result.reason == "", "device": udid, "state": state, "attempts": result.attempts, "elapsedMs": time.Since(start).Milliseconds()}
	attachUIMatches(out, result.matches)
	if result.elements != nil {
		attachUIObservation(out, result.elements, 20)
	}
	if result.reason != "" {
		out["reason"] = result.reason
		out["message"] = "UI condition was not verified."
		if result.err != nil {
			out["message"] = result.err.Error()
		}
	}
	return out, nil
}

func runUIAct(ctx context.Context, io uiWorkflowIO, udid string, in uiActIn) (map[string]any, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	timeout, interval, _ := workflowTiming(in.TimeoutMs, in.PollIntervalMs, 10000)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	start := time.Now()
	out := map[string]any{"success": false, "performed": false, "actionAttempted": false, "action": "tap", "device": udid}
	defer func() { out["elapsedMs"] = time.Since(start).Milliseconds() }()
	result := waitForUI(ctx, io, udid, uiWaitCondition{Selector: in.Selector}, interval, true)
	out["attempts"] = result.attempts
	attachUIMatches(out, result.matches)
	if result.reason != "" {
		for key, value := range result.blocked {
			out[key] = value
		}
		out["reason"] = result.reason
		if result.elements != nil {
			attachUIObservation(out, result.elements, in.ObservationLimit)
		}
		switch result.reason {
		case "ambiguous_selector":
			out["message"] = "Multiple elements match. Use a unique accessibilityId, add selector.role, or narrow selector.query; no tap was performed."
		case "timeout":
			out["message"] = "No unique actionable target appeared before the deadline; no tap was performed. Inspect matches for disabled or invalid-frame targets."
		default:
			if result.err != nil {
				out["message"] = result.err.Error()
			}
		}
		return out, nil
	}
	target := result.matches[0]
	if err := ctx.Err(); err != nil {
		out["reason"], out["message"] = contextReason(err), err.Error()
		return out, nil
	}
	out["actionAttempted"] = true
	if err := io.tap(ctx, udid, target); err != nil {
		out["performed"] = nil // A failed input process cannot prove whether the tap reached the app.
		out["reason"], out["actionOutcome"] = "action_failed", "unknown"
		out["message"] = "Tap completion is unknown; inspect the current UI before retrying. " + err.Error()
		return out, nil
	}
	out["performed"], out["actionOutcome"] = true, "completed"
	var observed []sim.Element
	if in.WaitFor != nil {
		post := waitForUI(ctx, io, udid, *in.WaitFor, interval, false)
		out["postconditionMet"], out["postconditionAttempts"] = post.reason == "", post.attempts
		observed = post.elements
		if post.reason != "" {
			out["reason"] = "postcondition_" + post.reason
			out["message"] = "The tap completed, but its postcondition was not verified; inspect the current UI before another action."
			if post.err != nil {
				out["detail"] = post.err.Error()
			}
			if observed != nil {
				attachUIObservation(out, observed, in.ObservationLimit)
			}
			return out, nil
		}
	} else {
		var err error
		observed, err = io.describe(ctx, udid)
		if err == nil {
			err = ctx.Err()
		}
		if err != nil {
			out["reason"] = "post_action_" + contextReason(err)
			out["message"] = "The tap completed, but the new UI could not be observed; inspect the current UI before another action."
			out["detail"] = err.Error()
			return out, nil
		}
	}
	attachUIObservation(out, observed, in.ObservationLimit)
	out["success"] = true
	return out, nil
}

func uiWorkflowToolResult(out map[string]any, err error) (*mcp.CallToolResult, map[string]any, error) {
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{IsError: out["success"] != true}, out, nil
}

func (a *App) handleUIWait(ctx context.Context, req *mcp.CallToolRequest, in uiWaitIn) (*mcp.CallToolResult, map[string]any, error) {
	if err := (uiWaitCondition{Selector: in.Selector, State: in.State}).validate(); err != nil {
		return nil, nil, err
	}
	timeout, _, err := workflowTiming(in.TimeoutMs, in.PollIntervalMs, 5000)
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	udid, err := a.resolveUDIDContext(ctx, in.SimulatorUuid)
	if err != nil {
		return nil, nil, err
	}
	out, err := runUIWait(ctx, a.uiWorkflowIO(), udid, in)
	return uiWorkflowToolResult(out, err)
}

func (a *App) handleUIAct(ctx context.Context, req *mcp.CallToolRequest, in uiActIn) (*mcp.CallToolResult, map[string]any, error) {
	if err := in.validate(); err != nil {
		return nil, nil, err
	}
	timeout, _, _ := workflowTiming(in.TimeoutMs, in.PollIntervalMs, 10000)
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	udid, err := a.resolveUDIDContext(ctx, in.SimulatorUuid)
	if err != nil {
		return nil, nil, err
	}
	out, err := runUIAct(ctx, a.uiWorkflowIO(), udid, in)
	return uiWorkflowToolResult(out, err)
}

func (a *App) registerUIWorkflowTools(srv *mcp.Server) {
	addTool(a, srv, toolMeta("ui_wait", "Wait for UI", "Wait for an accessibility selector to be present, absent, or enabled. Polls fresh UI under a bounded, cancellable deadline; returns compact observation, match count, and explicit timeout failures.", annRO()), a.handleUIWait)
	addTool(a, srv, toolMeta("ui_act", "Act on UI", "Tap a freshly resolved semantic selector, refusing ambiguity, disabled controls, invalid frames, and interrupts. Exact matches are preferred. Returns fresh compact UI; optional waitFor verifies the next state. A completed tap is never retried; performed remains true if verification fails.", annWrite()), a.handleUIAct)
}
