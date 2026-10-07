package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/xcautokit/xcautokit/skills"
)

func writeFixture(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

func readFixture(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestInitPreservesProjectConfiguration(t *testing.T) {
	dir := t.TempDir()
	agents := filepath.Join(dir, "AGENTS.md")
	skill := filepath.Join(dir, ".agents", "skills", "xcautokit", "SKILL.md")
	mcp := filepath.Join(dir, ".cursor", "mcp.json")
	writeFixture(t, agents, "# Existing project\nFollow our own rules.\n")
	writeFixture(t, skill, "Custom existing skill\n")
	writeFixture(t, mcp, `{"mcpServers":{"existing":{"command":"other","env":{"VALUE":"keep"}}},"custom":{"enabled":true},"largeId":12345678901234567890}`)
	if err := cmdInit([]string{"--dir", dir}); err != nil {
		t.Fatal(err)
	}
	if readFixture(t, agents) != "# Existing project\nFollow our own rules.\n" || readFixture(t, skill) != "Custom existing skill\n" {
		t.Fatal("existing instructions were overwritten")
	}
	var config map[string]json.RawMessage
	if err := json.Unmarshal([]byte(readFixture(t, mcp)), &config); err != nil {
		t.Fatal(err)
	}
	var servers map[string]json.RawMessage
	if err := json.Unmarshal(config["mcpServers"], &servers); err != nil {
		t.Fatal(err)
	}
	if len(servers) != 2 || len(servers["existing"]) == 0 || len(servers["xcautokit"]) == 0 || string(config["largeId"]) != "12345678901234567890" || len(config["custom"]) == 0 {
		t.Fatalf("unrelated config lost: %s", readFixture(t, mcp))
	}
	info, err := os.Stat(mcp)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("config permissions changed: %v", err)
	}
	before := readFixture(t, mcp)
	if err := cmdInit([]string{"--dir", dir}); err != nil {
		t.Fatal(err)
	}
	if readFixture(t, mcp) != before {
		t.Fatal("second init rewrote existing config")
	}
}

func TestInitPreservesExistingXCAutokit(t *testing.T) {
	dir := t.TempDir()
	mcp := filepath.Join(dir, ".cursor", "mcp.json")
	const custom = "{ \"mcpServers\": {\"xcautokit\": {\"command\":\"/custom/bin\",\"args\":[\"mcp\"]}} }\n"
	writeFixture(t, mcp, custom)
	if err := cmdInit([]string{"--dir", dir}); err != nil {
		t.Fatal(err)
	}
	if readFixture(t, mcp) != custom {
		t.Fatal("custom xcautokit setup was overwritten")
	}
	if readFixture(t, filepath.Join(dir, ".agents", "skills", "xcautokit", "SKILL.md")) != skills.Companion {
		t.Fatal("installed skill differs from bundled skill")
	}
}

func TestInitRejectsInvalidConfigBeforeWriting(t *testing.T) {
	for _, invalid := range []string{"not JSON", "null", "[]", `{"mcpServers":null}`, `{"mcpServers":[]}`} {
		t.Run(invalid, func(t *testing.T) {
			dir := t.TempDir()
			mcp := filepath.Join(dir, ".cursor", "mcp.json")
			writeFixture(t, mcp, invalid)
			if err := cmdInit([]string{"--dir", dir}); err == nil {
				t.Fatal("expected validation error")
			}
			if readFixture(t, mcp) != invalid {
				t.Fatal("invalid config changed")
			}
			if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); !os.IsNotExist(err) {
				t.Fatal("partial init created AGENTS.md")
			}
		})
	}
}

func TestInitRejectsSymlinkedConfig(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(dir, ".cursor")); err != nil {
		t.Fatal(err)
	}
	if err := cmdInit([]string{"--dir", dir}); err == nil {
		t.Fatal("expected symlink refusal")
	}
	if _, err := os.Stat(filepath.Join(outside, "mcp.json")); !os.IsNotExist(err) {
		t.Fatal("wrote through symlink outside requested project")
	}
}

func TestInitValidatesArguments(t *testing.T) {
	for _, args := range [][]string{{"--dir"}, {"--unknown"}, {"--dir", ""}} {
		if err := cmdInit(args); err == nil {
			t.Fatalf("accepted malformed arguments %v", args)
		}
	}
}

func TestInitDoesNotClobberNewlyCreatedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "AGENTS.md")
	file, err := prepareNewFile(path, []byte("generated"), dir)
	if err != nil {
		t.Fatal(err)
	}
	writeFixture(t, path, "created by another client")
	if err := writeInitFile(file); err == nil {
		t.Fatal("expected exclusive creation to fail")
	}
	if readFixture(t, path) != "created by another client" {
		t.Fatal("replaced file created after preflight")
	}
}

func TestInitDoesNotClobberChangedConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".cursor", "mcp.json")
	writeFixture(t, path, `{"mcpServers":{}}`)
	file, err := prepareMCPFile(path, dir)
	if err != nil {
		t.Fatal(err)
	}
	const updated = `{"mcpServers":{"new":{"command":"another-client"}}}`
	writeFixture(t, path, updated)
	if err := writeInitFile(file); err == nil {
		t.Fatal("expected changed config to fail")
	}
	if readFixture(t, path) != updated {
		t.Fatal("replaced config changed after preflight")
	}
}
