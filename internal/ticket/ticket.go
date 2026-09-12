package ticket

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os/exec"
	"sync"
	"time"
)

// Store holds opaque ticket handles for long-lived local operations
// (video recording, log capture).
//
// Tickets are process-local: they live in this MCP server process only.
// Agents must pass the ticket string back to the matching stop tool.
// If the MCP process restarts, outstanding tickets become invalid —
// start a new capture. This is an agent-threaded handle, not durable
// cross-process state.
type Store struct {
	mu   sync.Mutex
	recs map[string]*Record
}

type Record struct {
	ID        string         `json:"ticket"`
	Kind      string         `json:"kind"`
	CreatedAt time.Time      `json:"createdAt"`
	Path      string         `json:"path,omitempty"`
	UDID      string         `json:"udid,omitempty"`
	PID       int            `json:"pid,omitempty"`
	Cmd       *exec.Cmd      `json:"-"`
	Meta      map[string]any `json:"meta,omitempty"`
}

func New() *Store {
	return &Store{recs: map[string]*Record{}}
}

func (s *Store) Issue(kind string, rec *Record) *Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := "xca_" + kind[:min(3, len(kind))] + "_" + randomID(8)
	rec.ID = id
	rec.Kind = kind
	rec.CreatedAt = time.Now().UTC()
	if rec.Meta == nil {
		rec.Meta = map[string]any{}
	}
	s.recs[id] = rec
	return rec
}

func (s *Store) Get(id string) (*Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.recs[id]
	if !ok {
		return nil, fmt.Errorf("unknown ticket %q (tickets are process-local — restart means start a new capture)", id)
	}
	return r, nil
}

func (s *Store) Take(id string) (*Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.recs[id]
	if !ok {
		return nil, fmt.Errorf("unknown ticket %q (tickets are process-local — restart means start a new capture)", id)
	}
	delete(s.recs, id)
	return r, nil
}

func (s *Store) List(kind string) []*Record {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Record
	for _, r := range s.recs {
		if kind == "" || r.Kind == kind {
			out = append(out, r)
		}
	}
	return out
}

func randomID(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
