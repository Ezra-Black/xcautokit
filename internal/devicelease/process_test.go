package devicelease

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// This test binary doubles as a genuinely separate owner, so flock behavior is
// exercised across operating-system processes, including ungraceful shutdown.
func TestLeaseProcessHelper(t *testing.T) {
	if os.Getenv("XCAUTOKIT_LEASE_TEST_HELPER") != "1" {
		return
	}
	s := New(os.Getenv("XCAUTOKIT_LEASE_TEST_DIR"))
	_, token, err := s.Claim("device-process", "child-agent", "", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	fmt.Println("ready")
	var end func()
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		switch scanner.Text() {
		case "begin-short-lease":
			_, _, err = s.Claim("device-process", "child-agent", token, 60*time.Millisecond)
			if err == nil {
				end, err = s.Begin("device-process", token, "child-agent")
			}
			if err != nil {
				t.Fatal(err)
			}
			fmt.Println("active")
		case "end":
			if end == nil {
				t.Fatal("no active operation")
			}
			end()
			fmt.Println("ended")
		case "expire-idle":
			_, _, err = s.Claim("device-process", "child-agent", token, 30*time.Millisecond)
			if err != nil {
				t.Fatal(err)
			}
			fmt.Println("expiring")
		default:
			t.Fatal("unknown command")
		}
	}
}

type leaseChild struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	lines  <-chan string
	errors *bytes.Buffer
}

func startLeaseChild(t *testing.T, dir string) *leaseChild {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^TestLeaseProcessHelper$")
	cmd.Env = append(os.Environ(), "XCAUTOKIT_LEASE_TEST_HELPER=1", "XCAUTOKIT_LEASE_TEST_DIR="+dir)
	stderr := &bytes.Buffer{}
	cmd.Stderr = stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	child := &leaseChild{cmd: cmd, stdin: stdin, errors: stderr}
	lines := make(chan string, 10)
	child.lines = lines
	go func() {
		defer close(lines)
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			lines <- scanner.Text()
		}
	}()
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = stdin.Close(); _ = cmd.Wait() })
	child.expect(t, "ready")
	return child
}

func (c *leaseChild) expect(t *testing.T, want string) {
	t.Helper()
	select {
	case got := <-c.lines:
		if got != want {
			t.Fatalf("child response %q, want %q", got, want)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("child did not respond %q", want)
	}
}

func (c *leaseChild) send(t *testing.T, command, response string) {
	t.Helper()
	if _, err := fmt.Fprintln(c.stdin, command); err != nil {
		t.Fatal(err)
	}
	c.expect(t, response)
}

func TestCrossProcessOwnershipAndCrashRecovery(t *testing.T) {
	dir := t.TempDir()
	child := startLeaseChild(t, dir)
	other := New(dir)
	t.Cleanup(func() { _ = other.Close() })
	info, err := other.Status("device-process")
	if err != nil || !info.Claimed || info.Owner != "child-agent" {
		t.Fatalf("cross-process status: %+v %v", info, err)
	}
	if _, err := other.Begin("device-process", "", "parent-agent"); err == nil || !strings.Contains(err.Error(), "device_owned") {
		t.Fatalf("another process bypassed ownership: %v", err)
	}
	if err := child.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	// Wait reaps this child and establishes that its OS descriptors are closed.
	_ = child.cmd.Wait()
	info, err = other.Status("device-process")
	if err != nil || info.Claimed {
		t.Fatalf("crash left phantom lease: %+v %v", info, err)
	}
	end, err := other.Begin("device-process", "", "parent-agent")
	if err != nil {
		t.Fatalf("crash did not release OS lock: %v", err)
	}
	end()
}

func TestCrossProcessExpiryKeepsActiveOperationLocked(t *testing.T) {
	dir := t.TempDir()
	child := startLeaseChild(t, dir)
	other := New(dir)
	t.Cleanup(func() { _ = other.Close() })
	child.send(t, "begin-short-lease", "active")
	time.Sleep(100 * time.Millisecond)
	if _, err := other.Begin("device-process", "", "parent-agent"); err == nil {
		t.Fatal("expiry released another process's active mutation")
	}
	child.send(t, "end", "ended")
	end, err := other.Begin("device-process", "", "parent-agent")
	if err != nil {
		t.Fatalf("completed expired operation did not release lease: %v", err)
	}
	end()
}

func TestCrossProcessIdleExpiry(t *testing.T) {
	dir := t.TempDir()
	child := startLeaseChild(t, dir)
	other := New(dir)
	t.Cleanup(func() { _ = other.Close() })
	child.send(t, "expire-idle", "expiring")
	deadline := time.Now().Add(2 * time.Second)
	for {
		end, err := other.Begin("device-process", "", "parent-agent")
		if err == nil {
			end()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("idle lease did not expire: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestFailedRenewalPreservesOriginalExpiryTimer(t *testing.T) {
	dir := t.TempDir()
	owner, contender := New(dir), New(dir)
	t.Cleanup(func() { _ = owner.Close(); _ = contender.Close() })
	initial, token, err := owner.Claim("device-renewal", "owner", "", 500*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	// A read-only descriptor injects a real filesystem write failure without
	// releasing the original locked descriptor or changing permissions globally.
	readOnly, err := os.Open(filepath.Join(dir, "device-renewal.lock"))
	if err != nil {
		t.Fatal(err)
	}
	owner.mu.Lock()
	h := owner.held["device-renewal"]
	lockedFile := h.file
	h.file = readOnly
	owner.mu.Unlock()
	_, _, renewErr := owner.Claim("device-renewal", "owner", token, 10*time.Second)
	owner.mu.Lock()
	after := h.info.ExpiresAt
	h.file = lockedFile
	owner.mu.Unlock()
	_ = readOnly.Close()
	if renewErr == nil {
		t.Fatal("read-only lease metadata unexpectedly renewed")
	}
	if !after.Equal(initial.ExpiresAt) {
		t.Fatalf("failed renewal extended expiry from %v to %v", initial.ExpiresAt, after)
	}
	// No further calls to the owner: its original timer must release the lock.
	deadline := time.Now().Add(time.Second)
	for {
		end, err := contender.Begin("device-renewal", "", "contender")
		if err == nil {
			end()
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("failed renewal stranded idle lock: %v", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
