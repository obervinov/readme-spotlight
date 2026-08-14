// Package publish writes a rendered block into a target repository's README,
// replacing the region between the configured markers. It supports two modes:
// a direct commit to the target branch, or a pull request from a head branch.
//
// A run lands as a single commit covering every path it touches — README and
// SVG assets alike — and produces no commit at all when the output already
// matches what is on the branch.
package publish

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/obervinov/readme-spotlight/internal/config"
	"github.com/obervinov/readme-spotlight/internal/github"
	"github.com/obervinov/readme-spotlight/internal/logs"
)

const commitMessage = "chore(readme): refresh open-source contributions"

// API is the slice of the GitHub client this package needs. Narrowing it keeps
// the publisher from reaching for anything wider, and lets the commit path be
// exercised without a network. *github.Client implements it.
type API interface {
	GetFileMaybe(ctx context.Context, owner, repo, path, ref string) (github.File, bool, error)
	CommitFiles(ctx context.Context, owner, repo, branch, message string, files map[string]string) (sha string, changed bool, err error)
	BranchSHA(ctx context.Context, owner, repo, branch string) (string, error)
	EnsureBranch(ctx context.Context, owner, repo, branch, fromSHA string) error
	FindOpenPR(ctx context.Context, owner, repo, head, base string) (string, error)
	CreatePR(ctx context.Context, owner, repo, head, base, title, body string) (string, error)
}

// Result describes what a publish run did.
type Result struct {
	Changed bool   `json:"changed"` // false when the README already matched
	Mode    string `json:"mode"`    // "pr" or "commit"
	URL     string `json:"url"`     // PR URL (pr mode) or repo URL (commit mode)
}

// Merge replaces the region between start and end markers in content with block.
// If the markers are absent, a new section containing them is appended.
func Merge(content, start, end, block string) string {
	block = strings.TrimRight(block, "\n")
	i := strings.Index(content, start)
	j := strings.Index(content, end)
	if i >= 0 && j >= 0 && j > i {
		return content[:i+len(start)] + "\n" + block + "\n" + content[j:]
	}
	base := strings.TrimRight(content, "\n")
	return base + "\n\n" + start + "\n" + block + "\n" + end + "\n"
}

