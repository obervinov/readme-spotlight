// Package web serves the configuration UI and control endpoints. Templates are
// embedded so the binary is self-contained.
package web

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/obervinov/readme-spotlight/internal/auth"
	"github.com/obervinov/readme-spotlight/internal/config"
	"github.com/obervinov/readme-spotlight/internal/logs"
	"github.com/obervinov/readme-spotlight/internal/publish"
	"github.com/obervinov/readme-spotlight/internal/render"
	"github.com/obervinov/readme-spotlight/internal/runner"
	"github.com/obervinov/readme-spotlight/internal/scheduler"
	"github.com/obervinov/readme-spotlight/internal/store"
)

//go:embed templates/*.html
var templatesFS embed.FS

// Server holds the dependencies the HTTP handlers need and tracks the state of
// the most recent run (manual or scheduled).
type Server struct {
	store *store.Store
	run   *runner.Runner
	sched *scheduler.Scheduler
	tmpl  *template.Template

	mu      sync.Mutex
	running bool
	lastAt  time.Time
	lastMsg string
	lastErr string
}

// NewServer builds the HTTP server, parses templates and installs the cron job.
func NewServer(st *store.Store, r *runner.Runner, sc *scheduler.Scheduler) (*Server, error) {
	tmpl, err := template.ParseFS(templatesFS, "templates/*.html")
	if err != nil {
		return nil, err
	}
	s := &Server{store: st, run: r, sched: sc, tmpl: tmpl}
	cfg, _, err := st.GetConfig()
	if err != nil {
		return nil, err
	}
	if err := s.applySchedule(cfg.Schedule); err != nil {
		return nil, err
	}
	return s, nil
}

// Handler returns the router. The application routes are guarded by the given
// authenticator; /healthz and the authenticator's own routes stay public.
func (s *Server) Handler(a auth.Authenticator) http.Handler {
	app := http.NewServeMux()
	app.HandleFunc("GET /", s.index)
	app.HandleFunc("POST /config", s.saveConfig)
	app.HandleFunc("POST /run", s.runRefresh)
	app.HandleFunc("POST /publish", s.runPublish)

	root := http.NewServeMux()
	root.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	a.Routes(root)
	root.Handle("/", a.Wrap(app))
	return root
}

// applySchedule points the cron entry at the full pipeline on the given spec.
func (s *Server) applySchedule(spec string) error {
	return s.sched.Reschedule(spec, func() { s.start("scheduled", s.doFull) })
}

// start runs fn in the background, guarding against overlapping runs and
// recording the outcome. It returns false if a run is already in flight.
func (s *Server) start(label string, fn func(context.Context) (string, error)) bool {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return false
	}
	s.running = true
	s.mu.Unlock()

	logs.Infof("%s: started", label)
	go func() {
		msg, err := fn(context.Background())
		s.mu.Lock()
		s.running = false
		s.lastAt = time.Now()
		if err != nil {
			s.lastErr = label + ": " + err.Error()
			s.lastMsg = ""
		} else {
			s.lastErr = ""
			s.lastMsg = msg
		}
		s.mu.Unlock()
		if err != nil {
			logs.Infof("%s: FAILED — %v", label, err)
		} else {
			logs.Infof("%s: done — %s", label, msg)
		}
	}()
	return true
}

func (s *Server) doRefresh(ctx context.Context) (string, error) {
	n, err := s.run.Refresh(ctx)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("Refreshed — %d repositories collected", n), nil
}

func (s *Server) doPublish(ctx context.Context) (string, error) {
	res, err := s.run.Publish(ctx)
	if err != nil {
		return "", err
	}
	return publishMessage(res), nil
}

func (s *Server) doFull(ctx context.Context) (string, error) {
	count, pub, published, err := s.run.Full(ctx)
	if err != nil {
		return "", err
	}
	msg := fmt.Sprintf("Refreshed — %d repositories", count)
	if published {
		msg += "; " + publishMessage(pub)
	}
	return msg, nil
}

func publishMessage(res publish.Result) string {
	switch {
	case res.Mode == "pr" && res.Changed:
		return "opened/updated PR: " + res.URL
	case res.Mode == "pr":
		return "README already up to date (PR: " + res.URL + ")"
	case res.Changed:
		return "committed to README: " + res.URL
	default:
		return "README already up to date"
	}
}

type logLine struct {
	Time string
	Msg  string
}

type pageData struct {
	Config  config.Config
	LastRun string
	LastMsg string
	LastErr string
	Running bool
	Count   int
	Block   string
	SVGs    []template.HTML
	Logs    []logLine
}

