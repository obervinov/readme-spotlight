package render

import (
	"fmt"
	"strings"
)

// BannerAsset is the repo-relative path the banner SVG is written to.
const BannerAsset = "assets/banner.svg"

// BannerConfig is the persisted configuration of the header banner.
type BannerConfig struct {
	Enabled bool   `json:"enabled"`
	Name    string `json:"name"`    // large headline, e.g. "Oleg Bervinov"
	Role    string `json:"role"`    // subtitle, e.g. "Senior Platform Engineer"
	Tagline string `json:"tagline"` // muted line under the role
	Accent  string `json:"accent"`  // accent colour (hex)
}

// DefaultBanner returns a ready-to-edit banner configuration.
func DefaultBanner() BannerConfig {
	return BannerConfig{
		Enabled: true,
		Name:    "Hey, I'm Oleg",
		Role:    "Senior Platform Engineer",
		Tagline: "Turning chronic platform pain into self-serve patterns.",
		Accent:  "#3fb950",
	}
}

// Banner renders the header banner as a wide SVG card and returns the README
// embed plus the SVG asset.
func Banner(cfg BannerConfig) Output {
	accent := cfg.Accent
	if accent == "" {
		accent = "#3fb950"
	}
	name := firstNonEmpty(cfg.Name, "Your Name")
	role := firstNonEmpty(cfg.Role, "Your Role")

	const (
		w = 1000
		h = 240
	)
	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img">`, w, h, w, h)

	// Accent glow (top-right) + shared text styles.
	fmt.Fprintf(&b, `<defs><radialGradient id="glow" cx="50%%" cy="50%%" r="50%%">`+
		`<stop offset="0%%" stop-color="%s" stop-opacity="0.20"/>`+
		`<stop offset="100%%" stop-color="%s" stop-opacity="0"/></radialGradient></defs>`, accent, accent)
	b.WriteString(`<style>` +
		`.name{font:700 40px system-ui,-apple-system,Segoe UI,sans-serif;fill:#e6edf3}` +
		`.role{font:600 20px system-ui,sans-serif}` +
		`.tag{font:14px system-ui,sans-serif;fill:#8b949e}` +
		`</style>`)

	fmt.Fprintf(&b, `<rect x="0.5" y="0.5" width="%d" height="%d" rx="14" fill="#0d1117" stroke="#30363d"/>`, w-1, h-1)
	fmt.Fprintf(&b, `<circle cx="880" cy="52" r="200" fill="url(#glow)"/>`)
	// Accent rule beside the headline.
	fmt.Fprintf(&b, `<rect x="40" y="70" width="5" height="100" rx="2.5" fill="%s"/>`, accent)

	fmt.Fprintf(&b, `<text x="64" y="112" class="name">%s</text>`, escXML(name))
	fmt.Fprintf(&b, `<text x="64" y="148" class="role" fill="%s">%s</text>`, accent, escXML(role))
	for i, line := range wrapText(cfg.Tagline, 95, 2) {
		fmt.Fprintf(&b, `<text x="64" y="%d" class="tag">%s</text>`, 184+i*22, escXML(line))
	}

	b.WriteString(`</svg>`)

	img := `<img src="` + BannerAsset + `" alt="` + escAttr(name+" — "+role) + `">`
	return Output{Block: img, Assets: map[string]string{BannerAsset: b.String()}}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// wrapText greedily wraps s into lines of about maxChars, keeping at most
// maxLines and marking the last line with an ellipsis when content is dropped.
func wrapText(s string, maxChars, maxLines int) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var lines []string
	cur := ""
	for _, word := range strings.Fields(s) {
		cand := word
		if cur != "" {
			cand = cur + " " + word
		}
		if len(cand) > maxChars && cur != "" {
			lines = append(lines, cur)
			cur = word
		} else {
			cur = cand
		}
	}
	if cur != "" {
		lines = append(lines, cur)
	}
	if maxLines > 0 && len(lines) > maxLines {
		lines = lines[:maxLines]
		lines[maxLines-1] += " …"
	}
	return lines
}

func escAttr(s string) string {
	s = escXML(s)
	return strings.ReplaceAll(s, `"`, "&quot;")
}
