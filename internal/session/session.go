package session

import (
	"encoding/json"
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
	mu   sync.RWMutex
	path string
	cur  Defaults
}

func New() *Store {
	home, _ := os.UserHomeDir()
	path := filepath.Join(home, ".xcautokit", "session_defaults.json")
	_ = os.MkdirAll(filepath.Dir(path), 0755)
	s := &Store{path: path}
	_ = s.load()
	return s
}

func (s *Store) Get() Defaults {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.cur
}

func (s *Store) Set(patch Defaults) (Defaults, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if patch.ProjectPath != "" {
		s.cur.ProjectPath = patch.ProjectPath
	}
	if patch.Scheme != "" {
		s.cur.Scheme = patch.Scheme
	}
	if patch.Configuration != "" {
		s.cur.Configuration = patch.Configuration
	}
	if patch.SimulatorUdid != "" {
		s.cur.SimulatorUdid = patch.SimulatorUdid
	}
	if patch.SimulatorName != "" {
		s.cur.SimulatorName = patch.SimulatorName
	}
	if patch.DerivedDataPath != "" {
		s.cur.DerivedDataPath = patch.DerivedDataPath
	}
	if patch.UseLatestOS != nil {
		s.cur.UseLatestOS = patch.UseLatestOS
	}
	if patch.TabIdentifier != "" {
		s.cur.TabIdentifier = patch.TabIdentifier
	}
	if err := s.saveLocked(); err != nil {
		return s.cur, err
	}
	return s.cur, nil
}

func (s *Store) Clear(keys []string) (Defaults, int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
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
	if clear("projectPath") && s.cur.ProjectPath != "" {
		s.cur.ProjectPath = ""
		cleared++
	}
	if clear("scheme") && s.cur.Scheme != "" {
		s.cur.Scheme = ""
		cleared++
	}
	if clear("configuration") && s.cur.Configuration != "" {
		s.cur.Configuration = ""
		cleared++
	}
	if clear("simulatorUdid") && s.cur.SimulatorUdid != "" {
		s.cur.SimulatorUdid = ""
		cleared++
	}
	if clear("simulatorName") && s.cur.SimulatorName != "" {
		s.cur.SimulatorName = ""
		cleared++
	}
	if clear("derivedDataPath") && s.cur.DerivedDataPath != "" {
		s.cur.DerivedDataPath = ""
		cleared++
	}
	if clear("useLatestOS") && s.cur.UseLatestOS != nil {
		s.cur.UseLatestOS = nil
		cleared++
	}
	if clear("tabIdentifier") && s.cur.TabIdentifier != "" {
		s.cur.TabIdentifier = ""
		cleared++
	}
	if clearAll {
		s.cur = Defaults{}
	}
	if err := s.saveLocked(); err != nil {
		return s.cur, cleared, err
	}
	return s.cur, cleared, nil
}

func (s *Store) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return json.Unmarshal(data, &s.cur)
}

func (s *Store) saveLocked() error {
	data, err := json.MarshalIndent(s.cur, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.path, data, 0644)
}
