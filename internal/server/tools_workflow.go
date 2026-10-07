package server

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/xcautokit/xcautokit/internal/sim"
)

type workflowStep struct {
	Action    string           `json:"action" jsonschema:"tap, wait, type_text, swipe, button, or launch. No scripts or model calls."`
	Selector  *sim.Selector    `json:"selector,omitempty"`
	State     string           `json:"state,omitempty" jsonschema:"For wait: present, absent, or enabled"`
	WaitFor   *uiWaitCondition `json:"waitFor,omitempty" jsonschema:"Postcondition for tap"`
	Text      string           `json:"text,omitempty" jsonschema:"Literal text to type; use textFrom for runtime inputs"`
	TextFrom  string           `json:"textFrom,omitempty" jsonschema:"Key in inputs whose value is typed; input values are not saved in workflow definitions"`
	Direction string           `json:"direction,omitempty" jsonschema:"For swipe: up, down, left, or right"`
	Button    string           `json:"button,omitempty" jsonschema:"For button: home or lock"`
	BundleID  string           `json:"bundleId,omitempty" jsonschema:"For launch: app bundle identifier"`
	TimeoutMs int              `json:"timeoutMs,omitempty" jsonschema:"Step deadline, 1-60000; default 10000"`
}

type workflowRunIn struct {
	Steps         []workflowStep    `json:"steps,omitempty" jsonschema:"1-30 explicit steps, or supply workflow to replay a saved definition"`
	Workflow      string            `json:"workflow,omitempty" jsonschema:"Saved workflow name; mutually exclusive with steps"`
	Inputs        map[string]string `json:"inputs,omitempty"`
	SimulatorUuid string            `json:"simulatorUuid,omitempty"`
	TimeoutMs     int               `json:"timeoutMs,omitempty" jsonschema:"Overall deadline 1-120000 ms; default 60000"`
	Screenshot    *bool             `json:"screenshot,omitempty" jsonschema:"Capture a final/failure screenshot as evidence; default true"`
}

type workflowSaveIn struct {
	RunID       string `json:"runId" jsonschema:"Successful run ending with an explicit verified UI condition"`
	Name        string `json:"name" jsonschema:"New workflow name: lowercase letters, digits, hyphens; no overwrite"`
	Description string `json:"description,omitempty"`
}

type workflowDefinition struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Steps       []workflowStep `json:"steps"`
	SourceRun   string         `json:"sourceRun"`
	CreatedAt   time.Time      `json:"createdAt"`
}

type workflowStepResult struct {
	Index      int            `json:"index"`
	Action     string         `json:"action"`
	ObservedAt time.Time      `json:"observedAt"`
	Result     map[string]any `json:"result"`
}

type workflowTrace struct {
	RunID          string               `json:"runId"`
	Device         string               `json:"device"`
	StartedAt      time.Time            `json:"startedAt"`
	FinishedAt     time.Time            `json:"finishedAt"`
	Success        bool                 `json:"success"`
	Verified       bool                 `json:"verified"`
	Steps          []workflowStep       `json:"steps"`
	Results        []workflowStepResult `json:"results"`
	ScreenshotPath string               `json:"screenshotPath,omitempty"`
	EvidenceError  string               `json:"evidenceError,omitempty"`
}

var workflowNameRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
var runIDRE = regexp.MustCompile(`^[a-f0-9]{24}$`)

