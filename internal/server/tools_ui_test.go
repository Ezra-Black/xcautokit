package server

import (
	"errors"
	"testing"

	"github.com/xcautokit/xcautokit/internal/sim"
)

func TestDismissInterruptRequiresObservedDisappearance(t *testing.T) {
	target := sim.Interrupt{Kind: sim.KindAlert, Title: "Test", Message: "Try again"}
	button := sim.InterruptButton{Label: "Cancel", CenterX: 20, CenterY: 40}
	before := []sim.Interrupt{target}
	for _, tc := range []struct {
		name      string
		remaining []sim.Interrupt
		err       error
		dismissed any
		verified  bool
		success   bool
	}{
		{"gone", []sim.Interrupt{}, nil, true, true, true},
		{"still present", before, nil, false, true, false},
		{"new overlay", []sim.Interrupt{{Kind: sim.KindAlert, Title: "Different"}}, nil, true, true, true},
		{"readback failed", nil, errors.New("unavailable"), nil, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := dismissInterruptResult("device", "decline", target, button, before, tc.remaining, tc.err)
			if out["performed"] != true || out["dismissed"] != tc.dismissed || out["verified"] != tc.verified || out["success"] != tc.success {
				t.Fatalf("dishonest dismissal result: %#v", out)
			}
			if tc.err != nil && (out["hasInterrupt"] != nil || out["remaining"] != nil || out["lastKnownInterrupts"] == nil) {
				t.Fatalf("failed readback must not claim a clear screen: %#v", out)
			}
		})
	}
}
