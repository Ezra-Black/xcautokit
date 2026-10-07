package devicelease

import (
	"testing"
	"time"
)

func TestOwnershipAcrossStores(t *testing.T) {
	dir := t.TempDir()
	a, b := New(dir), New(dir)
	t.Cleanup(func() { a.Close(); b.Close() })
	_, token, err := a.Claim("device-1", "driver", "", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.Begin("device-1", "", "other"); err == nil {
		t.Fatal("second store acquired claimed device")
	}
	if _, err := a.Begin("device-1", "", "other"); err == nil {
		t.Fatal("same process bypassed ownership")
	}
	end, err := a.Begin("device-1", token, "driver")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Release("device-1", token); err == nil {
		t.Fatal("released busy device")
	}
	if _, err := a.Begin("device-1", token, "driver"); err == nil {
		t.Fatal("concurrent write allowed")
	}
	end()
	end()
	if err := a.Release("device-1", token); err != nil {
		t.Fatal(err)
	}
	end, err = b.Begin("device-1", "", "other")
	if err != nil {
		t.Fatal(err)
	}
	end()
}

func TestExpiryDoesNotUnlockActiveOperation(t *testing.T) {
	dir := t.TempDir()
	a, b := New(dir), New(dir)
	t.Cleanup(func() { a.Close(); b.Close() })
	_, token, err := a.Claim("device-1", "driver", "", 20*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	end, err := a.Begin("device-1", token, "driver")
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(40 * time.Millisecond)
	if _, err := b.Begin("device-1", "", "other"); err == nil {
		t.Fatal("expired during operation")
	}
	end()
	end, err = b.Begin("device-1", "", "other")
	if err != nil {
		t.Fatal(err)
	}
	end()
}

func TestIndependentDevicesAndCrashMetadata(t *testing.T) {
	dir := t.TempDir()
	a, b := New(dir), New(dir)
	t.Cleanup(func() { a.Close(); b.Close() })
	endA, err := a.Begin("device-1", "", "a")
	if err != nil {
		t.Fatal(err)
	}
	endB, err := b.Begin("device-2", "", "b")
	if err != nil {
		t.Fatal(err)
	}
	endA()
	endB()
	// A file from an old process is not proof of ownership; the OS lock is.
	info, err := b.Status("device-1")
	if err != nil || info.Claimed {
		t.Fatalf("stale ownership: %+v %v", info, err)
	}
	if _, _, err := a.Claim("../escape", "a", "", time.Minute); err == nil {
		t.Fatal("path traversal accepted")
	}
}
