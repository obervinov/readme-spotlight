package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/obervinov/readme-spotlight/internal/auth"
	"github.com/obervinov/readme-spotlight/internal/config"
	"github.com/obervinov/readme-spotlight/internal/github"
	"github.com/obervinov/readme-spotlight/internal/runner"
	"github.com/obervinov/readme-spotlight/internal/scheduler"
	"github.com/obervinov/readme-spotlight/internal/store"
)

const apiToken = "0123456789abcdef0123456789abcdef"

// newTestHandler builds a server backed by a throwaway SQLite file. withAPI
// selects whether the machine API is enabled.
func newTestHandler(t *testing.T, withAPI bool) (http.Handler, *store.Store) {
	t.Helper()

	st, err := store.Open("sqlite:" + t.TempDir() + "/test.db")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.SetConfig(config.Default()); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	sc := scheduler.New()
	srv, err := NewServer(st, runner.New(github.New("test-token"), st), sc)
	if err != nil {
		t.Fatalf("new server: %v", err)
	}

	var guard *auth.APIGuard
	if withAPI {
		if guard, err = auth.NewAPIGuard(apiToken); err != nil {
			t.Fatalf("new guard: %v", err)
		}
	}
	// Basic auth stands in for the interactive authenticator so an unauthorised
	// UI request is distinguishable from an API one.
	a, err := auth.New(t.Context(), auth.Config{Mode: "basic", BasicUser: "u", BasicPass: "p"})
	if err != nil {
		t.Fatalf("new authenticator: %v", err)
	}
	return srv.Handler(a, guard), st
}

func apiRequest(t *testing.T, h http.Handler, method, path, body string, opts ...func(*http.Request)) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
	}
	r.Header.Set("Authorization", "Bearer "+apiToken)
	for _, o := range opts {
		o(r)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func TestAPIDisabledReturnsNotFound(t *testing.T) {
	h, _ := newTestHandler(t, false)
	// Without the guard the /api/ prefix must 404 rather than fall through to the
	// UI handler (which would redirect or serve the page).
	for _, path := range []string{"/api/content", "/api/publish"} {
		rec := apiRequest(t, h, http.MethodGet, path, "")
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: status = %d, want 404 when RS_API_TOKEN is unset", path, rec.Code)
		}
	}
}

func TestAPIRequiresToken(t *testing.T) {
	h, _ := newTestHandler(t, true)
	r := httptest.NewRequest(http.MethodGet, "/api/content", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 without a bearer token", rec.Code)
	}
}

func TestAPINeedsNoBrowserSession(t *testing.T) {
	h, _ := newTestHandler(t, true)
	// The UI is behind basic auth...
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("UI status = %d, want 401", rec.Code)
	}
	// ...while the API accepts the bearer token on its own.
	if got := apiRequest(t, h, http.MethodGet, "/api/content", "").Code; got != http.StatusOK {
		t.Fatalf("API status = %d, want 200", got)
	}
}

