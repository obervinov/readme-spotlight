// Package render turns collected contributions into the block that gets
// injected into a profile README.
package render

import (
	"fmt"
	"sort"

	"github.com/obervinov/readme-spotlight/internal/model"
)

// Format selects the block layout.
const (
	FormatTable   = "table"   // a Markdown table, one row per repository
	FormatDetails = "details" // collapsible <details> per repository, with linkable items
	FormatSVG     = "svg"     // a styled SVG card (pretty, not clickable)
	FormatHybrid  = "hybrid"  // SVG card + collapsible clickable list
)

// SVGAsset is the repo-relative filename the contributions SVG card is written to.
const SVGAsset = "assets/contributions.svg"

// Output is a rendered block plus any asset files it references (e.g. the SVG
// card). Assets is keyed by repo-relative path.
type Output struct {
	Block  string
	Assets map[string]string
}

// Columns controls which optional columns appear in the table format.
// Repository is always shown.
type Columns struct {
	Stars    bool
	Language bool
	Commits  bool
	PRs      bool
	Issues   bool
	Reviews  bool
	Total    bool
}

// DefaultColumns is a sensible starting set.
func DefaultColumns() Columns {
	return Columns{Stars: true, Commits: true, PRs: true, Issues: true, Total: true}
}

// Options configures a single render pass.
type Options struct {
	Title   string
	Format  string // FormatTable (default) or FormatDetails
	Columns Columns
	SortBy  string // "stars" (default) or "total"
	Limit   int    // 0 = no limit
}

// Render returns just the README block for the configured format. Use
// RenderOutput when asset files (the SVG card) also need to be published.
func Render(contribs []model.Contribution, opt Options) string {
	return RenderOutput(contribs, opt).Block
}

// RenderOutput builds the README block and any asset files for the format.
func RenderOutput(contribs []model.Contribution, opt Options) Output {
	switch opt.Format {
	case FormatDetails:
		return Output{Block: Details(contribs, opt)}
	case FormatSVG:
		svg := SVG(contribs, opt)
		return Output{Block: imgBlock(opt.Title), Assets: map[string]string{SVGAsset: svg}}
	case FormatHybrid:
		svg := SVG(contribs, opt)
		// SVG carries the title; the details list drops it to avoid duplication.
		listOpt := opt
		listOpt.Title = ""
		block := imgBlock(opt.Title) + "\n\n" + Details(contribs, listOpt)
		return Output{Block: block, Assets: map[string]string{SVGAsset: svg}}
	default:
		return Output{Block: Markdown(contribs, opt)}
	}
}

// imgBlock is the <img> embed for the SVG card, optionally under a Markdown
// heading. It is left-aligned so the card lines up flush with the expandable
// list rendered beneath it in the hybrid format.
func imgBlock(title string) string {
	var head string
	if title != "" {
		head = "### " + title + "\n\n"
	}
	return head + `<img src="./` + SVGAsset + `" alt="Open-source contributions">`
}

// prepare returns a sorted, limited copy of contribs per the options.
func prepare(contribs []model.Contribution, opt Options) []model.Contribution {
	items := make([]model.Contribution, len(contribs))
	copy(items, contribs)

	switch opt.SortBy {
	case "total":
		sort.SliceStable(items, func(i, j int) bool { return items[i].Total() > items[j].Total() })
	default: // "stars"
		sort.SliceStable(items, func(i, j int) bool { return items[i].Stars > items[j].Stars })
	}
	if opt.Limit > 0 && len(items) > opt.Limit {
		items = items[:opt.Limit]
	}
	return items
}

// formatStars renders large star counts compactly (1234 -> "1.2k").
func formatStars(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	}
	return fmt.Sprintf("%.1fk", float64(n)/1000)
}
