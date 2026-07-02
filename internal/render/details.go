package render

import (
	"fmt"
	"strings"

	"github.com/obervinov/readme-spotlight/internal/model"
)

// Details renders each repository as a collapsible <details> block whose summary
// carries the headline counts and whose body lists the individual pull requests,
// issues and reviews as clickable links. GitHub renders <details>/<summary> and
// the Markdown inside them in profile READMEs.
func Details(contribs []model.Contribution, opt Options) string {
	items := prepare(contribs, opt)

	var b strings.Builder
	if opt.Title != "" {
		fmt.Fprintf(&b, "### %s\n\n", opt.Title)
	}

	for _, it := range items {
		fmt.Fprintf(&b, "<details>\n<summary>%s</summary>\n\n", summaryLine(it))
		for _, item := range it.Items {
			fmt.Fprintf(&b, "- %s [%s](%s)\n", itemLabel(item), escapeText(item.Title), item.URL)
		}
		if it.Commits > 0 && it.CommitsURL != "" {
			fmt.Fprintf(&b, "- 📝 [%d commit%s](%s)\n", it.Commits, plural(it.Commits), it.CommitsURL)
		}
		b.WriteString("</details>\n\n")
	}
	return strings.TrimRight(b.String(), "\n") + "\n"
}

// summaryLine builds the collapsed one-liner: repo name, stars and a count digest.
func summaryLine(c model.Contribution) string {
	var parts []string
	if c.Stars > 0 {
		parts = append(parts, "★ "+formatStars(c.Stars))
	}
	if c.Commits > 0 {
		parts = append(parts, fmt.Sprintf("%d commit%s", c.Commits, plural(c.Commits)))
	}
	if c.PRs > 0 {
		parts = append(parts, fmt.Sprintf("%d PR%s", c.PRs, plural(c.PRs)))
	}
	if c.Issues > 0 {
		parts = append(parts, fmt.Sprintf("%d issue%s", c.Issues, plural(c.Issues)))
	}
	if c.Reviews > 0 {
		parts = append(parts, fmt.Sprintf("%d review%s", c.Reviews, plural(c.Reviews)))
	}
	digest := strings.Join(parts, " · ")
	line := fmt.Sprintf("<b><a href=\"https://github.com/%s\">%s</a></b>", c.Repo, c.Repo)
	if digest != "" {
		line += " — " + digest
	}
	return line
}

func itemLabel(i model.Item) string {
	switch i.Kind {
	case "pr":
		if i.State == "MERGED" {
			return "🔀"
		}
		return "🔃"
	case "issue":
		return "🐛"
	case "review":
		return "👀"
	default:
		return "•"
	}
}

// escapeText keeps issue/PR titles from breaking the Markdown link syntax.
func escapeText(s string) string {
	s = strings.ReplaceAll(s, "]", "\\]")
	s = strings.ReplaceAll(s, "[", "\\[")
	return s
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
