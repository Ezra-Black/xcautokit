package sim

import (
	"fmt"
	"strings"
)

// Interrupt kinds detected from the accessibility tree.
const (
	KindAlert           = "alert"
	KindSheet           = "sheet"
	KindPermission      = "permission"
	KindBanner          = "banner"
	KindSpringBoard     = "springboard"
	KindUnknownOverlay  = "unknown_overlay"
)

// InterruptButton is a tappable control on an interrupt.
type InterruptButton struct {
	Label   string  `json:"label"`
	Role    string  `json:"role,omitempty"`
	CenterX float64 `json:"centerX"`
	CenterY float64 `json:"centerY"`
}

// Interrupt is a system/app overlay that may block automation.
type Interrupt struct {
	Kind       string            `json:"kind"`
	Title      string            `json:"title,omitempty"`
	Message    string            `json:"message,omitempty"`
	Buttons    []InterruptButton `json:"buttons,omitempty"`
	Confidence float64           `json:"confidence"`
}

var acceptLabels = []string{
	"allow", "allow once", "allow while using app", "ok", "continue", "yes",
	"turn on", "enable", "agree", "accept", "got it", "done", "join", "open",
	"use while using the app", "allow access", "share",
}

var declineLabels = []string{
	"don't allow", "dont allow", "do not allow", "not now", "cancel", "no",
	"close", "dismiss", "later", "deny", "don't share", "dont share",
	"not interested", "ask app not to track",
}

var dismissLabels = []string{
	"close", "dismiss", "ok", "got it", "done", "cancel", "not now",
}

var springboardIcons = []string{
	"safari", "maps", "photos", "calendar", "messages", "settings",
	"reminders", "news", "health", "wallet",
}

func normalizeLabel(s string) string {
	s = strings.TrimSpace(s)
	// Normalize curly/smart quotes to ASCII for matching Don't Allow, etc.
	replacer := strings.NewReplacer(
		"\u2018", "'", "\u2019", "'", "\u201C", `"`, "\u201D", `"`,
		"\u00B4", "'", "`", "'",
	)
	return replacer.Replace(s)
}

func normLower(s string) string {
	return strings.ToLower(normalizeLabel(s))
}

func typeBlob(e Element) string {
	return strings.ToLower(strings.Join([]string{e.Type, e.Role, e.RoleDescription}, " "))
}

func isButtonLike(e Element) bool {
	t := typeBlob(e)
	return strings.Contains(t, "button") ||
		strings.Contains(t, "axbutton") ||
		strings.EqualFold(e.Type, "Button")
}

func isAlertLike(e Element) bool {
	t := typeBlob(e)
	return strings.Contains(t, "alert") || strings.Contains(t, "axalert")
}

func isSheetLike(e Element) bool {
	t := typeBlob(e)
	return strings.Contains(t, "sheet") || strings.Contains(t, "axsheet") ||
		strings.Contains(t, "action sheet") || strings.Contains(t, "popover")
}

func isBannerLike(e Element) bool {
	t := typeBlob(e)
	label := strings.ToLower(labelOf(e))
	return strings.Contains(t, "banner") ||
		strings.Contains(t, "notification") ||
		strings.Contains(label, "notification")
}

