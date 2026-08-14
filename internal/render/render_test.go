package render

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/obervinov/readme-spotlight/internal/model"
)

func TestFormatStars(t *testing.T) {
	cases := map[int]string{0: "0", 999: "999", 1000: "1.0k", 23300: "23.3k"}
	for in, want := range cases {
		if got := formatStars(in); got != want {
			t.Errorf("formatStars(%d) = %q, want %q", in, got, want)
		}
	}
}

func TestWrapTextHonorsLineBudget(t *testing.T) {
	lines := wrapText("one two three four five six seven eight", 12, 2)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %v", len(lines), lines)
	}
	if !strings.HasSuffix(lines[len(lines)-1], "…") {
		t.Errorf("dropped content should be marked with an ellipsis, got %q", lines[1])
	}
}

// acceptanceSet mirrors the real contribution footprint the grouping has to get
// right. The two interesting rows are subzeroid/instagrapi — the most-starred
// repository of the lot, but the pull request was rejected — and derailed/popeye
// and hashicorp/terraform-helm, whose pull requests are still open. All three
// are filed issues, not landed code, however large their star counts.
func acceptanceSet() []model.Contribution {
	return []model.Contribution{
		{Repo: "subzeroid/instagrapi", Stars: 23900, PRs: 1, PRsMerged: 0, Issues: 1},
		{Repo: "locustbaby/trivy-ui", Stars: 41, PRs: 2, PRsMerged: 2, Commits: 6},
		{Repo: "derailed/popeye", Stars: 5800, PRs: 1, PRsMerged: 0},
		{Repo: "eitchtee/WYGIWYH", Stars: 890, PRs: 3, PRsMerged: 1, Commits: 12, Issues: 4},
		{Repo: "hashicorp/terraform-helm", Stars: 1100, PRs: 1, PRsMerged: 0, Issues: 2},
		{Repo: "a-earthperson/rxresume-mcp", Stars: 6, PRs: 1, PRsMerged: 1},
		{Repo: "cloudposse/terraform-aws-backup", Stars: 118, PRs: 1, PRsMerged: 1, Commits: 2},
		{Repo: "ncecere/terraform-provider-litellm", Stars: 61, PRs: 1, PRsMerged: 1},
		{Repo: "jaegertracing/helm-charts", Stars: 319, PRs: 2, PRsMerged: 2, Commits: 3},
		{Repo: "someone/reviewed-only", Stars: 12, Reviews: 1},
	}
}

func repoNames(items []model.Contribution) []string {
	out := make([]string, len(items))
	for i, it := range items {
		out[i] = it.Repo
	}
	return out
}

func TestGroupMergedSplitsLandedCodeFromFiledIssues(t *testing.T) {
	// SortBy is the live configuration's "total", which is exactly what put a
	// 41★ repository above a 23.9k★ one: inside a group, stars order the rows.
	got := groups(acceptanceSet(), Options{GroupMerged: true, SortBy: "total"})
	if len(got) != 2 {
		t.Fatalf("got %d groups, want 2", len(got))
	}

	if got[0].Title != GroupMergedTitle || got[1].Title != GroupIssuesTitle {
		t.Fatalf("group titles = %q, %q — merged code comes first", got[0].Title, got[1].Title)
	}
	wantMerged := []string{
		"eitchtee/WYGIWYH",
		"jaegertracing/helm-charts",
		"cloudposse/terraform-aws-backup",
		"ncecere/terraform-provider-litellm",
		"locustbaby/trivy-ui",
		"a-earthperson/rxresume-mcp",
	}
	if diff := equalSlices(repoNames(got[0].Items), wantMerged); diff != "" {
		t.Errorf("merged group: %s", diff)
	}
	wantFiled := []string{
		"subzeroid/instagrapi",
		"derailed/popeye",
		"hashicorp/terraform-helm",
		"someone/reviewed-only",
	}
	if diff := equalSlices(repoNames(got[1].Items), wantFiled); diff != "" {
		t.Errorf("issues group: %s", diff)
	}
}

// Grouping is a label, not a filter.
func TestGroupMergedDropsNothing(t *testing.T) {
	set := acceptanceSet()
	total := 0
	for _, g := range groups(set, Options{GroupMerged: true}) {
		total += len(g.Items)
	}
	if total != len(set) {
		t.Fatalf("groups hold %d repositories, want all %d", total, len(set))
	}
}

