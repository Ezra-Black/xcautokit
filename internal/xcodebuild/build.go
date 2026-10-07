package xcodebuild

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/xcautokit/xcautokit/internal/sim"
	"golang.org/x/sys/unix"
)

type Options struct {
	Context         context.Context
	Project         string
	Scheme          string
	Configuration   string
	Destination     string
	DerivedDataPath string
	SimulatorName   string
	SimulatorUdid   string
}

func (opts Options) context() context.Context {
	if opts.Context != nil {
		return opts.Context
	}
	return context.Background()
}

func resolveProjectArgs(project string) ([]string, error) {
	if project == "" {
		return nil, fmt.Errorf("project path required")
	}
	abs, err := filepath.Abs(project)
	if err != nil {
		return nil, err
	}
	switch {
	case strings.HasSuffix(abs, ".xcworkspace"):
		return []string{"-workspace", abs}, nil
	case strings.HasSuffix(abs, ".xcodeproj"):
		return []string{"-project", abs}, nil
	default:
		return nil, fmt.Errorf("project must be .xcodeproj or .xcworkspace: %s", abs)
	}
}

func destination(opts Options) string {
	if opts.Destination != "" {
		return opts.Destination
	}
	if opts.SimulatorUdid != "" {
		return fmt.Sprintf("platform=iOS Simulator,id=%s", opts.SimulatorUdid)
	}
	if opts.SimulatorName != "" {
		return fmt.Sprintf("platform=iOS Simulator,name=%s", opts.SimulatorName)
	}
	// Building does not require a booted device or a hard-coded device model.
	return "generic/platform=iOS Simulator"
}

// DerivedDataPath reuses incremental build artifacts across MCP restarts while
// isolating project, scheme, configuration, and simulator destinations.
func DerivedDataPath(opts Options) string {
	if opts.DerivedDataPath != "" {
		return opts.DerivedDataPath
	}
	project, _ := filepath.Abs(opts.Project)
	configuration := opts.Configuration
	if configuration == "" {
		configuration = "Debug"
	}
	sum := sha256.Sum256([]byte(strings.Join([]string{project, opts.Scheme, configuration, destination(opts)}, "\x00")))
	root := os.Getenv("XCAUTOKIT_STATE_DIR")
	if root == "" {
		home, _ := os.UserHomeDir()
		root = filepath.Join(home, ".xcautokit")
	}
	return filepath.Join(root, "DerivedData", hex.EncodeToString(sum[:12]))
}

func buildArgs(opts Options) ([]string, error) {
	proj, err := resolveProjectArgs(opts.Project)
	if err != nil {
		return nil, err
	}
	if opts.Scheme == "" {
		return nil, fmt.Errorf("scheme required")
	}
	cfg := opts.Configuration
	if cfg == "" {
		cfg = "Debug"
	}
	return append(proj, "-scheme", opts.Scheme, "-configuration", cfg,
		"-destination", destination(opts), "-derivedDataPath", DerivedDataPath(opts)), nil
}

type buildLockContextKey struct{}

func canonicalBuildDirectory(path string) (string, error) {
	canonical, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(canonical, 0700); err != nil {
		return "", err
	}
	return filepath.EvalSymlinks(canonical)
}