func collectButtons(e Element) []InterruptButton {
	var buttons []InterruptButton
	var walk func([]Element)
	walk = func(els []Element) {
		for _, c := range els {
			if isButtonLike(c) {
				lbl := strings.TrimSpace(labelOf(c))
				if lbl == "" {
					lbl = strings.TrimSpace(valueOf(c))
				}
				if lbl != "" {
					f := frameOf(c)
					cx, cy, ok := f.Center()
					if ok {
						buttons = append(buttons, InterruptButton{
							Label:   lbl,
							Role:    firstNonEmpty(c.Type, c.Role),
							CenterX: cx,
							CenterY: cy,
						})
					}
				}
			}
			if len(c.Children) > 0 {
				walk(c.Children)
			}
		}
	}
	walk(e.Children)
	// Self may be a button container with label-only children already walked.
	if len(buttons) == 0 && isButtonLike(e) {
		lbl := strings.TrimSpace(labelOf(e))
		if lbl != "" {
			f := frameOf(e)
			if cx, cy, ok := f.Center(); ok {
				buttons = append(buttons, InterruptButton{
					Label: lbl, Role: firstNonEmpty(e.Type, e.Role), CenterX: cx, CenterY: cy,
				})
			}
		}
	}
	return buttons
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func textCandidates(e Element) (title, message string) {
	var texts []string
	var walk func([]Element)
	walk = func(els []Element) {
		for _, c := range els {
			if isButtonLike(c) {
				if len(c.Children) > 0 {
					walk(c.Children)
				}
				continue
			}
			lbl := strings.TrimSpace(labelOf(c))
			val := strings.TrimSpace(valueOf(c))
			t := typeBlob(c)
			if lbl != "" && (strings.Contains(t, "static") || strings.Contains(t, "text") ||
				strings.Contains(t, "label") || c.Type == "" || strings.Contains(t, "axstatic")) {
				texts = append(texts, lbl)
			} else if val != "" && strings.Contains(t, "text") {
				texts = append(texts, val)
			} else if lbl != "" && !isButtonLike(c) && len(lbl) > 2 {
				// Keep short non-button labels as possible titles.
				if len(texts) < 4 {
					texts = append(texts, lbl)
				}
			}
			if len(c.Children) > 0 {
				walk(c.Children)
			}
		}
	}
	walk(e.Children)
	if root := strings.TrimSpace(labelOf(e)); root != "" && !isButtonLike(e) {
		texts = append([]string{root}, texts...)
	}
	if len(texts) == 0 {
		return "", ""
	}
	title = texts[0]
	if len(texts) > 1 {
		message = strings.Join(texts[1:], " ")
	}
	return title, message
}

func buttonSetLooksLikePermission(buttons []InterruptButton) bool {
	hasAllow, hasDeny := false, false
	for _, b := range buttons {
		l := normLower(b.Label)
		if isNegativeAllowLabel(l) || l == "cancel" || strings.Contains(l, "not now") || l == "deny" {
			hasDeny = true
			continue
		}
		if strings.Contains(l, "allow") || l == "ok" || l == "continue" || l == "accept" {
			hasAllow = true
		}
	}
	return hasAllow && hasDeny
}

func classifyNode(e Element) *Interrupt {
	buttons := collectButtons(e)
	title, message := textCandidates(e)
	blob := strings.ToLower(title + " " + message + " " + typeBlob(e))

	switch {
	case isAlertLike(e):
		kind := KindAlert
		conf := 0.9
		if buttonSetLooksLikePermission(buttons) ||
			strings.Contains(blob, "would like to") ||
			strings.Contains(blob, "access your") ||
			strings.Contains(blob, "track you") ||
			strings.Contains(blob, "location") ||
			strings.Contains(blob, "camera") ||
			strings.Contains(blob, "microphone") ||
			strings.Contains(blob, "notifications") {
			kind = KindPermission
			conf = 0.95
		}
		return &Interrupt{Kind: kind, Title: title, Message: message, Buttons: buttons, Confidence: conf}
	case isSheetLike(e):
		kind := KindSheet
		conf := 0.85
		if buttonSetLooksLikePermission(buttons) {
			kind = KindPermission
			conf = 0.9
		}
		return &Interrupt{Kind: kind, Title: title, Message: message, Buttons: buttons, Confidence: conf}
	case isBannerLike(e):
		return &Interrupt{Kind: KindBanner, Title: title, Message: message, Buttons: buttons, Confidence: 0.75}
	case buttonSetLooksLikePermission(buttons) && (title != "" || message != ""):
		return &Interrupt{Kind: KindPermission, Title: title, Message: message, Buttons: buttons, Confidence: 0.8}
	case len(buttons) >= 1 && (isAlertLike(e) || strings.Contains(blob, "alert")):
		return &Interrupt{Kind: KindAlert, Title: title, Message: message, Buttons: buttons, Confidence: 0.7}
	}
	return nil
}

func detectSpringBoard(elements []Element) *Interrupt {
	labels := map[string]bool{}
	var walk func([]Element)
	walk = func(els []Element) {
		for _, e := range els {
			l := strings.ToLower(strings.TrimSpace(labelOf(e)))
			id := strings.ToLower(strings.TrimSpace(idOf(e)))
			if l != "" {
				labels[l] = true
			}
			if id != "" {
				labels[id] = true
			}
			if len(e.Children) > 0 {
				walk(e.Children)
			}
		}
	}
	walk(elements)

	hits := 0
	for _, icon := range springboardIcons {
		if labels[icon] {
			hits++
		}
	}
	// Home screen typically shows several system icons together.
	if hits >= 3 && (labels["safari"] || labels["settings"]) {
		return &Interrupt{
			Kind:       KindSpringBoard,
			Title:      "SpringBoard",
			Message:    "Home screen is visible; target app may not be foreground",
			Confidence: 0.85,
		}
	}
	return nil
}

// DetectInterrupts walks an accessibility tree and returns overlays that may block automation.
func DetectInterrupts(elements []Element) []Interrupt {
	var out []Interrupt
	seen := map[string]bool{}

	var walk func([]Element)
	walk = func(els []Element) {
		for _, e := range els {
			if intr := classifyNode(e); intr != nil {
				key := intr.Kind + "|" + intr.Title + "|" + buttonKey(intr.Buttons)
				if !seen[key] {
					seen[key] = true
					out = append(out, *intr)
				}
			}
			if len(e.Children) > 0 {
				walk(e.Children)
			}
		}
	}
	walk(elements)

	if sb := detectSpringBoard(elements); sb != nil {
		key := sb.Kind + "|" + sb.Title
		if !seen[key] {
			out = append(out, *sb)
		}
	}

	// Prefer higher-confidence / more specific kinds first.
	sortInterrupts(out)
	return out
}

func buttonKey(buttons []InterruptButton) string {
	parts := make([]string, 0, len(buttons))
	for _, b := range buttons {
		parts = append(parts, strings.ToLower(b.Label))
	}
	return strings.Join(parts, ",")
}

func sortInterrupts(in []Interrupt) {
	// Simple insertion by confidence desc, permission before alert before sheet.
	priority := func(k string) int {
		switch k {
		case KindPermission:
			return 0
		case KindAlert:
			return 1
		case KindSheet:
			return 2
		case KindBanner:
			return 3
		case KindUnknownOverlay:
			return 4
		case KindSpringBoard:
			return 5
		default:
			return 6
		}
	}
	for i := 1; i < len(in); i++ {
		j := i
		for j > 0 {
			if priority(in[j-1].Kind) < priority(in[j].Kind) {
				break
			}
			if priority(in[j-1].Kind) == priority(in[j].Kind) && in[j-1].Confidence >= in[j].Confidence {
				break
			}
			in[j-1], in[j] = in[j], in[j-1]
			j--
		}
	}
}

// PickInterruptButton chooses a button for accept/decline/dismiss/button actions.
func PickInterruptButton(intr Interrupt, action, buttonLabel string) (*InterruptButton, error) {
	if len(intr.Buttons) == 0 && action != "button" {
		return nil, fmt.Errorf("interrupt %q has no tappable buttons", intr.Kind)
	}
	action = strings.ToLower(strings.TrimSpace(action))
	switch action {
	case "button":
		if buttonLabel == "" {
			return nil, fmt.Errorf("buttonLabel required when action=button")
		}
		return findButton(intr.Buttons, buttonLabel, true)
	case "accept":
		if b := matchLabelList(intr.Buttons, acceptLabels, true); b != nil {
			return b, nil
		}
		return nil, fmt.Errorf("no accept-like button found (labels: %s)", buttonLabels(intr.Buttons))
	case "decline":
		if b := matchLabelList(intr.Buttons, declineLabels, false); b != nil {
			return b, nil
		}
		return nil, fmt.Errorf("no decline-like button found (labels: %s)", buttonLabels(intr.Buttons))
	case "dismiss":
		if b := matchLabelList(intr.Buttons, dismissLabels, false); b != nil {
			return b, nil
		}
		// Prefer decline-like for soft dismiss, else single button.
		if b := matchLabelList(intr.Buttons, declineLabels, false); b != nil {
			return b, nil
		}
		if len(intr.Buttons) == 1 {
			return &intr.Buttons[0], nil
		}
		return nil, fmt.Errorf("no dismiss-like button found (labels: %s)", buttonLabels(intr.Buttons))
	default:
		return nil, fmt.Errorf("action must be accept, decline, dismiss, or button")
	}
}

func matchLabelList(buttons []InterruptButton, labels []string, skipNegativeAllow bool) *InterruptButton {
	// Exact match first
	for _, want := range labels {
		for i := range buttons {
			l := normLower(buttons[i].Label)
			if skipNegativeAllow && isNegativeAllowLabel(l) {
				continue
			}
			if l == want {
				return &buttons[i]
			}
		}
	}
	// Partial contains for "Allow While Using App" etc.
	for i := range buttons {
		l := normLower(buttons[i].Label)
		if skipNegativeAllow && isNegativeAllowLabel(l) {
			continue
		}
		for _, want := range labels {
			if strings.Contains(l, want) {
				return &buttons[i]
			}
		}
	}
	return nil
}

func isNegativeAllowLabel(l string) bool {
	return strings.Contains(l, "don't allow") ||
		strings.Contains(l, "dont allow") ||
		strings.Contains(l, "do not allow") ||
		strings.HasPrefix(l, "don't ") ||
		strings.HasPrefix(l, "dont ")
}

func findButton(buttons []InterruptButton, label string, required bool) (*InterruptButton, error) {
	want := normLower(label)
	for i := range buttons {
		if normLower(buttons[i].Label) == want {
			return &buttons[i], nil
		}
	}
	for i := range buttons {
		if strings.Contains(normLower(buttons[i].Label), want) {
			return &buttons[i], nil
		}
	}
	if required {
		return nil, fmt.Errorf("button %q not found (labels: %s)", label, buttonLabels(buttons))
	}
	return nil, fmt.Errorf("not found")
}

func buttonLabels(buttons []InterruptButton) string {
	parts := make([]string, 0, len(buttons))
	for _, b := range buttons {
		parts = append(parts, b.Label)
	}
	return strings.Join(parts, ", ")
}

// InterruptPreview returns a compact list for ui_summary enrichment.
func InterruptPreview(interrupts []Interrupt) []map[string]any {
	out := make([]map[string]any, 0, len(interrupts))
	for _, i := range interrupts {
		item := map[string]any{
			"kind":       i.Kind,
			"confidence": i.Confidence,
		}
		if i.Title != "" {
			item["title"] = i.Title
		}
		if len(i.Buttons) > 0 {
			labels := make([]string, 0, len(i.Buttons))
			for _, b := range i.Buttons {
				labels = append(labels, b.Label)
			}
			item["buttons"] = labels
		}
		out = append(out, item)
	}
	return out
}
