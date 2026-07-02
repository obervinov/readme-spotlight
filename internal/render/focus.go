package render

import (
	"fmt"
	"strings"
)

// FocusAsset is the repo-relative path the focus SVG is written to.
const FocusAsset = "assets/focus.svg"

// FocusItem is one focus area: a bold title and a muted one-to-three line blurb.
type FocusItem struct {
	Title string `json:"title"`
	Text  string `json:"text"`
}

// FocusConfig is the persisted configuration of the "What I Do" section.
type FocusConfig struct {
	Enabled bool        `json:"enabled"`
	Title   string      `json:"title"`
	Accent  string      `json:"accent"`
	Items   []FocusItem `json:"items"`
}

// DefaultFocus seeds platform-engineering focus areas the user can edit.
func DefaultFocus() FocusConfig {
	return FocusConfig{
		Enabled: true,
		Title:   "What I Do",
		Accent:  "#3fb950",
		Items: []FocusItem{
			{"Self-serve platforms & DevX", "Golden paths, paved roads and internal tooling that let teams ship without filing tickets."},
			{"AI / LLMOps platform", "LiteLLM, MCP and inference plumbing — turning LLM experiments into reliable platform capabilities."},
			{"Reliability & observability", "Prometheus, Grafana, ELK and Istio for SLO-driven operations and low MTTR."},
			{"Multi-cloud & IaC", "Terraform and Ansible across AWS and beyond, including large-scale platform migrations."},
			{"FinOps & efficiency", "Karpenter and dynamic provisioning for cost-aware, cloud-native infrastructure."},
		},
	}
}

// Focus renders the "What I Do" card as an SVG and returns the README embed
// plus the SVG asset. It returns an empty Output when there are no items.
func Focus(cfg FocusConfig) Output {
	if len(cfg.Items) == 0 {
		return Output{}
	}
	accent := firstNonEmpty(cfg.Accent, "#3fb950")
	title := firstNonEmpty(cfg.Title, "What I Do")

	const w = 1000

	// Pre-wrap descriptions and lay the body out, tracking the running height.
	var body strings.Builder
	y := 84
	for _, it := range cfg.Items {
		fmt.Fprintf(&body, `<rect x="40" y="%d" width="10" height="10" rx="2.5" fill="%s"/>`, y-10, accent)
		fmt.Fprintf(&body, `<text x="64" y="%d" class="ft">%s</text>`, y, escXML(it.Title))
		dy := y + 22
		for _, line := range wrapText(it.Text, 98, 3) {
			fmt.Fprintf(&body, `<text x="64" y="%d" class="fd">%s</text>`, dy, escXML(line))
			dy += 19
		}
		y = dy + 16
	}
	height := y - 2

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img">`, w, height, w, height)
	b.WriteString(`<style>` +
		`.sh{font:700 17px system-ui,-apple-system,Segoe UI,sans-serif;fill:#e6edf3}` +
		`.ft{font:600 15px system-ui,sans-serif;fill:#e6edf3}` +
		`.fd{font:13px system-ui,sans-serif;fill:#8b949e}` +
		`</style>`)
	fmt.Fprintf(&b, `<rect x="0.5" y="0.5" width="%d" height="%d" rx="12" fill="#0d1117" stroke="#30363d"/>`, w-1, height-1)
	fmt.Fprintf(&b, `<text x="40" y="48" class="sh">%s</text>`, escXML(title))
	b.WriteString(body.String())
	b.WriteString(`</svg>`)

	img := `<img src="` + FocusAsset + `" alt="` + escAttr(title) + `">`
	return Output{Block: img, Assets: map[string]string{FocusAsset: b.String()}}
}
