package xcodebuild

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func appSettings(target, dir, bundle string) map[string]any {
	return map[string]any{"target": target, "buildSettings": map[string]string{
		"PRODUCT_TYPE": "com.apple.product-type.application", "PLATFORM_NAME": "iphonesimulator",
		"TARGET_BUILD_DIR": dir, "FULL_PRODUCT_NAME": target + ".app", "PRODUCT_BUNDLE_IDENTIFIER": bundle,
	}}
}

func TestAppArtifactUsesApplicationTargetAndRejectsAmbiguity(t *testing.T) {
	targets := []map[string]any{
		{"target": "Tests", "buildSettings": map[string]string{"PRODUCT_BUNDLE_IDENTIFIER": "com.example.tests", "FULL_PRODUCT_NAME": "Tests.xctest"}},
		appSettings("Main", "/products/Debug-iphonesimulator", "com.example.main"),
	}
	data, _ := json.Marshal(targets)
	app, err := parseAppArtifact(data)
	if err != nil || app.BundleID != "com.example.main" || app.Path != "/products/Debug-iphonesimulator/Main.app" {
		t.Fatalf("wrong application artifact: %+v %v", app, err)
	}
	targets = append(targets, appSettings("Second", "/products/Debug-iphonesimulator", "com.example.second"))
	data, _ = json.Marshal(targets)
	if _, err := parseAppArtifact(data); err == nil || !strings.Contains(err.Error(), "multiple") {
		t.Fatalf("ambiguous app targets accepted: %v", err)
	}
	bad := appSettings("Bad", "/products", "")
	data, _ = json.Marshal([]map[string]any{bad})
	if _, err := parseAppArtifact(data); err == nil {
		t.Fatal("missing bundle ID accepted")
	}
	bad = appSettings("Bad", "/products", "com.example.bad")
	bad["buildSettings"].(map[string]string)["PLATFORM_NAME"] = "iphoneos"
	data, _ = json.Marshal([]map[string]any{bad})
	if _, err := parseAppArtifact(data); err == nil {
		t.Fatal("physical-device app accepted")
	}
}

func TestArgumentsPreserveDestinationAndIsolateDerivedData(t *testing.T) {
	t.Setenv("XCAUTOKIT_STATE_DIR", t.TempDir())
	opts := Options{Project: "/project/Example.xcworkspace", Scheme: "Example", Configuration: "Release", SimulatorUdid: "DEVICE-A"}
	args, err := buildArgs(opts)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-workspace /project/Example.xcworkspace") || !strings.Contains(joined, "-destination platform=iOS Simulator,id=DEVICE-A") || !strings.Contains(joined, "-configuration Release") {
		t.Fatalf("wrong args: %v", args)
	}
	first := DerivedDataPath(opts)
	if first != DerivedDataPath(opts) {
		t.Fatal("derived data changed between build and settings")
	}
	opts.SimulatorUdid = "DEVICE-B"
	if first == DerivedDataPath(opts) {
		t.Fatal("different simulators share build database")
	}
	opts.DerivedDataPath = "/chosen/cache"
	if DerivedDataPath(opts) != opts.DerivedDataPath {
		t.Fatal("explicit derived data ignored")
	}
	if got := destination(Options{}); got != "generic/platform=iOS Simulator" {
		t.Fatalf("hard-coded simulator default: %s", got)
	}
}

func fakeBuildTools(t *testing.T) (Options, string, string) {
	t.Helper()
	dir := t.TempDir()
	appDir := filepath.Join(dir, "products", "Main.app")
	if err := os.MkdirAll(appDir, 0700); err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(dir, "settings.json")
	data, _ := json.Marshal([]map[string]any{appSettings("Main", filepath.Dir(appDir), "com.example.main")})
	if err := os.WriteFile(settings, data, 0600); err != nil {
		t.Fatal(err)
	}
	events := filepath.Join(dir, "events.txt")
	t.Setenv("XCAUTOKIT_BUILD_TEST_EVENTS", events)
	t.Setenv("XCAUTOKIT_BUILD_TEST_SETTINGS", settings)
	t.Setenv("XCAUTOKIT_BUILD_TEST_FAILURE", "")
	t.Setenv("XCAUTOKIT_STATE_DIR", filepath.Join(dir, "state"))
	t.Setenv("PATH", dir+":/usr/bin:/bin")
	xcode := `#!/bin/sh
printf 'xcodebuild:%s\n' "$*" >> "$XCAUTOKIT_BUILD_TEST_EVENTS"
case " $* " in
  *" -showBuildSettings "*)
    if [ "$XCAUTOKIT_BUILD_TEST_FAILURE" = resolve_artifact ]; then echo 'bad settings'; exit 4; fi
    cat "$XCAUTOKIT_BUILD_TEST_SETTINGS" ;;
  *)
    if [ "$XCAUTOKIT_BUILD_TEST_FAILURE" = build ]; then echo 'compile failed'; exit 3; fi
    echo 'build succeeded' ;;
esac
`
	simctl := `#!/bin/sh
printf 'xcrun:%s\n' "$*" >> "$XCAUTOKIT_BUILD_TEST_EVENTS"
if [ "$1" != simctl ]; then exit 9; fi
if [ "$XCAUTOKIT_BUILD_TEST_FAILURE" = "$2" ]; then echo 'simctl stage failed'; exit 5; fi
case "$2" in install|launch) exit 0;; *) exit 8;; esac
`
	for name, script := range map[string]string{"xcodebuild": xcode, "xcrun": simctl} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(script), 0700); err != nil {
			t.Fatal(err)
		}
	}
	return Options{Project: filepath.Join(dir, "Example.xcodeproj"), Scheme: "Example", Configuration: "Release", SimulatorUdid: "TARGET-A", Destination: "platform=iOS Simulator,id=STALE"}, events, appDir
}

