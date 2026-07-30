package sim

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type Device struct {
	Name     string `json:"name"`
	Runtime  string `json:"runtime"`
	State    string `json:"state"`
	UDID     string `json:"udid"`
	IsBooted bool   `json:"isBooted"`
}

type Status struct {
	SimulatorState string  `json:"simulatorState"`
	HasBooted      bool    `json:"hasBooted"`
	BootedDevice   *Device `json:"bootedDevice,omitempty"`
}

func RunSimctl(args ...string) ([]byte, error) {
	cmd := exec.Command("xcrun", append([]string{"simctl"}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("simctl error: %w, output: %s", err, string(out))
	}
	return out, nil
}

func RunAxe(args ...string) ([]byte, error) {
	cmd := exec.Command("axe", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return out, fmt.Errorf("axe error: %w, output: %s", err, string(out))
	}
	return out, nil
}

func ListDevices() ([]Device, error) {
	out, err := RunSimctl("list", "devices", "--json")
	if err != nil {
		return nil, err
	}
	var data map[string]map[string][]Device
	if err := json.Unmarshal(out, &data); err != nil {
		return nil, err
	}
	var all []Device
	for runtime, devices := range data["devices"] {
		for _, d := range devices {
			d.Runtime = runtime
			d.IsBooted = d.State == "Booted"
			all = append(all, d)
		}
	}
	return all, nil
}

func BootedDevice() (*Device, error) {
	devices, err := ListDevices()
	if err != nil {
		return nil, err
	}
	for i := range devices {
		if devices[i].IsBooted {
			return &devices[i], nil
		}
	}
	return nil, nil
}

func ResolveUDID(udidOrName string) (string, error) {
	if udidOrName == "" || udidOrName == "booted" {
		d, err := BootedDevice()
		if err != nil {
			return "", err
		}
		if d == nil {
			return "", fmt.Errorf("no booted device")
		}
		return d.UDID, nil
	}
	devices, err := ListDevices()
	if err != nil {
		return "", err
	}
	for _, d := range devices {
		if d.UDID == udidOrName || strings.EqualFold(d.Name, udidOrName) {
			return d.UDID, nil
		}
	}
	return "", fmt.Errorf("device %q not found", udidOrName)
}

func GetStatus() (Status, error) {
	d, err := BootedDevice()
	if err != nil {
		return Status{SimulatorState: "shutdown", HasBooted: false}, err
	}
	if d == nil {
		return Status{SimulatorState: "shutdown", HasBooted: false}, nil
	}
	return Status{
		SimulatorState: "booted",
		HasBooted:      true,
		BootedDevice:   d,
	}, nil
}

func OpenSimulator() error {
	return exec.Command("open", "-a", "Simulator").Run()
}