func (s *Server) index(w http.ResponseWriter, r *http.Request) {
	cfg, out, err := s.run.Compose()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	data := pageData{Config: cfg, Block: out.Block}
	if snap, err := s.store.LatestSnapshot(); err == nil && snap != nil {
		data.Count = len(snap.Contributions)
	}
	for _, path := range sortedKeys(out.Assets) {
		data.SVGs = append(data.SVGs, template.HTML(out.Assets[path])) //nolint:gosec // our own generated SVG
	}

	s.mu.Lock()
	data.Running = s.running
	data.LastMsg = s.lastMsg
	data.LastErr = s.lastErr
	if !s.lastAt.IsZero() {
		data.LastRun = s.lastAt.Format("2006-01-02 15:04:05")
	}
	s.mu.Unlock()

	for _, e := range logs.Recent(50) {
		data.Logs = append(data.Logs, logLine{Time: e.Time.Format("15:04:05"), Msg: e.Msg})
	}

	if err := s.tmpl.ExecuteTemplate(w, "index.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

func (s *Server) saveConfig(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	cfg := config.Default()
	cfg.TargetRepo = strings.TrimSpace(r.FormValue("target_repo"))
	cfg.TargetBranch = orDefault(r.FormValue("target_branch"), cfg.TargetBranch)
	cfg.ReadmePath = orDefault(r.FormValue("readme_path"), cfg.ReadmePath)

	cfg.Banner.Enabled = r.FormValue("banner_enabled") != ""
	cfg.Banner.Name = strings.TrimSpace(r.FormValue("banner_name"))
	cfg.Banner.Role = strings.TrimSpace(r.FormValue("banner_role"))
	cfg.Banner.Tagline = strings.TrimSpace(r.FormValue("banner_tagline"))
	cfg.Banner.Accent = orDefault(r.FormValue("banner_accent"), cfg.Banner.Accent)

	cfg.Positioning.Enabled = r.FormValue("positioning_enabled") != ""
	cfg.Positioning.Text = strings.TrimSpace(r.FormValue("positioning_text"))
	cfg.Positioning.Accent = orDefault(r.FormValue("positioning_accent"), cfg.Positioning.Accent)

	cfg.Focus.Enabled = r.FormValue("focus_enabled") != ""
	cfg.Focus.Title = orDefault(r.FormValue("focus_title"), cfg.Focus.Title)
	cfg.Focus.Accent = orDefault(r.FormValue("focus_accent"), cfg.Focus.Accent)
	cfg.Focus.Items = config.ParseFocusItems(r.FormValue("focus_items"))

	cfg.Tech.Enabled = r.FormValue("tech_enabled") != ""
	cfg.Tech.Title = orDefault(r.FormValue("tech_title"), cfg.Tech.Title)
	cfg.Tech.Accent = orDefault(r.FormValue("tech_accent"), cfg.Tech.Accent)
	cfg.Tech.Groups = config.ParseTechGroups(r.FormValue("tech_groups"))

	cfg.Title = orDefault(r.FormValue("title"), cfg.Title)
	cfg.Format = orDefault(r.FormValue("format"), cfg.Format)
	cfg.SortBy = orDefault(r.FormValue("sort_by"), cfg.SortBy)
	cfg.Schedule = strings.TrimSpace(r.FormValue("schedule"))
	cfg.PublishMode = orDefault(r.FormValue("publish_mode"), cfg.PublishMode)
	cfg.PRBranch = orDefault(r.FormValue("pr_branch"), cfg.PRBranch)
	if n, err := strconv.Atoi(r.FormValue("limit")); err == nil {
		cfg.Limit = n
	}
	cfg.Columns = render.Columns{
		Stars:    r.FormValue("col_stars") != "",
		Language: r.FormValue("col_language") != "",
		Commits:  r.FormValue("col_commits") != "",
		PRs:      r.FormValue("col_prs") != "",
		Issues:   r.FormValue("col_issues") != "",
		Reviews:  r.FormValue("col_reviews") != "",
		Total:    r.FormValue("col_total") != "",
	}

	if err := s.store.SetConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if err := s.applySchedule(cfg.Schedule); err != nil {
		http.Error(w, "invalid schedule: "+err.Error(), http.StatusBadRequest)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) runRefresh(w http.ResponseWriter, r *http.Request) {
	s.start("refresh", s.doRefresh)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *Server) runPublish(w http.ResponseWriter, r *http.Request) {
	s.start("publish", s.doPublish)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return strings.TrimSpace(v)
}
