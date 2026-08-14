package model

import "testing"

// A pull request count alone cannot say whether code landed: a rejected pull
// request and one open for two and a half years both read as "1 PR".
func TestHasMergedCode(t *testing.T) {
	tests := []struct {
		name string
		c    Contribution
		want bool
	}{
		{"merged pull request", Contribution{PRs: 1, PRsMerged: 1}, true},
		{"commits on the default branch", Contribution{Commits: 4}, true},
		{"rejected pull request", Contribution{PRs: 1, PRsMerged: 0}, false},
		{"pull request still open", Contribution{PRs: 1, PRsMerged: 0, Issues: 1}, false},
		{"issues only", Contribution{Issues: 3}, false},
		{"reviews only", Contribution{Reviews: 2}, false},
		{"some of several pull requests merged", Contribution{PRs: 3, PRsMerged: 1}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.c.HasMergedCode(); got != tc.want {
				t.Fatalf("HasMergedCode() = %v, want %v for %+v", got, tc.want, tc.c)
			}
		})
	}
}

// PRsMerged is a subset of PRs, so counting it again would inflate the total.
func TestTotalDoesNotDoubleCountMergedPRs(t *testing.T) {
	c := Contribution{Commits: 2, PRs: 3, PRsMerged: 3, Issues: 1, Reviews: 1}
	if got := c.Total(); got != 7 {
		t.Fatalf("Total() = %d, want 7", got)
	}
}
