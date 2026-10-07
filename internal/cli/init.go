package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/xcautokit/xcautokit/skills"
)

type initFile struct {
	path     string
	data     []byte
	mode     os.FileMode
	changed  bool
	original []byte
}

func cmdInit(args []string) error {
	dir := "."
	for i := 0; i < len(args); i++ {
		if args[i] != "--dir" || i+1 == len(args) || args[i+1] == "" {
			return fmt.Errorf("usage: xcautokit init [--dir PATH]")
		}
		i++
		dir = args[i]
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return err
	}
	// Inspect all existing files before creating anything. A broken config must
	// not result in a partly installed setup or silently lose another server.
	agents, err := prepareNewFile(filepath.Join(abs, "AGENTS.md"), []byte(agentsTemplate()), abs)
	if err != nil {
		return err
	}
	companion, err := prepareNewFile(filepath.Join(abs, ".agents", "skills", "xcautokit", "SKILL.md"), []byte(skills.Companion), abs)
	if err != nil {
		return err
	}
	mcpFile, err := prepareMCPFile(filepath.Join(abs, ".cursor", "mcp.json"), abs)
	if err != nil {
		return err
	}
	for _, file := range []initFile{mcpFile, agents, companion} {
		if !file.changed {
			fmt.Printf("Preserved %s\n", file.path)
			continue
		}
		if err := writeInitFile(file); err != nil {
			return err
		}
		fmt.Printf("Wrote %s\n", file.path)
	}
	fmt.Println("Reload MCP and host skills in your client, then run: xcautokit doctor")
	return nil
}

func prepareNewFile(path string, data []byte, root string) (initFile, error) {
	if err := checkInitPath(path, root); err != nil {
		return initFile{}, err
	}
	_, err := os.Stat(path)
	if err == nil {
		return initFile{path: path}, nil
	}
	if !os.IsNotExist(err) {
		return initFile{}, err
	}
	return initFile{path: path, data: data, mode: 0644, changed: true}, nil
}

func prepareMCPFile(path, root string) (initFile, error) {
	if err := checkInitPath(path, root); err != nil {
		return initFile{}, err
	}
	file := initFile{path: path, mode: 0644}
	doc := map[string]json.RawMessage{}
	data, err := os.ReadFile(path)
	if err == nil {
		if err := json.Unmarshal(data, &doc); err != nil || doc == nil {
			return file, fmt.Errorf("preserving %s: expected a JSON object; fix the config before running init", path)
		}
		info, err := os.Stat(path)
		if err != nil {
			return file, err
		}
		file.mode = info.Mode().Perm()
		file.original = data
	} else if !os.IsNotExist(err) {
		return file, err
	}
	servers := map[string]json.RawMessage{}
	if raw, exists := doc["mcpServers"]; exists {
		if err := json.Unmarshal(raw, &servers); err != nil || servers == nil {
			return file, fmt.Errorf("preserving %s: mcpServers must be a JSON object", path)
		}
	}
	if _, exists := servers["xcautokit"]; exists {
		return file, nil
	}
	servers["xcautokit"] = json.RawMessage(`{"command":"npx","args":["-y","xcautokit@latest","mcp"]}`)
	doc["mcpServers"], err = json.Marshal(servers)
	if err != nil {
		return file, err
	}
	file.data, err = json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return file, err
	}
	file.data = append(file.data, '\n')
	file.changed = true
	return file, nil
}

// Refuse symlinks anywhere below the supplied destination rather than writing
// through a project config that redirects outside that project.
func checkInitPath(path, root string) error {
	for current := path; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("preserving symlink %s: init requires ordinary project files and directories", current)
			}
			if current == path && !info.Mode().IsRegular() {
				return fmt.Errorf("preserving %s: expected a regular file", path)
			}
			if current != path && !info.IsDir() {
				return fmt.Errorf("preserving %s: expected a directory", current)
			}
		} else if !os.IsNotExist(err) {
			return err
		}
		parent := filepath.Dir(current)
		if current == root || parent == current {
			return nil
		}
	}
}

func writeInitFile(file initFile) error {
	if err := os.MkdirAll(filepath.Dir(file.path), 0755); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(file.path), ".xcautokit-init-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err := f.Chmod(file.mode); err != nil {
		return err
	}
	if _, err := f.Write(file.data); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if file.original == nil {
		// An exclusive link publishes the complete file without replacing a file
		// created by another init/client after preflight.
		return os.Link(f.Name(), file.path)
	}
	current, err := os.ReadFile(file.path)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, file.original) {
		return fmt.Errorf("preserving %s: config changed during init; run init again", file.path)
	}
	return os.Rename(f.Name(), file.path)
}

func agentsTemplate() string {
	return `# XCAutokit

Use XCAutokit for Xcode and iOS Simulator work. Read .agents/skills/xcautokit/SKILL.md for tool routing, verification, and coordination by host-owned agents.

XCAutokit is deterministic: it runs no internal agents and makes no AI-provider calls. The host owns planning, interpretation, and any delegation.

Use an explicit simulator UUID when multiple devices are available. Prefer semantic UI actions, check expected outcomes, and inspect screenshots for visual changes. Resolve interrupts intentionally; never silently accept permissions.

Build and test tools use xcodebuild with explicit project settings. Separate live IDE tools use Xcode's bridge; enable Xcode Settings > Intelligence > Allow external agents to use Xcode tools for that surface.
`
}
