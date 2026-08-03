package config

import (
	"strings"
	"testing"

	"github.com/obervinov/readme-spotlight/internal/render"
)

func ptr[T any](v T) *T { return &v }

func TestApplyOnlyTouchesSuppliedFields(t *testing.T) {
	cfg := Default()
	cfg.TargetRepo = "obervinov/obervinov"

	patch := ContentPatch{Positioning: &render.PositioningConfig{Enabled: true, Text: "Platform engineer", Accent: "#3fb950"}}
	got, changed, err := patch.Apply(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if want := []string{"positioning"}; len(changed) != 1 || changed[0] != want[0] {
		t.Fatalf("changed = %v, want %v", changed, want)
	}
	if got.Positioning.Text != "Platform engineer" {
		t.Fatalf("positioning.text = %q, want the patched value", got.Positioning.Text)
	}
	if got.Banner.Name != cfg.Banner.Name {
		t.Fatalf("banner.name = %q, want it untouched", got.Banner.Name)
	}
	if got.TargetRepo != cfg.TargetRepo || got.PublishMode != cfg.PublishMode || got.Schedule != cfg.Schedule {
		t.Fatal("publishing fields must survive a content patch untouched")
	}
}

func TestApplyRejectsEmptyPatch(t *testing.T) {
	if _, _, err := (ContentPatch{}).Apply(Default()); err == nil {
		t.Fatal("want an error for a patch with no content fields")
	}
}

func TestApplyValidation(t *testing.T) {
	long := strings.Repeat("x", maxLongText+1)
	tests := []struct {
		name  string
		patch ContentPatch
	}{
		{"non-hex accent", ContentPatch{Banner: &render.BannerConfig{Accent: "red"}}},
		{"svg injection via accent", ContentPatch{Banner: &render.BannerConfig{Accent: `#fff" onload="alert(1)`}}},
		{"over-long tagline", ContentPatch{Banner: &render.BannerConfig{Accent: "#3fb950", Tagline: long}}},
		{"control character", ContentPatch{Title: ptr("Contributions\x00")}},
		{"unknown format", ContentPatch{Format: ptr("yaml")}},
		{"unknown sort", ContentPatch{SortBy: ptr("alphabetical")}},
		{"negative limit", ContentPatch{Limit: ptr(-1)}},
		{"limit above cap", ContentPatch{Limit: ptr(maxLimitRows + 1)}},
		{"too many focus items", ContentPatch{Focus: &render.FocusConfig{Accent: "#3fb950", Items: make([]render.FocusItem, maxFocusItem+1)}}},
		{"too many tech groups", ContentPatch{Tech: &render.TechConfig{Accent: "#3fb950", Groups: make([]render.TechGroup, maxTechGroup+1)}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := tc.patch.Apply(Default()); err == nil {
				t.Fatalf("want a validation error for %s", tc.name)
			}
		})
	}
}

func TestApplyAcceptsValidContent(t *testing.T) {
	patch := ContentPatch{
		Banner: &render.BannerConfig{Enabled: true, Name: "Oleg", Role: "Platform Engineer", Tagline: "Multi-line\ntagline\tok", Accent: "#3FB950"},
		Tech:   &render.TechConfig{Enabled: true, Title: "Technology & Tools", Accent: "", Groups: []render.TechGroup{{Name: "Languages", Items: []string{"Go", "Python"}}}},
		Format: ptr(render.FormatHybrid),
		SortBy: ptr("total"),
		Limit:  ptr(0),
	}
	got, changed, err := patch.Apply(Default())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(changed) != 5 {
		t.Fatalf("changed = %v, want all five supplied fields", changed)
	}
	if got.Tech.Groups[0].Items[1] != "Python" {
		t.Fatal("tech groups were not replaced")
	}
}

func TestContentOfProjectsSubset(t *testing.T) {
	cfg := Default()
	cfg.Title = "Open-Source Contributions"
	c := ContentOf(cfg)
	if c.Title != cfg.Title || c.Format != cfg.Format || c.Banner.Name != cfg.Banner.Name {
		t.Fatal("ContentOf must copy the content fields verbatim")
	}
}
