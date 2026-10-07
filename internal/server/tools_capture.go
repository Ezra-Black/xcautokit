package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/xcautokit/xcautokit/internal/sim"
	"github.com/xcautokit/xcautokit/internal/ticket"
)

type screenshotIn struct {
	OutputPath    string `json:"outputPath,omitempty" jsonschema:"Optional output path for the screenshot"`
	SimulatorUuid string `json:"simulatorUuid,omitempty"`
	Image         *bool  `json:"image,omitempty" jsonschema:"Return native MCP image content (default true); false returns metadata and path only"`
}

type recordStartIn struct {
	OutputPath    string `json:"outputPath,omitempty"`
	Codec         string `json:"codec,omitempty" jsonschema:"Video codec (default hevc)"`
	SimulatorUuid string `json:"simulatorUuid,omitempty"`
}

type recordStopIn struct {
	Ticket        string `json:"ticket,omitempty" jsonschema:"Process-local ticket from record_start"`
	SimulatorUuid string `json:"simulatorUuid,omitempty"`
}

type logStartIn struct {
	OutputPath    string `json:"outputPath,omitempty"`
	BundleId      string `json:"bundleId,omitempty"`
	Timeout       int    `json:"timeout,omitempty" jsonschema:"Automatically stop after this many seconds (default 30; maximum 86400)"`
	SimulatorUuid string `json:"simulatorUuid,omitempty"`
}

type logStopIn struct {
	Ticket     string `json:"ticket,omitempty" jsonschema:"Ticket from start_sim_log_cap"`
	CapturePid int    `json:"capturePid,omitempty" jsonschema:"Legacy PID field; prefer ticket"`
}

func capturePath(requested, dir, prefix, ext string) (string, error) {
	if requested == "" {
		requested = filepath.Join(dir, fmt.Sprintf("%s_%s.%s", prefix, time.Now().Format("20060102_150405.000000000"), ext))
	}
	path, err := filepath.Abs(requested)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "", err
	}
	return path, nil
}

func screenshotResult(path, udid string, includeImage bool) (*mcp.CallToolResult, map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, fmt.Errorf("read captured screenshot: %w", err)
	}
	dimensions, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("invalid captured PNG: %w", err)
	}
	out := map[string]any{"success": true, "path": path, "device": udid, "mimeType": "image/png", "width": dimensions.Width, "height": dimensions.Height, "coordinateSpace": "pixels"}
	if !includeImage {
		return nil, out, nil
	}
	metadata, err := json.Marshal(out)
	if err != nil {
		return nil, nil, err
	}
	return &mcp.CallToolResult{Content: []mcp.Content{
		&mcp.TextContent{Text: string(metadata)},
		&mcp.ImageContent{Data: data, MIMEType: "image/png"},
	}}, out, nil
}

