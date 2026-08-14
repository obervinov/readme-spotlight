package publish

import (
	"context"
	"strings"
	"testing"

	"github.com/obervinov/readme-spotlight/internal/config"
	"github.com/obervinov/readme-spotlight/internal/github"
)

func TestMergeReplacesBetweenMarkers(t *testing.T) {
	const (
		start = "<!--S-->"
		end   = "<!--E-->"
	)
	content := "intro\n" + start + "\nold body\n" + end + "\noutro\n"
	got := Merge(content, start, end, "NEW")

	want := "intro\n" + start + "\nNEW\n" + end + "\noutro\n"
	if got != want {
		t.Fatalf("merge replaced wrong region:\n got: %q\nwant: %q", got, want)
	}
}

func TestMergeAppendsWhenMarkersMissing(t *testing.T) {
	const (
		start = "<!--S-->"
		end   = "<!--E-->"
	)
	got := Merge("hello\n", start, end, "BODY")

	want := "hello\n\n" + start + "\nBODY\n" + end + "\n"
	if got != want {
		t.Fatalf("merge did not append cleanly:\n got: %q\nwant: %q", got, want)
	}
}

func TestMergeIsIdempotent(t *testing.T) {
	const (
		start = "<!--S-->"
		end   = "<!--E-->"
	)
	once := Merge("readme\n", start, end, "BODY")
	twice := Merge(once, start, end, "BODY")
	if once != twice {
		t.Fatalf("merge not idempotent:\n once:  %q\n twice: %q", once, twice)
	}
}

// fakeAPI is a repository whose files live in a map. It records every commit so
// a test can assert how many a run produced and which paths each one carried.
type fakeAPI struct {
	files   map[string]string
	commits []map[string]string
}

func (f *fakeAPI) GetFileMaybe(_ context.Context, _, _, path, _ string) (github.File, bool, error) {
	content, ok := f.files[path]
	if !ok {
		return github.File{}, false, nil
	}
	return github.File{Content: content, SHA: "sha-" + path}, true, nil
}

func (f *fakeAPI) CommitFiles(_ context.Context, _, _, _, _ string, files map[string]string) (string, bool, error) {
	changed := false
	snapshot := map[string]string{}
	for path, content := range files {
		snapshot[path] = content
		if f.files[path] != content {
			changed = true
		}
		f.files[path] = content
	}
	if !changed {
		return "", false, nil
	}
	f.commits = append(f.commits, snapshot)
	return "0123456789abcdef", true, nil
}

func (f *fakeAPI) BranchSHA(context.Context, string, string, string) (string, error) {
	return "base-sha", nil
}
func (f *fakeAPI) EnsureBranch(context.Context, string, string, string, string) error { return nil }
func (f *fakeAPI) FindOpenPR(context.Context, string, string, string, string) (string, error) {
	return "", nil
}
func (f *fakeAPI) CreatePR(context.Context, string, string, string, string, string, string) (string, error) {
	return "https://github.com/o/r/pull/1", nil
}

func testConfig() config.Config {
	cfg := config.Default()
	cfg.TargetRepo = "o/r"
	cfg.PublishMode = "commit"
	return cfg
}

const svgPath = "assets/contributions.svg"

