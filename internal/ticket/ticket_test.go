package ticket

import (
	"bufio"
	"context"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"
)

func ownedSleep(t *testing.T, s *Store, kind, udid string) *Record {
	t.Helper()
	cmd := exec.Command("/bin/sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	r := s.Issue(kind, &Record{Cmd: cmd, PID: cmd.Process.Pid, UDID: udid})
	t.Cleanup(func() { _ = r.Stop(context.Background(), 20*time.Millisecond) })
	return r
}

func TestTypedTicketValidationPreservesHandle(t *testing.T) {
	s := New()
	log := s.Issue("log", &Record{})
	if _, err := s.TakeKind(log.ID, "record"); err == nil {
		t.Fatal("wrong capture kind accepted")
	}
	if _, err := s.Get(log.ID); err != nil {
		t.Fatalf("wrong kind consumed handle: %v", err)
	}
	if _, err := s.TakeKind(log.ID, "log"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Take(log.ID); err == nil {
		t.Fatal("ticket consumed twice")
	}
}

func TestLegacyMatchingNeverGuessesDevice(t *testing.T) {
	s := New()
	a := s.Issue("record", &Record{UDID: "A"})
	b := s.Issue("record", &Record{UDID: "B"})
	if _, err := s.TakeMatching("record", ""); err == nil || !strings.Contains(err.Error(), "multiple") {
		t.Fatalf("ambiguous selection: %v", err)
	}
	if len(s.List("record")) != 2 {
		t.Fatal("ambiguity consumed a ticket")
	}
	if got, err := s.TakeMatching("record", "A"); err != nil || got.ID != a.ID {
		t.Fatalf("device matching: %+v %v", got, err)
	}
	if got, err := s.TakeMatching("record", ""); err != nil || got.ID != b.ID {
		t.Fatalf("single fallback: %+v %v", got, err)
	}
}

func TestTicketListIsNewestFirst(t *testing.T) {
	s := New()
	first := s.Issue("log", &Record{})
	second := s.Issue("log", &Record{})
	first.CreatedAt = time.Unix(1, 0)
	second.CreatedAt = time.Unix(2, 0)
	for i := 0; i < 20; i++ {
		if got := s.List("log"); got[0] != second || got[1] != first {
			t.Fatal("unstable ticket ordering")
		}
	}
}

func TestTimeoutReapsProcessAndRetainsTicket(t *testing.T) {
	s := New()
	r := ownedSleep(t, s, "log", "A")
	r.StopAfter(20 * time.Millisecond)
	select {
	case <-r.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("timeout did not stop child")
	}
	if !r.TimedOut() || r.Cmd.ProcessState == nil {
		t.Fatal("timeout did not mark/reap child")
	}
	if _, err := s.TakeKind(r.ID, "log"); err != nil {
		t.Fatalf("timeout lost artifact handle: %v", err)
	}
	if err := r.Stop(context.Background(), time.Second); err != nil {
		t.Fatalf("stopping timed out ticket: %v", err)
	}
}

func TestShutdownAndConcurrentStop(t *testing.T) {
	s := New()
	a := ownedSleep(t, s, "record", "A")
	b := ownedSleep(t, s, "log", "B")
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := a.Stop(context.Background(), 20*time.Millisecond); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	for _, r := range []*Record{a, b} {
		select {
		case <-r.Done():
		default:
			t.Fatal("shutdown left child running")
		}
	}
	if len(s.List("")) != 0 {
		t.Fatal("shutdown left tickets registered")
	}
	late := ownedSleep(t, s, "log", "C")
	select {
	case <-late.Done():
	default:
		t.Fatal("shutdown raced with issue and leaked child")
	}
}

func TestUnexpectedChildFailureIsReported(t *testing.T) {
	s := New()
	cmd := exec.Command("/bin/sh", "-c", "exit 7")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	r := s.Issue("record", &Record{Cmd: cmd})
	<-r.Done()
	if err := r.Stop(context.Background(), time.Second); err == nil || !strings.Contains(err.Error(), "exited unexpectedly") {
		t.Fatalf("unexpected child failure hidden: %v", err)
	}
}

func TestStopEscalatesAndReapsUnresponsiveChild(t *testing.T) {
	s := New()
	cmd := exec.Command("/bin/sh", "-c", "trap '' INT; echo ready; exec /bin/sleep 30")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	ready := bufio.NewScanner(stdout)
	if !ready.Scan() || ready.Text() != "ready" {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		t.Fatal("child did not install interrupt handler")
	}
	r := s.Issue("record", &Record{Cmd: cmd, PID: cmd.Process.Pid})
	t.Cleanup(func() { _ = r.Stop(context.Background(), time.Millisecond) })
	start := time.Now()
	if err := r.Stop(context.Background(), 20*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("forced stop exceeded bound")
	}
	if r.Cmd.ProcessState == nil {
		t.Fatal("forced stop did not reap child")
	}
}
