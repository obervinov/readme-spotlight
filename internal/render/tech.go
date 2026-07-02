package render

import (
	"fmt"
	"strings"
)

// TechAsset is the repo-relative path the tech SVG is written to.
const TechAsset = "assets/tech.svg"

// TechGroup is a labelled set of technologies.
type TechGroup struct {
	Name  string   `json:"name"`
	Items []string `json:"items"`
}

// TechConfig is the persisted configuration of the technology section.
type TechConfig struct {
	Enabled bool        `json:"enabled"`
	Title   string      `json:"title"`
	Accent  string      `json:"accent"`
	Groups  []TechGroup `json:"groups"`
}

// DefaultTech seeds a domain-grouped stack the user can edit.
func DefaultTech() TechConfig {
	return TechConfig{
		Enabled: true,
		Title:   "Technology & Tools",
		Accent:  "#3fb950",
		Groups: []TechGroup{
			{"Orchestration & Cloud", []string{"Kubernetes", "AWS", "Docker", "Helm"}},
			{"IaC & Config", []string{"Terraform", "Ansible"}},
			{"CI/CD & GitOps", []string{"GitLab CI", "Jenkins", "Argo CD"}},
			{"Observability", []string{"Prometheus", "Grafana", "ELK", "Istio"}},
			{"Data & Streaming", []string{"PostgreSQL", "MongoDB", "Redis", "Kafka"}},
			{"Languages", []string{"Go", "Python", "Bash"}},
			{"AI/LLM & DevX", []string{"LiteLLM", "MCP", "n8n", "Jupyter"}},
		},
	}
}

// Tech renders the grouped technology stack as an SVG and returns the README
// embed plus the SVG asset. Empty when there are no groups.
func Tech(cfg TechConfig) Output {
	if len(cfg.Groups) == 0 {
		return Output{}
	}
	accent := firstNonEmpty(cfg.Accent, "#3fb950")
	title := firstNonEmpty(cfg.Title, "Technology & Tools")

	const (
		w       = 1000
		x0      = 40
		maxX    = w - 40
		pillH   = 28
		pillGap = 8
	)

	var body strings.Builder
	y := 80 // running top of the current group's label
	for _, g := range cfg.Groups {
		if len(g.Items) == 0 {
			continue
		}
		fmt.Fprintf(&body, `<text x="%d" y="%d" class="tg" fill="%s">%s</text>`, x0, y+12, accent, escXML(strings.ToUpper(g.Name)))
		rowTop := y + 22
		cursor := x0
		for _, item := range g.Items {
			pw := pillWidth(item)
			if cursor != x0 && cursor+pw > maxX {
				cursor = x0
				rowTop += pillH + pillGap
			}
			fmt.Fprintf(&body, `<rect x="%d" y="%d" width="%d" height="%d" rx="14" fill="#161b22" stroke="#30363d"/>`, cursor, rowTop, pw, pillH)
			fmt.Fprintf(&body, `<text x="%d" y="%d" class="tp" text-anchor="middle">%s</text>`, cursor+pw/2, rowTop+18, escXML(item))
			cursor += pw + pillGap
		}
		y = rowTop + pillH + 22
	}
	height := y - 6

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img">`, w, height, w, height)
	b.WriteString(`<style>` +
		`.sh{font:700 17px system-ui,-apple-system,Segoe UI,sans-serif;fill:#e6edf3}` +
		`.tg{font:600 10.5px ui-monospace,SFMono-Regular,Menlo,monospace;fill:#8b949e;letter-spacing:.06em}` +
		`.tp{font:13px system-ui,sans-serif;fill:#adbac7}` +
		`</style>`)
	fmt.Fprintf(&b, `<rect x="0.5" y="0.5" width="%d" height="%d" rx="12" fill="#0d1117" stroke="#30363d"/>`, w-1, height-1)
	fmt.Fprintf(&b, `<text x="%d" y="48" class="sh">%s</text>`, x0, escXML(title))
	b.WriteString(body.String())
	b.WriteString(`</svg>`)

	img := `<img src="` + TechAsset + `" alt="` + escAttr(title) + `">`
	return Output{Block: img, Assets: map[string]string{TechAsset: b.String()}}
}

// pillWidth estimates a pill's width from its label length.
func pillWidth(label string) int {
	const charW = 7.4
	w := int(float64(len([]rune(label)))*charW) + 26
	if w < 48 {
		w = 48
	}
	return w
}
