package sim

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// FrameValue accepts AXe's object frame {"x","y","width","height"} or legacy string "{{x, y}, {w, h}}".
type FrameValue struct {
	Raw    string
	X      float64
	Y      float64
	Width  float64
	Height float64
	Ok     bool
}

func (f *FrameValue) UnmarshalJSON(b []byte) error {
	b = bytesTrim(b)
	if string(b) == "null" || len(b) == 0 {
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		f.Raw = s
		if x, y, w, h, ok := parseFrameString(s); ok {
			f.X, f.Y, f.Width, f.Height, f.Ok = x, y, w, h, true
		}
		return nil
	}
	var o struct {
		X      float64 `json:"x"`
		Y      float64 `json:"y"`
		Width  float64 `json:"width"`
		Height float64 `json:"height"`
	}
	if err := json.Unmarshal(b, &o); err != nil {
		return err
	}
	f.X, f.Y, f.Width, f.Height = o.X, o.Y, o.Width, o.Height
	f.Ok = true
	f.Raw = fmt.Sprintf("{{%g, %g}, {%g, %g}}", o.X, o.Y, o.Width, o.Height)
	return nil
}

func (f FrameValue) MarshalJSON() ([]byte, error) {
	if f.Raw != "" {
		return json.Marshal(f.Raw)
	}
	if f.Ok {
		return json.Marshal(map[string]float64{
			"x": f.X, "y": f.Y, "width": f.Width, "height": f.Height,
		})
	}
	return []byte("null"), nil
}

func (f FrameValue) String() string {
	if f.Raw != "" {
		return f.Raw
	}
	if f.Ok {
		return fmt.Sprintf("{{%g, %g}, {%g, %g}}", f.X, f.Y, f.Width, f.Height)
	}
	return ""
}

func (f FrameValue) Center() (float64, float64, bool) {
	if !f.Ok {
		if x, y, w, h, ok := parseFrameString(f.Raw); ok {
			return x + w/2, y + h/2, true
		}
		return 0, 0, false
	}
	return f.X + f.Width/2, f.Y + f.Height/2, true
}

type Element struct {
	Type             string     `json:"type,omitempty"`
	Label            string     `json:"label,omitempty"`
	Value            string     `json:"value,omitempty"`
	Identifier       string     `json:"identifier,omitempty"`
	Hint             string     `json:"hint,omitempty"`
	Help             string     `json:"help,omitempty"`
	Role             string     `json:"role,omitempty"`
	RoleDescription  string     `json:"role_description,omitempty"`
	Frame            FrameValue `json:"frame,omitempty"`
	Enabled          *bool      `json:"enabled,omitempty"`
	Focused          *bool      `json:"focused,omitempty"`
	Children         []Element  `json:"children,omitempty"`
	AXLabel          string     `json:"AXLabel,omitempty"`
	AXValue          string     `json:"AXValue,omitempty"`
	AXUniqueId       *string    `json:"AXUniqueId,omitempty"`
	AXFrame          string     `json:"AXFrame,omitempty"`
}

type Match struct {
	ID         int     `json:"id"`
	Type       string  `json:"type,omitempty"`
	Label      string  `json:"label,omitempty"`
	Value      string  `json:"value,omitempty"`
	Identifier string  `json:"identifier,omitempty"`
	Hint       string  `json:"hint,omitempty"`
	Frame      string  `json:"frame,omitempty"`
	CenterX    float64 `json:"centerX,omitempty"`
	CenterY    float64 `json:"centerY,omitempty"`
}

var frameRE = regexp.MustCompile(`\{\{([\d.]+),\s*([\d.]+)\},\s*\{([\d.]+),\s*([\d.]+)\}\}`)

