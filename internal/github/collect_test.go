package github

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// newTestClient points a Client at a stub GraphQL endpoint. respond receives the
// decoded request body so it can answer the viewer query and the per-year
// contribution queries differently.
func newTestClient(t *testing.T, respond func(query string) string) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Query string `json:"query"`
		}
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, respond(req.Query))
	}))
	t.Cleanup(srv.Close)

	c := New("test-token")
	c.endpoint = srv.URL
	return c
}

const viewerReply = `{"data":{"viewer":{"login":"obervinov","createdAt":"2026-01-01T00:00:00Z"}}}`

// A contribution GitHub cannot resolve for this token — the repository went
// private or was deleted — comes back as JSON null. Unmarshalling null into a
// struct silently yields the zero value, which used to aggregate under the empty
// repository name and render as a nameless block of bare bullets.
func TestCollectExternalDropsUnresolvableNodes(t *testing.T) {
	contribReply := `{"data":{"viewer":{"contributionsCollection":{
      "commitContributionsByRepository":[
        {"repository":{"nameWithOwner":"eitchtee/WYGIWYH","owner":{"login":"eitchtee"},"stargazerCount":42},
         "contributions":{"totalCount":3}},
        {"repository":null,"contributions":{"totalCount":7}}
      ],
      "pullRequestContributions":{"nodes":[
        {"pullRequest":{"title":"Fix the thing","url":"https://github.com/eitchtee/WYGIWYH/pull/557","state":"MERGED",
          "repository":{"nameWithOwner":"eitchtee/WYGIWYH","owner":{"login":"eitchtee"},"stargazerCount":42}}},
        {"pullRequest":null}
      ]},
      "issueContributions":{"nodes":[{"issue":null}]},
      "pullRequestReviewContributions":{"nodes":[{"pullRequestReview":null}]}
    }}}}`

	c := newTestClient(t, func(q string) string {
		if strings.Contains(q, "contributionsCollection") {
			return contribReply
		}
		return viewerReply
	})

	got, err := c.CollectExternal(t.Context())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 1 {
		names := make([]string, len(got))
		for i, g := range got {
			names[i] = g.Repo
		}
		t.Fatalf("collected %d repositories (%v), want only the resolvable one", len(got), names)
	}

	c0 := got[0]
	if c0.Repo != "eitchtee/WYGIWYH" {
		t.Fatalf("repo = %q, want eitchtee/WYGIWYH", c0.Repo)
	}
	// The null commit entry must not inflate the count, and the null PR, issue and
	// review must not appear as items or bump their counters.
	if c0.Commits != 3 {
		t.Fatalf("commits = %d, want 3 (the null repository's 7 must be dropped)", c0.Commits)
	}
	if c0.PRs != 1 || c0.Issues != 0 || c0.Reviews != 0 {
		t.Fatalf("counts = %d PR / %d issue / %d review, want 1/0/0", c0.PRs, c0.Issues, c0.Reviews)
	}
	if len(c0.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(c0.Items))
	}
	for _, it := range c0.Items {
		if it.Title == "" || it.URL == "" {
			t.Fatalf("item has an empty title or URL: %+v — it would render as a bare bullet", it)
		}
	}
}

// An item whose repository resolves but whose own title or URL is missing is just
// as unrenderable, and must not create an aggregate for that repository either.
func TestCollectExternalDropsItemsWithoutTitleOrURL(t *testing.T) {
	contribReply := `{"data":{"viewer":{"contributionsCollection":{
      "commitContributionsByRepository":[],
      "pullRequestContributions":{"nodes":[
        {"pullRequest":{"title":"","url":"","state":"OPEN",
          "repository":{"nameWithOwner":"someorg/somerepo","owner":{"login":"someorg"},"stargazerCount":1}}}
      ]},
      "issueContributions":{"nodes":[]},
      "pullRequestReviewContributions":{"nodes":[]}
    }}}}`

	c := newTestClient(t, func(q string) string {
		if strings.Contains(q, "contributionsCollection") {
			return contribReply
		}
		return viewerReply
	})

	got, err := c.CollectExternal(t.Context())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("collected %d repositories, want none — the only item was unrenderable", len(got))
	}
}

func TestCollectExternalSkipsOwnRepositories(t *testing.T) {
	contribReply := `{"data":{"viewer":{"contributionsCollection":{
      "commitContributionsByRepository":[
        {"repository":{"nameWithOwner":"obervinov/infrastructure","owner":{"login":"obervinov"},"stargazerCount":5},
         "contributions":{"totalCount":100}}
      ],
      "pullRequestContributions":{"nodes":[]},
      "issueContributions":{"nodes":[]},
      "pullRequestReviewContributions":{"nodes":[]}
    }}}}`

	c := newTestClient(t, func(q string) string {
		if strings.Contains(q, "contributionsCollection") {
			return contribReply
		}
		return viewerReply
	})

	got, err := c.CollectExternal(t.Context())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("collected %d repositories, want none — the viewer owns it", len(got))
	}
}
