package server

import (
	"encoding/json"
	"math"
	"reflect"
	"testing"

	"github.com/xcautokit/xcautokit/internal/sim"
)

func targetFixture(t *testing.T) []sim.Element {
	t.Helper()
	elements, err := sim.ParseDescribeUI(`[
 {"type":"Button","label":"Continue later","identifier":"later","frame":{"x":200,"y":20,"width":100,"height":44}},
 {"type":"Button","label":"Continue","identifier":"continue","frame":{"x":10,"y":20,"width":100,"height":44}},
 {"type":"Group","label":"Send","identifier":"send","frame":{"x":10,"y":80,"width":200,"height":44}},
 {"type":"Button","label":"Send","identifier":"send","frame":{"x":20,"y":90,"width":100,"height":44}},
 {"type":"Button","label":"Disabled","enabled":false,"frame":{"x":20,"y":150,"width":100,"height":44}},
 {"type":"Button","label":"Zero","frame":{"x":20,"y":200,"width":0,"height":44}}
]`)
	if err != nil {
		t.Fatal(err)
	}
	return elements
}

func TestLegacyTargetPrefersExactAndRequiresUniqueMatch(t *testing.T) {
	elements := targetFixture(t)
	x, y, err := resolveTargetPointFromElements(elements, map[string]any{"label": "Continue"})
	if err != nil || x != 60 || y != 42 {
		t.Fatalf("partial first match chosen over exact: %v,%v,%v", x, y, err)
	}
	if _, _, err := resolveTargetPointFromElements(elements, map[string]any{"accessibilityId": "send"}); err == nil {
		t.Fatal("duplicate identifier must be rejected")
	}
	x, y, err = resolveTargetPointFromElements(elements, map[string]any{"accessibilityId": "send", "role": "Button"})
	if err != nil || x != 70 || y != 112 {
		t.Fatalf("role filter failed: %v,%v,%v", x, y, err)
	}
	for _, index := range []any{float64(1), 1, json.Number("1")} {
		x, y, err = resolveTargetPointFromElements(elements, map[string]any{"accessibilityId": "send", "index": index})
		if err != nil || x != 70 || y != 112 {
			t.Fatalf("explicit integer index failed: %v,%v,%v", x, y, err)
		}
	}
}

func TestLegacyTargetRejectsInvalidSelectionAndGeometry(t *testing.T) {
	elements := targetFixture(t)
	for _, index := range []any{float64(-1), 0.9, 2.0, "1", nil, math.NaN(), math.Inf(1), json.Number("1.5")} {
		if _, _, err := resolveTargetPointFromElements(elements, map[string]any{"accessibilityId": "send", "index": index}); err == nil {
			t.Fatalf("invalid index accepted: %#v", index)
		}
	}
	for _, target := range []map[string]any{
		{"label": "Disabled"}, {"label": "Zero"}, {"label": 5}, {"label": "Continue", "text": "Continue"}, {"label": "Continue", "match": false},
	} {
		if _, _, err := resolveTargetPointFromElements(elements, target); err == nil {
			t.Fatalf("invalid target accepted: %#v", target)
		}
	}
}

func TestLongPressUsesHeldTouchInsteadOfDelayedTap(t *testing.T) {
	args, duration, err := longPressArgs("device", 12.5, 20.25, 1.5)
	want := []string{"touch", "-x", "12.5", "-y", "20.25", "--down", "--up", "--delay", "1.5", "--udid", "device"}
	if err != nil || duration != 1.5 || !reflect.DeepEqual(args, want) {
		t.Fatalf("wrong hold command: %#v duration=%v error=%v", args, duration, err)
	}
	if _, duration, err := longPressArgs("device", 1, 1, 0); err != nil || duration != 1 {
		t.Fatal("omitted duration must default to one second")
	}
	for _, duration := range []float64{-1, 11, math.Inf(1), math.NaN()} {
		if _, _, err := longPressArgs("device", 1, 1, duration); err == nil {
			t.Fatalf("invalid duration accepted: %v", duration)
		}
	}
}

func TestHardwareButtonNamesKeepTheirMeaning(t *testing.T) {
	for _, name := range []string{"home", "lock", "side-button", "siri", "apple-pay"} {
		got, err := hardwareButtonName(name)
		if err != nil || got != name {
			t.Fatalf("button %s mapped to %s: %v", name, got, err)
		}
	}
	if _, err := hardwareButtonName("unknown"); err == nil {
		t.Fatal("unknown button accepted")
	}
}

func TestTypedTextCannotBecomeCommandOptions(t *testing.T) {
	for _, text := range []string{"--help", "--file", "-example", "ordinary text"} {
		want := []string{"type", "--udid", "device", "--", text}
		if got := typeTextArgs("device", text); !reflect.DeepEqual(got, want) {
			t.Fatalf("typed text was not protected as one literal argument: %#v", got)
		}
	}
}

func TestTypedTextPreflightPreventsPartialUnsupportedInput(t *testing.T) {
	for _, text := range []string{"Hello World!", "--file /tmp/example", "Line one\nLine two\r\tEnd", "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789 !@#$%^&*()_+-={}[]|\\:;\"'<>,.?/`~"} {
		if err := validateTypeText(text); err != nil {
			t.Fatalf("supported keyboard text rejected: %v", err)
		}
	}
	for _, text := range []string{"", "caf\u00e9", "cost \u20ac5", "emoji \U0001f44d", "bad\x00input", "bad\x7finput", "bad\x01input"} {
		if err := validateTypeText(text); err == nil {
			t.Fatalf("unsupported text accepted: %q", text)
		}
	}
}
