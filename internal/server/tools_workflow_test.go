package server

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/xcautokit/xcautokit/internal/sim"
)

func TestWorkflowStopsAfterPartialFailure(t *testing.T) {
	steps := []workflowStep{{Action: "tap"}, {Action: "type_text"}, {Action: "tap"}}
	calls := 0
	results, ok := executeWorkflow(context.Background(), steps, func(context.Context, workflowStep) (map[string]any, error) {
		calls++
		return map[string]any{"success": calls != 2, "performed": true}, nil
	})
	if ok || calls != 2 || len(results) != 2 || results[1].Result["performed"] != true {
		t.Fatalf("unexpected execution: %v %d %+v", ok, calls, results)
	}
}

func TestRunResourcesAreScopedAndPreserveEvidence(t *testing.T) {
	root := t.TempDir()
	id := "0123456789abcdef01234567"
	dir := filepath.Join(root, "runs", id)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	if err := writeExclusiveJSON(filepath.Join(dir, "trace.json"), map[string]any{"success": true}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "screen.png"), []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	trace, err := readRunResource(root, "xcautokit://runs/"+id)
	if err != nil || len(trace.Contents) != 1 || trace.Contents[0].Text == "" {
		t.Fatalf("trace: %+v %v", trace, err)
	}
	screen, err := readRunResource(root, "xcautokit://runs/"+id+"/screen")
	if err != nil || string(screen.Contents[0].Blob) != "fixture" {
		t.Fatalf("screen: %+v %v", screen, err)
	}
	for _, uri := range []string{"xcautokit://runs/../../secret", "xcautokit://runs/", "xcautokit://runs/" + id + "/trace.json", "xcautokit://runs/" + id + "/screen/.."} {
		if _, err := readRunResource(root, uri); err == nil {
			t.Fatalf("invalid resource accepted: %s", uri)
		}
	}
}

func TestCompactWorkflowPreservesFullTrace(t *testing.T) {
	results := []workflowStepResult{{Result: map[string]any{"success": false, "performed": true, "observation": "full", "reason": "timeout"}}}
	compact := compactWorkflowResults(results)
	if compact[0].Result["observation"] != nil || results[0].Result["observation"] != "full" || compact[0].Result["performed"] != true {
		t.Fatal("compaction dropped outcome or mutated original")
	}
}

func TestWorkflowRejectsInvalidLaterStepsBeforeRunning(t *testing.T) {
	selector := &sim.Selector{By: "label", Query: "Continue"}
	steps := []workflowStep{{Action: "tap", Selector: selector}, {Action: "type_text", TextFrom: "email"}}
	if err := validateWorkflow(steps, nil); err == nil {
		t.Fatal("missing runtime input accepted")
	}
	if err := validateWorkflow(steps, map[string]string{"email": "test@example.com"}); err != nil {
		t.Fatal(err)
	}
	if workflowVerified(steps) {
		t.Fatal("typing alone verifies task")
	}
	steps = append(steps, workflowStep{Action: "wait", Selector: selector})
	if !workflowVerified(steps) {
		t.Fatal("final assertion not recognized")
	}
	steps[2].Action = "shell"
	if err := validateWorkflow(steps, map[string]string{"email": "test@example.com"}); err == nil {
		t.Fatal("arbitrary execution accepted")
	}
}

func TestWorkflowRejectsIgnoredConditionsAndUnsupportedInput(t *testing.T) {
	condition := &uiWaitCondition{Selector: sim.Selector{By: "label", Query: "Saved"}}
	for _, step := range []workflowStep{
		{Action: "type_text", Text: "hello", WaitFor: condition},
		{Action: "launch", BundleID: "com.example.app", State: "absent"},
		{Action: "type_text", Text: "\u00e9"},
		{Action: "tap", Selector: &sim.Selector{By: "invalid", Query: "Save"}, WaitFor: condition},
	} {
		if err := validateWorkflow([]workflowStep{step}, nil); err == nil {
			t.Fatalf("invalid request accepted: %+v", step)
		}
	}
}

func TestWorkflowCancellationPerformsNoAction(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	results, ok := executeWorkflow(ctx, []workflowStep{{Action: "tap"}}, func(context.Context, workflowStep) (map[string]any, error) { called = true; return nil, nil })
	if called || ok || len(results) != 1 || results[0].Result["performed"] != false {
		t.Fatal("cancelled run executed a step")
	}
}

func TestArtifactCannotOverwrite(t *testing.T) {
	path := t.TempDir() + "/trace.json"
	if err := writeExclusiveJSON(path, map[string]any{"first": true}); err != nil {
		t.Fatal(err)
	}
	if err := writeExclusiveJSON(path, map[string]any{"first": false}); err == nil {
		t.Fatal("overwrote evidence")
	}
}
