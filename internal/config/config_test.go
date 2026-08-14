package config

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/robfig/cron/v3"
)

// The refresh runs weekly: contributions move on the scale of weeks, and a
// daily run mostly rewrote the same file.
func TestDefaultScheduleIsWeekly(t *testing.T) {
	spec := Default().Schedule
	sched, err := cron.ParseStandard(spec)
	if err != nil {
		t.Fatalf("default schedule %q is not a valid cron spec: %v", spec, err)
	}
	first := sched.Next(time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC))
	if gap := sched.Next(first).Sub(first); gap != 7*24*time.Hour {
		t.Fatalf("default schedule %q fires every %v, want once a week", spec, gap)
	}
}

// A stored configuration written before these options existed must keep working:
// the new flags decode to their off state.
func TestConfigFromOlderVersionKeepsWorking(t *testing.T) {
	const stored = `{"target_repo":"obervinov/obervinov","schedule":"0 6 * * *","sort_by":"total","format":"hybrid"}`
	var cfg Config
	if err := json.Unmarshal([]byte(stored), &cfg); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if cfg.GroupMerged || cfg.SkipStarOnlyChanges {
		t.Fatalf("new options must default to off, got group_merged=%v skip_star_only_changes=%v",
			cfg.GroupMerged, cfg.SkipStarOnlyChanges)
	}
	if cfg.Schedule != "0 6 * * *" || cfg.SortBy != "total" {
		t.Fatal("an existing configuration must survive the upgrade unchanged")
	}
}

func TestParseFocusItems(t *testing.T) {
	items := ParseFocusItems("Platforms | golden paths\nJust a title\n\n  Spaced | with desc  ")
	if len(items) != 3 {
		t.Fatalf("expected 3 items, got %d: %+v", len(items), items)
	}
	if items[0].Title != "Platforms" || items[0].Text != "golden paths" {
		t.Errorf("bad first item: %+v", items[0])
	}
	if items[1].Title != "Just a title" || items[1].Text != "" {
		t.Errorf("title-only item should have empty text: %+v", items[1])
	}
	if items[2].Title != "Spaced" || items[2].Text != "with desc" {
		t.Errorf("item fields should be trimmed: %+v", items[2])
	}
}

func TestParseTechGroups(t *testing.T) {
	groups := ParseTechGroups("Cloud: AWS, Kubernetes , Docker\nLangs: Go")
	if len(groups) != 2 {
		t.Fatalf("expected 2 groups, got %d: %+v", len(groups), groups)
	}
	if groups[0].Name != "Cloud" || len(groups[0].Items) != 3 || groups[0].Items[1] != "Kubernetes" {
		t.Errorf("bad first group: %+v", groups[0])
	}
	if groups[1].Name != "Langs" || len(groups[1].Items) != 1 {
		t.Errorf("bad second group: %+v", groups[1])
	}
}

func TestFocusTextRoundTrip(t *testing.T) {
	c := Default()
	got := ParseFocusItems(c.FocusText())
	if len(got) != len(c.Focus.Items) {
		t.Fatalf("round trip changed item count: %d -> %d", len(c.Focus.Items), len(got))
	}
	for i := range got {
		if got[i] != c.Focus.Items[i] {
			t.Errorf("item %d changed: %+v -> %+v", i, c.Focus.Items[i], got[i])
		}
	}
}
