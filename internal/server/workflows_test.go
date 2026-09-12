package server

import "testing"

func TestParseWorkflows(t *testing.T) {
	all := parseWorkflows("")
	if !all.enabled("xcode") || !all.enabled("device") {
		t.Fatalf("empty should enable all")
	}

	core := parseWorkflows("core")
	if core.enabled("xcode") {
		t.Fatalf("core should omit xcode")
	}
	if !core.enabled("ui") || !core.enabled("build") {
		t.Fatalf("core should include ui/build")
	}

	custom := parseWorkflows("device,ui")
	if !custom.enabled("device") || !custom.enabled("ui") {
		t.Fatalf("custom missing groups")
	}
	if custom.enabled("xcode") || custom.enabled("capture") {
		t.Fatalf("custom should not enable unlisted groups")
	}

	build := parseWorkflows("build")
	if !build.enabled("project") || !build.enabled("session") {
		t.Fatalf("build should imply project/session")
	}
}
