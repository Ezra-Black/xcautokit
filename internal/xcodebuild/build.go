package xcodebuild

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Options struct {
	Project         string
	Scheme          string
	Configuration   string
	Destination     string
	DerivedDataPath string
	SimulatorName   string
	SimulatorUdid   string
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
	name := opts.SimulatorName
	if name == "" {
		name = "iPhone 16"
	}
	return fmt.Sprintf("platform=iOS Simulator,name=%s", name)
}

func run(args []string) (string, error) {
	cmd := exec.Command("xcodebuild", args...)
	out, err := cmd.CombinedOutput()
	text := string(out)
	if err != nil {
		return text, fmt.Errorf("xcodebuild failed: %v\n%s", err, text)
	}
	return text, nil
}

func Build(opts Options) (string, error) {
	proj, err := resolveProjectArgs(opts.Project)
	if err != nil {
		return "", err
	}
	cfg := opts.Configuration
	if cfg == "" {
		cfg = "Debug"
	}
	args := append(proj,
		"-scheme", opts.Scheme,
		"-configuration", cfg,
		"-destination", destination(opts),
		"-quiet",
		"build",
	)
	if opts.DerivedDataPath != "" {
		args = append(args, "-derivedDataPath", opts.DerivedDataPath)
	}
	return run(args)
}

func Test(opts Options) (string, error) {
	proj, err := resolveProjectArgs(opts.Project)
	if err != nil {
		return "", err
	}
	cfg := opts.Configuration
	if cfg == "" {
		cfg = "Debug"
	}
	args := append(proj,
		"-scheme", opts.Scheme,
		"-configuration", cfg,
		"-destination", destination(opts),
		"test",
	)
	if opts.DerivedDataPath != "" {
		args = append(args, "-derivedDataPath", opts.DerivedDataPath)
	}
	return run(args)
}

func Clean(opts Options) (string, error) {
	proj, err := resolveProjectArgs(opts.Project)
	if err != nil {
		return "", err
	}
	cfg := opts.Configuration
	if cfg == "" {
		cfg = "Debug"
	}
	args := append(proj, "-scheme", opts.Scheme, "-configuration", cfg, "clean")
	return run(args)
}

func ListSchemes(project string) (string, error) {
	proj, err := resolveProjectArgs(project)
	if err != nil {
		return "", err
	}
	args := append(proj, "-list")
	return run(args)
}

func ShowBuildSettings(opts Options) (string, error) {
	proj, err := resolveProjectArgs(opts.Project)
	if err != nil {
		return "", err
	}
	args := append(proj, "-scheme", opts.Scheme, "-showBuildSettings")
	if opts.Configuration != "" {
		args = append(args, "-configuration", opts.Configuration)
	}
	return run(args)
}

func BundleID(opts Options) (string, error) {
	out, err := ShowBuildSettings(opts)
	if err != nil {
		return "", err
	}
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "PRODUCT_BUNDLE_IDENTIFIER") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1]), nil
			}
		}
	}
	return "", fmt.Errorf("PRODUCT_BUNDLE_IDENTIFIER not found")
}

func DiscoverProjects(root string) ([]string, error) {
	if root == "" {
		root, _ = os.Getwd()
	}
	var found []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() && (name == "Pods" || name == ".git" || name == "DerivedData" || name == "build" || name == "node_modules") {
			return filepath.SkipDir
		}
		if d.IsDir() && (strings.HasSuffix(name, ".xcodeproj") || strings.HasSuffix(name, ".xcworkspace")) {
			// Prefer workspaces; still report both.
			found = append(found, path)
			return filepath.SkipDir
		}
		return nil
	})
	return found, err
}

func BuildAndRun(opts Options, bundleID string) (string, error) {
	log, err := Build(opts)
	if err != nil {
		return log, err
	}
	udid := opts.SimulatorUdid
	if udid == "" {
		udid = "booted"
	}
	// Best-effort: find .app via derived data / common path is left to caller.
	return log + "\nBuild succeeded. Launch via app_launch with bundle ID: " + bundleID, nil
}
