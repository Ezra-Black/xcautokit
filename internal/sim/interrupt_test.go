package sim

import (
	"encoding/json"
	"testing"
)

func parseFixture(t *testing.T, raw string) []Element {
	t.Helper()
	els, err := ParseDescribeUI(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return els
}

func TestDetectPermissionAlert(t *testing.T) {
	raw := `[
	  {
	    "type": "Application",
	    "label": "PennyWise",
	    "frame": {"x":0,"y":0,"width":402,"height":874},
	    "children": [
	      {
	        "type": "AXAlert",
	        "label": "“PennyWise” Would Like to Access Your Location",
	        "frame": {"x":40,"y":280,"width":320,"height":220},
	        "children": [
	          {"type": "StaticText", "label": "“PennyWise” Would Like to Access Your Location", "frame": {"x":50,"y":290,"width":300,"height":40}},
	          {"type": "StaticText", "label": "Your location is used to show nearby deals.", "frame": {"x":50,"y":330,"width":300,"height":60}},
	          {"type": "Button", "label": "Don't Allow", "frame": {"x":50,"y":420,"width":140,"height":44}},
	          {"type": "Button", "label": "Allow While Using App", "frame": {"x":200,"y":420,"width":150,"height":44}}
	        ]
	      }
	    ]
	  }
	]`
	intr := DetectInterrupts(parseFixture(t, raw))
	if len(intr) == 0 {
		t.Fatal("expected interrupt")
	}
	if intr[0].Kind != KindPermission {
		t.Fatalf("kind=%s want permission", intr[0].Kind)
	}
	if len(intr[0].Buttons) < 2 {
		t.Fatalf("buttons=%d", len(intr[0].Buttons))
	}
	btn, err := PickInterruptButton(intr[0], "accept", "")
	if err != nil {
		t.Fatal(err)
	}
	if btn.Label != "Allow While Using App" {
		t.Fatalf("accept=%q", btn.Label)
	}
	btn, err = PickInterruptButton(intr[0], "decline", "")
	if err != nil {
		t.Fatal(err)
	}
	if btn.Label != "Don't Allow" {
		t.Fatalf("decline=%q", btn.Label)
	}
}

func TestDetectOKAlert(t *testing.T) {
	raw := `[{
	  "type": "AXAlert",
	  "label": "Network Error",
	  "children": [
	    {"type": "StaticText", "label": "Network Error"},
	    {"type": "StaticText", "label": "Please try again later."},
	    {"type": "Button", "label": "OK", "frame": {"x":160,"y":400,"width":80,"height":44}}
	  ]
	}]`
	intr := DetectInterrupts(parseFixture(t, raw))
	if len(intr) == 0 || intr[0].Kind != KindAlert {
		t.Fatalf("got %#v", intr)
	}
	btn, err := PickInterruptButton(intr[0], "dismiss", "")
	if err != nil {
		t.Fatal(err)
	}
	if btn.Label != "OK" {
		t.Fatalf("dismiss=%q", btn.Label)
	}
}

func TestDetectCancelSheet(t *testing.T) {
	raw := `[{
	  "type": "AXSheet",
	  "label": "Share",
	  "children": [
	    {"type": "Button", "label": "Copy", "frame": {"x":20,"y":600,"width":360,"height":44}},
	    {"type": "Button", "label": "Cancel", "frame": {"x":20,"y":660,"width":360,"height":44}}
	  ]
	}]`
	intr := DetectInterrupts(parseFixture(t, raw))
	if len(intr) == 0 || intr[0].Kind != KindSheet {
		t.Fatalf("got %#v", intr)
	}
	btn, err := PickInterruptButton(intr[0], "decline", "")
	if err != nil {
		t.Fatal(err)
	}
	if btn.Label != "Cancel" {
		t.Fatalf("got %q", btn.Label)
	}
	btn, err = PickInterruptButton(intr[0], "button", "Copy")
	if err != nil {
		t.Fatal(err)
	}
	if btn.Label != "Copy" {
		t.Fatalf("got %q", btn.Label)
	}
}

func TestDetectNoInterruptAppUI(t *testing.T) {
	raw := `[{
	  "type": "Application",
	  "label": "PennyWise",
	  "children": [
	    {"type": "Button", "label": "Sign In", "frame": {"x":40,"y":400,"width":320,"height":48}},
	    {"type": "Button", "label": "Create Account", "frame": {"x":40,"y":460,"width":320,"height":48}},
	    {"type": "StaticText", "label": "Welcome to PennyWise"}
	  ]
	}]`
	intr := DetectInterrupts(parseFixture(t, raw))
	for _, i := range intr {
		if i.Kind != KindSpringBoard {
			t.Fatalf("unexpected interrupt %#v", i)
		}
	}
	if len(intr) != 0 {
		t.Fatalf("expected no interrupts, got %#v", intr)
	}
}

func TestDetectSpringBoard(t *testing.T) {
	raw := `[{
	  "type": "Application",
	  "label": " ",
	  "children": [
	    {"type": "Button", "identifier": "Safari", "label": "Safari", "frame": {"x":20,"y":700,"width":68,"height":68}},
	    {"type": "Button", "identifier": "Messages", "label": "Messages", "frame": {"x":100,"y":700,"width":68,"height":68}},
	    {"type": "Button", "identifier": "Maps", "label": "Maps", "frame": {"x":20,"y":100,"width":160,"height":160}},
	    {"type": "Button", "identifier": "Photos", "label": "Photos", "frame": {"x":100,"y":300,"width":68,"height":68}},
	    {"type": "Button", "identifier": "Settings", "label": "Settings", "frame": {"x":300,"y":300,"width":68,"height":68}}
	  ]
	}]`
	intr := DetectInterrupts(parseFixture(t, raw))
	found := false
	for _, i := range intr {
		if i.Kind == KindSpringBoard {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected springboard, got %#v", intr)
	}
}

func TestInterruptPreviewJSON(t *testing.T) {
	intr := []Interrupt{{
		Kind: KindPermission, Title: "Location", Confidence: 0.95,
		Buttons: []InterruptButton{{Label: "Allow"}, {Label: "Don't Allow"}},
	}}
	prev := InterruptPreview(intr)
	b, err := json.Marshal(prev)
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(b) {
		t.Fatal("invalid json")
	}
}

func TestPickInterruptButtonRequiresAction(t *testing.T) {
	intr := Interrupt{
		Kind: KindAlert,
		Buttons: []InterruptButton{
			{Label: "OK", CenterX: 1, CenterY: 2},
		},
	}
	if _, err := PickInterruptButton(intr, "explode", ""); err == nil {
		t.Fatal("expected error")
	}
}

func TestDetectFlatPermissionCurlyApostrophe(t *testing.T) {
	// Live iOS dialogs use U+2019 in "Don't Allow" and may omit AXAlert wrapper.
	raw := `[{
	  "type": "Application",
	  "label": " ",
	  "role": "AXApplication",
	  "frame": {"x":0,"y":0,"width":402,"height":874},
	  "children": [
	    {"type": "StaticText", "label": "Allow widgets from “Maps” to use your location?", "frame": {"x":71,"y":366,"width":260,"height":42}},
	    {"type": "StaticText", "label": "This app’s widgets will be able to use your location.", "frame": {"x":71,"y":416,"width":260,"height":58}},
	    {"type": "Button", "label": "Don’t Allow", "role": "AXButton", "frame": {"x":57,"y":494,"width":140,"height":48}},
	    {"type": "Button", "label": "Allow", "role": "AXButton", "frame": {"x":205,"y":494,"width":140,"height":48}}
	  ]
	}]`
	intr := DetectInterrupts(parseFixture(t, raw))
	if len(intr) == 0 || intr[0].Kind != KindPermission {
		t.Fatalf("got %#v", intr)
	}
	btn, err := PickInterruptButton(intr[0], "decline", "")
	if err != nil {
		t.Fatal(err)
	}
	if normLower(btn.Label) != "don't allow" {
		t.Fatalf("decline=%q", btn.Label)
	}
}

func TestOrdinaryNotificationSettingsAreNotInterrupts(t *testing.T) {
	raw := `[{"type":"Application","label":"Settings","children":[{"type":"Button","label":"Notifications","frame":{"x":20,"y":100,"width":200,"height":44}},{"type":"Button","label":"Continue","frame":{"x":20,"y":200,"width":200,"height":44}},{"type":"Button","label":"Cancel","frame":{"x":20,"y":300,"width":200,"height":44}}]}]`
	if got := DetectInterrupts(parseFixture(t, raw)); len(got) != 0 { t.Fatalf("ordinary screen blocked: %+v", got) }
}

func TestDismissDoesNotGrantSingleButtonPermission(t *testing.T) {
	intr := Interrupt{Kind:KindPermission, Buttons:[]InterruptButton{{Label:"Allow"}}}
	if _,err := PickInterruptButton(intr,"dismiss","");err==nil { t.Fatal("dismiss silently granted permission") }
}