func validateWorkflow(steps []workflowStep, inputs map[string]string) error {
	if len(steps) == 0 || len(steps) > 30 {
		return fmt.Errorf("workflow must contain 1-30 steps")
	}
	for i, step := range steps {
		if step.WaitFor != nil && step.Action != "tap" {
			return fmt.Errorf("step %d: waitFor is only valid for tap", i)
		}
		if step.State != "" && step.Action != "wait" {
			return fmt.Errorf("step %d: state is only valid for wait", i)
		}
		if step.TimeoutMs < 0 || step.TimeoutMs > 60000 {
			return fmt.Errorf("step %d: timeoutMs must be 1-60000 or omitted", i)
		}
		var err error
		switch step.Action {
		case "tap", "wait":
			if step.Selector == nil {
				return fmt.Errorf("step %d: selector is required", i)
			}
			err = step.Selector.Validate()
			if err != nil {
				return fmt.Errorf("step %d: %w", i, err)
			}
			if step.Action == "wait" {
				err = (uiWaitCondition{Selector: *step.Selector, State: step.State}).validate()
			}
			if step.WaitFor != nil {
				if step.Action != "tap" {
					return fmt.Errorf("step %d: waitFor is only valid for tap", i)
				}
				err = step.WaitFor.validate()
			}
		case "type_text":
			if (step.Text == "") == (step.TextFrom == "") {
				return fmt.Errorf("step %d: provide exactly one of text or textFrom", i)
			}
			if step.TextFrom != "" {
				if value, ok := inputs[step.TextFrom]; !ok || value == "" {
					return fmt.Errorf("step %d: missing nonempty input %q", i, step.TextFrom)
				}
			}
			text := step.Text
			if step.TextFrom != "" {
				text = inputs[step.TextFrom]
			}
			err = validateTypeText(text)
		case "swipe":
			if step.Direction != "up" && step.Direction != "down" && step.Direction != "left" && step.Direction != "right" {
				return fmt.Errorf("step %d: invalid swipe direction", i)
			}
		case "button":
			if step.Button != "home" && step.Button != "lock" {
				return fmt.Errorf("step %d: button must be home or lock", i)
			}
		case "launch":
			if strings.TrimSpace(step.BundleID) == "" {
				return fmt.Errorf("step %d: bundleId is required", i)
			}
		default:
			return fmt.Errorf("step %d: unsupported action %q", i, step.Action)
		}
		if err != nil {
			return fmt.Errorf("step %d: %w", i, err)
		}
	}
	return nil
}

func workflowVerified(steps []workflowStep) bool {
	if len(steps) == 0 {
		return false
	}
	last := steps[len(steps)-1]
	return last.Action == "wait" || (last.Action == "tap" && last.WaitFor != nil)
}

// executeWorkflow never retries a mutation. Every next step depends on the
// previous result, and a partial/unknown action outcome stops the run.
func executeWorkflow(ctx context.Context, steps []workflowStep, execute func(context.Context, workflowStep) (map[string]any, error)) ([]workflowStepResult, bool) {
	results := make([]workflowStepResult, 0, len(steps))
	for i, step := range steps {
		if err := ctx.Err(); err != nil {
			results = append(results, workflowStepResult{Index: i, Action: step.Action, ObservedAt: time.Now().UTC(), Result: map[string]any{"success": false, "performed": false, "reason": contextReason(err)}})
			return results, false
		}
		out, err := execute(ctx, step)
		if err != nil {
			out = map[string]any{"success": false, "reason": "step_failed", "message": err.Error(), "performed": nil}
		}
		if out == nil {
			out = map[string]any{"success": false, "reason": "empty_step_result"}
		}
		results = append(results, workflowStepResult{Index: i, Action: step.Action, ObservedAt: time.Now().UTC(), Result: out})
		if out["success"] != true {
			return results, false
		}
	}
	return results, true
}

func (a *App) executeWorkflowStep(ctx context.Context, req *mcp.CallToolRequest, udid string, step workflowStep, inputs map[string]string) (map[string]any, error) {
	timeout := step.TimeoutMs
	if timeout == 0 {
		timeout = 10000
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(timeout)*time.Millisecond)
	defer cancel()
	if step.Action == "tap" {
		_, out, err := a.handleUIAct(ctx, req, uiActIn{Selector: *step.Selector, WaitFor: step.WaitFor, SimulatorUuid: udid, TimeoutMs: timeout})
		return out, err
	}
	if step.Action == "wait" {
		_, out, err := a.handleUIWait(ctx, req, uiWaitIn{Selector: *step.Selector, State: step.State, SimulatorUuid: udid, TimeoutMs: timeout})
		return out, err
	}
	if step.Action == "type_text" || step.Action == "swipe" {
		if blocked := a.guardInputBlocksContext(ctx, udid); blocked != nil {
			return blocked, nil
		}
	}
	var err error
	switch step.Action {
	case "type_text":
		text := step.Text
		if step.TextFrom != "" {
			text = inputs[step.TextFrom]
		}
		_, err = sim.RunAxeContext(ctx, typeTextArgs(udid, text)...)
		if err != nil {
			// Command errors can echo arguments containing private typed input.
			err = fmt.Errorf("text input command failed; outcome unknown")
		}
	case "swipe":
		var x1, y1, x2, y2 float64
		x1, y1, x2, y2, err = a.semanticSwipePoints(ctx, udid, step.Direction, "medium")
		if err == nil {
			_, err = sim.RunAxeContext(ctx, "swipe", "--start-x", fmt.Sprint(x1), "--start-y", fmt.Sprint(y1), "--end-x", fmt.Sprint(x2), "--end-y", fmt.Sprint(y2), "--udid", udid)
		}
	case "button":
		_, err = sim.RunAxeContext(ctx, "button", step.Button, "--udid", udid)
	case "launch":
		_, err = sim.RunSimctlContext(ctx, "launch", udid, step.BundleID)
	}
	if err != nil {
		return map[string]any{"success": false, "performed": nil, "reason": "action_outcome_unknown", "message": err.Error()}, nil
	}
	out := map[string]any{"success": true, "performed": true, "device": udid}
	els, err := a.uiWorkflowIO().describe(ctx, udid)
	if err != nil {
		out["success"] = false
		out["reason"] = "observation_failed"
		out["message"] = err.Error()
		return out, nil
	}
	attachUIObservation(out, els, 20)
	if len(blockingInterrupts(sim.DetectInterrupts(els))) > 0 {
		out["success"] = false
		out["reason"] = "blocking_interrupt"
		out["nextTool"] = "ui_dismiss_interrupt"
	}
	return out, nil
}

func writeExclusiveJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		_ = os.Remove(path)
		return err
	}
	return f.Close()
}

func readSmallJSON(path string, value any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, 2*1024*1024+1))
	if err != nil {
		return err
	}
	if len(data) > 2*1024*1024 {
		return fmt.Errorf("workflow artifact is too large")
	}
	return json.Unmarshal(data, value)
}

func (a *App) registerWorkflowTools(srv *mcp.Server) {
	addTool(a, srv, toolMeta("workflow_run", "Run Workflow", "Execute a bounded host-supplied or saved sequence. Stops on first failure; never plans, calls AI, or retries side effects. Returns a saved trace and final screenshot. Final declared assertions determine verified=true.", annWrite()), a.handleWorkflowRun)
	addTool(a, srv, toolMeta("workflow_save", "Save Verified Workflow", "Save a successful run that ended with a verified UI condition. Runtime inputs are omitted from the definition. Existing names are never overwritten.", annWrite()), func(ctx context.Context, req *mcp.CallToolRequest, in workflowSaveIn) (*mcp.CallToolResult, map[string]any, error) {
		if !runIDRE.MatchString(in.RunID) || !workflowNameRE.MatchString(in.Name) {
			return nil, nil, fmtError("invalid runId or workflow name")
		}
		var trace workflowTrace
		if err := readSmallJSON(filepath.Join(stateDir(), "runs", in.RunID, "trace.json"), &trace); err != nil {
			return nil, nil, err
		}
		if !trace.Success || !trace.Verified || !workflowVerified(trace.Steps) {
			return nil, nil, fmtError("only successful runs ending in an explicit verified condition can be saved")
		}
		dir := filepath.Join(stateDir(), "workflows")
		if err := os.MkdirAll(dir, 0700); err != nil {
			return nil, nil, err
		}
		definition := workflowDefinition{Name: in.Name, Description: in.Description, Steps: trace.Steps, SourceRun: in.RunID, CreatedAt: time.Now().UTC()}
		path := filepath.Join(dir, in.Name+".json")
		if err := writeExclusiveJSON(path, definition); err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"success": true, "name": in.Name, "path": path, "stepCount": len(trace.Steps)}, nil
	})
	addTool(a, srv, toolMeta("workflow_list", "List Workflows", "List saved deterministic workflows; no simulator interaction.", annRO()), func(ctx context.Context, req *mcp.CallToolRequest, in emptyIn) (*mcp.CallToolResult, map[string]any, error) {
		entries, err := os.ReadDir(filepath.Join(stateDir(), "workflows"))
		if err != nil && !os.IsNotExist(err) {
			return nil, nil, err
		}
		items := []map[string]any{}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
				continue
			}
			var d workflowDefinition
			if err := readSmallJSON(filepath.Join(stateDir(), "workflows", entry.Name()), &d); err != nil {
				continue
			}
			items = append(items, map[string]any{"name": d.Name, "description": d.Description, "stepCount": len(d.Steps), "sourceRun": d.SourceRun})
		}
		sort.Slice(items, func(i, j int) bool { return items[i]["name"].(string) < items[j]["name"].(string) })
		return nil, map[string]any{"workflows": items, "count": len(items)}, nil
	})
}

