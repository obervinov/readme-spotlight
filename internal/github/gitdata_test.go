package github

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeGit is a stub of the git data API. baseTree is what the branch points at;
// treeSHA is what creating a tree returns, so a test can make the new tree
// identical to the base one.
type fakeGit struct {
	baseTree string
	treeSHA  string

	calls    []string       // "METHOD /path", in order
	treeBody map[string]any // the body of the last create-tree call
	refBody  map[string]any // the body of the last ref update
}

func (f *fakeGit) server(t *testing.T) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.calls = append(f.calls, r.Method+" "+r.URL.Path)
		body := map[string]any{}
		if raw, _ := io.ReadAll(r.Body); len(raw) > 0 {
			if err := json.Unmarshal(raw, &body); err != nil {
				t.Errorf("decode %s %s: %v", r.Method, r.URL.Path, err)
			}
		}
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/ref/heads/main":
			_, _ = io.WriteString(w, `{"object":{"sha":"head-sha"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/git/commits/head-sha":
			_, _ = io.WriteString(w, `{"tree":{"sha":"`+f.baseTree+`"}}`)
		case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/git/trees":
			f.treeBody = body
			_, _ = io.WriteString(w, `{"sha":"`+f.treeSHA+`"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/git/commits":
			_, _ = io.WriteString(w, `{"sha":"new-commit-sha"}`)
		case r.Method == http.MethodPatch && r.URL.Path == "/repos/o/r/git/refs/heads/main":
			f.refBody = body
			_, _ = io.WriteString(w, `{"object":{"sha":"new-commit-sha"}}`)
		default:
			t.Errorf("unexpected call %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)

	c := New("test-token")
	c.restBase = srv.URL
	return c
}

func (f *fakeGit) count(method, path string) int {
	n := 0
	for _, c := range f.calls {
		if c == method+" "+path {
			n++
		}
	}
	return n
}

// Every changed path belongs to the same commit. Writing them one at a time
// through the contents API is what left a README commit and an SVG commit side
// by side in the profile's history for a single run.
func TestCommitFilesWritesEverythingInOneCommit(t *testing.T) {
	f := &fakeGit{baseTree: "base-tree", treeSHA: "new-tree"}
	c := f.server(t)

	files := map[string]string{
		"README.md":                "# profile\n",
		"assets/contributions.svg": "<svg/>",
		"assets/banner.svg":        "<svg/>",
	}
	sha, changed, err := c.CommitFiles(t.Context(), "o", "r", "main", "msg", files)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !changed || sha != "new-commit-sha" {
		t.Fatalf("changed = %v, sha = %q, want true and the new commit", changed, sha)
	}
	if n := f.count(http.MethodPost, "/repos/o/r/git/commits"); n != 1 {
		t.Fatalf("created %d commits, want exactly 1", n)
	}
	if n := f.count(http.MethodPatch, "/repos/o/r/git/refs/heads/main"); n != 1 {
		t.Fatalf("updated the branch ref %d times, want 1", n)
	}

	entries, ok := f.treeBody["tree"].([]any)
	if !ok || len(entries) != len(files) {
		t.Fatalf("tree carried %v entries, want all %d files", f.treeBody["tree"], len(files))
	}
	if f.treeBody["base_tree"] != "base-tree" {
		t.Errorf("base_tree = %v, want the branch's current tree", f.treeBody["base_tree"])
	}
	if f.refBody["force"] != false {
		t.Errorf("force = %v — a ref that moved under us must fail, not overwrite", f.refBody["force"])
	}
}

// Trees are content-addressed, so identical bytes for every path produce the
// branch's own tree SHA. That is the run that must not commit at all.
func TestCommitFilesSkipsAnUnchangedTree(t *testing.T) {
	f := &fakeGit{baseTree: "same-tree", treeSHA: "same-tree"}
	c := f.server(t)

	sha, changed, err := c.CommitFiles(t.Context(), "o", "r", "main", "msg", map[string]string{"README.md": "# profile\n"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if changed || sha != "" {
		t.Fatalf("changed = %v, sha = %q, want no commit", changed, sha)
	}
	if n := f.count(http.MethodPost, "/repos/o/r/git/commits"); n != 0 {
		t.Errorf("created %d commits, want none", n)
	}
	if n := f.count(http.MethodPatch, "/repos/o/r/git/refs/heads/main"); n != 0 {
		t.Errorf("moved the branch ref %d times, want none", n)
	}
}

func TestCommitFilesWithoutFilesDoesNothing(t *testing.T) {
	f := &fakeGit{baseTree: "base-tree", treeSHA: "new-tree"}
	c := f.server(t)

	if _, changed, err := c.CommitFiles(t.Context(), "o", "r", "main", "msg", nil); err != nil || changed {
		t.Fatalf("changed = %v, err = %v, want no work for an empty file set", changed, err)
	}
	if len(f.calls) != 0 {
		t.Fatalf("made %v, want no API calls at all", f.calls)
	}
}