func bytesTrim(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

func parseFrameString(frame string) (x, y, w, h float64, ok bool) {
	m := frameRE.FindStringSubmatch(frame)
	if len(m) != 5 {
		return 0, 0, 0, 0, false
	}
	x, _ = strconv.ParseFloat(m[1], 64)
	y, _ = strconv.ParseFloat(m[2], 64)
	w, _ = strconv.ParseFloat(m[3], 64)
	h, _ = strconv.ParseFloat(m[4], 64)
	return x, y, w, h, true
}

func DescribeUI(udid string) (string, error) {
	args := []string{"describe-ui"}
	if udid != "" {
		args = append(args, "--udid", udid)
	}
	out, err := RunAxe(args...)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func ParseDescribeUI(raw string) ([]Element, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("empty UI hierarchy")
	}
	var arr []Element
	if err := json.Unmarshal([]byte(raw), &arr); err == nil {
		return arr, nil
	}
	var one Element
	if err := json.Unmarshal([]byte(raw), &one); err == nil {
		return []Element{one}, nil
	}
	return nil, fmt.Errorf("failed to parse UI hierarchy")
}

func FrameCenter(frame string) (float64, float64, bool) {
	x, y, w, h, ok := parseFrameString(frame)
	if !ok {
		return 0, 0, false
	}
	return x + w/2, y + h/2, true
}

func labelOf(e Element) string {
	if e.Label != "" {
		return e.Label
	}
	return e.AXLabel
}

func valueOf(e Element) string {
	if e.Value != "" {
		return e.Value
	}
	return e.AXValue
}

func idOf(e Element) string {
	if e.Identifier != "" {
		return e.Identifier
	}
	if e.AXUniqueId != nil {
		return *e.AXUniqueId
	}
	return ""
}

func frameOf(e Element) FrameValue {
	if e.Frame.Ok || e.Frame.Raw != "" {
		return e.Frame
	}
	if e.AXFrame != "" {
		fv := FrameValue{Raw: e.AXFrame}
		if x, y, w, h, ok := parseFrameString(e.AXFrame); ok {
			fv.X, fv.Y, fv.Width, fv.Height, fv.Ok = x, y, w, h, true
		}
		return fv
	}
	return FrameValue{}
}

func Find(elements []Element, by, query string) []Match {
	var matches []Match
	var walk func([]Element)
	nextID := 0
	walk = func(els []Element) {
		for _, e := range els {
			ok := false
			switch by {
			case "accessibilityId", "identifier":
				ok = strings.EqualFold(idOf(e), query) || strings.Contains(strings.ToLower(idOf(e)), strings.ToLower(query))
			case "label":
				ok = strings.Contains(strings.ToLower(labelOf(e)), strings.ToLower(query))
			case "text", "value":
				ok = strings.Contains(strings.ToLower(valueOf(e)), strings.ToLower(query)) ||
					strings.Contains(strings.ToLower(labelOf(e)), strings.ToLower(query))
			case "hint":
				ok = strings.Contains(strings.ToLower(e.Hint), strings.ToLower(query)) ||
					strings.Contains(strings.ToLower(e.Help), strings.ToLower(query))
			case "role", "type":
				ok = strings.EqualFold(e.Type, query) ||
					strings.Contains(strings.ToLower(e.Type), strings.ToLower(query)) ||
					strings.Contains(strings.ToLower(e.Role), strings.ToLower(query))
			case "predicate":
				blob := strings.ToLower(strings.Join([]string{e.Type, labelOf(e), valueOf(e), idOf(e), e.Hint, e.Role}, " "))
				ok = strings.Contains(blob, strings.ToLower(query))
			default:
				ok = strings.Contains(strings.ToLower(labelOf(e)), strings.ToLower(query))
			}
			if ok {
				f := frameOf(e)
				cx, cy, _ := f.Center()
				matches = append(matches, Match{
					ID:         nextID,
					Type:       e.Type,
					Label:      labelOf(e),
					Value:      valueOf(e),
					Identifier: idOf(e),
					Hint:       e.Hint,
					Frame:      f.String(),
					CenterX:    cx,
					CenterY:    cy,
				})
				nextID++
			}
			if len(e.Children) > 0 {
				walk(e.Children)
			}
		}
	}
	walk(elements)
	return matches
}

func Search(elements []Element, query string) []Match {
	return Find(elements, "text", query)
}

func Summarize(elements []Element) []map[string]any {
	var out []map[string]any
	var walk func([]Element, int)
	walk = func(els []Element, depth int) {
		for _, e := range els {
			label := labelOf(e)
			if label != "" || idOf(e) != "" {
				item := map[string]any{
					"type":  e.Type,
					"depth": depth,
				}
				if label != "" {
					item["label"] = label
				}
				if id := idOf(e); id != "" {
					item["identifier"] = id
				}
				if f := frameOf(e); f.String() != "" {
					item["frame"] = f.String()
				}
				out = append(out, item)
			}
			if len(e.Children) > 0 {
				walk(e.Children, depth+1)
			}
		}
	}
	walk(elements, 0)
	return out
}

func ElementAtPoint(elements []Element, x, y float64) *Match {
	var best *Match
	var bestArea float64 = -1
	var walk func([]Element)
	walk = func(els []Element) {
		for _, e := range els {
			f := frameOf(e)
			if f.Ok && x >= f.X && x <= f.X+f.Width && y >= f.Y && y <= f.Y+f.Height {
				area := f.Width * f.Height
				if best == nil || (area > 0 && (bestArea < 0 || area < bestArea)) {
					cx, cy, _ := f.Center()
					cp := Match{
						Type:       e.Type,
						Label:      labelOf(e),
						Value:      valueOf(e),
						Identifier: idOf(e),
						Hint:       e.Hint,
						Frame:      f.String(),
						CenterX:    cx,
						CenterY:    cy,
					}
					best = &cp
					bestArea = area
				}
			}
			if len(e.Children) > 0 {
				walk(e.Children)
			}
		}
	}
	walk(elements)
	return best
}

func ScreenSizeFromUI(elements []Element) (w, h float64) {
	w, h = 390, 844
	var walk func([]Element)
	walk = func(els []Element) {
		for _, e := range els {
			f := frameOf(e)
			if f.Ok {
				if f.Width > w {
					w = f.Width
				}
				if f.Height > h {
					h = f.Height
				}
			}
			if len(e.Children) > 0 {
				walk(e.Children)
			}
		}
	}
	walk(elements)
	return w, h
}
