package session

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultsAreProcessLocalAndDoNotAlias(t *testing.T) {
	t.Setenv("XCAUTOKIT_SESSION_FILE", "")
	a, b := New(), New()
	latest := true
	set, err := a.Set(Defaults{ProjectPath: "/project/A", SimulatorUdid: "A", UseLatestOS: &latest})
	if err != nil {
		t.Fatal(err)
	}
	latest = false
	*set.UseLatestOS = false
	got := a.Get()
	if !*got.UseLatestOS {
		t.Fatal("set input/output aliases internal defaults")
	}
	*got.UseLatestOS = false
	if !*a.Get().UseLatestOS {
		t.Fatal("Get aliases internal defaults")
	}
	if b.Get().ProjectPath != "" || b.Get().SimulatorUdid != "" {
		t.Fatal("independent sessions inherited state")
	}
	if got, n, err := a.Clear([]string{"simulatorUdid", "useLatestOS"}); err != nil || n != 2 || got.SimulatorUdid != "" || got.UseLatestOS != nil || got.ProjectPath != "/project/A" {
		t.Fatalf("selective clear: %+v %d %v", got, n, err)
	}
}

func TestExplicitPersistenceAndAtomicFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "defaults.json")
	t.Setenv("XCAUTOKIT_SESSION_FILE", path)
	a := New()
	if _, err := a.Set(Defaults{Scheme: "Example"}); err != nil {
		t.Fatal(err)
	}
	if got := New().Get().Scheme; got != "Example" {
		t.Fatalf("explicit persistence lost: %q", got)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatalf("private defaults mode: %v %v", info, err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := a.Set(Defaults{Scheme: "Other"}); err == nil {
		t.Fatal("expected rename failure")
	}
	if got := a.Get().Scheme; got != "Example" {
		t.Fatalf("failed save changed memory: %q", got)
	}
	if _, n, err := a.Clear(nil); err == nil || n != 0 {
		t.Fatalf("failed clear should not commit: %d %v", n, err)
	}
}

func TestInvalidPersistenceIsNotOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "defaults.json")
	bad := []byte(`{"scheme":42}`)
	if err := os.WriteFile(path, bad, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("XCAUTOKIT_SESSION_FILE", path)
	a := New()
	if _, err := a.Set(Defaults{Scheme: "Replacement"}); err == nil {
		t.Fatal("invalid persisted defaults silently replaced")
	}
	got, err := os.ReadFile(path)
	if err != nil || string(got) != string(bad) {
		t.Fatalf("invalid file was overwritten: %s %v", got, err)
	}
}
