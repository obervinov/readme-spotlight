// Package publish writes a rendered block into a target repository's README,
// replacing the region between the configured markers. It supports two modes:
// a direct commit to the target branch, or a pull request from a head branch.
package publish

import (
	"context"
	"fmt"
	"strings"

	"github.com/obervinov/readme-spotlight/internal/config"
	"github.com/obervinov/readme-spotlight/internal/github"
	"github.com/obervinov/readme-spotlight/internal/logs"
)

const commitMessage = "chore(readme): refresh open-source contributions"

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
func Publish(ctx context.Context, gh *github.Client, cfg config.Config, block string, assets map[string]string) (Result, error) {
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

	changed := false

	// Asset files (overwrite as-is).
	for path, content := range assets {
		ch, err := putFileIfChanged(ctx, gh, owner, repo, path, workBranch, content)
		if err != nil {
			return Result{}, fmt.Errorf("write %s: %w", path, err)
		}
		if ch {
			logs.Infof("publish: wrote %s", path)
			changed = true
		}
	}

	// README (merge block between markers).
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
	newContent := Merge(readme.Content, cfg.MarkerStart, cfg.MarkerEnd, block)
	if newContent != readme.Content {
		if err := gh.PutFile(ctx, owner, repo, cfg.ReadmePath, workBranch, commitMessage, newContent, readme.SHA); err != nil {
			return Result{}, fmt.Errorf("commit README: %w", err)
		}
		logs.Infof("publish: updated %s on %s", cfg.ReadmePath, workBranch)
		changed = true
	}

	if cfg.PublishMode == "commit" {
		if !changed {
			logs.Infof("publish: already up to date, nothing to commit")
		}
		res.Changed = changed
		return res, nil
	}

	// PR mode: ensure a PR exists for the head branch.
	url, err := gh.FindOpenPR(ctx, owner, repo, cfg.PRBranch, cfg.TargetBranch)
	if err != nil {
		return Result{}, fmt.Errorf("look up existing PR: %w", err)
	}
	if url == "" {
		url, err = gh.CreatePR(ctx, owner, repo, cfg.PRBranch, cfg.TargetBranch,
			"Refresh open-source contributions",
			"Automated update of the open-source contributions block by readme-spotlight.")
		if err != nil {
			return Result{}, fmt.Errorf("create PR: %w", err)
		}
		logs.Infof("publish: opened PR %s", url)
	} else {
		logs.Infof("publish: existing PR %s", url)
	}
	res.Changed = changed
	res.URL = url
	return res, nil
}

// putFileIfChanged writes content to path on branch only if it differs from the
// current file there. It reports whether a write happened.
func putFileIfChanged(ctx context.Context, gh *github.Client, owner, repo, path, branch, content string) (bool, error) {
	cur, found, err := gh.GetFileMaybe(ctx, owner, repo, path, branch)
	if err != nil {
		return false, err
	}
	if found && cur.Content == content {
		return false, nil
	}
	sha := ""
	if found {
		sha = cur.SHA
	}
	return true, gh.PutFile(ctx, owner, repo, path, branch, commitMessage, content, sha)
}

func splitRepo(full string) (owner, repo string, ok bool) {
	parts := strings.SplitN(full, "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}
