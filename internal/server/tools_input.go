package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/xcautokit/xcautokit/internal/sim"
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
	Text          string `json:"text" jsonschema:"Text to type into the focused field. AXe supports printable ASCII plus tab and newline; unsupported characters are rejected before input."`
	SimulatorUuid string `json:"simulatorUuid,omitempty"`
}

type longPressIn struct {
	X             float64 `json:"x" jsonschema:"X coordinate"`
	Y             float64 `json:"y" jsonschema:"Y coordinate"`
	Duration      float64 `json:"duration,omitempty" jsonschema:"Hold duration in seconds, greater than 0 and at most 10 (default 1.0)"`
	SimulatorUuid string  `json:"simulatorUuid,omitempty"`
}

type buttonIn struct {
	ButtonType    string `json:"buttonType" jsonschema:"Hardware button: home, lock, side-button, siri, apple-pay"`
	SimulatorUuid string `json:"simulatorUuid,omitempty"`
}

type keyPressIn struct {
	KeyCode       int    `json:"keyCode,omitempty" jsonschema:"Keycode to press"`
	Key           string `json:"key,omitempty" jsonschema:"Named key (optional alternative)"`
	SimulatorUuid string `json:"simulatorUuid,omitempty"`
}

type keySequenceIn struct {
	Keys          []int  `json:"keys" jsonschema:"Sequence of keycodes to press"`
	DelayMs       int    `json:"delayMs,omitempty" jsonschema:"Delay between key presses in milliseconds"`
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
	Target        map[string]any `json:"target,omitempty" jsonschema:"Element selector: by+query or accessibilityId/label/text/role shorthand; optional match, role filter, and integer index. Ambiguous targets require an explicit index."`
	FromTarget    map[string]any `json:"fromTarget,omitempty"`
	ToTarget      map[string]any `json:"toTarget,omitempty"`
	SimulatorUuid string         `json:"simulatorUuid,omitempty"`
}

