package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/xcautokit/xcautokit/internal/sim"
)

const workflowReadyFixture = `[{"type":"Button","role":"AXButton","AXLabel":"Continue","AXUniqueId":"continue","enabled":true,"frame":{"x":10,"y":20,"width":100,"height":44}}]`
const workflowDoneFixture = `[{"type":"StaticText","AXLabel":"Welcome","frame":{"x":20,"y":20,"width":100,"height":44}}]`
const workflowLoadingFixture = `[{"type":"StaticText","AXLabel":"Loading"}]`

type workflowFake struct {
	snapshots []string
	reads     int
	taps      []sim.SelectorMatch
	readErrAt int
	tapErr    error
}

func (fake *workflowFake) io(t *testing.T) uiWorkflowIO {
	t.Helper()
	return uiWorkflowIO{
		describe: func(ctx context.Context, udid string) ([]sim.Element, error) {
			fake.reads++
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			if fake.reads == fake.readErrAt {
				return nil, errors.New("accessibility unavailable")
			}
			index := fake.reads - 1
			if index >= len(fake.snapshots) {
				index = len(fake.snapshots) - 1
			}
			return sim.ParseDescribeUI(fake.snapshots[index])
		},
		tap: func(ctx context.Context, udid string, match sim.SelectorMatch) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			fake.taps = append(fake.taps, match)
			return fake.tapErr
		},
		block: inputBlockFromSnapshot,
	}
}

func continueSelector() sim.Selector {
	return sim.Selector{By: "accessibilityId", Query: "continue"}
}

func TestUIWaitFreshPresentAbsentAndEnabled(t *testing.T) {
	for _, tc := range []struct {
		name      string
		state     string
		selector  sim.Selector
		snapshots []string
	}{
		{"present", "present", continueSelector(), []string{workflowLoadingFixture, workflowReadyFixture}},
		{"absent", "absent", sim.Selector{By: "label", Query: "Loading"}, []string{workflowLoadingFixture, workflowReadyFixture}},
		{"enabled", "enabled", continueSelector(), []string{`[{"type":"Button","identifier":"continue","enabled":false,"frame":{"x":1,"y":1,"width":44,"height":44}}]`, workflowReadyFixture}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &workflowFake{snapshots: tc.snapshots}
			out, err := runUIWait(context.Background(), fake.io(t), "device", uiWaitIn{Selector: tc.selector, State: tc.state, TimeoutMs: 1000, PollIntervalMs: 50})
			if err != nil || out["success"] != true || fake.reads != 2 || len(fake.taps) != 0 {
				t.Fatalf("wait failed: %#v, %v, reads=%d", out, err, fake.reads)
			}
		})
	}
}

