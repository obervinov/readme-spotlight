package render

import (
	"fmt"
	"strings"
)

// PositioningAsset is the repo-relative path the positioning SVG is written to.
const PositioningAsset = "assets/positioning.svg"

// PositioningConfig is the persisted configuration of the one-line positioning
// statement shown under the banner.
type PositioningConfig struct {
	Enabled bool   `json:"enabled"`
	Text    string `json:"text"`
	Accent  string `json:"accent"`
}

// DefaultPositioning seeds the positioning statement.
func DefaultPositioning() PositioningConfig {
	return PositioningConfig{
		Enabled: true,
		Accent:  "#3fb950",
		Text:    "AI/LLM platform & Developer Experience · Kubernetes · multi-cloud · IaC · LLMOps",
	}
}

// Positioning renders the statement as a slim SVG strip: the lead sentence in
// bright type, the remainder muted beneath it.
func Positioning(cfg PositioningConfig) Output {
	text := strings.TrimSpace(cfg.Text)
	if text == "" {
		return Output{}
	}
	accent := firstNonEmpty(cfg.Accent, "#3fb950")

	lead, tail := text, ""
	if i := strings.Index(text, ". "); i >= 0 {
		lead, tail = text[:i+1], strings.TrimSpace(text[i+2:])
	}
	leadLines := wrapText(lead, 90, 3)
	tailLines := wrapText(tail, 104, 2)

	const (
		w  = 1000
		tx = 64
	)
	var body strings.Builder
	y := 42
	for _, line := range leadLines {
		fmt.Fprintf(&body, `<text x="%d" y="%d" class="pl">%s</text>`, tx, y, escXML(line))
		y += 25
	}
	if len(tailLines) > 0 {
		y += 3
		for _, line := range tailLines {
			fmt.Fprintf(&body, `<text x="%d" y="%d" class="pt">%s</text>`, tx, y, escXML(line))
			y += 21
		}
	}
	height := y + 2

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img">`, w, height, w, height)
	b.WriteString(`<style>` +
		`.pl{font:600 16px system-ui,-apple-system,Segoe UI,sans-serif;fill:#e6edf3}` +
		`.pt{font:14px system-ui,sans-serif;fill:#8b949e;font-style:italic}` +
		`</style>`)
	fmt.Fprintf(&b, `<rect x="0.5" y="0.5" width="%d" height="%d" rx="12" fill="#0d1117" stroke="#30363d"/>`, w-1, height-1)
	fmt.Fprintf(&b, `<rect x="40" y="22" width="5" height="%d" rx="2.5" fill="%s"/>`, height-44, accent)
	b.WriteString(body.String())
	b.WriteString(`</svg>`)

	img := `<img src="` + PositioningAsset + `" alt="` + escAttr(lead) + `">`
	return Output{Block: img, Assets: map[string]string{PositioningAsset: b.String()}}
}
