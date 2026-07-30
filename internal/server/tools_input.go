package server

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/xcautokit/xcautokit/internal/sim"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type tapIn struct {
	X             float64 `json:"x" jsonschema:"X coordinate"`
	Y             float64 `json:"y" jsonschema:"Y coordinate"`
	SimulatorUuid string  `json:"simulatorUuid,omitempty" jsonschema:"Optional simulator UUID"`
}

type swipeCoordIn struct {
	X1            float64 `json:"x1,omitempty" jsonschema:"Start X (coordinate mode)"`
	Y1            float64 `json:"y1,omitempty" jsonschema:"Start Y (coordinate mode)"`
	X2            float64 `json:"x2,omitempty" jsonschema:"End X (coordinate mode)"`
	Y2            float64 `json:"y2,omitempty" jsonschema:"End Y (coordinate mode)"`
	Direction     string  `json:"direction,omitempty" jsonschema:"Semantic swipe direction: up, down, left, right"`
	Distance      string  `json:"distance,omitempty" jsonschema:"short, medium, or long"`
	DurationMs    int     `json:"durationMs,omitempty" jsonschema:"Swipe duration in milliseconds"`
	SimulatorUuid string  `json:"simulatorUuid,omitempty"`
}

type typeTextIn struct {
	Text          string `json:"text" jsonschema:"Text to type into the focused field"`
	SimulatorUuid string `json:"simulatorUuid,omitempty"`
}

type longPressIn struct {
	X             float64 `json:"x" jsonschema:"X coordinate"`
	Y             float64 `json:"y" jsonschema:"Y coordinate"`
	Duration      float64 `json:"duration,omitempty" jsonschema:"Duration in seconds (default 1.0)"`
	SimulatorUuid string  `json:"simulatorUuid,omitempty"`
}

type buttonIn struct {
	ButtonType    string `json:"buttonType" jsonschema:"Hardware button: home, lock, side-button, siri"`
	SimulatorUuid string `json:"simulatorUuid,omitempty"`
}

type keyPressIn struct {
	KeyCode       int    `json:"keyCode,omitempty" jsonschema:"Keycode to press"`
	Key           string `json:"key,omitempty" jsonschema:"Named key (optional alternative)"`
	SimulatorUuid string `json:"simulatorUuid,omitempty"`
}

type keySequenceIn struct {
	Keys          []int `json:"keys" jsonschema:"Sequence of keycodes to press"`
	DelayMs       int   `json:"delayMs,omitempty" jsonschema:"Delay between key presses in milliseconds"`
	SimulatorUuid string `json:"simulatorUuid,omitempty"`
}

type gestureIn struct {
	Gesture       string         `json:"gesture,omitempty" jsonschema:"tap, swipe, long_press, scroll, drag — or use preset"`
	Preset        string         `json:"preset,omitempty" jsonschema:"AXe preset: scroll-up, scroll-down, swipe-from-left-edge, etc."`
	Direction     string         `json:"direction,omitempty" jsonschema:"up, down, left, right"`
	Distance      string         `json:"distance,omitempty" jsonschema:"short, medium, long"`
	Duration      float64        `json:"duration,omitempty" jsonschema:"Duration in seconds or ms depending on gesture"`
	X             float64        `json:"x,omitempty"`
	Y             float64        `json:"y,omitempty"`
	Target        map[string]any `json:"target,omitempty" jsonschema:"Element selector for target-based gestures"`
	FromTarget    map[string]any `json:"fromTarget,omitempty"`
	ToTarget      map[string]any `json:"toTarget,omitempty"`
	SimulatorUuid string         `json:"simulatorUuid,omitempty"`
}

