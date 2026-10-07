package sim

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"syscall"
	"time"
)

type Device struct {
	Name     string `json:"name"`
	Runtime  string `json:"runtime"`
	State    string `json:"state"`
	UDID     string `json:"udid"`
	IsBooted bool   `json:"isBooted"`
}

type Status struct {
	SimulatorState    string   `json:"simulatorState"`
	HasBooted         bool     `json:"hasBooted"`
	BootedDevice      *Device  `json:"bootedDevice,omitempty"`
	BootedDevices     []Device `json:"bootedDevices,omitempty"`
	SelectionRequired bool     `json:"selectionRequired,omitempty"`
}

func RunSimctl(args ...string) ([]byte, error) {
	return RunSimctlContext(context.Background(), args...)
}

// RunSimctlContext propagates request cancellation and bounds a stuck simctl process.
func RunSimctlContext(ctx context.Context, args ...string) ([]byte, error) {
	return runCommand(ctx, 2*time.Minute, "simctl", "xcrun", append([]string{"simctl"}, args...)...)
}

func RunAxe(args ...string) ([]byte, error) {
	return RunAxeContext(context.Background(), args...)
}

// RunAxeContext propagates request cancellation and bounds a stuck accessibility query.
func RunAxeContext(ctx context.Context, args ...string) ([]byte, error) {
	return runCommand(ctx, 30*time.Second, "axe", "axe", args...)
}

func runCommand(ctx context.Context, timeout time.Duration, label, executable string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if err == syscall.ESRCH {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.WaitDelay = time.Second
	out, err := cmd.CombinedOutput()
	if ctx.Err() != nil {
		return out, fmt.Errorf("%s interrupted: %w", label, ctx.Err())
	}
	if err != nil {
		return out, fmt.Errorf("%s error: %w, output: %s", label, err, string(out))
	}
	return out, nil
}

func ListDevices() ([]Device, error) {
	return ListDevicesContext(context.Background())
}

func ListDevicesContext(ctx context.Context) ([]Device, error) {
	out, err := RunSimctlContext(ctx, "list", "devices", "--json")
	if err != nil {
		return nil, err
	}
	return parseDevices(out)
}

func parseDevices(out []byte) ([]Device, error) {
	var data struct {
		Devices map[string][]Device `json:"devices"`
	}
	if err := json.Unmarshal(out, &data); err != nil {
		return nil, err
	}
	all := make([]Device, 0)
	for runtime, devices := range data.Devices {
		for _, d := range devices {
			d.Runtime = runtime
			d.IsBooted = d.State == "Booted"
			all = append(all, d)
		}
	}
	// simctl groups devices in a JSON object. Never expose Go map iteration order.
	sort.Slice(all, func(i, j int) bool {
		if all[i].Runtime != all[j].Runtime {
			return all[i].Runtime < all[j].Runtime
		}
		if all[i].Name != all[j].Name {
			return all[i].Name < all[j].Name
		}
		return all[i].UDID < all[j].UDID
	})
	return all, nil
}

func BootedDevice() (*Device, error) {
	devices, err := ListDevices()
	if err != nil {
		return nil, err
	}
	booted := bootedDevices(devices)
	if len(booted) > 1 {
		return nil, ambiguousDeviceError("booted", booted)
	}
	if len(booted) == 0 {
		return nil, nil
	}
	return &booted[0], nil
}

func bootedDevices(devices []Device) []Device {
	var booted []Device
	for _, d := range devices {
		if d.IsBooted {
			booted = append(booted, d)
		}
	}
	return booted
}

func ResolveUDID(udidOrName string) (string, error) {
	return ResolveUDIDContext(context.Background(), udidOrName)
}

func ResolveUDIDContext(ctx context.Context, udidOrName string) (string, error) {
	devices, err := ListDevicesContext(ctx)
	if err != nil {
		return "", err
	}
	return resolveDevice(devices, udidOrName)
}

func resolveDevice(devices []Device, udidOrName string) (string, error) {
	var matches []Device
	if udidOrName == "" || udidOrName == "booted" {
		matches = bootedDevices(devices)
		if len(matches) == 0 {
			return "", fmt.Errorf("no booted device; boot a simulator with device_boot or pass simulatorUuid explicitly")
		}
	} else {
		// An explicit UDID always takes precedence over a coincidentally matching name.
		for _, d := range devices {
			if strings.EqualFold(d.UDID, udidOrName) {
				return d.UDID, nil
			}
		}
		for _, d := range devices {
			if strings.EqualFold(d.Name, udidOrName) {
				matches = append(matches, d)
			}
		}
	}
	if len(matches) > 1 {
		return "", ambiguousDeviceError(udidOrName, matches)
	}
	if len(matches) == 1 {
		return matches[0].UDID, nil
	}
	return "", fmt.Errorf("device %q not found; use device_list to find an available simulator", udidOrName)
}

func ambiguousDeviceError(selector string, devices []Device) error {
	candidates := make([]string, 0, len(devices))
	for _, d := range devices {
		candidates = append(candidates, fmt.Sprintf("%s (%s, %s)", d.Name, d.Runtime, d.UDID))
	}
	sort.Strings(candidates)
	return fmt.Errorf("device selector %q is ambiguous: %s; pass simulatorUuid explicitly or set session_set_defaults.simulatorUdid", selector, strings.Join(candidates, "; "))
}

func GetStatus() (Status, error) {
	return GetStatusContext(context.Background())
}

func GetStatusContext(ctx context.Context) (Status, error) {
	devices, err := ListDevicesContext(ctx)
	if err != nil {
		return Status{SimulatorState: "unknown", HasBooted: false}, err
	}
	return statusForDevices(devices), nil
}

func statusForDevices(devices []Device) Status {
	booted := bootedDevices(devices)
	if len(booted) == 0 {
		return Status{SimulatorState: "shutdown", HasBooted: false}
	}
	status := Status{
		SimulatorState:    "booted",
		HasBooted:         true,
		BootedDevices:     booted,
		SelectionRequired: len(booted) > 1,
	}
	if len(booted) == 1 {
		status.BootedDevice = &booted[0]
	}
	return status
}

func OpenSimulator() error {
	return OpenSimulatorContext(context.Background())
}

func OpenSimulatorContext(ctx context.Context) error {
	_, err := runCommand(ctx, 30*time.Second, "open Simulator", "open", "-a", "Simulator")
	return err
}
