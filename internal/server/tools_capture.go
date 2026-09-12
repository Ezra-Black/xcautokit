package server

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/xcautokit/xcautokit/internal/sim"
	"github.com/xcautokit/xcautokit/internal/ticket"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type screenshotIn struct {
	OutputPath    string `json:"outputPath,omitempty" jsonschema:"Optional output path for the screenshot"`
	SimulatorUuid string `json:"simulatorUuid,omitempty"`
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
	Timeout       int    `json:"timeout,omitempty" jsonschema:"Timeout in seconds (default 30)"`
	SimulatorUuid string `json:"simulatorUuid,omitempty"`
}

type logStopIn struct {
	Ticket     string `json:"ticket,omitempty" jsonschema:"Ticket from start_sim_log_cap"`
	CapturePid int    `json:"capturePid,omitempty" jsonschema:"Legacy PID field; prefer ticket"`
}

func (a *App) registerCaptureTools(srv *mcp.Server) {
	mcp.AddTool(srv, toolMeta("screenshot", "Screenshot", "Capture screenshot and return path", annRO()), func(ctx context.Context, req *mcp.CallToolRequest, in screenshotIn) (*mcp.CallToolResult, map[string]any, error) {
		udid, err := a.resolveUDID(in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		path := in.OutputPath
		if path == "" {
			path = filepath.Join(a.Cfg.ScreenshotDir, fmt.Sprintf("shot_%s.png", time.Now().Format("20060102_150405")))
		}
		_ = os.MkdirAll(filepath.Dir(path), 0755)
		if _, err := sim.RunAxe("screenshot", "--output", path, "--udid", udid); err != nil {
			if _, err2 := sim.RunSimctl("io", udid, "screenshot", path); err2 != nil {
				return nil, nil, err
			}
		}
		return nil, map[string]any{"success": true, "path": path, "device": udid}, nil
	})

	mcp.AddTool(srv, toolMeta("record_start", "Start Recording", "Start video recording. Returns a process-local ticket for record_stop (invalid after MCP restart).", annWrite()), func(ctx context.Context, req *mcp.CallToolRequest, in recordStartIn) (*mcp.CallToolResult, map[string]any, error) {
		udid, err := a.resolveUDID(in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		path := in.OutputPath
		if path == "" {
			path = filepath.Join(a.Cfg.RecordingDir, fmt.Sprintf("rec_%s.mp4", time.Now().Format("20060102_150405")))
		}
		_ = os.MkdirAll(filepath.Dir(path), 0755)
		codec := in.Codec
		if codec == "" {
			codec = a.Cfg.RecordingCodec
		}
		cmd := exec.Command("xcrun", "simctl", "io", udid, "recordVideo", "--codec", codec, path)
		if err := cmd.Start(); err != nil {
			return nil, nil, err
		}
		rec := a.Tickets.Issue("record", &ticket.Record{
			Path: path,
			UDID: udid,
			PID:  cmd.Process.Pid,
			Cmd:  cmd,
		})
		return nil, map[string]any{
			"success": true,
			"ticket":  rec.ID,
			"path":    path,
			"pid":     cmd.Process.Pid,
			"device":  udid,
			"message": "Pass ticket to record_stop (process-local; restart invalidates it)",
		}, nil
	})

	mcp.AddTool(srv, toolMeta("record_stop", "Stop Recording", "Stop video recording using the ticket from record_start", annWrite()), func(ctx context.Context, req *mcp.CallToolRequest, in recordStopIn) (*mcp.CallToolResult, map[string]any, error) {
		var rec *ticket.Record
		var err error
		if in.Ticket != "" {
			rec, err = a.Tickets.Take(in.Ticket)
			if err != nil {
				return nil, nil, err
			}
		} else {
			// Fallback: stop newest record ticket for udid
			list := a.Tickets.List("record")
			if len(list) == 0 {
				return nil, nil, fmtError("no active recording ticket; call record_start first")
			}
			rec, err = a.Tickets.Take(list[0].ID)
			if err != nil {
				return nil, nil, err
			}
		}
		if rec.Cmd != nil && rec.Cmd.Process != nil {
			_ = rec.Cmd.Process.Signal(os.Interrupt)
			_ = rec.Cmd.Wait()
		}
		return nil, map[string]any{
			"success": true,
			"ticket":  rec.ID,
			"path":    rec.Path,
			"device":  rec.UDID,
		}, nil
	})

	mcp.AddTool(srv, toolMeta("start_sim_log_cap", "Start Log Capture", "Start capturing simulator logs. Returns a process-local ticket for stop_sim_log_cap.", annWrite()), func(ctx context.Context, req *mcp.CallToolRequest, in logStartIn) (*mcp.CallToolResult, map[string]any, error) {
		udid, err := a.resolveUDID(in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		path := in.OutputPath
		if path == "" {
			path = filepath.Join(a.Cfg.ScreenshotDir, fmt.Sprintf("logs_%s.txt", time.Now().Format("20060102_150405")))
		}
		_ = os.MkdirAll(filepath.Dir(path), 0755)
		f, err := os.Create(path)
		if err != nil {
			return nil, nil, err
		}
		args := []string{"simctl", "spawn", udid, "log", "stream", "--level", "debug"}
		if in.BundleId != "" {
			args = append(args, "--predicate", fmt.Sprintf("subsystem CONTAINS \"%s\" OR processImagePath CONTAINS \"%s\"", in.BundleId, in.BundleId))
		}
		cmd := exec.Command("xcrun", args...)
		cmd.Stdout = f
		cmd.Stderr = f
		if err := cmd.Start(); err != nil {
			f.Close()
			return nil, nil, err
		}
		rec := a.Tickets.Issue("log", &ticket.Record{
			Path: path,
			UDID: udid,
			PID:  cmd.Process.Pid,
			Cmd:  cmd,
			Meta: map[string]any{"bundleId": in.BundleId},
		})
		return nil, map[string]any{
			"success":    true,
			"ticket":     rec.ID,
			"path":       path,
			"capturePid": cmd.Process.Pid,
			"device":     udid,
			"message":    "Pass ticket to stop_sim_log_cap (process-local; restart invalidates it)",
		}, nil
	})

	mcp.AddTool(srv, toolMeta("stop_sim_log_cap", "Stop Log Capture", "Stop simulator log capture using ticket (preferred) or capturePid", annWrite()), func(ctx context.Context, req *mcp.CallToolRequest, in logStopIn) (*mcp.CallToolResult, map[string]any, error) {
		stopped := 0
		if in.Ticket != "" {
			rec, err := a.Tickets.Take(in.Ticket)
			if err != nil {
				return nil, nil, err
			}
			if rec.Cmd != nil && rec.Cmd.Process != nil {
				_ = rec.Cmd.Process.Kill()
				_ = rec.Cmd.Wait()
			}
			stopped = 1
			return nil, map[string]any{
				"success": true, "stopped": stopped, "ticket": rec.ID, "path": rec.Path,
			}, nil
		}
		if in.CapturePid > 0 {
			list := a.Tickets.List("log")
			for _, r := range list {
				if r.PID == in.CapturePid {
					rec, _ := a.Tickets.Take(r.ID)
					if rec != nil && rec.Cmd != nil && rec.Cmd.Process != nil {
						_ = rec.Cmd.Process.Kill()
						_ = rec.Cmd.Wait()
					}
					stopped++
				}
			}
		} else {
			for _, r := range a.Tickets.List("log") {
				rec, _ := a.Tickets.Take(r.ID)
				if rec != nil && rec.Cmd != nil && rec.Cmd.Process != nil {
					_ = rec.Cmd.Process.Kill()
					_ = rec.Cmd.Wait()
				}
				stopped++
			}
		}
		return nil, map[string]any{
			"success": true,
			"stopped": stopped,
			"message": fmt.Sprintf("Stopped %d log capture session(s)", stopped),
		}, nil
	})
}
