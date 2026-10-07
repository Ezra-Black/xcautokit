package sim

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

// Mirrors AXe's native describe-ui field names, including nullable AX values
// and object geometry, rather than assuming generic WebDriver attributes.
const axeSelectorFixture = `[{"type":"Application","role":"AXApplication","AXLabel":"Example","AXValue":null,"AXUniqueId":null,"enabled":true,"frame":{"x":0,"y":0,"width":390,"height":844},"children":[
 {"type":"Button","role":"AXButton","AXLabel":"Save Changes","AXUniqueId":"saveChanges","enabled":true,"frame":{"x":20,"y":100,"width":160,"height":44}},
 {"type":"Button","role":"AXButton","AXLabel":"Save","AXUniqueId":"save","enabled":true,"frame":{"x":20,"y":150,"width":160,"height":44}},
 {"type":"Group","role":"AXGroup","AXLabel":"Send","AXUniqueId":"send","frame":{"x":20,"y":200,"width":180,"height":44},"children":[
  {"type":"Button","role":"AXButton","AXLabel":"Send","AXUniqueId":"send","enabled":true,"AXFrame":"{{30, 200}, {160, 44}}"}
 ]},
 {"type":"TextField","AXLabel":"Search","AXValue":"weather","AXUniqueId":"query","enabled":true,"frame":{"x":20,"y":260,"width":200,"height":44}}
]}]`

func TestSelectExactPreferredAndExplicitContains(t *testing.T) {
	elements := parseFixture(t, axeSelectorFixture)
	matches, err := Select(elements, Selector{By: "label", Query: "save"})
	if err != nil || len(matches) != 1 || matches[0].Identifier != "save" {
		t.Fatalf("exact match must win over earlier partial: %#v, %v", matches, err)
	}
	if !matches[0].Actionable || matches[0].CenterX != 100 || matches[0].CenterY != 172 {
		t.Fatalf("invalid native AXe geometry: %#v", matches[0])
	}
	matches, err = Select(elements, Selector{By: "label", Query: "save", Match: "contains"})
	if err != nil || len(matches) != 2 {
		t.Fatalf("explicit contains should retain both: %#v, %v", matches, err)
	}
	matches, _ = Select(elements, Selector{By: "label", Query: "Changes", Match: "exact"})
	if len(matches) != 0 {
		t.Fatal("exact selector must not return substrings")
	}
}

func TestSelectRetainsAmbiguityAndRoleDisambiguates(t *testing.T) {
	elements := parseFixture(t, axeSelectorFixture)
	matches, _ := Select(elements, Selector{By: "accessibilityId", Query: "send"})
	if len(matches) != 2 {
		t.Fatalf("duplicate identifiers must remain ambiguous, got %#v", matches)
	}
	for _, role := range []string{"Button", "AXButton", "XCUIElementTypeButton"} {
		matches, err := Select(elements, Selector{By: "accessibilityId", Query: "send", Role: role})
		if err != nil || len(matches) != 1 || matches[0].Type != "Button" {
			t.Fatalf("role %s did not select control: %#v %v", role, matches, err)
		}
	}
	matches, _ = Select(elements, Selector{By: "accessibilityId", Query: "SEND"})
	if len(matches) != 0 {
		t.Fatal("accessibility identifiers must remain case-sensitive")
	}
	matches, _ = Select(elements, Selector{By: "role", Query: "AXTextField"})
	if len(matches) != 1 || matches[0].Identifier != "query" {
		t.Fatalf("role aliases did not normalize: %#v", matches)
	}
	matches, _ = Select(elements, Selector{By: "text", Query: "WEATHER"})
	if len(matches) != 1 || matches[0].Value != "weather" {
		t.Fatalf("text must find AXValue: %#v", matches)
	}
}

