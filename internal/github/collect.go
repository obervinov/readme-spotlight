// Package github collects a user's external open-source contributions via the
// GitHub GraphQL API. "External" means repositories not owned by the user
// themselves — the contributions GitHub's profile page hides or buries.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/obervinov/readme-spotlight/internal/logs"
	"github.com/obervinov/readme-spotlight/internal/model"
)

const endpoint = "https://api.github.com/graphql"

// Client talks to the GitHub GraphQL API with a personal access token.
type Client struct {
	token    string
	http     *http.Client
	endpoint string
}

// New returns a Client authenticated with the given token.
func New(token string) *Client {
	return &Client{token: token, http: &http.Client{Timeout: 30 * time.Second}, endpoint: endpoint}
}

// query executes a GraphQL query and decodes the data into out.
func (c *Client) query(ctx context.Context, q string, vars map[string]any, out any) error {
	body, err := json.Marshal(map[string]any{"query": q, "variables": vars})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("github graphql: unexpected status %s", resp.Status)
	}

	var env struct {
		Data   json.RawMessage `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return err
	}
	if len(env.Errors) > 0 {
		return fmt.Errorf("github graphql: %s", env.Errors[0].Message)
	}
	return json.Unmarshal(env.Data, out)
}

// Viewer returns the authenticated user's login and account creation time.
func (c *Client) Viewer(ctx context.Context) (login string, createdAt time.Time, err error) {
	var out struct {
		Viewer struct {
			Login     string    `json:"login"`
			CreatedAt time.Time `json:"createdAt"`
		} `json:"viewer"`
	}
	err = c.query(ctx, `query { viewer { login createdAt } }`, nil, &out)
	return out.Viewer.Login, out.Viewer.CreatedAt, err
}

// repoFields mirrors the shared repoFields GraphQL fragment.
type repoFields struct {
	NameWithOwner string `json:"nameWithOwner"`
	Owner         struct {
		Login string `json:"login"`
	} `json:"owner"`
	IsInOrganization bool   `json:"isInOrganization"`
	StargazerCount   int    `json:"stargazerCount"`
	Description      string `json:"description"`
	PrimaryLanguage  *struct {
		Name string `json:"name"`
	} `json:"primaryLanguage"`
}

// The per-year query pulls commit counts by repository plus the individual
// pull requests, issues and reviews (each with a title and URL) so the renderer
// can build an expandable, linkable tree. Item lists are capped at 100 per
// activity type per year, which is well beyond any realistic personal volume.
const contribQuery = `
query($from:DateTime!, $to:DateTime!) {
  viewer {
    contributionsCollection(from:$from, to:$to) {
      commitContributionsByRepository(maxRepositories:100) {
        repository { ...repoFields }
        contributions { totalCount }
      }
      pullRequestContributions(first:100) {
        nodes { pullRequest { title url state repository { ...repoFields } } }
      }
      issueContributions(first:100) {
        nodes { issue { title url state repository { ...repoFields } } }
      }
      pullRequestReviewContributions(first:100) {
        nodes { pullRequestReview { pullRequest { title url repository { ...repoFields } } } }
      }
    }
  }
}
fragment repoFields on Repository {
  nameWithOwner
  owner { login }
  isInOrganization
  stargazerCount
  description
  primaryLanguage { name }
}`

// CollectExternal walks the viewer's contributions year-by-year from account
// creation to now, aggregates them per repository, and returns only those
// owned by someone other than the viewer, sorted by star count (descending).
func (c *Client) CollectExternal(ctx context.Context) ([]model.Contribution, error) {
	login, createdAt, err := c.Viewer(ctx)
	if err != nil {
		return nil, err
	}
	logs.Infof("collect: user=%s scanning %d–%d", login, createdAt.UTC().Year(), time.Now().UTC().Year())

	agg := map[string]*model.Contribution{}
	// GraphQL returns null for a node the token cannot resolve — a contribution to
	// a repository that has since gone private or been deleted. Unmarshalling null
	// into a struct leaves it zero-valued silently, so without this guard every
	// such node aggregates under the empty repository name and surfaces as a
	// nameless block full of blank, unlinked entries.
	skipped := 0
	ensure := func(rf repoFields) *model.Contribution {
		if rf.NameWithOwner == "" {
			skipped++
			return nil
		}
		cur := agg[rf.NameWithOwner]
		if cur == nil {
			cur = &model.Contribution{Repo: rf.NameWithOwner, Owner: rf.Owner.Login, IsOrg: rf.IsInOrganization}
			agg[rf.NameWithOwner] = cur
		}
		if rf.StargazerCount > cur.Stars {
			cur.Stars = rf.StargazerCount
		}
		if cur.Language == "" && rf.PrimaryLanguage != nil {
			cur.Language = rf.PrimaryLanguage.Name
		}
		if cur.Description == "" {
			cur.Description = rf.Description
		}
		return cur
	}

	// add records one linkable contribution. An item without a title or URL is
	// unrenderable — it would become a bare bullet — so it is dropped before it can
	// create an aggregate or inflate a count.
	add := func(rf repoFields, it model.Item) {
		if it.Title == "" || it.URL == "" {
			skipped++
			return
		}
		e := ensure(rf)
		if e == nil {
			return
		}
		switch it.Kind {
		case "pr":
			e.PRs++
		case "issue":
			e.Issues++
		case "review":
			e.Reviews++
		}
		e.Items = append(e.Items, it)
	}

	nowYear := time.Now().UTC().Year()
	for y := createdAt.UTC().Year(); y <= nowYear; y++ {
		from := time.Date(y, time.January, 1, 0, 0, 0, 0, time.UTC)
		to := time.Date(y, time.December, 31, 23, 59, 59, 0, time.UTC)

		var out struct {
			Viewer struct {
				C struct {
					Commit []struct {
						Repository    repoFields `json:"repository"`
						Contributions struct {
							TotalCount int `json:"totalCount"`
						} `json:"contributions"`
					} `json:"commitContributionsByRepository"`
					PR struct {
						Nodes []struct {
							PullRequest struct {
								Title      string     `json:"title"`
								URL        string     `json:"url"`
								State      string     `json:"state"`
								Repository repoFields `json:"repository"`
							} `json:"pullRequest"`
						} `json:"nodes"`
					} `json:"pullRequestContributions"`
					Issue struct {
						Nodes []struct {
							Issue struct {
								Title      string     `json:"title"`
								URL        string     `json:"url"`
								State      string     `json:"state"`
								Repository repoFields `json:"repository"`
							} `json:"issue"`
						} `json:"nodes"`
					} `json:"issueContributions"`
					Review struct {
						Nodes []struct {
							PullRequestReview struct {
								PullRequest struct {
									Title      string     `json:"title"`
									URL        string     `json:"url"`
									Repository repoFields `json:"repository"`
								} `json:"pullRequest"`
							} `json:"pullRequestReview"`
						} `json:"nodes"`
					} `json:"pullRequestReviewContributions"`
				} `json:"contributionsCollection"`
			} `json:"viewer"`
		}
		vars := map[string]any{"from": from.Format(time.RFC3339), "to": to.Format(time.RFC3339)}
		if err := c.query(ctx, contribQuery, vars, &out); err != nil {
			return nil, fmt.Errorf("collect %d: %w", y, err)
		}

		cc := out.Viewer.C
		for _, cb := range cc.Commit {
			if e := ensure(cb.Repository); e != nil {
				e.Commits += cb.Contributions.TotalCount
			}
		}
		for _, n := range cc.PR.Nodes {
			pr := n.PullRequest
			add(pr.Repository, model.Item{Kind: "pr", Title: pr.Title, URL: pr.URL, State: pr.State})
		}
		for _, n := range cc.Issue.Nodes {
			is := n.Issue
			add(is.Repository, model.Item{Kind: "issue", Title: is.Title, URL: is.URL, State: is.State})
		}
		for _, n := range cc.Review.Nodes {
			pr := n.PullRequestReview.PullRequest
			add(pr.Repository, model.Item{Kind: "review", Title: pr.Title, URL: pr.URL})
		}
	}
	if skipped > 0 {
		logs.Infof("collect: skipped %d unresolvable contribution(s) (private or deleted repositories)", skipped)
	}

	out := make([]model.Contribution, 0, len(agg))
	for _, v := range agg {
		if v.Owner == login {
			continue // skip the user's own repositories
		}
		if v.Commits > 0 {
			v.CommitsURL = fmt.Sprintf("https://github.com/%s/commits?author=%s", v.Repo, login)
		}
		out = append(out, *v)
	}
	logs.Infof("collect: %d external repositories", len(out))
	return out, nil
}
