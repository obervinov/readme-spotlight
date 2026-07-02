// Package model holds the core domain types shared across readme-spotlight.
package model

// Item is a single linkable contribution: a pull request, issue or review.
type Item struct {
	Kind  string // "pr", "issue" or "review"
	Title string
	URL   string
	State string // e.g. "OPEN", "MERGED", "CLOSED" (empty for reviews)
}

// Contribution is the aggregated contribution footprint for a single repository.
type Contribution struct {
	Repo        string // "owner/name"
	Owner       string
	IsOrg       bool // true when the owner is an Organization, false for a user account
	Stars       int
	Language    string
	Description string

	Commits    int
	CommitsURL string // link to the author-filtered commit list (empty when Commits == 0)
	PRs        int
	Issues     int
	Reviews    int

	Items []Item // individual PRs, issues and reviews (commits are counted only)
}

// Total is the combined contribution count across all activity types.
func (c Contribution) Total() int {
	return c.Commits + c.PRs + c.Issues + c.Reviews
}
