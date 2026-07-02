package render

import (
	"fmt"
	"strings"

	"github.com/obervinov/readme-spotlight/internal/model"
)

const (
	svgW    = 740
	svgRowH = 26
)

// SVG renders the contributions as a styled card. Because GitHub serves it as
// an image it can carry colours, fonts and layout that sanitized README HTML
// cannot — at the cost of not being clickable (the hybrid format pairs it with
// a clickable list).
func SVG(contribs []model.Contribution, opt Options) string {
	items := prepare(contribs, opt)

	// Right-anchored x positions for the numeric columns.
	const (
		xRepo    = 24
		xStars   = 460
		xCommits = 524
		xPRs     = 588
		xIssues  = 652
		xTotal   = 716
	)
	const (
		titleY    = 42
		subY      = 63
		headY     = 92
		firstRowY = headY + 22
	)
	height := firstRowY + len(items)*svgRowH + 8

	total := 0
	for _, it := range items {
		total += it.Total()
	}
	title := opt.Title
	if title == "" {
		title = "Open-Source Contributions"
	}

	var b strings.Builder
	fmt.Fprintf(&b, `<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 %d %d" role="img">`, svgW, height, svgW, height)
	b.WriteString(`<style>` +
		`.t{font:600 17px system-ui,-apple-system,Segoe UI,sans-serif;fill:#e6edf3}` +
		`.s{font:12px system-ui,sans-serif;fill:#8b949e}` +
		`.h{font:600 10.5px ui-monospace,SFMono-Regular,Menlo,monospace;fill:#8b949e;letter-spacing:.05em}` +
		`.r{font:13px ui-monospace,SFMono-Regular,Menlo,monospace;fill:#58a6ff}` +
		`.n{font:13px ui-monospace,SFMono-Regular,Menlo,monospace;fill:#909dab}` +
		`.z{font:13px ui-monospace,SFMono-Regular,Menlo,monospace;fill:#484f58}` +
		`.tot{font:600 13px ui-monospace,SFMono-Regular,Menlo,monospace;fill:#e6edf3}` +
		`.star{font:13px ui-monospace,SFMono-Regular,Menlo,monospace;fill:#d29922}` +
		`</style>`)

	// Card background.
	fmt.Fprintf(&b, `<rect x="0.5" y="0.5" width="%d" height="%d" rx="10" fill="#0d1117" stroke="#30363d"/>`, svgW-1, height-1)

	// Title + subtitle.
	fmt.Fprintf(&b, `<text x="%d" y="%d" class="t">%s</text>`, xRepo, titleY, escXML(title))
	fmt.Fprintf(&b, `<text x="%d" y="%d" class="s">%d repositories · %d contributions</text>`, xRepo, subY, len(items), total)

	// Column headers.
	fmt.Fprintf(&b, `<text x="%d" y="%d" class="h">REPOSITORY</text>`, xRepo, headY)
	hdr := func(x int, s string) {
		fmt.Fprintf(&b, `<text x="%d" y="%d" class="h" text-anchor="end">%s</text>`, x, headY, s)
	}
	hdr(xStars, "★")
	hdr(xCommits, "COMMITS")
	hdr(xPRs, "PRS")
	hdr(xIssues, "ISSUES")
	hdr(xTotal, "TOTAL")
	fmt.Fprintf(&b, `<line x1="%d" y1="%d" x2="%d" y2="%d" stroke="#21262d"/>`, xRepo, headY+8, xTotal, headY+8)

	// Rows.
	for i, it := range items {
		y := firstRowY + i*svgRowH
		baseline := y + 17
		if i%2 == 1 {
			fmt.Fprintf(&b, `<rect x="12" y="%d" width="%d" height="%d" rx="5" fill="#ffffff08"/>`, y, svgW-24, svgRowH)
		}
		fmt.Fprintf(&b, `<text x="%d" y="%d" class="r">%s</text>`, xRepo, baseline, escXML(truncate(it.Repo, 42)))
		fmt.Fprintf(&b, `<text x="%d" y="%d" class="star" text-anchor="end">★ %s</text>`, xStars, baseline, formatStars(it.Stars))
		num := func(x, v int) {
			cls := "n"
			if v == 0 {
				cls = "z"
			}
			fmt.Fprintf(&b, `<text x="%d" y="%d" class="%s" text-anchor="end">%d</text>`, x, baseline, cls, v)
		}
		num(xCommits, it.Commits)
		num(xPRs, it.PRs)
		num(xIssues, it.Issues)
		fmt.Fprintf(&b, `<text x="%d" y="%d" class="tot" text-anchor="end">%d</text>`, xTotal, baseline, it.Total())
	}

	b.WriteString(`</svg>`)
	return b.String()
}

func escXML(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func truncate(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max-1]) + "…"
}