func TestBuildRunInstallsFreshArtifactBeforeLaunch(t *testing.T) {
	opts, events, appDir := fakeBuildTools(t)
	result, err := BuildRun(opts, "")
	if err != nil || !result.Built || !result.Installed || !result.Launched || result.Stage != "complete" {
		t.Fatalf("build run: %+v %v", result, err)
	}
	data, err := os.ReadFile(events)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 4 {
		t.Fatalf("unexpected calls: %s", data)
	}
	for _, line := range lines[:2] {
		if !strings.Contains(line, "platform=iOS Simulator,id=TARGET-A") || strings.Contains(line, "STALE") || !strings.Contains(line, DerivedDataPath(Options{Project: opts.Project, Scheme: opts.Scheme, Configuration: opts.Configuration, SimulatorUdid: "TARGET-A"})) {
			t.Fatalf("build/settings target mismatch: %s", line)
		}
	}
	if lines[2] != "xcrun:simctl install TARGET-A "+appDir || lines[3] != "xcrun:simctl launch --terminate-running-process TARGET-A com.example.main" {
		t.Fatalf("wrong install/launch sequence: %s", data)
	}
}

func TestBuildRunFailureStagesNeverLaunchOldApp(t *testing.T) {
	for _, tc := range []struct {
		stage            string
		built, installed bool
		commands         int
	}{
		{"build", false, false, 1}, {"resolve_artifact", true, false, 2}, {"install", true, false, 3}, {"launch", true, true, 4},
	} {
		t.Run(tc.stage, func(t *testing.T) {
			opts, events, _ := fakeBuildTools(t)
			t.Setenv("XCAUTOKIT_BUILD_TEST_FAILURE", tc.stage)
			result, err := BuildRun(opts, "")
			if err == nil || result.Built != tc.built || result.Installed != tc.installed || result.Launched || result.Stage != tc.stage {
				t.Fatalf("incorrect failure flags: %+v %v", result, err)
			}
			data, _ := os.ReadFile(events)
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(lines) != tc.commands {
				t.Fatalf("continued after failure: %s", data)
			}
		})
	}
}

func TestBuildRunMissingArtifactDoesNotInstall(t *testing.T) {
	opts, events, appDir := fakeBuildTools(t)
	if err := os.Remove(appDir); err != nil {
		t.Fatal(err)
	}
	result, err := BuildRun(opts, "")
	if err == nil || result.Stage != "resolve_artifact" || !result.Built || result.Installed || result.Launched {
		t.Fatalf("missing built artifact: %+v %v", result, err)
	}
	data, _ := os.ReadFile(events)
	if strings.Contains(string(data), "xcrun:") {
		t.Fatal("missing app caused simulator mutation")
	}
}

func TestBuildCommandCancellation(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XCAUTOKIT_STATE_DIR", filepath.Join(dir, "state"))
	if err := os.WriteFile(filepath.Join(dir, "xcodebuild"), []byte("#!/bin/sh\nsleep 30 &\nwait\n"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+":/usr/bin:/bin")
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := Build(Options{Context: ctx, Project: "/test/Example.xcodeproj", Scheme: "Example"})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lost cancellation: %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("cancelled compiler child kept command open")
	}
}

func TestBuildDirectorySerializesAndWaitCanCancel(t *testing.T) {
	path := t.TempDir()
	unlock, err := lockBuildDirectory(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if release, err := lockBuildDirectory(ctx, path); !errors.Is(err, context.DeadlineExceeded) {
		if release != nil {
			release()
		}
		t.Fatalf("parallel build entered occupied directory: %v", err)
	}
	other, err := lockBuildDirectory(context.Background(), path+"-other")
	if err != nil {
		t.Fatal(err)
	}
	other()
}