func (a *App) registerInputTools(srv *mcp.Server) {
	mcp.AddTool(srv, &mcp.Tool{
		Name:        "tap",
		Description: "Tap at x,y coordinates. Blocked while a system alert/permission sheet is present — use ui_dismiss_interrupt first.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in tapIn) (*mcp.CallToolResult, map[string]any, error) {
		udid, err := a.resolveUDID(in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		if blocked := a.guardInputBlocks(udid); blocked != nil {
			return nil, blocked, nil
		}
		if _, err := sim.RunAxe("tap", "-x", fmt.Sprintf("%.0f", in.X), "-y", fmt.Sprintf("%.0f", in.Y), "--udid", udid); err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"success": true, "x": in.X, "y": in.Y, "device": udid}, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "swipe",
		Description: "Perform swipe by coordinates (x1,y1,x2,y2) or semantic direction (up/down/left/right). Blocked while interrupt overlays are present.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in swipeCoordIn) (*mcp.CallToolResult, map[string]any, error) {
		udid, err := a.resolveUDID(in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		if blocked := a.guardInputBlocks(udid); blocked != nil {
			return nil, blocked, nil
		}
		x1, y1, x2, y2 := in.X1, in.Y1, in.X2, in.Y2
		if in.Direction != "" {
			x1, y1, x2, y2, err = a.semanticSwipePoints(udid, in.Direction, in.Distance)
			if err != nil {
				return nil, nil, err
			}
		}
		if x1 == 0 && y1 == 0 && x2 == 0 && y2 == 0 {
			return nil, nil, fmtError("provide direction or x1,y1,x2,y2")
		}
		args := []string{"swipe",
			"--start-x", fmt.Sprintf("%.0f", x1),
			"--start-y", fmt.Sprintf("%.0f", y1),
			"--end-x", fmt.Sprintf("%.0f", x2),
			"--end-y", fmt.Sprintf("%.0f", y2),
			"--udid", udid,
		}
		if in.DurationMs > 0 {
			args = append(args, "--duration", fmt.Sprintf("%.2f", float64(in.DurationMs)/1000.0))
		}
		if _, err := sim.RunAxe(args...); err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{
			"success": true, "x1": x1, "y1": y1, "x2": x2, "y2": y2,
			"direction": in.Direction, "device": udid,
		}, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "type_text",
		Description: "Type text into focused input field. Blocked while interrupt overlays are present.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in typeTextIn) (*mcp.CallToolResult, map[string]any, error) {
		if in.Text == "" {
			return nil, nil, fmtError("text parameter required")
		}
		udid, err := a.resolveUDID(in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		if blocked := a.guardInputBlocks(udid); blocked != nil {
			return nil, blocked, nil
		}
		if _, err := sim.RunAxe("type", in.Text, "--udid", udid); err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"success": true, "text": in.Text, "device": udid}, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "long_press",
		Description: "Perform long press gesture on coordinates. Blocked while interrupt overlays are present.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in longPressIn) (*mcp.CallToolResult, map[string]any, error) {
		udid, err := a.resolveUDID(in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		if blocked := a.guardInputBlocks(udid); blocked != nil {
			return nil, blocked, nil
		}
		dur := in.Duration
		if dur <= 0 {
			dur = 1.0
		}
		if _, err := sim.RunAxe("tap",
			"-x", fmt.Sprintf("%.0f", in.X),
			"-y", fmt.Sprintf("%.0f", in.Y),
			"--pre-delay", fmt.Sprintf("%.2f", dur/2),
			"--post-delay", fmt.Sprintf("%.2f", dur/2),
			"--udid", udid,
		); err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"success": true, "x": in.X, "y": in.Y, "duration": dur, "device": udid}, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "button",
		Description: "Press hardware button (home, lock, side-button, siri)",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in buttonIn) (*mcp.CallToolResult, map[string]any, error) {
		udid, err := a.resolveUDID(in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		buttonMap := map[string]string{
			"home": "home", "lock": "lock", "side-button": "lock", "siri": "home",
		}
		b, ok := buttonMap[in.ButtonType]
		if !ok {
			return nil, nil, fmtError("unknown buttonType: " + in.ButtonType)
		}
		if _, err := sim.RunAxe("button", b, "--udid", udid); err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"success": true, "buttonType": in.ButtonType, "device": udid}, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "key_press",
		Description: "Press individual keys or hardware buttons by keycode. Blocked while interrupt overlays are present.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in keyPressIn) (*mcp.CallToolResult, map[string]any, error) {
		udid, err := a.resolveUDID(in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		if blocked := a.guardInputBlocks(udid); blocked != nil {
			return nil, blocked, nil
		}
		code := in.KeyCode
		if code == 0 && in.Key != "" {
			if n, err := strconv.Atoi(in.Key); err == nil {
				code = n
			}
		}
		if code == 0 {
			return nil, nil, fmtError("keyCode required")
		}
		if _, err := sim.RunAxe("key", strconv.Itoa(code), "--udid", udid); err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"success": true, "keyCode": code, "device": udid}, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "key_sequence",
		Description: "Press sequence of keys with optional delay. Blocked while interrupt overlays are present.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in keySequenceIn) (*mcp.CallToolResult, map[string]any, error) {
		if len(in.Keys) == 0 {
			return nil, nil, fmtError("keys parameter required")
		}
		udid, err := a.resolveUDID(in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		if blocked := a.guardInputBlocks(udid); blocked != nil {
			return nil, blocked, nil
		}
		codes := make([]string, len(in.Keys))
		for i, k := range in.Keys {
			codes[i] = strconv.Itoa(k)
		}
		args := []string{"key-sequence", "--keycodes", strings.Join(codes, ","), "--udid", udid}
		if in.DelayMs > 0 {
			args = append(args, "--delay", fmt.Sprintf("%.3f", float64(in.DelayMs)/1000.0))
		}
		if _, err := sim.RunAxe(args...); err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"success": true, "count": len(in.Keys), "device": udid}, nil
	})

	mcp.AddTool(srv, &mcp.Tool{
		Name:        "gesture",
		Description: "Unified semantic gesture tool (tap, swipe, long_press, scroll, drag) or AXe presets. Blocked while interrupt overlays are present.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, in gestureIn) (*mcp.CallToolResult, map[string]any, error) {
		udid, err := a.resolveUDID(in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		if blocked := a.guardInputBlocks(udid); blocked != nil {
			return nil, blocked, nil
		}
		if in.Preset != "" {
			args := []string{"gesture", in.Preset, "--udid", udid}
			if in.Duration > 0 {
				args = append(args, "--duration", fmt.Sprintf("%.2f", in.Duration))
			}
			if _, err := sim.RunAxe(args...); err != nil {
				return nil, nil, err
			}
			return nil, map[string]any{"success": true, "preset": in.Preset, "device": udid}, nil
		}
		g := in.Gesture
		if g == "" {
			return nil, nil, fmtError("gesture or preset required")
		}
		switch g {
		case "scroll", "swipe":
			dir := in.Direction
			if dir == "" {
				dir = "down"
			}
			x1, y1, x2, y2, err := a.semanticSwipePoints(udid, dir, in.Distance)
			if err != nil {
				return nil, nil, err
			}
			if _, err := sim.RunAxe("swipe",
				"--start-x", fmt.Sprintf("%.0f", x1),
				"--start-y", fmt.Sprintf("%.0f", y1),
				"--end-x", fmt.Sprintf("%.0f", x2),
				"--end-y", fmt.Sprintf("%.0f", y2),
				"--udid", udid,
			); err != nil {
				return nil, nil, err
			}
			return nil, map[string]any{"success": true, "gesture": g, "direction": dir, "device": udid}, nil
		case "tap":
			x, y := in.X, in.Y
			if in.Target != nil {
				x, y, err = a.resolveTargetPoint(udid, in.Target)
				if err != nil {
					return nil, nil, err
				}
			}
			if _, err := sim.RunAxe("tap", "-x", fmt.Sprintf("%.0f", x), "-y", fmt.Sprintf("%.0f", y), "--udid", udid); err != nil {
				return nil, nil, err
			}
			return nil, map[string]any{"success": true, "gesture": "tap", "x": x, "y": y, "device": udid}, nil
		case "long_press":
			x, y := in.X, in.Y
			if in.Target != nil {
				x, y, err = a.resolveTargetPoint(udid, in.Target)
				if err != nil {
					return nil, nil, err
				}
			}
			dur := in.Duration
			if dur <= 0 {
				dur = 1
			}
			if _, err := sim.RunAxe("tap", "-x", fmt.Sprintf("%.0f", x), "-y", fmt.Sprintf("%.0f", y),
				"--pre-delay", fmt.Sprintf("%.2f", dur/2), "--post-delay", fmt.Sprintf("%.2f", dur/2), "--udid", udid); err != nil {
				return nil, nil, err
			}
			return nil, map[string]any{"success": true, "gesture": "long_press", "x": x, "y": y, "device": udid}, nil
		case "drag":
			if in.FromTarget == nil || in.ToTarget == nil {
				return nil, nil, fmtError("drag gesture requires fromTarget and toTarget")
			}
			x1, y1, err := a.resolveTargetPoint(udid, in.FromTarget)
			if err != nil {
				return nil, nil, err
			}
			x2, y2, err := a.resolveTargetPoint(udid, in.ToTarget)
			if err != nil {
				return nil, nil, err
			}
			if _, err := sim.RunAxe("swipe",
				"--start-x", fmt.Sprintf("%.0f", x1), "--start-y", fmt.Sprintf("%.0f", y1),
				"--end-x", fmt.Sprintf("%.0f", x2), "--end-y", fmt.Sprintf("%.0f", y2),
				"--udid", udid,
			); err != nil {
				return nil, nil, err
			}
			return nil, map[string]any{"success": true, "gesture": "drag", "device": udid}, nil
		default:
			return nil, nil, fmtError("unknown gesture: " + g)
		}
	})
}

func (a *App) semanticSwipePoints(udid, direction, distance string) (x1, y1, x2, y2 float64, err error) {
	raw, err := sim.DescribeUI(udid)
	w, h := 390.0, 844.0
	if err == nil {
		if els, perr := sim.ParseDescribeUI(raw); perr == nil {
			w, h = sim.ScreenSizeFromUI(els)
		}
	}
	frac := 0.35
	switch distance {
	case "short":
		frac = 0.2
	case "long":
		frac = 0.55
	}
	cx, cy := w/2, h/2
	switch direction {
	case "up":
		return cx, cy + h*frac/2, cx, cy - h*frac/2, nil
	case "down":
		return cx, cy - h*frac/2, cx, cy + h*frac/2, nil
	case "left":
		return cx + w*frac/2, cy, cx - w*frac/2, cy, nil
	case "right":
		return cx - w*frac/2, cy, cx + w*frac/2, cy, nil
	default:
		return 0, 0, 0, 0, fmtError("invalid direction: " + direction + " (must be up, down, left, or right)")
	}
}

func (a *App) resolveTargetPoint(udid string, target map[string]any) (float64, float64, error) {
	by, _ := target["by"].(string)
	query, _ := target["query"].(string)
	if by == "" {
		if v, ok := target["accessibilityId"].(string); ok && v != "" {
			by, query = "accessibilityId", v
		} else if v, ok := target["label"].(string); ok && v != "" {
			by, query = "label", v
		} else if v, ok := target["text"].(string); ok && v != "" {
			by, query = "text", v
		} else if v, ok := target["role"].(string); ok && v != "" {
			by, query = "role", v
		}
	}
	if by == "" || query == "" {
		return 0, 0, fmtError("target requires by+query or accessibilityId/label/text/role")
	}
	raw, err := sim.DescribeUI(udid)
	if err != nil {
		return 0, 0, err
	}
	els, err := sim.ParseDescribeUI(raw)
	if err != nil {
		return 0, 0, err
	}
	matches := sim.Find(els, by, query)
	if len(matches) == 0 {
		return 0, 0, fmtError(fmt.Sprintf("element not found: by=%s, query=%s", by, query))
	}
	idx := 0
	if v, ok := target["index"].(float64); ok {
		idx = int(v)
	}
	if idx < 0 || idx >= len(matches) {
		return 0, 0, fmtError(fmt.Sprintf("index %d out of range (found %d matches)", idx, len(matches)))
	}
	return matches[idx].CenterX, matches[idx].CenterY, nil
}