// This advisory lock coordinates XCAutokit processes only. External Xcode or
// xcodebuild processes do not participate in this locking protocol.
func lockBuildDirectory(ctx context.Context, path string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if path == "" {
		return func() {}, nil
	}
	canonical, err := canonicalBuildDirectory(path)
	if err != nil {
		return nil, err
	}
	if held, _ := ctx.Value(buildLockContextKey{}).(string); held == canonical {
		return func() {}, nil
	}
	file, err := os.OpenFile(filepath.Join(canonical, ".xcautokit-build.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	for {
		if err := ctx.Err(); err != nil {
			_ = file.Close()
			return nil, err
		}
		err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			var once sync.Once
			return func() { once.Do(func() { _ = unix.Flock(int(file.Fd()), unix.LOCK_UN); _ = file.Close() }) }, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			_ = file.Close()
			return nil, err
		}
		timer := time.NewTimer(25 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			_ = file.Close()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func run(ctx context.Context, timeout time.Duration, args []string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for i, arg := range args {
		if arg == "-derivedDataPath" && i+1 < len(args) {
			unlock, err := lockBuildDirectory(ctx, args[i+1])
			if err != nil {
				return "", fmt.Errorf("waiting for build directory: %w", err)
			}
			defer unlock()
			break
		}
	}
	cmd := exec.CommandContext(ctx, "xcodebuild", args...)
	// xcodebuild spawns compilers and test helpers. Cancelling only the parent
	// can leave those tools writing into the build directory after the call ends.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if err == syscall.ESRCH {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = 2 * time.Second
	out, err := cmd.CombinedOutput()
	text := string(out)
	if ctx.Err() != nil {
		return text, fmt.Errorf("xcodebuild interrupted: %w", ctx.Err())
	}
	if err != nil {
		return text, fmt.Errorf("xcodebuild failed: %w\n%s", err, text)
	}
	return text, nil
}

func Build(opts Options) (string, error) {
	args, err := buildArgs(opts)
	if err != nil {
		return "", err
	}
	return run(opts.context(), 20*time.Minute, append(args, "-quiet", "build"))
}

func Test(opts Options) (string, error) {
	args, err := buildArgs(opts)
	if err != nil {
		return "", err
	}
	return run(opts.context(), 30*time.Minute, append(args, "test"))
}

func Clean(opts Options) (string, error) {
	args, err := buildArgs(opts)
	if err != nil {
		return "", err
	}
	return run(opts.context(), 5*time.Minute, append(args, "clean"))
}

func ListSchemes(project string) (string, error) {
	return ListSchemesContext(context.Background(), project)
}

func ListSchemesContext(ctx context.Context, project string) (string, error) {
	proj, err := resolveProjectArgs(project)
	if err != nil {
		return "", err
	}
	return run(ctx, 2*time.Minute, append(proj, "-list"))
}

func ShowBuildSettings(opts Options) (string, error) {
	args, err := buildArgs(opts)
	if err != nil {
		return "", err
	}
	return run(opts.context(), 2*time.Minute, append(args, "-showBuildSettings"))
}

type Artifact struct {
	Path     string `json:"path"`
	BundleID string `json:"bundleId"`
	Target   string `json:"target"`
}

// AppArtifact resolves the scheme's simulator application using the identical
// destination, configuration and derived data passed to the build itself.
func AppArtifact(opts Options) (Artifact, error) {
	args, err := buildArgs(opts)
	if err != nil {
		return Artifact{}, err
	}
	out, err := run(opts.context(), 2*time.Minute, append(args, "-showBuildSettings", "-json"))
	if err != nil {
		return Artifact{}, err
	}
	return parseAppArtifact([]byte(out))
}

func parseAppArtifact(data []byte) (Artifact, error) {
	var targets []struct {
		Target   string            `json:"target"`
		Settings map[string]string `json:"buildSettings"`
	}
	if err := json.Unmarshal(data, &targets); err != nil {
		return Artifact{}, fmt.Errorf("invalid JSON build settings: %w", err)
	}
	var candidates []Artifact
	for _, target := range targets {
		s := target.Settings
		name := s["FULL_PRODUCT_NAME"]
		if name == "" {
			name = s["WRAPPER_NAME"]
		}
		if !strings.HasSuffix(name, ".app") {
			continue
		}
		productType := s["PRODUCT_TYPE"]
		if productType != "" && productType != "com.apple.product-type.application" {
			continue
		}
		platform := s["PLATFORM_NAME"]
		if platform != "iphonesimulator" {
			continue
		}
		if filepath.Base(name) != name || !filepath.IsAbs(s["TARGET_BUILD_DIR"]) || s["PRODUCT_BUNDLE_IDENTIFIER"] == "" {
			return Artifact{}, fmt.Errorf("app target %q is missing a valid product path or bundle identifier", target.Target)
		}
		candidates = append(candidates, Artifact{Path: filepath.Join(s["TARGET_BUILD_DIR"], name), BundleID: s["PRODUCT_BUNDLE_IDENTIFIER"], Target: target.Target})
	}
	if len(candidates) == 0 {
		return Artifact{}, fmt.Errorf("no iOS Simulator application target found in scheme build settings")
	}
	if len(candidates) > 1 {
		names := make([]string, 0, len(candidates))
		for _, app := range candidates {
			names = append(names, app.Target+" ("+app.BundleID+")")
		}
		sort.Strings(names)
		return Artifact{}, fmt.Errorf("multiple simulator application targets in scheme: %s; choose a scheme containing one application", strings.Join(names, ", "))
	}
	return candidates[0], nil
}

func BundleID(opts Options) (string, error) {
	app, err := AppArtifact(opts)
	return app.BundleID, err
}

type RunResult struct {
	Built     bool     `json:"built"`
	Installed bool     `json:"installed"`
	Launched  bool     `json:"launched"`
	Stage     string   `json:"stage"`
	Output    string   `json:"output"`
	App       Artifact `json:"app"`
}

// BuildRun builds, resolves, installs, and launches in that order. Stage flags
// remain accurate on failure; an already-installed older app is never launched.
func BuildRun(opts Options, expectedBundleID string) (RunResult, error) {
	result := RunResult{Stage: "resolve_device"}
	if opts.SimulatorUdid == "" || opts.SimulatorUdid == "booted" {
		return result, fmt.Errorf("build and run requires a resolved simulator UDID")
	}
	opts.Destination = "platform=iOS Simulator,id=" + opts.SimulatorUdid
	result.Stage = "build"
	if _, err := buildArgs(opts); err != nil {
		return result, err
	}
	// Keep the selected artifact protected until installation and launch finish;
	// per-command locks alone allow another build or clean between these stages.
	ctx, cancel := context.WithTimeout(opts.context(), 30*time.Minute)
	defer cancel()
	path, err := canonicalBuildDirectory(DerivedDataPath(opts))
	if err != nil {
		return result, err
	}
	result.Stage = "wait_build"
	unlock, err := lockBuildDirectory(ctx, path)
	if err != nil {
		return result, fmt.Errorf("waiting for build directory: %w", err)
	}
	defer unlock()
	opts.Context = context.WithValue(ctx, buildLockContextKey{}, path)
	result.Stage = "build"
	result.Output, err = Build(opts)
	if err != nil {
		return result, err
	}
	result.Built = true
	result.Stage = "resolve_artifact"
	result.App, err = AppArtifact(opts)
	if err != nil {
		return result, err
	}
	if expectedBundleID != "" && result.App.BundleID != expectedBundleID {
		return result, fmt.Errorf("built bundle ID %q does not match requested %q", result.App.BundleID, expectedBundleID)
	}
	info, err := os.Stat(result.App.Path)
	if err != nil {
		return result, fmt.Errorf("built app is unavailable at %s: %w", result.App.Path, err)
	}
	if !info.IsDir() {
		return result, fmt.Errorf("built app path is not a directory: %s", result.App.Path)
	}
	result.Stage = "install"
	if _, err := sim.RunSimctlContext(opts.context(), "install", opts.SimulatorUdid, result.App.Path); err != nil {
		return result, err
	}
	result.Installed = true
	result.Stage = "launch"
	if _, err := sim.RunSimctlContext(opts.context(), "launch", "--terminate-running-process", opts.SimulatorUdid, result.App.BundleID); err != nil {
		return result, err
	}
	result.Launched = true
	result.Stage = "complete"
	return result, nil
}

func BuildAndRun(opts Options, bundleID string) (string, error) {
	result, err := BuildRun(opts, bundleID)
	return result.Output, err
}

func DiscoverProjects(root string) ([]string, error) {
	return DiscoverProjectsContext(context.Background(), root)
}

func DiscoverProjectsContext(ctx context.Context, root string) ([]string, error) {
	if root == "" {
		root, _ = os.Getwd()
	}
	var found []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() && (name == "Pods" || name == ".git" || name == "DerivedData" || name == "build" || name == "node_modules") {
			return filepath.SkipDir
		}
		if d.IsDir() && (strings.HasSuffix(name, ".xcodeproj") || strings.HasSuffix(name, ".xcworkspace")) {
			found = append(found, path)
			return filepath.SkipDir
		}
		return nil
	})
	return found, err
}
