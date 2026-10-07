package ticket

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// Store owns long-lived child processes. Tickets and processes belong exclusively
// to this MCP server; callers must start a new capture after a server restart.
type Store struct {
	mu     sync.Mutex
	recs   map[string]*Record
	closed bool
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

	done          chan struct{}
	waitErr       error
	stopOnce      sync.Once
	stopRequested atomic.Bool
	timedOut      atomic.Bool
}

func New() *Store {
	return &Store{recs: map[string]*Record{}}
}

// Issue takes ownership of a started command, including its single Wait call.
// A completed ticket is retained until consumed so its artifact remains findable.
func (s *Store) Issue(kind string, rec *Record) *Record {
	rec.ID = "xca_" + kind[:min(3, len(kind))] + "_" + randomID(8)
	rec.Kind = kind
	rec.CreatedAt = time.Now().UTC()
	if rec.Meta == nil {
		rec.Meta = map[string]any{}
	}
	rec.done = make(chan struct{})
	if rec.Cmd != nil && rec.Cmd.Process != nil {
		go func() {
			rec.waitErr = rec.Cmd.Wait()
			close(rec.done)
		}()
	} else {
		close(rec.done)
	}
	s.mu.Lock()
	closed := s.closed
	if !closed {
		s.recs[rec.ID] = rec
	}
	s.mu.Unlock()
	if closed {
		_ = rec.Stop(context.Background(), time.Second)
	}
	return rec
}

func unknownTicket(id string) error {
	return fmt.Errorf("unknown ticket %q (tickets are process-local — restart means start a new capture)", id)
}

func (s *Store) Get(id string) (*Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.recs[id]
	if !ok {
		return nil, unknownTicket(id)
	}
	return r, nil
}

func (s *Store) Take(id string) (*Record, error) { return s.TakeKind(id, "") }

// TakeKind validates before removal: accidentally using a log ticket with
// record_stop must not lose the caller's only handle to the log capture.
func (s *Store) TakeKind(id, kind string) (*Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.recs[id]
	if !ok {
		return nil, unknownTicket(id)
	}
	if kind != "" && r.Kind != kind {
		return nil, fmt.Errorf("ticket %q is a %s capture, not %s; use its matching stop tool", id, r.Kind, kind)
	}
	delete(s.recs, id)
	return r, nil
}

// TakeMatching is the legacy no-ticket path. It never guesses when several
// captures match, and matching and removal happen under the same lock.
func (s *Store) TakeMatching(kind, udid string) (*Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var matches []*Record
	for _, r := range s.recs {
		if r.Kind == kind && (udid == "" || r.UDID == udid) {
			matches = append(matches, r)
		}
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("no %s capture ticket matches device %q; start a new capture", kind, udid)
	}
	if len(matches) > 1 {
		ids := make([]string, 0, len(matches))
		for _, r := range matches {
			ids = append(ids, r.ID)
		}
		sort.Strings(ids)
		return nil, fmt.Errorf("multiple %s capture tickets match; pass an explicit ticket: %v", kind, ids)
	}
	r := matches[0]
	delete(s.recs, r.ID)
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
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.After(out[j].CreatedAt)
	})
	return out
}

func (r *Record) Done() <-chan struct{} { return r.done }
func (r *Record) TimedOut() bool        { return r.timedOut.Load() }

// Stop lets simctl flush video and logs, then kills and reaps an unresponsive
// child. Concurrent stop, timeout, and shutdown all share one Wait goroutine.
func (r *Record) Stop(ctx context.Context, grace time.Duration) error {
	select {
	case <-r.done:
		return r.exitError()
	default:
	}
	var signalErr error
	r.stopOnce.Do(func() {
		r.stopRequested.Store(true)
		signalErr = r.Cmd.Process.Signal(os.Interrupt)
	})
	if signalErr != nil && !errors.Is(signalErr, os.ErrProcessDone) {
		return signalErr
	}
	timer := time.NewTimer(grace)
	defer timer.Stop()
	select {
	case <-r.done:
		return nil
	case <-ctx.Done():
	case <-timer.C:
	}
	if err := r.Cmd.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	// Once killed, cleanup must finish even if the caller has cancelled.
	reap := time.NewTimer(2 * time.Second)
	defer reap.Stop()
	select {
	case <-r.done:
		return nil
	case <-reap.C:
		return fmt.Errorf("capture %s did not exit after termination", r.ID)
	}
}

func (r *Record) exitError() error {
	if r.waitErr != nil && !r.stopRequested.Load() {
		return fmt.Errorf("%s capture exited unexpectedly: %w (output: %s)", r.Kind, r.waitErr, r.Path)
	}
	return nil
}

func (r *Record) StopAfter(timeout time.Duration) {
	go func() {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case <-r.done:
			return
		case <-timer.C:
			r.timedOut.Store(true)
			_ = r.Stop(context.Background(), time.Second)
		}
	}()
}

// Close drains all owned capture processes concurrently, including recordings
// that were never explicitly stopped by their caller.
func (s *Store) Close() error {
	s.mu.Lock()
	s.closed = true
	list := make([]*Record, 0, len(s.recs))
	for id, r := range s.recs {
		list = append(list, r)
		delete(s.recs, id)
	}
	s.mu.Unlock()
	var wg sync.WaitGroup
	errs := make(chan error, len(list))
	for _, r := range list {
		wg.Add(1)
		go func() { defer wg.Done(); errs <- r.Stop(context.Background(), 3*time.Second) }()
	}
	wg.Wait()
	close(errs)
	var all []error
	for err := range errs {
		if err != nil {
			all = append(all, err)
		}
	}
	return errors.Join(all...)
}

func randomID(n int) string {
	b := make([]byte, n)
	// crypto/rand.Read is guaranteed to succeed or terminate on supported Go.
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
