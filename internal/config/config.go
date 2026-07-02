// Package config holds the user-editable configuration for a running instance.
// Everything here is persisted in the database except the GitHub token, which
// is read from the GITHUB_TOKEN environment variable so a PAT never lands in
// storage.
package config

import (
	"strings"

	"github.com/obervinov/readme-spotlight/internal/render"
)

// Config is the full editable configuration of an instance.
type Config struct {
	TargetRepo   string `json:"target_repo"`   // "owner/name" of the profile repo to update
	TargetBranch string `json:"target_branch"` // branch the README lives on
	ReadmePath   string `json:"readme_path"`   // path to the README within the repo
	MarkerStart  string `json:"marker_start"`  // HTML comment marking the start of the managed region
	MarkerEnd    string `json:"marker_end"`    // HTML comment marking the end
	Schedule     string `json:"schedule"`      // cron spec for the refresh job
	PublishMode  string `json:"publish_mode"`  // "pr" or "commit"
	PRBranch     string `json:"pr_branch"`     // head branch used in "pr" mode

	// Sections are rendered into the managed region in a fixed order:
	// banner, focus, then contributions. (More sections land here as they ship.)
	Banner      render.BannerConfig      `json:"banner"`
	Positioning render.PositioningConfig `json:"positioning"`
	Focus       render.FocusConfig       `json:"focus"`
	Tech        render.TechConfig        `json:"tech"`

	// Contributions section settings.
	Title   string         `json:"title"` // heading above the contributions block
	Format  string         `json:"format"`
	Columns render.Columns `json:"columns"` // only used by the table format
	SortBy  string         `json:"sort_by"` // "stars" or "total"
	Limit   int            `json:"limit"`   // max rows, 0 = all
}

// Default returns a ready-to-run configuration with sensible values.
func Default() Config {
	return Config{
		TargetBranch: "main",
		ReadmePath:   "README.md",
		MarkerStart:  "<!--SPOTLIGHT:START-->",
		MarkerEnd:    "<!--SPOTLIGHT:END-->",
		Schedule:     "0 6 * * *", // daily at 06:00
		PublishMode:  "pr",
		PRBranch:     "readme-spotlight/update",

		Banner:      render.DefaultBanner(),
		Positioning: render.DefaultPositioning(),
		Focus:       render.DefaultFocus(),
		Tech:        render.DefaultTech(),

		Title:   "Open-Source Contributions",
		Format:  render.FormatHybrid,
		Columns: render.DefaultColumns(),
		SortBy:  "stars",
		Limit:   0,
	}
}

// FocusText renders the focus items as editable "Title | description" lines for
// the web form.
func (c Config) FocusText() string {
	var b strings.Builder
	for _, it := range c.Focus.Items {
		b.WriteString(it.Title)
		if it.Text != "" {
			b.WriteString(" | ")
			b.WriteString(it.Text)
		}
		b.WriteString("\n")
	}
	return b.String()
}

// ParseFocusItems parses "Title | description" lines back into focus items.
func ParseFocusItems(text string) []render.FocusItem {
	var items []render.FocusItem
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		title, desc, _ := strings.Cut(line, "|")
		items = append(items, render.FocusItem{
			Title: strings.TrimSpace(title),
			Text:  strings.TrimSpace(desc),
		})
	}
	return items
}

// TechText renders the tech groups as editable "Group: item, item" lines.
func (c Config) TechText() string {
	var b strings.Builder
	for _, g := range c.Tech.Groups {
		b.WriteString(g.Name)
		b.WriteString(": ")
		b.WriteString(strings.Join(g.Items, ", "))
		b.WriteString("\n")
	}
	return b.String()
}

// ParseTechGroups parses "Group: item, item" lines back into tech groups.
func ParseTechGroups(text string) []render.TechGroup {
	var groups []render.TechGroup
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		name, rest, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		var items []string
		for _, it := range strings.Split(rest, ",") {
			if s := strings.TrimSpace(it); s != "" {
				items = append(items, s)
			}
		}
		groups = append(groups, render.TechGroup{Name: strings.TrimSpace(name), Items: items})
	}
	return groups
}

// RenderOptions projects the config onto the renderer's option struct.
func (c Config) RenderOptions() render.Options {
	return render.Options{
		Title:   c.Title,
		Format:  c.Format,
		Columns: c.Columns,
		SortBy:  c.SortBy,
		Limit:   c.Limit,
	}
}
