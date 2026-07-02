package render

import (
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
