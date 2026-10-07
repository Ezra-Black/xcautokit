package xcodebuild

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildLockProcessHelper(t *testing.T) {
	mode := os.Getenv("XCAUTOKIT_BUILD_LOCK_HELPER")
	if mode == "" {
		return
	}
	if mode == "path" {
		fmt.Println("PATH=" + DerivedDataPath(Options{Project: "/shared/Example.xcodeproj", Scheme: "Example", SimulatorUdid: "DEVICE-A"}))
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	unlock, err := lockBuildDirectory(ctx, os.Getenv("XCAUTOKIT_BUILD_LOCK_DIR"))
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	fmt.Println("ready")
	_, _ = io.Copy(io.Discard, os.Stdin)
}

func TestDerivedDataStableAcrossProcessRestart(t *testing.T) {
	t.Setenv("XCAUTOKIT_STATE_DIR", t.TempDir())
	want := DerivedDataPath(Options{Project: "/shared/Example.xcodeproj", Scheme: "Example", SimulatorUdid: "DEVICE-A"})
	cmd := exec.Command(os.Args[0], "-test.run=^TestBuildLockProcessHelper$")
	cmd.Env = append(os.Environ(), "XCAUTOKIT_BUILD_LOCK_HELPER=path")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("helper failed: %s %v", out, err)
	}
	first := strings.SplitN(string(out), "\n", 2)[0]
	if first != "PATH="+want || strings.Contains(want, "process-") {
		t.Fatalf("restart changed derived data: child %q, parent %q", first, want)
	}
}

func TestBuildDirectoryCrossProcessExclusionAndCrashRecovery(t *testing.T) {
	path := t.TempDir()
	cmd := exec.Command(os.Args[0], "-test.run=^TestBuildLockProcessHelper$")
	cmd.Env = append(os.Environ(), "XCAUTOKIT_BUILD_LOCK_HELPER=lock", "XCAUTOKIT_BUILD_LOCK_DIR="+path)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stdin.Close(); _ = cmd.Process.Kill(); _ = cmd.Wait() })
	ready := make(chan string, 1)
	go func() { line, _ := bufio.NewReader(stdout).ReadString('\n'); ready <- line }()
	select {
	case line := <-ready:
		if line != "ready\n" {
			t.Fatalf("child lock failed: %q", line)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("child did not acquire build lock")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	if release, err := lockBuildDirectory(ctx, path); !errors.Is(err, context.DeadlineExceeded) {
		if release != nil {
			release()
		}
		t.Fatalf("cross-process build entered locked directory: %v", err)
	}
	other, err := lockBuildDirectory(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	other()
	if err := cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = cmd.Wait()
	ctx, releaseCancel := context.WithTimeout(context.Background(), time.Second)
	defer releaseCancel()
	release, err := lockBuildDirectory(ctx, path)
	if err != nil {
		t.Fatalf("crashed builder stranded lock: %v", err)
	}
	release()
}

func TestBuildRunProtectsArtifactUntilInstallAndLaunchComplete(t *testing.T) {
	opts, events, _ := fakeBuildTools(t)
	dir := filepath.Dir(events)
	ready, proceed := filepath.Join(dir, "install-ready"), filepath.Join(dir, "install-proceed")
	t.Setenv("XCAUTOKIT_INSTALL_READY", ready)
	t.Setenv("XCAUTOKIT_INSTALL_PROCEED", proceed)
	script := `#!/bin/sh
printf 'xcrun:%s\n' "$*" >> "$XCAUTOKIT_BUILD_TEST_EVENTS"
if [ "$2" = install ]; then
  echo ready > "$XCAUTOKIT_INSTALL_READY"
  while [ ! -f "$XCAUTOKIT_INSTALL_PROCEED" ]; do sleep 0.01; done
fi
`
	if err := os.WriteFile(filepath.Join(dir, "xcrun"), []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	opts.Context = ctx
	done := make(chan error, 1)
	go func() {
		result, err := BuildRun(opts, "")
		if err == nil && !result.Launched {
			err = fmt.Errorf("app was not launched")
		}
		done <- err
	}()
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(ready); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("build did not reach install stage")
		}
		time.Sleep(5 * time.Millisecond)
	}
	cleanOpts := opts
	cleanOpts.Destination = "platform=iOS Simulator,id=TARGET-A"
	cleanCtx, cleanCancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cleanCancel()
	cleanOpts.Context = cleanCtx
	if _, err := Clean(cleanOpts); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("clean altered artifact while installing: %v", err)
	}
	calls, _ := os.ReadFile(events)
	if strings.Contains(string(calls), " clean") {
		t.Fatalf("clean ran inside active pipeline: %s", calls)
	}
	if err := os.WriteFile(proceed, []byte("continue"), 0600); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("nested build locks deadlocked")
	}
	// Release at the pipeline boundary lets the next clean run normally.
	cleanOpts.Context = context.Background()
	if _, err := Clean(cleanOpts); err != nil {
		t.Fatalf("pipeline did not release build directory: %v", err)
	}
}
