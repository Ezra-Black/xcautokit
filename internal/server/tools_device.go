package server

import (
	"context"

	"github.com/xcautokit/xcautokit/internal/sim"
	"github.com/modelcontextprotocol/go-sdk/mcp"
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
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "status",
		Description: "Get current simulator status (active UUID, boot status, device type, OS version)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in statusIn) (*mcp.CallToolResult, map[string]any, error) {
		st, err := sim.GetStatus()
		if err != nil {
			return nil, nil, err
		}
		out := map[string]any{
			"simulatorState": st.SimulatorState,
			"hasBooted":      st.HasBooted,
		}
		if st.BootedDevice != nil {
			out["bootedDevice"] = st.BootedDevice
			out["udid"] = st.BootedDevice.UDID
			out["name"] = st.BootedDevice.Name
			out["runtime"] = st.BootedDevice.Runtime
		}
		bridge := a.Bridge.Status(ctx)
		out["xcodeBackend"] = bridge
		return nil, out, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "device_list",
		Description: "List all available iOS Simulator devices",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in emptyIn) (*mcp.CallToolResult, map[string]any, error) {
		devices, err := sim.ListDevices()
		if err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"count": len(devices), "devices": devices}, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "device_boot",
		Description: "Boot a simulator device by UDID or name",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in deviceBootIn) (*mcp.CallToolResult, map[string]any, error) {
		if in.UDID == "" {
			return nil, nil, fmtError("udid parameter required")
		}
		udid, err := sim.ResolveUDID(in.UDID)
		if err != nil {
			// Try boot by provided value anyway
			udid = in.UDID
		}
		if _, err := sim.RunSimctl("boot", udid); err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"success": true, "udid": udid}, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "device_shutdown",
		Description: "Shutdown a running simulator device",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in deviceShutdownIn) (*mcp.CallToolResult, map[string]any, error) {
		udid := in.UDID
		if udid == "" {
			udid = "booted"
		}
		if _, err := sim.RunSimctl("shutdown", udid); err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"success": true, "udid": udid}, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "open_sim",
		Description: "Bring iOS Simulator app to foreground",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in emptyIn) (*mcp.CallToolResult, map[string]any, error) {
		if err := sim.OpenSimulator(); err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"success": true, "message": "Simulator app brought to foreground"}, nil
	})
}

type simpleError string

func (e simpleError) Error() string { return string(e) }

func fmtError(msg string) error { return simpleError(msg) }