func (a *App) handleWorkflowRun(ctx context.Context, req *mcp.CallToolRequest, in workflowRunIn) (*mcp.CallToolResult, map[string]any, error) {
	if in.Workflow != "" {
		if len(in.Steps) > 0 || !workflowNameRE.MatchString(in.Workflow) {
			return nil, nil, fmtError("supply steps or a valid workflow name, not both")
		}
		var definition workflowDefinition
		if err := readSmallJSON(filepath.Join(stateDir(), "workflows", in.Workflow+".json"), &definition); err != nil {
			return nil, nil, err
		}
		in.Steps = definition.Steps
	}
	if err := validateWorkflow(in.Steps, in.Inputs); err != nil {
		return nil, nil, err
	}
	if in.TimeoutMs == 0 {
		in.TimeoutMs = 60000
	}
	if in.TimeoutMs < 1 || in.TimeoutMs > 120000 {
		return nil, nil, fmtError("timeoutMs must be 1-120000")
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(in.TimeoutMs)*time.Millisecond)
	defer cancel()
	udid, err := a.resolveUDIDContext(ctx, in.SimulatorUuid)
	if err != nil {
		return nil, nil, err
	}
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return nil, nil, err
	}
	id := hex.EncodeToString(b)
	dir := filepath.Join(stateDir(), "runs", id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, nil, err
	}
	trace := workflowTrace{RunID: id, Device: udid, StartedAt: time.Now().UTC(), Steps: in.Steps}
	trace.Results, trace.Success = executeWorkflow(ctx, in.Steps, func(ctx context.Context, step workflowStep) (map[string]any, error) {
		return a.executeWorkflowStep(ctx, req, udid, step, in.Inputs)
	})
	trace.Verified = trace.Success && workflowVerified(in.Steps)
	if (in.Screenshot == nil || *in.Screenshot) && ctx.Err() == nil {
		path := filepath.Join(dir, "screen.png")
		if _, err := sim.RunSimctlContext(ctx, "io", udid, "screenshot", "--type=png", path); err != nil {
			trace.EvidenceError = err.Error()
		} else {
			trace.ScreenshotPath = path
		}
	}
	trace.FinishedAt = time.Now().UTC()
	path := filepath.Join(dir, "trace.json")
	if err := writeExclusiveJSON(path, trace); err != nil {
		return nil, map[string]any{"success": false, "reason": "evidence_write_failed", "actionsSucceeded": trace.Success, "message": err.Error(), "results": trace.Results}, nil
	}
	out := map[string]any{"success": trace.Success, "verified": trace.Verified, "runId": id, "device": udid, "tracePath": path, "traceURI": "xcautokit://runs/" + id, "results": compactWorkflowResults(trace.Results), "screenshotPath": trace.ScreenshotPath, "verificationScope": "Only the supplied UI conditions were checked; host must assess overall task completion."}
	if trace.ScreenshotPath != "" {
		out["screenshotURI"] = "xcautokit://runs/" + id + "/screen"
	}
	if len(trace.Results) > 0 {
		last := trace.Results[len(trace.Results)-1].Result
		for _, key := range []string{"observation", "hasInterrupt", "interrupts"} {
			if value, ok := last[key]; ok {
				out[key] = value
			}
		}
	}
	if trace.EvidenceError != "" {
		out["evidenceError"] = trace.EvidenceError
	}
	if !trace.Success {
		out["failedStep"] = len(trace.Results) - 1
		out["nextAction"] = "Inspect the failed step and current UI before resuming. Earlier actions are not rolled back; do not blindly replay."
	}
	return nil, out, nil
}

// Keep full snapshots in the persisted trace, not in every step of the model's
// response. This bounds routine tool output while retaining inspectable evidence.
func compactWorkflowResults(results []workflowStepResult) []workflowStepResult {
	compact := make([]workflowStepResult, 0, len(results))
	for _, step := range results {
		out := map[string]any{}
		for key, value := range step.Result {
			if key != "observation" && key != "interrupts" && key != "matches" {
				out[key] = value
			}
		}
		step.Result = out
		compact = append(compact, step)
	}
	return compact
}
