// Package runner wires the pipeline steps together: collecting contributions,
// persisting a snapshot, rendering the block, and publishing it to a README.
package runner

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/obervinov/readme-spotlight/internal/config"
	"github.com/obervinov/readme-spotlight/internal/github"
	"github.com/obervinov/readme-spotlight/internal/model"
	"github.com/obervinov/readme-spotlight/internal/publish"
	"github.com/obervinov/readme-spotlight/internal/render"
	"github.com/obervinov/readme-spotlight/internal/store"
)

// Runner executes the individual pipeline steps.
type Runner struct {
	gh    *github.Client
	store *store.Store
}

// New returns a Runner bound to a GitHub client and store.
func New(gh *github.Client, st *store.Store) *Runner {
	return &Runner{gh: gh, store: st}
}

// Refresh collects the latest external contributions and stores a snapshot. It
// does not touch the target README. Returns the number of repositories found.
func (r *Runner) Refresh(ctx context.Context) (int, error) {
	contribs, err := r.gh.CollectExternal(ctx)
	if err != nil {
		return 0, err
	}
	if _, err := r.store.SaveSnapshot(time.Now(), contribs); err != nil {
		return 0, err
	}
	return len(contribs), nil
}

// Compose builds the full managed region (all enabled sections in order) plus
// their SVG assets, from the stored config and the latest snapshot. Sections
// that need data render empty until a refresh has run.
func (r *Runner) Compose() (config.Config, render.Output, error) {
	cfg, _, err := r.store.GetConfig()
	if err != nil {
		return cfg, render.Output{}, err
	}
	var contribs []model.Contribution
	if snap, err := r.store.LatestSnapshot(); err == nil && snap != nil {
		contribs = snap.Contributions
	}
	return cfg, compose(cfg, contribs), nil
}

// compose concatenates the section blocks and merges their assets in order.
func compose(cfg config.Config, contribs []model.Contribution) render.Output {
	var blocks []string
	assets := map[string]string{}
	add := func(o render.Output) {
		if o.Block != "" {
			blocks = append(blocks, o.Block)
		}
		for k, v := range o.Assets {
			assets[k] = v
		}
	}

	if cfg.Banner.Enabled {
		add(render.Banner(cfg.Banner))
	}
	if cfg.Positioning.Enabled {
		add(render.Positioning(cfg.Positioning))
	}
	if cfg.Focus.Enabled {
		add(render.Focus(cfg.Focus))
	}
	if cfg.Tech.Enabled {
		add(render.Tech(cfg.Tech))
	}
	add(render.RenderOutput(contribs, cfg.RenderOptions()))

	return render.Output{Block: strings.Join(blocks, "\n\n"), Assets: assets}
}

// DefaultPRBranch is the head branch used when publishing is forced through a
// pull request and no branch is configured.
const DefaultPRBranch = "readme-spotlight/update"

// Publish renders the composed region from the latest snapshot and writes it to
// the target README (PR or direct commit, per config). It never collects fresh
// data, so it is cheap to invoke repeatedly while testing.
func (r *Runner) Publish(ctx context.Context) (publish.Result, error) {
	return r.publish(ctx, false)
}

// PublishPR publishes through a pull request whatever the stored publish mode
// says. The machine API uses this: an automated caller can propose a README
// change but never land a commit unreviewed.
func (r *Runner) PublishPR(ctx context.Context) (publish.Result, error) {
	return r.publish(ctx, true)
}

func (r *Runner) publish(ctx context.Context, forcePR bool) (publish.Result, error) {
	cfg, out, err := r.Compose()
	if err != nil {
		return publish.Result{}, err
	}
	if forcePR {
		// cfg is a copy, so the stored configuration is untouched.
		cfg.PublishMode = "pr"
		if cfg.PRBranch == "" {
			cfg.PRBranch = DefaultPRBranch
		}
	}
	if cfg.TargetRepo == "" {
		return publish.Result{}, errors.New("set a target repository first")
	}
	snap, err := r.store.LatestSnapshot()
	if err != nil {
		return publish.Result{}, err
	}
	if snap == nil {
		return publish.Result{}, errors.New("no data yet — refresh first")
	}
	return publish.Publish(ctx, r.gh, cfg, out.Block, out.Assets)
}

// Full runs a refresh and then, if a target repository is configured, publishes.
// This is what the scheduler invokes.
func (r *Runner) Full(ctx context.Context) (count int, pub publish.Result, published bool, err error) {
	count, err = r.Refresh(ctx)
	if err != nil {
		return 0, publish.Result{}, false, err
	}
	cfg, _, err := r.store.GetConfig()
	if err != nil {
		return count, publish.Result{}, false, err
	}
	if cfg.TargetRepo == "" {
		return count, publish.Result{}, false, nil
	}
	pub, err = r.Publish(ctx)
	return count, pub, err == nil, err
}
