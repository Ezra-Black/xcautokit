package server

import (
	"context"
	"encoding/json"
	"github.com/xcautokit/xcautokit/internal/session"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildRunUsesPinnedDeviceAndReportsInstallFailure(t *testing.T) {
	dir := t.TempDir()
	appDir := filepath.Join(dir, "products", "Example.app")
	if err := os.MkdirAll(appDir, 0700); err != nil {
		t.Fatal(err)
	}
	settings, _ := json.Marshal([]map[string]any{{"target": "Example", "buildSettings": map[string]string{
		"PRODUCT_TYPE": "com.apple.product-type.application", "PLATFORM_NAME": "iphonesimulator", "TARGET_BUILD_DIR": filepath.Dir(appDir),
		"FULL_PRODUCT_NAME": "Example.app", "PRODUCT_BUNDLE_IDENTIFIER": "com.example.new",
	}}})
	fixture := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(fixture, settings, 0600); err != nil {
		t.Fatal(err)
	}
	events := filepath.Join(dir, "events.txt")
	t.Setenv("XCAUTOKIT_PROJECT_TEST_SETTINGS", fixture)
	t.Setenv("XCAUTOKIT_PROJECT_TEST_EVENTS", events)
	t.Setenv("XCAUTOKIT_STATE_DIR", filepath.Join(dir, "state"))
	t.Setenv("XCAUTOKIT_SESSION_FILE", "")
	t.Setenv("PATH", dir+":/usr/bin:/bin")
	t.Setenv("XCAUTOKIT_PROJECT_TEST_FAIL", "install")
	scripts := map[string]string{
		"xcodebuild": `#!/bin/sh
printf 'build:%s\n' "$*" >> "$XCAUTOKIT_PROJECT_TEST_EVENTS"
case " $* " in *" -showBuildSettings "*) cat "$XCAUTOKIT_PROJECT_TEST_SETTINGS";; esac
`,
		"xcrun": `#!/bin/sh
printf 'sim:%s\n' "$*" >> "$XCAUTOKIT_PROJECT_TEST_EVENTS"
if [ "$2" = "$XCAUTOKIT_PROJECT_TEST_FAIL" ]; then echo install-failed; exit 5; fi
exit 0
`,
		"axe": "#!/bin/sh\nprintf '[]'\n",
	}
	for name, script := range scripts {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	a := &App{Session: session.New()}
	if _, err := a.Session.Set(session.Defaults{ProjectPath: filepath.Join(dir, "Example.xcodeproj"), Scheme: "Example", SimulatorUdid: "STALE-SESSION"}); err != nil {
		t.Fatal(err)
	}
	ctx := context.WithValue(context.Background(), deviceContextKey{}, "PINNED-DEVICE")
	res, out, err := a.handleBuildRunSim(ctx, nil, projectIn{})
	if err != nil || res == nil || !res.IsError || out["success"] != false || out["built"] != true || out["installed"] != false || out["launched"] != false || out["stage"] != "install" {
		t.Fatalf("incorrect failure response: %+v %+v %v", res, out, err)
	}
	calls, _ := os.ReadFile(events)
	if strings.Contains(string(calls), "STALE-SESSION") || strings.Contains(string(calls), "simctl launch") || !strings.Contains(string(calls), "simctl install PINNED-DEVICE "+appDir) {
		t.Fatalf("wrong device or stale launch: %s", calls)
	}
	t.Setenv("XCAUTOKIT_PROJECT_TEST_FAIL", "")
	res, out, err = a.handleBuildRunSim(ctx, nil, projectIn{})
	if err != nil || (res != nil && res.IsError) || out["success"] != true || out["built"] != true || out["installed"] != true || out["launched"] != true || out["bundleId"] != "com.example.new" || out["interruptCheck"] != "complete" {
		t.Fatalf("incorrect successful response: %+v %+v %v", res, out, err)
	}
}
