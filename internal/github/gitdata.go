package github

import (
	"context"
	"fmt"
	"net/http"
	"sort"
)

// Writing through the contents API (PutFile) creates one commit per file, so a
// run that refreshes the README and its SVG cards produced a commit per file,
// all with the same message, in the profile's history. The git data API builds
// one tree from every changed path and hangs a single commit off it instead.

// blobMode is the git file mode for a regular non-executable file.
const blobMode = "100644"

// CommitFiles writes every file in files (keyed by repo-relative path) to branch
// in a single commit and returns the new commit SHA.
//
// It reports changed=false and writes nothing when the resulting tree is the one
// the branch already points at. Git trees are content-addressed, so that
// comparison is exact: identical bytes for every path yield the identical tree
// SHA, which is the API-level equivalent of `git diff --cached --quiet`.
func (c *Client) CommitFiles(ctx context.Context, owner, repo, branch, message string, files map[string]string) (sha string, changed bool, err error) {
	if len(files) == 0 {
		return "", false, nil
	}

	headSHA, err := c.BranchSHA(ctx, owner, repo, branch)
	if err != nil {
		return "", false, fmt.Errorf("resolve %s: %w", branch, err)
	}
	baseTree, err := c.commitTree(ctx, owner, repo, headSHA)
	if err != nil {
		return "", false, err
	}

	// Sorted so the request is deterministic and diffable in a log.
	paths := make([]string, 0, len(files))
	for p := range files {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	type entry struct {
		Path    string `json:"path"`
		Mode    string `json:"mode"`
		Type    string `json:"type"`
		Content string `json:"content"`
	}
	entries := make([]entry, 0, len(paths))
	for _, p := range paths {
		entries = append(entries, entry{Path: p, Mode: blobMode, Type: "blob", Content: files[p]})
	}

	var tree struct {
		SHA string `json:"sha"`
	}
	body := map[string]any{"base_tree": baseTree, "tree": entries}
	if err := c.rest(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/%s/git/trees", owner, repo), body, &tree); err != nil {
		return "", false, fmt.Errorf("create tree: %w", err)
	}
	if tree.SHA == baseTree {
		return "", false, nil
	}

	var commit struct {
		SHA string `json:"sha"`
	}
	body = map[string]any{"message": message, "tree": tree.SHA, "parents": []string{headSHA}}
	if err := c.rest(ctx, http.MethodPost, fmt.Sprintf("/repos/%s/%s/git/commits", owner, repo), body, &commit); err != nil {
		return "", false, fmt.Errorf("create commit: %w", err)
	}

	// No force: a ref that moved since the head SHA was read means someone else
	// committed, and the run should fail rather than drop their work.
	ref := fmt.Sprintf("/repos/%s/%s/git/refs/heads/%s", owner, repo, branch)
	if err := c.rest(ctx, http.MethodPatch, ref, map[string]any{"sha": commit.SHA, "force": false}, nil); err != nil {
		return "", false, fmt.Errorf("update %s: %w", branch, err)
	}
	return commit.SHA, true, nil
}

// commitTree returns the tree SHA a commit points at.
func (c *Client) commitTree(ctx context.Context, owner, repo, commitSHA string) (string, error) {
	var out struct {
		Tree struct {
			SHA string `json:"sha"`
		} `json:"tree"`
	}
	p := fmt.Sprintf("/repos/%s/%s/git/commits/%s", owner, repo, commitSHA)
	if err := c.rest(ctx, http.MethodGet, p, nil, &out); err != nil {
		return "", fmt.Errorf("read commit %s: %w", commitSHA, err)
	}
	return out.Tree.SHA, nil
}
