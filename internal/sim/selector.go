package sim

import (
	"fmt"
	"strings"
)

// Selector is a deterministic accessibility selector, never an expression or a
// model-generated query. Role narrows duplicate labels/identifiers to a control.
type Selector struct {
	By    string `json:"by" jsonschema:"accessibilityId, label, text, value, or role"`
	Query string `json:"query" jsonschema:"Nonempty value to match. Accessibility identifiers are case-sensitive; other fields are case-insensitive."`
	Match string `json:"match,omitempty" jsonschema:"auto (default) prefers exact matches and uses substring matches only if none are exact; exact or contains can be explicit"`
	Role  string `json:"role,omitempty" jsonschema:"Optional exact role filter, for example Button or TextField; AX and XCUIElementType prefixes are accepted"`
}

func (s Selector) Validate() error {
	switch strings.ToLower(strings.TrimSpace(s.By)) {
	case "accessibilityid", "identifier", "label", "text", "value", "role", "type":
	default:
		return fmt.Errorf("selector.by must be accessibilityId, label, text, value, or role")
	}
	if strings.TrimSpace(s.Query) == "" {
		return fmt.Errorf("selector.query must not be blank")
	}
	if (strings.EqualFold(strings.TrimSpace(s.By), "role") || strings.EqualFold(strings.TrimSpace(s.By), "type")) && normalizedRole(s.Query) == "" {
		return fmt.Errorf("selector.query must name a role, not only an AX or XCUIElementType prefix")
	}
	if s.Role != "" && normalizedRole(s.Role) == "" {
		return fmt.Errorf("selector.role must name a role")
	}
	switch strings.ToLower(strings.TrimSpace(s.Match)) {
	case "", "auto", "exact", "contains":
	default:
		return fmt.Errorf("selector.match must be auto, exact, or contains")
	}
	return nil
}

type SelectorMatch struct {
	Match
	Role       string `json:"role,omitempty"`
	Enabled    *bool  `json:"enabled,omitempty"`
	Actionable bool   `json:"actionable"`
	Reason     string `json:"reason,omitempty"`
}

func normalizedRole(role string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	role = strings.TrimPrefix(role, "xcuielementtype")
	role = strings.TrimPrefix(role, "ax")
	return strings.NewReplacer(" ", "", "_", "", "-", "").Replace(role)
}

func roleMatches(e Element, query string) bool {
	want := normalizedRole(query)
	return want != "" && (normalizedRole(e.Type) == want || normalizedRole(e.Role) == want)
}

// Select returns all matches at the strongest available match level. It does
// not choose an arbitrary element or discard disabled/invalid matches: doing so
// could silently turn an ambiguous selector into the wrong action.
func Select(elements []Element, selector Selector) ([]SelectorMatch, error) {
	if err := selector.Validate(); err != nil {
		return nil, err
	}
	by := strings.ToLower(strings.TrimSpace(selector.By))
	mode := strings.ToLower(strings.TrimSpace(selector.Match))
	query := selector.Query
	caseSensitive := by == "accessibilityid" || by == "identifier"
	if !caseSensitive {
		query = strings.ToLower(query)
	}
	if by == "role" || by == "type" {
		query = normalizedRole(query)
	}
	exact := []SelectorMatch{}
	partial := []SelectorMatch{}
	nextID := 0
	var walk func([]Element, bool, *FrameValue)
	walk = func(els []Element, ancestorDisabled bool, viewport *FrameValue) {
		for _, e := range els {
			id := nextID
			nextID++
			disabled := ancestorDisabled || (e.Enabled != nil && !*e.Enabled)
			frame := frameOf(e)
			currentViewport := viewport
			if roleMatches(e, "Application") {
				if _, _, ok := frame.Center(); ok {
					currentViewport = &frame
				}
			}
			if selector.Role == "" || roleMatches(e, selector.Role) {
				var values []string
				switch by {
				case "accessibilityid", "identifier":
					values = []string{idOf(e)}
				case "label":
					values = []string{labelOf(e)}
				case "text":
					values = []string{labelOf(e), valueOf(e)}
				case "value":
					values = []string{valueOf(e)}
				case "role", "type":
					values = []string{normalizedRole(e.Type), normalizedRole(e.Role)}
				}
				isExact, isPartial := false, false
				for _, value := range values {
					if !caseSensitive {
						value = strings.ToLower(value)
					}
					isExact = isExact || value == query
					isPartial = isPartial || strings.Contains(value, query)
				}
				if isExact || (isPartial && mode != "exact") {
					cx, cy, valid := frame.Center()
					m := SelectorMatch{Match: Match{
						ID: id, Type: e.Type, Label: labelOf(e), Value: valueOf(e),
						Identifier: idOf(e), Hint: e.Hint, Frame: frame.String(), CenterX: cx, CenterY: cy,
					}, Role: e.Role, Enabled: e.Enabled, Actionable: true}
					switch {
					case disabled:
						m.Actionable, m.Reason = false, "disabled"
						disabledValue := false
						m.Enabled = &disabledValue
					case !valid:
						m.Actionable, m.Reason = false, "invalid_frame"
					case cx < 0 || cy < 0:
						m.Actionable, m.Reason = false, "outside_screen"
					case currentViewport != nil && (cx < currentViewport.X || cy < currentViewport.Y || cx >= currentViewport.X+currentViewport.Width || cy >= currentViewport.Y+currentViewport.Height):
						m.Actionable, m.Reason = false, "outside_screen"
					}
					if isExact {
						exact = append(exact, m)
					} else {
						partial = append(partial, m)
					}
				}
			}
			walk(e.Children, disabled, currentViewport)
		}
	}
	walk(elements, false, nil)
	if mode == "contains" {
		return append(exact, partial...), nil
	}
	if len(exact) > 0 || mode == "exact" {
		return exact, nil
	}
	return partial, nil
}

type ObservationElement struct {
	Type       string `json:"type,omitempty"`
	Role       string `json:"role,omitempty"`
	Label      string `json:"label,omitempty"`
	Value      string `json:"value,omitempty"`
	Identifier string `json:"identifier,omitempty"`
	Frame      string `json:"frame,omitempty"`
	Enabled    *bool  `json:"enabled,omitempty"`
	Depth      int    `json:"depth"`
}

type Observation struct {
	Elements  []ObservationElement `json:"elements"`
	Total     int                  `json:"total"`
	Truncated bool                 `json:"truncated"`
}

// ObserveUI returns enough fresh context for the next decision without dumping
// an unbounded accessibility tree. Truncation is explicit, including long text.
func ObserveUI(elements []Element, limit int) Observation {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	out := Observation{Elements: []ObservationElement{}}
	short := func(value string) string {
		runes := []rune(value)
		if len(runes) > 240 {
			out.Truncated = true
			return string(runes[:240]) + "…"
		}
		return value
	}
	var walk func([]Element, int)
	walk = func(els []Element, depth int) {
		for _, e := range els {
			if labelOf(e) != "" || idOf(e) != "" || valueOf(e) != "" {
				out.Total++
				if len(out.Elements) < limit {
					out.Elements = append(out.Elements, ObservationElement{
						Type: short(e.Type), Role: short(e.Role), Label: short(labelOf(e)), Value: short(valueOf(e)),
						Identifier: short(idOf(e)), Frame: short(frameOf(e).String()), Enabled: e.Enabled, Depth: depth,
					})
				} else {
					out.Truncated = true
				}
			}
			walk(e.Children, depth+1)
		}
	}
	walk(elements, 0)
	return out
}
