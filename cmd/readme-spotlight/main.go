// Command readme-spotlight collects a user's external open-source
// contributions and keeps a block in a profile README up to date.
//
// Run modes:
//
//	readme-spotlight                 # start the web UI + scheduler
//	readme-spotlight --print         # collect and print the block to stdout, then exit
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/obervinov/readme-spotlight/internal/auth"
	"github.com/obervinov/readme-spotlight/internal/github"
	"github.com/obervinov/readme-spotlight/internal/render"
	"github.com/obervinov/readme-spotlight/internal/runner"
	"github.com/obervinov/readme-spotlight/internal/scheduler"
	"github.com/obervinov/readme-spotlight/internal/store"
	"github.com/obervinov/readme-spotlight/internal/web"
)

func main() {
	var (
		addr     = flag.String("addr", envOr("RS_ADDR", ":8080"), "web UI listen address (env RS_ADDR)")
		dsn      = flag.String("db", envOr("RS_DATABASE_DSN", "sqlite:./data/spotlight.db"), "database DSN, sqlite:PATH or postgres://... (env RS_DATABASE_DSN)")
		printOut = flag.Bool("print", false, "collect and print the block to stdout, then exit")
		format   = flag.String("format", "table", "block format for --print: table | details")
	)
	flag.Parse()

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		log.Fatal("GITHUB_TOKEN is not set")
	}
	gh := github.New(token)

	if *printOut {
		contribs, err := gh.CollectExternal(context.Background())
		if err != nil {
			log.Fatal(err)
		}
		out := render.RenderOutput(contribs, render.Options{
			Title: "Open-Source Contributions", Format: *format, Columns: render.DefaultColumns(), SortBy: "stars",
		})
		for path, content := range out.Assets {
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				log.Fatal(err)
			}
			fmt.Fprintf(os.Stderr, "wrote asset %s\n", path)
		}
		fmt.Print(out.Block)
		return
	}

	if err := ensureSQLiteDir(*dsn); err != nil {
		log.Fatal(err)
	}
	st, err := store.Open(*dsn)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	// Seed default config on first boot.
	cfg, ok, err := st.GetConfig()
	if err != nil {
		log.Fatal(err)
	}
	if !ok {
		if err := st.SetConfig(cfg); err != nil {
			log.Fatal(err)
		}
	}

	run := runner.New(gh, st)
	sched := scheduler.New()

	// NewServer installs the cron job (full pipeline) for the stored schedule.
	srv, err := web.NewServer(st, run, sched)
	if err != nil {
		log.Fatal(err)
	}
	sched.Start()
	defer sched.Stop()

	authenticator, err := auth.New(context.Background(), auth.FromEnv())
	if err != nil {
		log.Fatal(err)
	}

	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           srv.Handler(authenticator),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("readme-spotlight listening on %s", *addr)
	if err := httpSrv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

// envOr returns the environment variable value for key, or def when unset.
func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// ensureSQLiteDir creates the parent directory for a sqlite DSN if needed.
func ensureSQLiteDir(dsn string) error {
	if !strings.HasPrefix(dsn, "sqlite:") {
		return nil
	}
	dir := filepath.Dir(strings.TrimPrefix(dsn, "sqlite:"))
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}
