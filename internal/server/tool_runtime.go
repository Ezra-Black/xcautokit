package server

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func stateDir() string {
	if dir := os.Getenv("XCAUTOKIT_STATE_DIR"); dir != "" {
		return dir
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".xcautokit")
}

func deviceMutation(name string) bool {
	switch name {
	case "tap", "swipe", "gesture", "long_press", "button", "type_text", "key_press", "key_sequence",
		"ui_act", "ui_dismiss_interrupt", "app_install", "app_launch", "app_terminate", "open_url",
		"device_boot", "device_shutdown", "build_run_sim", "test_sim", "launch_app_logs_sim", "workflow_run":
		return true
	}
	return false
}

// addTool centralizes filtering, mutation ownership, and failure semantics.
// MCP schema validation runs before acquisition; handlers validate semantics
// before performing any action.
func addTool[In any](a *App, srv *mcp.Server, meta *mcp.Tool, handler mcp.ToolHandlerFor[In, map[string]any]) {
	for _, info := range allToolCatalog() {
		if info.Name == meta.Name && !workflowsFromEnv().enabled(info.Category) {
			return
		}
	}
	mutatesDevice := deviceMutation(meta.Name)
	if mutatesDevice {
		schema, err := jsonschema.For[In](nil)
		if err != nil {
			panic(err)
		}
		if schema.Properties == nil {
			schema.Properties = map[string]*jsonschema.Schema{}
		}
		schema.Properties["leaseToken"] = &jsonschema.Schema{Type: "string", Description: "Token from device_claim; required while a device is claimed. Omit for an automatic lock covering this operation."}
		meta.InputSchema = schema
	}
	mcp.AddTool(srv, meta, func(ctx context.Context, req *mcp.CallToolRequest, in In) (*mcp.CallToolResult, map[string]any, error) {
		started := time.Now()
		if mutatesDevice {
			var args struct {
				SimulatorUuid string `json:"simulatorUuid"`
				UDID          string `json:"udid"`
				LeaseToken    string `json:"leaseToken"`
				Destination   string `json:"destination"`
				TimeoutMs     int    `json:"timeoutMs"`
			}
			if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
				return nil, nil, err
			}
			if meta.Name == "ui_act" || meta.Name == "workflow_run" {
				limit, maximum := 10000, 60000
				if meta.Name == "workflow_run" {
					limit, maximum = 60000, 120000
				}
				if args.TimeoutMs != 0 {
					limit = args.TimeoutMs
				}
				if limit < 1 || limit > maximum {
					return nil, nil, fmt.Errorf("timeoutMs must be 1-%d", maximum)
				}
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, time.Duration(limit)*time.Millisecond)
				defer cancel()
			}
			selector := firstNonEmpty(args.SimulatorUuid, args.UDID)
			if meta.Name == "build_run_sim" || meta.Name == "test_sim" {
				var err error
				selector, err = deviceFromDestination(args.Destination, selector)
				if err != nil {
					return nil, nil, err
				}
			}
			udid, err := a.resolveUDIDContext(ctx, selector)
			if err != nil {
				return nil, nil, err
			}
			end, err := a.Leases.Begin(udid, args.LeaseToken, meta.Name)
			if err != nil {
				return &mcp.CallToolResult{IsError: true}, map[string]any{"success": false, "reason": "device_owned", "message": err.Error(), "device": udid, "nextTool": "device_lease_status"}, nil
			}
			defer end()
			ctx = context.WithValue(ctx, deviceContextKey{}, udid)
		}
		res, out, err := handler(ctx, req, in)
		if err != nil {
			return res, out, err
		}
		if out != nil {
			out["elapsedMs"] = time.Since(started).Milliseconds()
			if success, exists := out["success"].(bool); exists && !success {
				if res == nil {
					res = &mcp.CallToolResult{}
				}
				res.IsError = true
			}
		}
		return res, out, nil
	})
}

func deviceFromDestination(destination, fallback string) (string, error) {
	if destination == "" {
		return fallback, nil
	}
	var id, name, platform, osVersion string
	for _, part := range strings.Split(destination, ",") {
		kv := strings.SplitN(strings.TrimSpace(part), "=", 2)
		if len(kv) != 2 {
			return "", fmt.Errorf("invalid destination; use platform=iOS Simulator,id=<UDID>")
		}
		switch kv[0] {
		case "id":
			id = kv[1]
		case "name":
			name = kv[1]
		case "platform":
			platform = kv[1]
		case "OS":
			osVersion = kv[1]
		}
	}
	if platform != "iOS Simulator" {
		return "", fmt.Errorf("this operation requires platform=iOS Simulator,id=<UDID>")
	}
	if id != "" {
		return id, nil
	}
	if osVersion != "" {
		return "", fmt.Errorf("use a simulator UDID from device_list for an OS-specific destination")
	}
	if name != "" {
		return name, nil
	}
	return "", fmt.Errorf("destination must specify a simulator id or unique name")
}