func TestGroupMergedOffKeepsOneFlatList(t *testing.T) {
	set := acceptanceSet()
	got := groups(set, Options{SortBy: "stars"})
	if len(got) != 1 || got[0].Title != "" {
		t.Fatalf("ungrouped render must stay a single untitled list, got %d group(s)", len(got))
	}
	if got[0].Items[0].Repo != "subzeroid/instagrapi" {
		t.Errorf("flat list should still be star-sorted, first = %q", got[0].Items[0].Repo)
	}
	if len(got[0].Items) != len(set) {
		t.Errorf("flat list holds %d repositories, want %d", len(got[0].Items), len(set))
	}
}

func TestGroupMergedAppliesLimitPerGroup(t *testing.T) {
	got := groups(acceptanceSet(), Options{GroupMerged: true, Limit: 2})
	for _, g := range got {
		if len(g.Items) != 2 {
			t.Errorf("group %q has %d rows, want the limit of 2", g.Title, len(g.Items))
		}
	}
}

// An empty group would render as a heading over nothing.
func TestGroupMergedSkipsEmptyGroups(t *testing.T) {
	only := []model.Contribution{{Repo: "acme/widget", Stars: 3, Issues: 1}}
	got := groups(only, Options{GroupMerged: true})
	if len(got) != 1 || got[0].Title != GroupIssuesTitle {
		t.Fatalf("got %d group(s), want only %q", len(got), GroupIssuesTitle)
	}
}

func TestFormatsRenderBothGroups(t *testing.T) {
	set := acceptanceSet()
	formats := map[string]func(Options) string{
		"details": func(o Options) string { return Details(set, o) },
		"table":   func(o Options) string { return Markdown(set, o) },
		"svg":     func(o Options) string { return SVG(set, o) },
		"hybrid": func(o Options) string {
			o.Format = FormatHybrid
			out := RenderOutput(set, o)
			return out.Block + out.Assets[SVGAsset]
		},
	}
	for name, render := range formats {
		t.Run(name, func(t *testing.T) {
			opt := Options{GroupMerged: true, Columns: DefaultColumns()}
			out := render(opt)
			for _, title := range []string{GroupMergedTitle, GroupIssuesTitle} {
				// The SVG card sets its headings in the same upper case as its
				// column headers.
				if !strings.Contains(out, title) && !strings.Contains(out, strings.ToUpper(title)) {
					t.Errorf("%s output is missing the %q group heading", name, title)
				}
			}
			for _, c := range set {
				if !strings.Contains(out, truncate(c.Repo, 42)) {
					t.Errorf("%s output dropped %s", name, c.Repo)
				}
			}
			if flat := render(Options{Columns: DefaultColumns()}); strings.Contains(flat, GroupMergedTitle) {
				t.Errorf("%s output shows a group heading with grouping off", name)
			}
		})
	}
}

// The card is sized from its rows, so the group headings have to be counted too
// or the last repository falls outside the viewBox.
func TestSVGHeightCoversGroupHeadings(t *testing.T) {
	set := acceptanceSet()
	grouped := SVG(set, Options{GroupMerged: true})
	flat := SVG(set, Options{})

	want := 2 * svgRowH // one heading row per group
	if got := svgHeight(t, grouped) - svgHeight(t, flat); got != want {
		t.Fatalf("grouped card is %dpx taller, want %dpx for the two headings", got, want)
	}
}

func svgHeight(t *testing.T, svg string) int {
	t.Helper()
	_, rest, ok := strings.Cut(svg, `height="`)
	if !ok {
		t.Fatal("SVG has no height attribute")
	}
	raw, _, _ := strings.Cut(rest, `"`)
	n, err := strconv.Atoi(raw)
	if err != nil {
		t.Fatalf("height %q is not a number: %v", raw, err)
	}
	return n
}

func equalSlices(got, want []string) string {
	if len(got) == len(want) {
		same := true
		for i := range got {
			if got[i] != want[i] {
				same = false
				break
			}
		}
		if same {
			return ""
		}
	}
	return fmt.Sprintf("got %v, want %v", got, want)
}

func TestHybridProducesSVGAsset(t *testing.T) {
	contribs := []model.Contribution{{Repo: "acme/widget", Owner: "acme", Stars: 12, Issues: 1}}
	out := RenderOutput(contribs, Options{Format: FormatHybrid, Title: "Contributions"})
	if _, ok := out.Assets[SVGAsset]; !ok {
		t.Fatalf("hybrid format did not emit the SVG asset")
	}
	if !strings.Contains(out.Block, SVGAsset) {
		t.Errorf("hybrid block should embed the SVG asset path")
	}
	if !strings.Contains(out.Block, "acme/widget") {
		t.Errorf("hybrid block should list the repository in the expandable part")
	}
}