func (a *App) registerCaptureTools(srv *mcp.Server) {
	addTool(a, srv, toolMeta("screenshot", "Screenshot", "Capture a screenshot with inline image content, dimensions, and a saved path. Set image=false for path-only delivery.", annRO()), func(ctx context.Context, req *mcp.CallToolRequest, in screenshotIn) (*mcp.CallToolResult, map[string]any, error) {
		udid, err := a.resolveUDIDContext(ctx, in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		path, err := capturePath(in.OutputPath, a.Cfg.ScreenshotDir, "shot", "png")
		if err != nil {
			return nil, nil, err
		}
		if _, err := sim.RunAxeContext(ctx, "screenshot", "--output", path, "--udid", udid); err != nil {
			if ctx.Err() != nil {
				return nil, nil, ctx.Err()
			}
			if _, fallbackErr := sim.RunSimctlContext(ctx, "io", udid, "screenshot", "--type=png", path); fallbackErr != nil {
				return nil, nil, fmt.Errorf("screenshot failed with axe (%v) and simctl: %w", err, fallbackErr)
			}
		}
		return screenshotResult(path, udid, in.Image == nil || *in.Image)
	})

	addTool(a, srv, toolMeta("record_start", "Start Recording", "Start video recording. Returns a process-local ticket for record_stop (invalid after MCP restart).", annWrite()), func(ctx context.Context, req *mcp.CallToolRequest, in recordStartIn) (*mcp.CallToolResult, map[string]any, error) {
		udid, err := a.resolveUDIDContext(ctx, in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		path, err := capturePath(in.OutputPath, a.Cfg.RecordingDir, "rec", "mp4")
		if err != nil {
			return nil, nil, err
		}
		codec := in.Codec
		if codec == "" {
			codec = a.Cfg.RecordingCodec
		}
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		cmd := exec.Command("xcrun", "simctl", "io", udid, "recordVideo", "--codec", codec, path)
		if err := cmd.Start(); err != nil {
			return nil, nil, err
		}
		rec := a.Tickets.Issue("record", &ticket.Record{Path: path, UDID: udid, PID: cmd.Process.Pid, Cmd: cmd})
		return nil, map[string]any{
			"success": true, "ticket": rec.ID, "path": path, "pid": cmd.Process.Pid, "device": udid,
			"message": "Pass ticket to record_stop (process-local; restart invalidates it)",
		}, nil
	})

	addTool(a, srv, toolMeta("record_stop", "Stop Recording", "Stop video recording using the ticket from record_start. Without a ticket, a single matching recording is required.", annWrite()), func(ctx context.Context, req *mcp.CallToolRequest, in recordStopIn) (*mcp.CallToolResult, map[string]any, error) {
		var rec *ticket.Record
		var err error
		if in.Ticket != "" {
			rec, err = a.Tickets.TakeKind(in.Ticket, "record")
		} else {
			udid := ""
			if in.SimulatorUuid != "" {
				udid, err = a.resolveUDIDContext(ctx, in.SimulatorUuid)
				if err != nil {
					return nil, nil, err
				}
			}
			rec, err = a.Tickets.TakeMatching("record", udid)
		}
		if err != nil {
			return nil, nil, err
		}
		if err := rec.Stop(ctx, 5*time.Second); err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"success": true, "ticket": rec.ID, "path": rec.Path, "device": rec.UDID}, nil
	})

	addTool(a, srv, toolMeta("start_sim_log_cap", "Start Log Capture", "Start capturing simulator logs until stopped or timeout (default 30 seconds). Returns a process-local ticket for stop_sim_log_cap.", annWrite()), func(ctx context.Context, req *mcp.CallToolRequest, in logStartIn) (*mcp.CallToolResult, map[string]any, error) {
		timeout := in.Timeout
		if timeout == 0 {
			timeout = 30
		}
		if timeout < 0 || timeout > 86400 {
			return nil, nil, fmt.Errorf("timeout must be between 1 and 86400 seconds (0 uses the 30-second default)")
		}
		udid, err := a.resolveUDIDContext(ctx, in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		path, err := capturePath(in.OutputPath, a.Cfg.ScreenshotDir, "logs", "txt")
		if err != nil {
			return nil, nil, err
		}
		f, err := os.Create(path)
		if err != nil {
			return nil, nil, err
		}
		defer f.Close() // Child owns its inherited descriptors; do not leak the parent's copies.
		args := []string{"simctl", "spawn", udid, "log", "stream", "--level", "debug"}
		if in.BundleId != "" {
			bundle := strconv.Quote(in.BundleId)
			args = append(args, "--predicate", fmt.Sprintf("subsystem CONTAINS %s OR processImagePath CONTAINS %s", bundle, bundle))
		}
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		cmd := exec.Command("xcrun", args...)
		cmd.Stdout, cmd.Stderr = f, f
		if err := cmd.Start(); err != nil {
			return nil, nil, err
		}
		rec := a.Tickets.Issue("log", &ticket.Record{Path: path, UDID: udid, PID: cmd.Process.Pid, Cmd: cmd, Meta: map[string]any{"bundleId": in.BundleId}})
		rec.StopAfter(time.Duration(timeout) * time.Second)
		return nil, map[string]any{
			"success": true, "ticket": rec.ID, "path": path, "capturePid": cmd.Process.Pid, "device": udid, "timeout": timeout,
			"message": "Pass ticket to stop_sim_log_cap (also valid after timeout; restart invalidates it)",
		}, nil
	})

	addTool(a, srv, toolMeta("stop_sim_log_cap", "Stop Log Capture", "Stop simulator log capture using ticket (preferred) or capturePid. With neither, stop this server's log captures.", annWrite()), func(ctx context.Context, req *mcp.CallToolRequest, in logStopIn) (*mcp.CallToolResult, map[string]any, error) {
		if in.Ticket != "" {
			rec, err := a.Tickets.TakeKind(in.Ticket, "log")
			if err != nil {
				return nil, nil, err
			}
			if err := rec.Stop(ctx, time.Second); err != nil {
				return nil, nil, err
			}
			return nil, map[string]any{"success": true, "stopped": 1, "ticket": rec.ID, "path": rec.Path, "timedOut": rec.TimedOut()}, nil
		}
		stopped := 0
		for _, r := range a.Tickets.List("log") {
			if in.CapturePid > 0 && r.PID != in.CapturePid {
				continue
			}
			rec, err := a.Tickets.TakeKind(r.ID, "log")
			if err != nil {
				continue
			} // Another stop may have consumed it.
			if err := rec.Stop(ctx, time.Second); err != nil {
				return nil, nil, err
			}
			stopped++
		}
		if in.CapturePid > 0 && stopped == 0 {
			return nil, nil, fmt.Errorf("no log capture owned by this server has PID %d; pass its process-local ticket", in.CapturePid)
		}
		return nil, map[string]any{"success": true, "stopped": stopped, "message": fmt.Sprintf("Stopped %d log capture session(s)", stopped)}, nil
	})
}
