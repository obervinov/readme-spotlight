package config

import "testing"

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