func TestGetContentReturnsOnlyContentFields(t *testing.T) {
	h, _ := newTestHandler(t, true)
	rec := apiRequest(t, h, http.MethodGet, "/api/content", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, key := range []string{"banner", "positioning", "focus", "tech", "title", "format", "sort_by", "limit"} {
		if _, ok := body[key]; !ok {
			t.Fatalf("response is missing %q", key)
		}
	}
	for _, key := range []string{"target_repo", "target_branch", "readme_path", "publish_mode", "pr_branch", "schedule", "marker_start", "marker_end"} {
		if _, ok := body[key]; ok {
			t.Fatalf("response exposes %q, which is not part of the content subset", key)
		}
	}
}

func TestPatchContentPersists(t *testing.T) {
	h, st := newTestHandler(t, true)
	rec := apiRequest(t, h, http.MethodPatch, "/api/content",
		`{"positioning":{"enabled":true,"text":"Platform & LLMOps","accent":"#3fb950"}}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body: %s)", rec.Code, rec.Body.String())
	}
	cfg, _, err := st.GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Positioning.Text != "Platform & LLMOps" {
		t.Fatalf("stored positioning.text = %q, want the patched value", cfg.Positioning.Text)
	}
	if cfg.Banner.Name != config.Default().Banner.Name {
		t.Fatal("an unrelated section changed")
	}
}

func TestPatchRejectsPublishingFields(t *testing.T) {
	h, st := newTestHandler(t, true)
	bodies := map[string]string{
		"target repo":  `{"target_repo":"attacker/evil"}`,
		"publish mode": `{"publish_mode":"commit"}`,
		"readme path":  `{"readme_path":".github/workflows/ci.yml"}`,
		"pr branch":    `{"pr_branch":"main"}`,
		"schedule":     `{"schedule":"* * * * *"}`,
		"markers":      `{"marker_start":"<!--x-->"}`,
		"nested junk":  `{"banner":{"accent":"#3fb950","target_repo":"attacker/evil"}}`,
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			rec := apiRequest(t, h, http.MethodPatch, "/api/content", body)
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400 (body: %s)", rec.Code, rec.Body.String())
			}
		})
	}
	cfg, _, err := st.GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	def := config.Default()
	if cfg.TargetRepo != def.TargetRepo || cfg.PublishMode != def.PublishMode || cfg.ReadmePath != def.ReadmePath || cfg.Schedule != def.Schedule {
		t.Fatal("a rejected patch must not change stored publishing fields")
	}
}

func TestPatchRejectsInvalidContent(t *testing.T) {
	h, _ := newTestHandler(t, true)
	bodies := map[string]string{
		"accent injection": `{"banner":{"accent":"#fff\" onload=\"alert(1)"}}`,
		"empty patch":      `{}`,
		"bad format":       `{"format":"yaml"}`,
		"malformed json":   `{"title":`,
	}
	for name, body := range bodies {
		t.Run(name, func(t *testing.T) {
			if got := apiRequest(t, h, http.MethodPatch, "/api/content", body).Code; got != http.StatusBadRequest {
				t.Fatalf("status = %d, want 400", got)
			}
		})
	}
}

func TestPatchRequiresJSONContentType(t *testing.T) {
	h, _ := newTestHandler(t, true)
	rec := apiRequest(t, h, http.MethodPatch, "/api/content", `{"title":"x"}`, func(r *http.Request) {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	})
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("status = %d, want 415", rec.Code)
	}
}

func TestPatchRejectsOversizedBody(t *testing.T) {
	h, _ := newTestHandler(t, true)
	body := `{"title":"` + strings.Repeat("x", apiMaxBody+1) + `"}`
	if got := apiRequest(t, h, http.MethodPatch, "/api/content", body).Code; got != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 for a body over the size cap", got)
	}
}

func TestPublishAlwaysUsesPullRequestMode(t *testing.T) {
	h, st := newTestHandler(t, true)
	// Configure the riskiest stored setting: direct commits to a real repo.
	cfg := config.Default()
	cfg.TargetRepo = "obervinov/obervinov"
	cfg.PublishMode = "commit"
	if err := st.SetConfig(cfg); err != nil {
		t.Fatal(err)
	}
	// No snapshot exists, so the run stops before any GitHub call — enough to
	// prove the request is accepted and routed, without network access.
	rec := apiRequest(t, h, http.MethodPost, "/api/publish", "")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502 with the underlying error (body: %s)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "refresh first") {
		t.Fatalf("body = %s, want the no-data error", rec.Body.String())
	}
	// The stored mode must be unchanged: forcing PR mode happens on a copy.
	after, _, err := st.GetConfig()
	if err != nil {
		t.Fatal(err)
	}
	if after.PublishMode != "commit" {
		t.Fatalf("stored publish_mode = %q, want it left alone", after.PublishMode)
	}
}

func TestPublishRejectsWrongMethod(t *testing.T) {
	h, _ := newTestHandler(t, true)
	if got := apiRequest(t, h, http.MethodGet, "/api/publish", "").Code; got != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", got)
	}
}
