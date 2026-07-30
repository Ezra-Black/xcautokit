package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/xcautokit/xcautokit/internal/sim"
)

func cmdInterrupt(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: xcautokit interrupt check|dismiss [--udid UDID] [--action accept|decline|dismiss|button] [--button LABEL]")
	}
	switch args[0] {
	case "check":
		return interruptCheck(args[1:])
	case "dismiss":
		return interruptDismiss(args[1:])
	default:
		return fmt.Errorf("unknown interrupt subcommand %q", args[0])
	}
}

func interruptFlags(args []string) (udid, action, button string) {
	udid = "booted"
	action = "decline"
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--udid", "-u":
			if i+1 < len(args) {
				udid = args[i+1]
				i++
			}
		case "--action", "-a":
			if i+1 < len(args) {
				action = args[i+1]
				i++
			}
		case "--button", "-b":
			if i+1 < len(args) {
				button = args[i+1]
				i++
			}
		}
	}
	return udid, action, button
}

func interruptCheck(args []string) error {
	udidArg, _, _ := interruptFlags(args)
	udid, err := sim.ResolveUDID(udidArg)
	if err != nil {
		return err
	}
	raw, err := sim.DescribeUI(udid)
	if err != nil {
		return err
	}
	els, err := sim.ParseDescribeUI(raw)
	if err != nil {
		return err
	}
	intr := sim.DetectInterrupts(els)
	out := map[string]any{
		"device":       udid,
		"hasInterrupt": len(intr) > 0,
		"count":        len(intr),
		"interrupts":   intr,
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func interruptDismiss(args []string) error {
	udidArg, action, buttonLabel := interruptFlags(args)
	udid, err := sim.ResolveUDID(udidArg)
	if err != nil {
		return err
	}
	raw, err := sim.DescribeUI(udid)
	if err != nil {
		return err
	}
	els, err := sim.ParseDescribeUI(raw)
	if err != nil {
		return err
	}
	intr := sim.DetectInterrupts(els)
	if len(intr) == 0 {
		return json.NewEncoder(os.Stdout).Encode(map[string]any{
			"dismissed": false, "device": udid, "message": "no interrupt detected",
		})
	}
	target := intr[0]
	for _, i := range intr {
		if i.Kind != sim.KindSpringBoard {
			target = i
			break
		}
	}
	if target.Kind == sim.KindSpringBoard {
		return fmt.Errorf("springboard detected — use home + app_launch, not dismiss")
	}
	btn, err := sim.PickInterruptButton(target, strings.ToLower(action), buttonLabel)
	if err != nil {
		return err
	}
	if _, err := sim.RunAxe("tap",
		"-x", fmt.Sprintf("%.0f", btn.CenterX),
		"-y", fmt.Sprintf("%.0f", btn.CenterY),
		"--udid", udid,
	); err != nil {
		return err
	}
	time.Sleep(250 * time.Millisecond)
	raw2, _ := sim.DescribeUI(udid)
	remaining := []sim.Interrupt{}
	if els2, err2 := sim.ParseDescribeUI(raw2); err2 == nil {
		remaining = sim.DetectInterrupts(els2)
	}
	return json.NewEncoder(os.Stdout).Encode(map[string]any{
		"dismissed":    true,
		"device":       udid,
		"action":       action,
		"pressed":      btn,
		"kind":         target.Kind,
		"remaining":    remaining,
		"hasInterrupt": len(remaining) > 0,
	})
}
