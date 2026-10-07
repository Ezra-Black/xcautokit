package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type Defaults struct {
	ProjectPath     string `json:"projectPath,omitempty"`
	Scheme          string `json:"scheme,omitempty"`
	Configuration   string `json:"configuration,omitempty"`
	SimulatorUdid   string `json:"simulatorUdid,omitempty"`
	SimulatorName   string `json:"simulatorName,omitempty"`
	DerivedDataPath string `json:"derivedDataPath,omitempty"`
	UseLatestOS     *bool  `json:"useLatestOS,omitempty"`
	TabIdentifier   string `json:"tabIdentifier,omitempty"`
}

type Store struct {
	mu      sync.RWMutex
	path    string
	cur     Defaults
	loadErr error
}

// New creates isolated defaults for this MCP process. Persistence is opt-in:
// set XCAUTOKIT_SESSION_FILE to an explicit file dedicated to this agent/project.
func New() *Store {
	s := &Store{path: os.Getenv("XCAUTOKIT_SESSION_FILE")}
	if s.path != "" {
		s.loadErr = s.load()
	}
	return s
}

func clone(d Defaults) Defaults {
	if d.UseLatestOS != nil {
		value := *d.UseLatestOS
		d.UseLatestOS = &value
	}
	return d
}

func (s *Store) Get() Defaults {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return clone(s.cur)
}

func (s *Store) Set(patch Defaults) (Defaults, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadErr != nil {
		return clone(s.cur), s.loadErr
	}
	next := clone(s.cur)
	if patch.ProjectPath != "" {
		next.ProjectPath = patch.ProjectPath
	}
	if patch.Scheme != "" {
		next.Scheme = patch.Scheme
	}
	if patch.Configuration != "" {
		next.Configuration = patch.Configuration
	}
	if patch.SimulatorUdid != "" {
		next.SimulatorUdid = patch.SimulatorUdid
	}
	if patch.SimulatorName != "" {
		next.SimulatorName = patch.SimulatorName
	}
	if patch.DerivedDataPath != "" {
		next.DerivedDataPath = patch.DerivedDataPath
	}
	if patch.UseLatestOS != nil {
		value := *patch.UseLatestOS
		next.UseLatestOS = &value
	}
	if patch.TabIdentifier != "" {
		next.TabIdentifier = patch.TabIdentifier
	}
	if err := s.saveLocked(next); err != nil {
		return clone(s.cur), err
	}
	s.cur = next
	return clone(s.cur), nil
}

func (s *Store) Clear(keys []string) (Defaults, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadErr != nil {
		return clone(s.cur), 0, s.loadErr
	}
	next := clone(s.cur)
	cleared := 0
	clearAll := len(keys) == 0
	clear := func(k string) bool {
		if clearAll {
			return true
		}
		for _, want := range keys {
			if want == k {
				return true
			}
		}
		return false
	}
	if clear("projectPath") && next.ProjectPath != "" {
		next.ProjectPath = ""
		cleared++
	}
	if clear("scheme") && next.Scheme != "" {
		next.Scheme = ""
		cleared++
	}
	if clear("configuration") && next.Configuration != "" {
		next.Configuration = ""
		cleared++
	}
	if clear("simulatorUdid") && next.SimulatorUdid != "" {
		next.SimulatorUdid = ""
		cleared++
	}
	if clear("simulatorName") && next.SimulatorName != "" {
		next.SimulatorName = ""
		cleared++
	}
	if clear("derivedDataPath") && next.DerivedDataPath != "" {
		next.DerivedDataPath = ""
		cleared++
	}
	if clear("useLatestOS") && next.UseLatestOS != nil {
		next.UseLatestOS = nil
		cleared++
	}
	if clear("tabIdentifier") && next.TabIdentifier != "" {
		next.TabIdentifier = ""
		cleared++
	}
	if clearAll {
		next = Defaults{}
	}
	if err := s.saveLocked(next); err != nil {
		return clone(s.cur), 0, err
	}
	s.cur = next
	return clone(s.cur), cleared, nil
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var loaded Defaults
	if err := json.Unmarshal(data, &loaded); err != nil {
		return fmt.Errorf("load session defaults %s: %w", s.path, err)
	}
	s.cur = loaded
	return nil
}

func (s *Store) saveLocked(next Defaults) error {
	if s.path == "" {
		return nil
	}
	data, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(s.path), ".session-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), s.path)
}
