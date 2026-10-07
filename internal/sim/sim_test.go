package sim

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestDeviceListingOrderAndSelection(t *testing.T) {
	data := []byte(`{"devices":{"runtime-B":[{"name":"Phone","udid":"B","state":"Booted"}],"runtime-A":[{"name":"Phone","udid":"A","state":"Booted"},{"name":"Tablet","udid":"C","state":"Shutdown"}]}}`)
	for i := 0; i < 30; i++ {
		devices, err := parseDevices(data)
		if err != nil {
			t.Fatal(err)
		}
		ids := []string{devices[0].UDID, devices[1].UDID, devices[2].UDID}
		if !reflect.DeepEqual(ids, []string{"A", "C", "B"}) {
			t.Fatalf("unstable order: %v", ids)
		}
		for _, selector := range []string{"", "booted", "Phone", "phone"} {
			if id, err := resolveDevice(devices, selector); err == nil || id != "" || !strings.Contains(err.Error(), "ambiguous") || !strings.Contains(err.Error(), "runtime-B, B") {
				t.Fatalf("ambiguous selector %q returned %q, %v", selector, id, err)
			}
		}
		if id, err := resolveDevice(devices, "b"); err != nil || id != "B" {
			t.Fatalf("explicit UDID: %q, %v", id, err)
		}
		if id, err := resolveDevice(devices, "tablet"); err != nil || id != "C" {
			t.Fatalf("unique name: %q, %v", id, err)
		}
		status := statusForDevices(devices)
		if !status.HasBooted || !status.SelectionRequired || status.BootedDevice != nil || len(status.BootedDevices) != 2 {
			t.Fatalf("incorrect multi-device status: %+v", status)
		}
	}
}

func TestUniqueBootedAndMissingDevices(t *testing.T) {
	devices := []Device{{Name: "Phone", UDID: "A", IsBooted: true}}
	if id, err := resolveDevice(devices, "booted"); err != nil || id != "A" {
		t.Fatalf("single booted: %q %v", id, err)
	}
	status := statusForDevices(devices)
	if status.BootedDevice == nil || status.SelectionRequired {
		t.Fatalf("single status: %+v", status)
	}
	if _, err := resolveDevice(nil, "booted"); err == nil || !strings.Contains(err.Error(), "no booted device") {
		t.Fatalf("missing booted: %v", err)
	}
	if _, err := resolveDevice(devices, "missing"); err == nil || !strings.Contains(err.Error(), "device_list") {
		t.Fatalf("missing named: %v", err)
	}
	if _, err := parseDevices([]byte(`{"devices":false}`)); err == nil {
		t.Fatal("invalid simctl JSON accepted")
	}
}

func TestCommandCancellationAndTimeout(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := runCommand(ctx, time.Second, "test", "/bin/sleep", "30"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation not preserved: %v", err)
	}
	start := time.Now()
	if _, err := runCommand(context.Background(), 20*time.Millisecond, "test", "/bin/sleep", "30"); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout not preserved: %v", err)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("command failed to stop promptly")
	}
}

func TestCommandCancellationStopsChildProcessGroup(t *testing.T) {
	start := time.Now()
	_, err := runCommand(context.Background(), 20*time.Millisecond, "test", "/bin/sh", "-c", "sleep 30 & wait")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lost cancellation: %v", err)
	}
	if time.Since(start) > 800*time.Millisecond {
		t.Fatal("descendant held command output open after cancellation")
	}
}
