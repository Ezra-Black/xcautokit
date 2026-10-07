package server

import (
	"os"
	"strings"
)

// workflowSet controls which tool groups are registered.
// Empty / "all" means every group.
//
// Env: XCAUTOKIT_WORKFLOWS=device,ui,input,app,capture,project,build,xcode
// Shorthand: core (= everything except xcode), full / all / empty (= everything)
type workflowSet map[string]bool

func parseWorkflows(raw string) workflowSet {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" || raw == "all" || raw == "full" {
		return workflowSet{"all": true}
	}
	if raw == "core" {
		return workflowSet{
			"device": true, "ui": true, "input": true, "app": true,
			"capture": true, "project": true, "build": true, "session": true, "coordination": true, "workflow": true,
		}
	}
	out := workflowSet{}
	for _, part := range strings.Split(raw, ",") {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		out[p] = true
		if p == "ui" || p == "input" || p == "app" || p == "build" || p == "device" || p == "workflow" {
			out["coordination"] = true
		}
		// session tools live with project registration
		if p == "project" {
			out["session"] = true
		}
		if p == "build" {
			out["project"] = true
			out["session"] = true
		}
	}
	if len(out) == 0 {
		return workflowSet{"all": true}
	}
	return out
}

func workflowsFromEnv() workflowSet {
	return parseWorkflows(os.Getenv("XCAUTOKIT_WORKFLOWS"))
}

func (w workflowSet) enabled(group string) bool {
	if w["all"] {
		return true
	}
	return w[group]
}