func TestSelectInvalidOrDisabledTargetsRemainVisible(t *testing.T) {
	elements := parseFixture(t, `[{"type":"Application","frame":{"x":0,"y":0,"width":390,"height":844},"children":[
 {"type":"Group","enabled":false,"children":[{"type":"Button","label":"Disabled by parent","enabled":true,"frame":{"x":1,"y":1,"width":44,"height":44}}]},
 {"type":"Button","label":"Zero","frame":{"x":1,"y":1,"width":0,"height":44}},
 {"type":"Button","label":"Missing","frame":{"width":44,"height":44}},
 {"type":"Button","label":"Negative","frame":{"x":-100,"y":1,"width":44,"height":44}},
 {"type":"Button","label":"Offscreen","frame":{"x":1,"y":1000,"width":44,"height":44}}
]}]`)
	want := map[string]string{"Disabled by parent": "disabled", "Zero": "invalid_frame", "Missing": "invalid_frame", "Negative": "outside_screen", "Offscreen": "outside_screen"}
	for label, reason := range want {
		matches, err := Select(elements, Selector{By: "label", Query: label})
		if err != nil || len(matches) != 1 || matches[0].Actionable || matches[0].Reason != reason {
			t.Fatalf("%s: matches=%#v err=%v", label, matches, err)
		}
	}
}

func TestSelectorRejectsInvalidQueries(t *testing.T) {
	for _, selector := range []Selector{{}, {By: "predicate", Query: "x"}, {By: "label", Query: " "}, {By: "label", Query: "x", Match: "regex"}, {By: "role", Query: "AX"}, {By: "label", Query: "x", Role: " "}} {
		if _, err := Select(nil, selector); err == nil {
			t.Fatalf("invalid selector accepted: %#v", selector)
		}
	}
}

func TestFrameParsingRequiresCompleteFiniteGeometry(t *testing.T) {
	for _, raw := range []string{`"{{-12.5, +2}, {3e2, .5}}"`, `{"x":-12.5,"y":2,"width":300,"height":0.5}`} {
		var frame FrameValue
		if err := json.Unmarshal([]byte(raw), &frame); err != nil {
			t.Fatal(err)
		}
		x, y, ok := frame.Center()
		if !ok || x != 137.5 || y != 2.25 {
			t.Fatalf("frame=%#v center=(%g,%g,%v)", frame, x, y, ok)
		}
		if err := json.Unmarshal([]byte(`null`), &frame); err != nil || frame.Ok {
			t.Fatal("null must clear previously decoded geometry")
		}
	}
	for _, raw := range []string{`{}`, `{"x":0,"y":0,"width":44}`, `"garbage{{0,0},{44,44}}"`, `"{{0,0},{1e999,44}}"`, `"{{0,0},{-44,44}}"`, `"{{0,0},{0,44}}"`} {
		var frame FrameValue
		if err := json.Unmarshal([]byte(raw), &frame); err != nil {
			t.Fatal(err)
		}
		if _, _, ok := frame.Center(); ok {
			t.Fatalf("bad geometry accepted: %s", raw)
		}
	}
	for _, bad := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		if _, _, ok := (FrameValue{X: bad, Width: 44, Height: 44, Ok: true}).Center(); ok {
			t.Fatal("nonfinite frame accepted")
		}
	}
}

func TestParseDescribeUIRejectsUnknownSnapshots(t *testing.T) {
	for _, raw := range []string{"", "null", "{}", "[{}]", "[null]", `{"error":"accessibility unavailable"}`, `{"children":[{}]}`, `{"type":"Application","children":[null]}`, `{"type":"Button","frame":{"x":"bad"}}`} {
		if _, err := ParseDescribeUI(raw); err == nil {
			t.Fatalf("malformed snapshot accepted: %s", raw)
		}
	}
	elements, err := ParseDescribeUI("[]")
	if err != nil || elements == nil || len(elements) != 0 {
		t.Fatalf("valid empty snapshot rejected: %#v %v", elements, err)
	}
}

func TestObserveUIBoundsOutputAndReportsTruncation(t *testing.T) {
	elements := parseFixture(t, axeSelectorFixture)
	observation := ObserveUI(elements, 2)
	if len(observation.Elements) != 2 || observation.Total != 6 || !observation.Truncated {
		t.Fatalf("bad compact observation: %#v", observation)
	}
	observation = ObserveUI([]Element{{Label: strings.Repeat("界", 300)}}, 20)
	if !observation.Truncated || len([]rune(observation.Elements[0].Label)) != 241 {
		t.Fatalf("Unicode text must truncate explicitly: %#v", observation)
	}
}