func TestUIWaitTimeoutAndCancellationBoundObservation(t *testing.T) {
	io := uiWorkflowIO{describe: func(ctx context.Context, _ string) ([]sim.Element, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	started := time.Now()
	out, err := runUIWait(context.Background(), io, "device", uiWaitIn{Selector: continueSelector(), TimeoutMs: 15})
	if err != nil || out["success"] != false || out["reason"] != "timeout" || time.Since(started) > time.Second {
		t.Fatalf("unbounded wait: %#v %v elapsed=%v", out, err, time.Since(started))
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	fake := &workflowFake{snapshots: []string{workflowReadyFixture}}
	out, err = runUIWait(ctx, fake.io(t), "device", uiWaitIn{Selector: continueSelector()})
	if err != nil || out["reason"] != "cancelled" || fake.reads != 0 {
		t.Fatalf("cancelled request performed IO: %#v %v reads=%d", out, err, fake.reads)
	}
}

func TestUIActExactTargetAndFreshPostActionObservation(t *testing.T) {
	fake := &workflowFake{snapshots: []string{`[
 {"type":"Button","label":"Continue later","frame":{"x":200,"y":20,"width":100,"height":44}},
 {"type":"Button","label":"Continue","frame":{"x":10,"y":20,"width":100,"height":44}}
]`, workflowDoneFixture}}
	out, err := runUIAct(context.Background(), fake.io(t), "device", uiActIn{Selector: sim.Selector{By: "label", Query: "Continue"}})
	if err != nil || out["success"] != true || out["performed"] != true || len(fake.taps) != 1 || fake.taps[0].CenterX != 60 {
		t.Fatalf("bad action: %#v %v taps=%#v", out, err, fake.taps)
	}
	observation := out["observation"].(sim.Observation)
	if fake.reads != 2 || observation.Elements[0].Label != "Welcome" {
		t.Fatalf("post-action observation is not fresh: %#v reads=%d", observation, fake.reads)
	}
}

func TestUIActRejectsAmbiguityBeforeInput(t *testing.T) {
	fake := &workflowFake{snapshots: []string{`[
 {"type":"Button","identifier":"continue","frame":{"x":10,"y":20,"width":100,"height":44}},
 {"type":"Button","identifier":"continue","enabled":false,"frame":{"x":200,"y":20,"width":100,"height":44}}
]`}}
	out, err := runUIAct(context.Background(), fake.io(t), "device", uiActIn{Selector: continueSelector()})
	if err != nil || out["reason"] != "ambiguous_selector" || out["performed"] != false || len(fake.taps) != 0 || fake.reads != 1 {
		t.Fatalf("ambiguous action must fail without choosing enabled first match: %#v %v", out, err)
	}
}

func TestUIActWaitsForEnabledFreshGeometry(t *testing.T) {
	fake := &workflowFake{snapshots: []string{
		`[{"type":"Button","identifier":"continue","enabled":false,"frame":{"x":200,"y":20,"width":100,"height":44}}]`,
		workflowReadyFixture, workflowDoneFixture,
	}}
	out, err := runUIAct(context.Background(), fake.io(t), "device", uiActIn{Selector: continueSelector(), TimeoutMs: 1000, PollIntervalMs: 50})
	if err != nil || out["success"] != true || len(fake.taps) != 1 || fake.taps[0].CenterX != 60 || fake.reads != 3 {
		t.Fatalf("action reused stale geometry or did not wait: %#v %v taps=%#v", out, err, fake.taps)
	}
}

func TestUIActNeverTapsDisabledOrInvalidFrames(t *testing.T) {
	for _, raw := range []string{
		`[{"type":"Button","identifier":"continue","enabled":false,"frame":{"x":1,"y":1,"width":44,"height":44}}]`,
		`[{"type":"Button","identifier":"continue","frame":{"x":1,"y":1,"width":0,"height":44}}]`,
		`[{"type":"Button","identifier":"continue","frame":{"width":44,"height":44}}]`,
	} {
		fake := &workflowFake{snapshots: []string{raw}}
		out, err := runUIAct(context.Background(), fake.io(t), "device", uiActIn{Selector: continueSelector(), TimeoutMs: 10})
		if err != nil || out["reason"] != "timeout" || out["performed"] != false || len(fake.taps) != 0 {
			t.Fatalf("non-actionable element accepted: %#v %v", out, err)
		}
	}
}

func TestUIActPreservesInterruptSafety(t *testing.T) {
	t.Setenv("XCAUTOKIT_INTERRUPT_GUARD", "on")
	fake := &workflowFake{snapshots: []string{`[{"type":"AXAlert","label":"Permission required","children":[{"type":"Button","label":"Continue","identifier":"continue","frame":{"x":10,"y":20,"width":100,"height":44}}]}]`}}
	out, err := runUIAct(context.Background(), fake.io(t), "device", uiActIn{Selector: continueSelector()})
	if err != nil || out["blocked"] != true || out["performed"] != false || len(fake.taps) != 0 || fake.reads != 1 {
		t.Fatalf("interrupt action did not fail closed on same snapshot: %#v %v", out, err)
	}
}

func TestUIActPostconditionNeverRepeatsTap(t *testing.T) {
	fake := &workflowFake{snapshots: []string{workflowReadyFixture, workflowLoadingFixture, workflowDoneFixture}}
	out, err := runUIAct(context.Background(), fake.io(t), "device", uiActIn{Selector: continueSelector(), TimeoutMs: 1000, PollIntervalMs: 50, WaitFor: &uiWaitCondition{Selector: sim.Selector{By: "text", Query: "Welcome"}}})
	if err != nil || out["success"] != true || out["postconditionMet"] != true || len(fake.taps) != 1 || fake.reads != 3 {
		t.Fatalf("bad verified action: %#v %v reads=%d taps=%d", out, err, fake.reads, len(fake.taps))
	}
	fake = &workflowFake{snapshots: []string{workflowReadyFixture, workflowLoadingFixture}}
	out, err = runUIAct(context.Background(), fake.io(t), "device", uiActIn{Selector: continueSelector(), TimeoutMs: 15, WaitFor: &uiWaitCondition{Selector: sim.Selector{By: "text", Query: "Welcome"}}})
	if err != nil || out["success"] != false || out["performed"] != true || out["reason"] != "postcondition_timeout" || len(fake.taps) != 1 {
		t.Fatalf("verification failure lost completed action: %#v %v taps=%d", out, err, len(fake.taps))
	}
	result, structured, err := uiWorkflowToolResult(out, nil)
	if err != nil || !result.IsError || structured["performed"] != true {
		t.Fatal("MCP must mark verification failure without losing performed=true")
	}
}

func TestUIActDistinguishesObservationAndInputFailures(t *testing.T) {
	for _, tc := range []struct {
		name      string
		readErrAt int
		performed any
		taps      int
	}{
		{"before action", 1, false, 0},
		{"after action", 2, true, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &workflowFake{snapshots: []string{workflowReadyFixture}, readErrAt: tc.readErrAt}
			out, err := runUIAct(context.Background(), fake.io(t), "device", uiActIn{Selector: continueSelector()})
			if err != nil || out["success"] != false || out["performed"] != tc.performed || len(fake.taps) != tc.taps {
				t.Fatalf("bad failure reporting: %#v %v", out, err)
			}
		})
	}
	fake := &workflowFake{snapshots: []string{workflowReadyFixture}, tapErr: errors.New("input process exited")}
	out, err := runUIAct(context.Background(), fake.io(t), "device", uiActIn{Selector: continueSelector()})
	if err != nil || out["performed"] != nil || out["actionAttempted"] != true || out["actionOutcome"] != "unknown" || fake.reads != 1 {
		t.Fatalf("input failure must not claim no side effect: %#v %v", out, err)
	}
}

func TestUIWaitDoesNotReturnStaleObservationAfterFailedRead(t *testing.T) {
	fake := &workflowFake{snapshots: []string{workflowLoadingFixture}, readErrAt: 2}
	out, err := runUIWait(context.Background(), fake.io(t), "device", uiWaitIn{Selector: continueSelector(), TimeoutMs: 1000, PollIntervalMs: 50})
	if err != nil || out["reason"] != "observation_failed" || out["observation"] != nil || out["count"] != 0 {
		t.Fatalf("failed fresh read reused prior UI: %#v %v", out, err)
	}
}

func TestUIWorkflowsValidateBeforeIO(t *testing.T) {
	fake := &workflowFake{snapshots: []string{workflowReadyFixture}}
	for _, in := range []uiActIn{
		{Selector: continueSelector(), Action: "press"},
		{Selector: continueSelector(), TimeoutMs: 60001},
		{Selector: continueSelector(), ObservationLimit: -1},
		{Selector: continueSelector(), PollIntervalMs: 1},
		{Selector: continueSelector(), WaitFor: &uiWaitCondition{Selector: continueSelector(), State: "settled"}},
	} {
		if _, err := runUIAct(context.Background(), fake.io(t), "device", in); err == nil {
			t.Fatalf("invalid action options accepted: %#v", in)
		}
	}
	if fake.reads != 0 || len(fake.taps) != 0 {
		t.Fatal("validation errors must not perform IO")
	}
}
