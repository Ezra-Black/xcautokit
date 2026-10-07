package server

import (
	"context"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type deviceClaimIn struct {
	SimulatorUuid string `json:"simulatorUuid,omitempty"`
	Owner         string `json:"owner" jsonschema:"Short host-assigned driver or task label"`
	TTLSeconds    int    `json:"ttlSeconds,omitempty" jsonschema:"Lease duration 10-900 seconds; default 300. Renew with leaseToken."`
	LeaseToken    string `json:"leaseToken,omitempty" jsonschema:"Existing token to renew; omit for a new claim"`
}

type deviceReleaseIn struct {
	SimulatorUuid string `json:"simulatorUuid,omitempty"`
	LeaseToken    string `json:"leaseToken"`
}

func (a *App) registerCoordinationTools(srv *mcp.Server) {
	addTool(a, srv, toolMeta("device_claim", "Claim Simulator", "Reserve one simulator for a host agent across tool calls. Other writers are refused, including other MCP processes. Reads remain available. The server never creates agents.", annWrite()), func(ctx context.Context, req *mcp.CallToolRequest, in deviceClaimIn) (*mcp.CallToolResult, map[string]any, error) {
		if in.Owner == "" || len(in.Owner) > 120 {
			return nil, nil, fmtError("owner is required (max 120 characters)")
		}
		if in.TTLSeconds == 0 {
			in.TTLSeconds = 300
		}
		if in.TTLSeconds < 10 || in.TTLSeconds > 900 {
			return nil, nil, fmtError("ttlSeconds must be 10-900")
		}
		udid, err := a.resolveUDIDContext(ctx, in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		info, token, err := a.Leases.Claim(udid, in.Owner, in.LeaseToken, time.Duration(in.TTLSeconds)*time.Second)
		if err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"success": true, "lease": info, "leaseToken": token}, nil
	})
	addTool(a, srv, toolMeta("device_release", "Release Simulator", "Release your claimed simulator after its active operation finishes.", annWrite()), func(ctx context.Context, req *mcp.CallToolRequest, in deviceReleaseIn) (*mcp.CallToolResult, map[string]any, error) {
		udid, err := a.resolveUDIDContext(ctx, in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		if err := a.Leases.Release(udid, in.LeaseToken); err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"success": true, "device": udid}, nil
	})
	addTool(a, srv, toolMeta("device_lease_status", "Simulator Ownership", "Inspect simulator ownership without revealing the driver's token. Locks coordinate XCAutokit processes on this machine.", annRO()), func(ctx context.Context, req *mcp.CallToolRequest, in simOnlyIn) (*mcp.CallToolResult, map[string]any, error) {
		udid, err := a.resolveUDIDContext(ctx, in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		info, err := a.Leases.Status(udid)
		if err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"lease": info}, nil
	})
}
