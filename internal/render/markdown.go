package render

import (
	"fmt"
	"strings"

	"github.com/obervinov/readme-spotlight/internal/model"
)

// Markdown renders the contributions as a heading plus a Markdown table. It
// returns only the block body; marker comments are added by the publisher.
func Markdown(contribs []model.Contribution, opt Options) string {
	items := prepare(contribs, opt)
	c := opt.Columns

	var head, sep []string
	head = append(head, "Repository")
	sep = append(sep, "---")
	col := func(on bool, name, align string) {
		if on {
			head = append(head, name)
			sep = append(sep, align)
		}
	}
	col(c.Language, "Language", ":--")
	col(c.Stars, "★", "--:")
	col(c.Commits, "Commits", "--:")
	col(c.PRs, "PRs", "--:")
	col(c.Issues, "Issues", "--:")
	col(c.Reviews, "Reviews", "--:")
	col(c.Total, "Total", "--:")

	var b strings.Builder
	if opt.Title != "" {
		fmt.Fprintf(&b, "### %s\n\n", opt.Title)
	}
	fmt.Fprintf(&b, "| %s |\n", strings.Join(head, " | "))
	fmt.Fprintf(&b, "| %s |\n", strings.Join(sep, " | "))

	for _, it := range items {
		cells := []string{fmt.Sprintf("[%s](https://github.com/%s)", it.Repo, it.Repo)}
		if c.Language {
			cells = append(cells, it.Language)
		}
		if c.Stars {
			cells = append(cells, formatStars(it.Stars))
		}
		if c.Commits {
			cells = append(cells, fmt.Sprintf("%d", it.Commits))
		}
		if c.PRs {
			cells = append(cells, fmt.Sprintf("%d", it.PRs))
		}
		if c.Issues {
			cells = append(cells, fmt.Sprintf("%d", it.Issues))
		}
		if c.Reviews {
			cells = append(cells, fmt.Sprintf("%d", it.Reviews))
		}
		if c.Total {
			cells = append(cells, fmt.Sprintf("%d", it.Total()))
		}
		fmt.Fprintf(&b, "| %s |\n", strings.Join(cells, " | "))
	}
	return b.String()
}