// The contents API commits one file at a time, which left a README commit and a
// separate SVG commit in the profile's history for every run.
func TestPublishWritesOneCommitForEveryPath(t *testing.T) {
	cfg := testConfig()
	gh := &fakeAPI{files: map[string]string{cfg.ReadmePath: "# profile\n"}}

	res, err := Publish(context.Background(), gh, cfg, "BLOCK", map[string]string{svgPath: "<svg>a</svg>"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Changed {
		t.Fatal("first publish should report a change")
	}
	if len(gh.commits) != 1 {
		t.Fatalf("commits = %d, want exactly 1 covering every changed path", len(gh.commits))
	}
	got := gh.commits[0]
	if _, ok := got[svgPath]; !ok {
		t.Errorf("the commit does not carry %s", svgPath)
	}
	if !strings.Contains(got[cfg.ReadmePath], "BLOCK") {
		t.Errorf("the commit does not carry the rendered block: %q", got[cfg.ReadmePath])
	}
}

func TestPublishSkipsCommitWhenNothingChanged(t *testing.T) {
	cfg := testConfig()
	gh := &fakeAPI{files: map[string]string{cfg.ReadmePath: "# profile\n"}}
	assets := map[string]string{svgPath: "<svg>a</svg>"}

	if _, err := Publish(context.Background(), gh, cfg, "BLOCK", assets); err != nil {
		t.Fatalf("first publish: %v", err)
	}
	res, err := Publish(context.Background(), gh, cfg, "BLOCK", assets)
	if err != nil {
		t.Fatalf("second publish: %v", err)
	}
	if res.Changed {
		t.Error("a byte-identical run must not report a change")
	}
	if len(gh.commits) != 1 {
		t.Fatalf("commits = %d, want the second run to add none", len(gh.commits))
	}
}

// Third-party star counts drift on their own — a repository went 40★ to 41★
// between two runs — so the plain guard still fires on star-only churn. The
// suppression is opt-in, and off it must keep committing.
func TestPublishStarOnlyChange(t *testing.T) {
	const (
		before = "<svg>★ 40 acme/widget</svg>"
		after  = "<svg>★ 41 acme/widget</svg>"
	)
	tests := []struct {
		name       string
		skip       bool
		wantCommit int
		wantChange bool
	}{
		{"off by default: star churn is published", false, 1, true},
		{"opt-in: star churn is skipped", true, 0, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg := testConfig()
			cfg.SkipStarOnlyChanges = tc.skip
			gh := &fakeAPI{files: map[string]string{cfg.ReadmePath: "# profile\n"}}
			if _, err := Publish(context.Background(), gh, cfg, "BLOCK", map[string]string{svgPath: before}); err != nil {
				t.Fatalf("seed publish: %v", err)
			}
			gh.commits = nil

			res, err := Publish(context.Background(), gh, cfg, "BLOCK", map[string]string{svgPath: after})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(gh.commits) != tc.wantCommit {
				t.Errorf("commits = %d, want %d", len(gh.commits), tc.wantCommit)
			}
			if res.Changed != tc.wantChange {
				t.Errorf("changed = %v, want %v", res.Changed, tc.wantChange)
			}
			if !tc.skip {
				return
			}
			if gh.files[svgPath] != before {
				t.Errorf("a skipped run must leave the branch untouched, got %q", gh.files[svgPath])
			}
		})
	}
}

// Suppression is about stars only: a real edit that happens to arrive in the
// same run as star churn must still be published.
func TestPublishStarSuppressionKeepsRealChanges(t *testing.T) {
	cfg := testConfig()
	cfg.SkipStarOnlyChanges = true
	gh := &fakeAPI{files: map[string]string{cfg.ReadmePath: "# profile\n"}}
	if _, err := Publish(context.Background(), gh, cfg, "BLOCK", map[string]string{svgPath: "<svg>★ 40 acme/widget</svg>"}); err != nil {
		t.Fatalf("seed publish: %v", err)
	}
	gh.commits = nil

	res, err := Publish(context.Background(), gh, cfg, "BLOCK", map[string]string{svgPath: "<svg>★ 41 acme/gadget</svg>"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Changed || len(gh.commits) != 1 {
		t.Fatalf("changed = %v, commits = %d — a renamed repository is not star churn", res.Changed, len(gh.commits))
	}
}

func TestOnlyStarsMovedHandlesEveryStarFormat(t *testing.T) {
	files := map[string]string{
		"details": "★ 23.9k · 3 commits",
		"svg":     `<text class="star">★ 1.2k</text>`,
	}
	current := map[string]string{
		"details": "★ 23.8k · 3 commits",
		"svg":     `<text class="star">★ 1.1k</text>`,
	}
	if !onlyStarsMoved([]string{"details", "svg"}, files, current) {
		t.Error("abbreviated star counts should be recognised as star churn")
	}
	current["details"] = "★ 23.8k · 4 commits"
	if onlyStarsMoved([]string{"details", "svg"}, files, current) {
		t.Error("a changed commit count is not star churn")
	}
	if onlyStarsMoved(nil, files, current) {
		t.Error("no changed paths is not a star-only change")
	}
}