func (a *App) registerInputTools(srv *mcp.Server) {
	addTool(a, srv, toolMeta("tap", "Tap", "Tap at x,y coordinates. Blocked while a system alert/permission sheet is present — use ui_dismiss_interrupt first.", annWrite()), func(ctx context.Context, req *mcp.CallToolRequest, in tapIn) (*mcp.CallToolResult, map[string]any, error) {
		udid, err := a.resolveUDIDContext(ctx, in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		if blocked := a.guardInputBlocksContext(ctx, udid); blocked != nil {
			return nil, blocked, nil
		}
		if _, err := sim.RunAxeContext(ctx, "tap", "-x", fmt.Sprintf("%.0f", in.X), "-y", fmt.Sprintf("%.0f", in.Y), "--udid", udid); err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"success": true, "x": in.X, "y": in.Y, "device": udid}, nil
	})

	addTool(a, srv, toolMeta("swipe", "Swipe", "Perform swipe by coordinates (x1,y1,x2,y2) or semantic direction (up/down/left/right). Blocked while interrupt overlays are present.", annWrite()), func(ctx context.Context, req *mcp.CallToolRequest, in swipeCoordIn) (*mcp.CallToolResult, map[string]any, error) {
		udid, err := a.resolveUDIDContext(ctx, in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		if blocked := a.guardInputBlocksContext(ctx, udid); blocked != nil {
			return nil, blocked, nil
		}
		x1, y1, x2, y2 := in.X1, in.Y1, in.X2, in.Y2
		if in.Direction != "" {
			x1, y1, x2, y2, err = a.semanticSwipePoints(ctx, udid, in.Direction, in.Distance)
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
		if _, err := sim.RunAxeContext(ctx, args...); err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{
			"success": true, "x1": x1, "y1": y1, "x2": x2, "y2": y2,
			"direction": in.Direction, "device": udid,
		}, nil
	})

	addTool(a, srv, toolMeta("type_text", "Type Text", "Type text into focused input field. Blocked while interrupt overlays are present.", annWrite()), func(ctx context.Context, req *mcp.CallToolRequest, in typeTextIn) (*mcp.CallToolResult, map[string]any, error) {
		if err := validateTypeText(in.Text); err != nil {
			return nil, nil, err
		}
		udid, err := a.resolveUDIDContext(ctx, in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		if blocked := a.guardInputBlocksContext(ctx, udid); blocked != nil {
			return nil, blocked, nil
		}
		if _, err := sim.RunAxeContext(ctx, typeTextArgs(udid, in.Text)...); err != nil {
			return nil, map[string]any{"success": false, "performed": nil, "reason": "action_outcome_unknown", "device": udid, "message": "Text input did not complete reliably. Inspect the current field before retrying."}, nil
		}
		return nil, map[string]any{"success": true, "characters": utf8.RuneCountInString(in.Text), "device": udid}, nil
	})

	addTool(a, srv, toolMeta("long_press", "Long Press", "Perform long press gesture on coordinates. Blocked while interrupt overlays are present.", annWrite()), func(ctx context.Context, req *mcp.CallToolRequest, in longPressIn) (*mcp.CallToolResult, map[string]any, error) {
		udid, err := a.resolveUDIDContext(ctx, in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		if blocked := a.guardInputBlocksContext(ctx, udid); blocked != nil {
			return nil, blocked, nil
		}
		dur, err := performLongPress(ctx, udid, in.X, in.Y, in.Duration)
		if err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"success": true, "x": in.X, "y": in.Y, "duration": dur, "device": udid}, nil
	})

	addTool(a, srv, toolMeta("button", "Hardware Button", "Press hardware button (home, lock, side-button, siri, apple-pay)", annWrite()), func(ctx context.Context, req *mcp.CallToolRequest, in buttonIn) (*mcp.CallToolResult, map[string]any, error) {
		udid, err := a.resolveUDIDContext(ctx, in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		b, err := hardwareButtonName(in.ButtonType)
		if err != nil {
			return nil, nil, err
		}
		if _, err := sim.RunAxeContext(ctx, "button", b, "--udid", udid); err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"success": true, "buttonType": in.ButtonType, "device": udid}, nil
	})

	addTool(a, srv, toolMeta("key_press", "Key Press", "Press individual keys or hardware buttons by keycode. Blocked while interrupt overlays are present.", annWrite()), func(ctx context.Context, req *mcp.CallToolRequest, in keyPressIn) (*mcp.CallToolResult, map[string]any, error) {
		udid, err := a.resolveUDIDContext(ctx, in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		if blocked := a.guardInputBlocksContext(ctx, udid); blocked != nil {
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
		if _, err := sim.RunAxeContext(ctx, "key", strconv.Itoa(code), "--udid", udid); err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"success": true, "keyCode": code, "device": udid}, nil
	})

	addTool(a, srv, toolMeta("key_sequence", "Key Sequence", "Press sequence of keys with optional delay. Blocked while interrupt overlays are present.", annWrite()), func(ctx context.Context, req *mcp.CallToolRequest, in keySequenceIn) (*mcp.CallToolResult, map[string]any, error) {
		if len(in.Keys) == 0 {
			return nil, nil, fmtError("keys parameter required")
		}
		udid, err := a.resolveUDIDContext(ctx, in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		if blocked := a.guardInputBlocksContext(ctx, udid); blocked != nil {
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
		if _, err := sim.RunAxeContext(ctx, args...); err != nil {
			return nil, nil, err
		}
		return nil, map[string]any{"success": true, "count": len(in.Keys), "device": udid}, nil
	})

	addTool(a, srv, toolMeta("gesture", "Gesture", "Unified semantic gesture tool (tap, swipe, long_press, scroll, drag) or AXe presets. Blocked while interrupt overlays are present.", annWrite()), func(ctx context.Context, req *mcp.CallToolRequest, in gestureIn) (*mcp.CallToolResult, map[string]any, error) {
		udid, err := a.resolveUDIDContext(ctx, in.SimulatorUuid)
		if err != nil {
			return nil, nil, err
		}
		if blocked := a.guardInputBlocksContext(ctx, udid); blocked != nil {
			return nil, blocked, nil
		}
		if in.Preset != "" {
			args := []string{"gesture", in.Preset, "--udid", udid}
			if in.Duration > 0 {
				args = append(args, "--duration", fmt.Sprintf("%.2f", in.Duration))
			}
			if _, err := sim.RunAxeContext(ctx, args...); err != nil {
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
			x1, y1, x2, y2, err := a.semanticSwipePoints(ctx, udid, dir, in.Distance)
			if err != nil {
				return nil, nil, err
			}
			if _, err := sim.RunAxeContext(ctx, "swipe",
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
				x, y, err = a.resolveTargetPoint(ctx, udid, in.Target)
				if err != nil {
					return nil, nil, err
				}
			}
			if _, err := sim.RunAxeContext(ctx, "tap", "-x", fmt.Sprintf("%.0f", x), "-y", fmt.Sprintf("%.0f", y), "--udid", udid); err != nil {
				return nil, nil, err
			}
			return nil, map[string]any{"success": true, "gesture": "tap", "x": x, "y": y, "device": udid}, nil
		case "long_press":
			x, y := in.X, in.Y
			if in.Target != nil {
				x, y, err = a.resolveTargetPoint(ctx, udid, in.Target)
				if err != nil {
					return nil, nil, err
				}
			}
			dur, err := performLongPress(ctx, udid, x, y, in.Duration)
			if err != nil {
				return nil, nil, err
			}
			return nil, map[string]any{"success": true, "gesture": "long_press", "x": x, "y": y, "duration": dur, "device": udid}, nil
		case "drag":
			if in.FromTarget == nil || in.ToTarget == nil {
				return nil, nil, fmtError("drag gesture requires fromTarget and toTarget")
			}
			points, err := a.resolveTargetPoints(ctx, udid, in.FromTarget, in.ToTarget)
			if err != nil {
				return nil, nil, err
			}
			x1, y1, x2, y2 := points[0].x, points[0].y, points[1].x, points[1].y
			if _, err := sim.RunAxeContext(ctx, "swipe",
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

func (a *App) semanticSwipePoints(ctx context.Context, udid, direction, distance string) (x1, y1, x2, y2 float64, err error) {
	raw, err := sim.DescribeUIContext(ctx, udid)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	els, err := sim.ParseDescribeUI(raw)
	if err != nil {
		return 0, 0, 0, 0, err
	}
	if blocked := inputBlockFromSnapshot(udid, els, nil); blocked != nil {
		return 0, 0, 0, 0, fmt.Errorf("swipe blocked: %v", blocked["message"])
	}
	w, h := sim.ScreenSizeFromUI(els)
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

func (a *App) resolveTargetPoint(ctx context.Context, udid string, target map[string]any) (float64, float64, error) {
	points, err := a.resolveTargetPoints(ctx, udid, target)
	if err != nil {
		return 0, 0, err
	}
	return points[0].x, points[0].y, nil
}

type targetPoint struct{ x, y float64 }

// Resolve both ends of a drag against one guarded snapshot, avoiding stale
// coordinates from two independently observed screens.
func (a *App) resolveTargetPoints(ctx context.Context, udid string, targets ...map[string]any) ([]targetPoint, error) {
	raw, err := sim.DescribeUIContext(ctx, udid)
	if err != nil {
		return nil, err
	}
	els, err := sim.ParseDescribeUI(raw)
	if err != nil {
		return nil, err
	}
	if blocked := inputBlockFromSnapshot(udid, els, nil); blocked != nil {
		return nil, fmt.Errorf("target input blocked: %v", blocked["message"])
	}
	points := make([]targetPoint, len(targets))
	for i, target := range targets {
		x, y, err := resolveTargetPointFromElements(els, target)
		if err != nil {
			return nil, err
		}
		points[i] = targetPoint{x, y}
	}
	return points, nil
}

func resolveTargetPointFromElements(els []sim.Element, target map[string]any) (float64, float64, error) {
	selector, err := selectorFromTarget(target)
	if err != nil {
		return 0, 0, err
	}
	matches, err := sim.Select(els, selector)
	if err != nil {
		return 0, 0, err
	}
	if len(matches) == 0 {
		return 0, 0, fmt.Errorf("element not found: by=%s, query=%s", selector.By, selector.Query)
	}
	idx := 0
	if value, supplied := target["index"]; supplied {
		idx, err = explicitTargetIndex(value, len(matches))
		if err != nil {
			return 0, 0, err
		}
	} else if len(matches) > 1 {
		return 0, 0, fmt.Errorf("ambiguous target: %d matches; use a unique accessibilityId, a role filter, or an explicit integer index", len(matches))
	}
	if !matches[idx].Actionable {
		return 0, 0, fmt.Errorf("target is not actionable: %s", matches[idx].Reason)
	}
	return matches[idx].CenterX, matches[idx].CenterY, nil
}

func selectorFromTarget(target map[string]any) (sim.Selector, error) {
	values := map[string]string{}
	for _, key := range []string{"by", "query", "accessibilityId", "label", "text", "role", "match"} {
		if raw, present := target[key]; present {
			value, ok := raw.(string)
			if !ok {
				return sim.Selector{}, fmt.Errorf("target.%s must be a string", key)
			}
			values[key] = value
		}
	}
	selector := sim.Selector{By: values["by"], Query: values["query"], Match: values["match"], Role: values["role"]}
	if selector.By == "" {
		for _, key := range []string{"accessibilityId", "label", "text"} {
			if values[key] != "" {
				if selector.By != "" {
					return sim.Selector{}, fmt.Errorf("target must specify one primary selector; role may be used as a filter")
				}
				selector.By, selector.Query = key, values[key]
			}
		}
		if selector.By == "" && values["role"] != "" {
			selector.By, selector.Query, selector.Role = "role", values["role"], ""
		}
	}
	if err := selector.Validate(); err != nil {
		return sim.Selector{}, err
	}
	return selector, nil
}

func explicitTargetIndex(value any, count int) (int, error) {
	var number float64
	switch v := value.(type) {
	case float64:
		number = v
	case int:
		number = float64(v)
	case json.Number:
		parsed, err := strconv.ParseInt(string(v), 10, 64)
		if err != nil {
			return 0, fmt.Errorf("target.index must be an integer")
		}
		number = float64(parsed)
	default:
		return 0, fmt.Errorf("target.index must be an integer")
	}
	if math.IsNaN(number) || math.IsInf(number, 0) || math.Trunc(number) != number || number < 0 || number >= float64(count) {
		return 0, fmt.Errorf("target.index must be an integer from 0 to %d", count-1)
	}
	return int(number), nil
}

func hardwareButtonName(name string) (string, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "home", "lock", "side-button", "siri", "apple-pay":
		return name, nil
	default:
		return "", fmt.Errorf("unknown buttonType: %s", name)
	}
}

// The separator keeps user text such as --help or --file literal, rather than
// allowing the input to become an AXe command-line option.
func typeTextArgs(udid, text string) []string {
	return []string{"type", "--udid", udid, "--", text}
}

func validateTypeText(text string) error {
	if text == "" {
		return fmt.Errorf("text parameter required")
	}
	position := 0
	for _, character := range text {
		position++
		if character == '\n' || character == '\r' || character == '\t' {
			continue
		}
		if character < ' ' || character > '~' {
			return fmt.Errorf("AXe text input supports printable ASCII plus tab and newline; unsupported character at position %d (no text was entered)", position)
		}
	}
	return nil
}

func longPressArgs(udid string, x, y, duration float64) ([]string, float64, error) {
	if duration == 0 {
		duration = 1
	}
	if math.IsNaN(duration) || math.IsInf(duration, 0) || duration <= 0 || duration > 10 {
		return nil, 0, fmt.Errorf("long press duration must be greater than 0 and at most 10 seconds")
	}
	if math.IsNaN(x) || math.IsNaN(y) || math.IsInf(x, 0) || math.IsInf(y, 0) || x < 0 || y < 0 {
		return nil, 0, fmt.Errorf("long press coordinates must be finite and nonnegative")
	}
	return []string{"touch", "-x", strconv.FormatFloat(x, 'f', -1, 64), "-y", strconv.FormatFloat(y, 'f', -1, 64), "--down", "--up", "--delay", strconv.FormatFloat(duration, 'f', -1, 64), "--udid", udid}, duration, nil
}

func performLongPress(ctx context.Context, udid string, x, y, duration float64) (float64, error) {
	args, duration, err := longPressArgs(udid, x, y, duration)
	if err != nil {
		return 0, err
	}
	if err := ctx.Err(); err != nil {
		return duration, err
	}
	if _, err := sim.RunAxeContext(ctx, args...); err != nil {
		// A cancelled touch process may have sent down but not up. Release the
		// contact while the caller still owns the device, under a separate bound.
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_, cleanupErr := sim.RunAxeContext(cleanupCtx, "touch", "-x", args[2], "-y", args[4], "--up", "--udid", udid)
		if cleanupErr != nil {
			return duration, fmt.Errorf("long press failed: %w; touch release could not be confirmed: %v", err, cleanupErr)
		}
		return duration, err
	}
	return duration, nil
}