// Publish writes the block into the target README and any asset files (keyed by
// repo-relative path, e.g. the SVG card) according to cfg. Assets may be nil.
func Publish(ctx context.Context, gh API, cfg config.Config, block string, assets map[string]string) (Result, error) {
	owner, repo, ok := splitRepo(cfg.TargetRepo)
	if !ok {
		return Result{}, fmt.Errorf("invalid target_repo %q (want owner/name)", cfg.TargetRepo)
	}
	logs.Infof("publish: target=%s/%s branch=%s path=%s mode=%s assets=%d", owner, repo, cfg.TargetBranch, cfg.ReadmePath, cfg.PublishMode, len(assets))

	// Resolve the branch we write to. In PR mode this is the head branch,
	// created from the base branch on first run.
	workBranch := cfg.TargetBranch
	res := Result{Mode: "commit", URL: fmt.Sprintf("https://github.com/%s/%s", owner, repo)}
	if cfg.PublishMode == "pr" {
		baseSHA, err := gh.BranchSHA(ctx, owner, repo, cfg.TargetBranch)
		if err != nil {
			return Result{}, fmt.Errorf("resolve base branch: %w", err)
		}
		if err := gh.EnsureBranch(ctx, owner, repo, cfg.PRBranch, baseSHA); err != nil {
			return Result{}, fmt.Errorf("ensure head branch: %w", err)
		}
		workBranch = cfg.PRBranch
		res = Result{Mode: "pr"}
		logs.Infof("publish: head branch %s ready", cfg.PRBranch)
	}

	// Everything this run wants on the branch, in one map: the asset files as
	// they are, and the README with the block merged into its managed region.
	files := make(map[string]string, len(assets)+1)
	for path, content := range assets {
		files[path] = content
	}

	readme, found, err := gh.GetFileMaybe(ctx, owner, repo, cfg.ReadmePath, workBranch)
	if err != nil {
		return Result{}, fmt.Errorf("read README: %w", err)
	}
	if !found {
		return Result{}, fmt.Errorf("README not found at %s on %s", cfg.ReadmePath, workBranch)
	}
	if !strings.Contains(readme.Content, cfg.MarkerStart) {
		logs.Infof("publish: markers not found in README — appending a new block at the end")
	}
	files[cfg.ReadmePath] = Merge(readme.Content, cfg.MarkerStart, cfg.MarkerEnd, block)

	// What actually differs from the branch. current[path] is the content there
	// now, or "" when the file does not exist yet.
	current := map[string]string{cfg.ReadmePath: readme.Content}
	for path := range assets {
		cur, found, err := gh.GetFileMaybe(ctx, owner, repo, path, workBranch)
		if err != nil {
			return Result{}, fmt.Errorf("read %s: %w", path, err)
		}
		if found {
			current[path] = cur.Content
		}
	}
	diff := changedPaths(files, current)

	changed := false
	switch {
	case len(diff) == 0:
		logs.Infof("publish: already up to date, nothing to commit")
	case cfg.SkipStarOnlyChanges && onlyStarsMoved(diff, files, current):
		logs.Infof("publish: skipping — only star counts moved in %s", strings.Join(diff, ", "))
	default:
		sha, wrote, err := gh.CommitFiles(ctx, owner, repo, workBranch, commitMessage, files)
		if err != nil {
			return Result{}, err
		}
		changed = wrote
		if wrote {
			logs.Infof("publish: committed %s to %s (%s)", strings.Join(diff, ", "), workBranch, sha[:min(7, len(sha))])
		} else {
			logs.Infof("publish: already up to date, nothing to commit")
		}
	}

	if cfg.PublishMode == "commit" {
		res.Changed = changed
		return res, nil
	}

	// PR mode: ensure a PR exists for the head branch.
	url, err := gh.FindOpenPR(ctx, owner, repo, cfg.PRBranch, cfg.TargetBranch)
	if err != nil {
		return Result{}, fmt.Errorf("look up existing PR: %w", err)
	}
	switch {
	case url != "":
		logs.Infof("publish: existing PR %s", url)
	case !changed:
		// Nothing was committed, so the head branch carries nothing to propose.
		// Asking for a PR here would only earn a 422.
		logs.Infof("publish: nothing to propose, no PR opened")
	default:
		url, err = gh.CreatePR(ctx, owner, repo, cfg.PRBranch, cfg.TargetBranch,
			"Refresh open-source contributions",
			"Automated update of the open-source contributions block by readme-spotlight.")
		if err != nil {
			return Result{}, fmt.Errorf("create PR: %w", err)
		}
		logs.Infof("publish: opened PR %s", url)
	}
	res.Changed = changed
	res.URL = url
	return res, nil
}

// changedPaths returns the paths whose new content differs from what is on the
// branch, sorted so logs read the same way twice.
func changedPaths(files, current map[string]string) []string {
	var out []string
	for path, content := range files {
		if current[path] != content {
			out = append(out, path)
		}
	}
	sort.Strings(out)
	return out
}

// starCount matches a rendered star count wherever a format marks one with ★:
// "★ 890" in a <details> summary, "★ 23.9k" in the SVG card.
var starCount = regexp.MustCompile(`★\s*[0-9][0-9.,]*k?`)

// onlyStarsMoved reports whether every changed file becomes identical once star
// counts are blanked out — i.e. the run's own content is the same and only other
// people's stargazers drifted (a repository going 40★ to 41★ between two runs).
//
// It cannot see a star count the format renders as a bare number, which the
// table format's star column is; there the guard simply does not trigger.
func onlyStarsMoved(diff []string, files, current map[string]string) bool {
	for _, path := range diff {
		if starCount.ReplaceAllString(files[path], "★") != starCount.ReplaceAllString(current[path], "★") {
			return false
		}
	}
	return len(diff) > 0
}

func splitRepo(full string) (owner, repo string, ok bool) {
	parts := strings.SplitN(full, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}
