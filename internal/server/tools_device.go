package server

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/xcautokit/xcautokit/internal/sim"
)

type emptyIn struct{}

type statusIn struct {
	SimulatorUuid string `json:"simulatorUuid,omitempty" jsonschema:"Optional simulator UUID (defaults to booted)"`
}

type deviceBootIn struct {
	UDID string `json:"udid" jsonschema:"Device UDID or name to boot"`
}

type deviceShutdownIn struct {
	UDID string `json:"udid,omitempty" jsonschema:"Device UDID to shutdown (defaults to booted)"`
}

func (a *App) registerDeviceTools(srv *mcp.Server) {
	addTool(a, srv, toolMeta("status", "Status", "Get current simulator status (active UUID, boot status, device type, OS version)", annRO()), func(ctx context.Context, req *mcp.CallToolRequest, in statusIn) (*mcp.CallToolResult, map[string]any, error) {
		st, err := sim.GetStatusContext(ctx)
		if err != nil {
			return nil, nil, err
		}
		out := map[string]any{
			"simulatorState":    st.SimulatorState,
			"hasBooted":         st.HasBooted,
			"bootedDevices":     st.BootedDevices,
			"selectionRequired": st.SelectionRequired,
		}
		if in.SimulatorUuid != "" {
			udid, err := a.resolveUDIDContext(ctx, in.SimulatorUuid)
			if err != nil {
				return nil, nil, err
			}
			out["selectedDevice"] = udid
			out["udid"] = udid
			out["selectionRequired"] = false
			devices, err := sim.ListDevicesContext(ctx)
			if err != nil {
				return nil, nil, err
			}
			for _, device := range devices {
				if device.UDID == udid {
					out["name"] = device.Name
					out["runtime"] = device.Runtime
					out["selectedDeviceState"] = device.State
					break
				}
			}
		}
		if st.BootedDevice != nil {
			out["bootedDevice"] = st.BootedDevice
			if in.SimulatorUuid == "" {
				out["udid"] = st.BootedDevice.UDID
				out["name"] = st.BootedDevice.Name
				out["runtime"] = st.BootedDevice.Runtime
			}
		}
		bridge := a.Bridge.Status(ctx)
		out["xcodeBackend"] = bridge
		return nil, out, nil
	})

	addTool(a, srv, toolMeta("device_list", "Device List", "List all available iOS Simulator devices", annRO()), func(ctx context.Context, req *mcp.CallToolRequest, in emptyIn) (*mcp.CallToolResult, map[string]any, error) {
		devices, err := sim.ListDevicesContext(ctx)
		if err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"count": len(devices), "devices": devices}, nil
	})

	addTool(a, srv, toolMeta("device_boot", "Boot Device", "Boot a simulator device by UDID or name", annWrite()), func(ctx context.Context, req *mcp.CallToolRequest, in deviceBootIn) (*mcp.CallToolResult, map[string]any, error) {
		if in.UDID == "" {
			return nil, nil, fmtError("udid parameter required")
		}
		udid, err := a.resolveUDIDContext(ctx, in.UDID)
		if err != nil {
			return nil, nil, err
		}
		devices, err := sim.ListDevicesContext(ctx)
		if err != nil {
			return nil, nil, err
		}
		alreadyBooted := false
		for _, device := range devices {
			if device.UDID == udid && device.IsBooted {
				alreadyBooted = true
			}
		}
		if !alreadyBooted {
			if _, err := sim.RunSimctlContext(ctx, "boot", udid); err != nil {
				return nil, nil, err
			}
		}
		if _, err := sim.RunSimctlContext(ctx, "bootstatus", udid, "-b"); err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"success": true, "udid": udid, "alreadyBooted": alreadyBooted}, nil
	})

	addTool(a, srv, toolMeta("device_shutdown", "Shutdown Device", "Shutdown a running simulator device", annDestructive()), func(ctx context.Context, req *mcp.CallToolRequest, in deviceShutdownIn) (*mcp.CallToolResult, map[string]any, error) {
		udid, err := a.resolveUDIDContext(ctx, in.UDID)
		if err != nil {
			return nil, nil, err
		}
		if _, err := sim.RunSimctlContext(ctx, "shutdown", udid); err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"success": true, "udid": udid}, nil
	})

	addTool(a, srv, toolMeta("open_sim", "Open Simulator", "Bring iOS Simulator app to foreground", annWrite()), func(ctx context.Context, req *mcp.CallToolRequest, in emptyIn) (*mcp.CallToolResult, map[string]any, error) {
		if err := sim.OpenSimulatorContext(ctx); err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"success": true, "message": "Simulator app brought to foreground"}, nil
	})
}

type simpleError string

func (e simpleError) Error() string { return string(e) }

func fmtError(msg string) error { return simpleError(msg) }
