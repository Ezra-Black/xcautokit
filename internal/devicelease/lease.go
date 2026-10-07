// Package devicelease coordinates simulator writers across MCP processes.
// Locks are advisory: other programs that do not use XCAutokit are unaffected.
package devicelease

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

type Info struct {
	Device    string    `json:"device"`
	Owner     string    `json:"owner,omitempty"`
	Claimed   bool      `json:"claimed"`
	ExpiresAt time.Time `json:"expiresAt,omitempty"`
	Busy      bool      `json:"busy,omitempty"`
}

type held struct {
	info   Info
	token  string
	file   *os.File
	timer  *time.Timer
	active bool
}

type Store struct {
	mu     sync.Mutex
	dir    string
	held   map[string]*held
	closed bool
}

func New(dir string) *Store { return &Store{dir: dir, held: make(map[string]*held)} }

var deviceID = regexp.MustCompile(`^[A-Za-z0-9-]{1,80}$`)

func (s *Store) open(device string) (*os.File, error) {
	if !deviceID.MatchString(device) {
		return nil, fmt.Errorf("invalid device identifier")
	}
	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return nil, err
	}
	return os.OpenFile(filepath.Join(s.dir, device+".lock"), os.O_CREATE|os.O_RDWR, 0600)
}

func lock(f *os.File) error { return unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB) }
func unlock(f *os.File)     { _ = unix.Flock(int(f.Fd()), unix.LOCK_UN); _ = f.Close() }
func writeInfo(h *held) error {
	b, err := json.Marshal(h.info)
	if err != nil {
		return err
	}
	if err = h.file.Truncate(0); err != nil {
		return err
	}
	_, err = h.file.WriteAt(b, 0)
	return err
}

func (s *Store) releaseLocked(device string, h *held) {
	if h.timer != nil {
		h.timer.Stop()
	}
	delete(s.held, device)
	_ = h.file.Truncate(0)
	unlock(h.file)
}

func (s *Store) expireLocked(device string) {
	h := s.held[device]
	if h != nil && !h.active && !h.info.ExpiresAt.IsZero() && !time.Now().Before(h.info.ExpiresAt) {
		s.releaseLocked(device, h)
	}
}

func (s *Store) scheduleLocked(device string, h *held, ttl time.Duration) {
	if h.timer != nil {
		h.timer.Stop()
	}
	h.timer = time.AfterFunc(ttl, func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.held[device] == h {
			s.expireLocked(device)
		}
	})
}

// Claim reserves a device across tool calls. The token must accompany writes,
// even in the same MCP process, so host subagents cannot accidentally share it.
func (s *Store) Claim(device, owner, token string, ttl time.Duration) (Info, string, error) {
	if ttl <= 0 {
		return Info{}, "", fmt.Errorf("lease duration must be positive")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return Info{}, "", fmt.Errorf("device lease store is closed")
	}
	s.expireLocked(device)
	if h := s.held[device]; h != nil {
		if token == "" || token != h.token {
			return h.info, "", fmt.Errorf("device_owned: simulator is reserved by %q; pass its leaseToken or use another device", h.info.Owner)
		}
		previous := h.info
		h.info.ExpiresAt = time.Now().UTC().Add(ttl)
		if err := writeInfo(h); err != nil {
			// The existing timer still expires the original lease. Extending
			// memory without rescheduling would make that timer fire too early
			// and leave an idle lock held indefinitely after a failed renewal.
			h.info = previous
			return Info{}, "", err
		}
		s.scheduleLocked(device, h, ttl)
		return h.info, h.token, nil
	}
	if token != "" {
		return Info{}, "", fmt.Errorf("lease_expired: claim the device again")
	}
	f, err := s.open(device)
	if err != nil {
		return Info{}, "", err
	}
	if err = lock(f); err != nil {
		f.Close()
		return Info{}, "", fmt.Errorf("device_owned: another XCAutokit process owns %s", device)
	}
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		unlock(f)
		return Info{}, "", err
	}
	h := &held{info: Info{Device: device, Owner: owner, Claimed: true, ExpiresAt: time.Now().UTC().Add(ttl)}, token: hex.EncodeToString(b), file: f}
	if err := writeInfo(h); err != nil {
		unlock(f)
		return Info{}, "", err
	}
	s.held[device] = h
	s.scheduleLocked(device, h, ttl)
	return h.info, h.token, nil
}

// Begin serializes a mutation. Unclaimed devices receive a transient lock for
// the whole operation. Expiry never releases a device mid-operation.
func (s *Store) Begin(device, token, owner string) (func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, fmt.Errorf("device lease store is closed")
	}
	s.expireLocked(device)
	h := s.held[device]
	if h != nil {
		if token == "" || token != h.token {
			return nil, fmt.Errorf("device_owned: simulator is reserved by %q; supply leaseToken", h.info.Owner)
		}
		if h.active {
			return nil, fmt.Errorf("device_busy: another operation is running on %s", device)
		}
	} else {
		if token != "" {
			return nil, fmt.Errorf("lease_expired: claim the device again")
		}
		f, err := s.open(device)
		if err != nil {
			return nil, err
		}
		if err = lock(f); err != nil {
			f.Close()
			return nil, fmt.Errorf("device_owned: another XCAutokit process owns %s", device)
		}
		h = &held{info: Info{Device: device, Owner: owner}, file: f}
		s.held[device] = h
	}
	h.active = true
	h.info.Busy = true
	if err := writeInfo(h); err != nil {
		h.active = false
		h.info.Busy = false
		if !h.info.Claimed {
			s.releaseLocked(device, h)
		}
		return nil, err
	}
	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.held[device] != h {
				return
			}
			h.active = false
			h.info.Busy = false
			if !h.info.Claimed || s.closed || !time.Now().Before(h.info.ExpiresAt) {
				s.releaseLocked(device, h)
			} else {
				_ = writeInfo(h)
			}
		})
	}, nil
}

func (s *Store) Release(device, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked(device)
	h := s.held[device]
	if h == nil || token == "" || token != h.token {
		return fmt.Errorf("unknown lease for %s", device)
	}
	if h.active {
		return fmt.Errorf("device_busy: wait for the active operation before releasing")
	}
	s.releaseLocked(device, h)
	return nil
}

func (s *Store) Status(device string) (Info, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked(device)
	if h := s.held[device]; h != nil {
		return h.info, nil
	}
	f, err := s.open(device)
	if err != nil {
		return Info{}, err
	}
	if err = lock(f); err == nil {
		unlock(f)
		return Info{Device: device}, nil
	}
	defer f.Close()
	if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
		return Info{}, err
	}
	var info Info
	if err := json.NewDecoder(f).Decode(&info); err != nil {
		return Info{Device: device, Claimed: true, Owner: "another XCAutokit process"}, nil
	}
	info.Claimed = true
	return info, nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	for device, h := range s.held {
		if !h.active {
			s.releaseLocked(device, h)
		}
	}
	return nil
}
