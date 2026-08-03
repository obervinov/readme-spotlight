package config

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/obervinov/readme-spotlight/internal/render"
)

// The content subset is what an automated caller (see the HTTP API) is allowed
// to touch: the words and colours of the rendered sections, and nothing else.
//
// Deliberately absent are TargetRepo, TargetBranch, ReadmePath, the markers,
// PublishMode, PRBranch and Schedule. Those decide *where* and *how* the service
// writes with its GitHub token, so keeping them out of reach caps the damage a
// leaked API token can do to "the profile README says something silly" — via a
// pull request, never a direct commit.
type Content struct {
	Banner      render.BannerConfig      `json:"banner"`
	Positioning render.PositioningConfig `json:"positioning"`
	Focus       render.FocusConfig       `json:"focus"`
	Tech        render.TechConfig        `json:"tech"`

	Title  string `json:"title"`
	Format string `json:"format"`
	SortBy string `json:"sort_by"`
	Limit  int    `json:"limit"`
}

// ContentOf projects a stored configuration onto the content subset.
func ContentOf(c Config) Content {
	return Content{
		Banner:      c.Banner,
		Positioning: c.Positioning,
		Focus:       c.Focus,
		Tech:        c.Tech,
		Title:       c.Title,
		Format:      c.Format,
		SortBy:      c.SortBy,
		Limit:       c.Limit,
	}
}

// ContentPatch is a partial update of the content subset: fields left out of the
// JSON body keep their stored value. Section objects are replaced wholesale, so
// a caller editing one focus item sends the whole focus section back.
type ContentPatch struct {
	Banner      *render.BannerConfig      `json:"banner,omitempty"`
	Positioning *render.PositioningConfig `json:"positioning,omitempty"`
	Focus       *render.FocusConfig       `json:"focus,omitempty"`
	Tech        *render.TechConfig        `json:"tech,omitempty"`

	Title  *string `json:"title,omitempty"`
	Format *string `json:"format,omitempty"`
	SortBy *string `json:"sort_by,omitempty"`
	Limit  *int    `json:"limit,omitempty"`
}

// Field length and cardinality limits. They bound how much text a caller can
// push into the README even with a valid token.
const (
	maxShortText = 200  // headline-sized strings
	maxLongText  = 600  // taglines and blurbs
	maxFocusItem = 24   // focus rows
	maxTechGroup = 24   // tech groups
	maxTechItems = 40   // items per tech group
	maxLimitRows = 1000 // contributions rows
)

var hexColour = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// Apply validates the patch and returns cfg with the provided fields replaced,
// alongside the names of the fields the caller supplied (for the audit log). cfg
// is taken by value, so a rejected patch leaves the caller's copy untouched.
func (p ContentPatch) Apply(cfg Config) (Config, []string, error) {
	var changed []string

	if p.Banner != nil {
		if err := validateBanner(*p.Banner); err != nil {
			return cfg, nil, err
		}
		cfg.Banner = *p.Banner
		changed = append(changed, "banner")
	}
	if p.Positioning != nil {
		if err := validatePositioning(*p.Positioning); err != nil {
			return cfg, nil, err
		}
		cfg.Positioning = *p.Positioning
		changed = append(changed, "positioning")
	}
	if p.Focus != nil {
		if err := validateFocus(*p.Focus); err != nil {
			return cfg, nil, err
		}
		cfg.Focus = *p.Focus
		changed = append(changed, "focus")
	}
	if p.Tech != nil {
		if err := validateTech(*p.Tech); err != nil {
			return cfg, nil, err
		}
		cfg.Tech = *p.Tech
		changed = append(changed, "tech")
	}
	if p.Title != nil {
		if err := validateText("title", *p.Title, maxShortText); err != nil {
			return cfg, nil, err
		}
		cfg.Title = *p.Title
		changed = append(changed, "title")
	}
	if p.Format != nil {
		if err := validateEnum("format", *p.Format, render.FormatTable, render.FormatDetails, render.FormatSVG, render.FormatHybrid); err != nil {
			return cfg, nil, err
		}
		cfg.Format = *p.Format
		changed = append(changed, "format")
	}
	if p.SortBy != nil {
		if err := validateEnum("sort_by", *p.SortBy, "stars", "total"); err != nil {
			return cfg, nil, err
		}
		cfg.SortBy = *p.SortBy
		changed = append(changed, "sort_by")
	}
	if p.Limit != nil {
		if *p.Limit < 0 || *p.Limit > maxLimitRows {
			return cfg, nil, fmt.Errorf("limit must be between 0 and %d", maxLimitRows)
		}
		cfg.Limit = *p.Limit
		changed = append(changed, "limit")
	}

	if len(changed) == 0 {
		return cfg, nil, fmt.Errorf("patch is empty: provide at least one content field")
	}
	return cfg, changed, nil
}

func validateBanner(b render.BannerConfig) error {
	if err := validateAccent("banner.accent", b.Accent); err != nil {
		return err
	}
	if err := validateText("banner.name", b.Name, maxShortText); err != nil {
		return err
	}
	if err := validateText("banner.role", b.Role, maxShortText); err != nil {
		return err
	}
	return validateText("banner.tagline", b.Tagline, maxLongText)
}

func validatePositioning(p render.PositioningConfig) error {
	if err := validateAccent("positioning.accent", p.Accent); err != nil {
		return err
	}
	return validateText("positioning.text", p.Text, maxLongText)
}

func validateFocus(f render.FocusConfig) error {
	if err := validateAccent("focus.accent", f.Accent); err != nil {
		return err
	}
	if err := validateText("focus.title", f.Title, maxShortText); err != nil {
		return err
	}
	if len(f.Items) > maxFocusItem {
		return fmt.Errorf("focus.items: at most %d items (got %d)", maxFocusItem, len(f.Items))
	}
	for i, it := range f.Items {
		if err := validateText(fmt.Sprintf("focus.items[%d].title", i), it.Title, maxShortText); err != nil {
			return err
		}
		if err := validateText(fmt.Sprintf("focus.items[%d].text", i), it.Text, maxLongText); err != nil {
			return err
		}
	}
	return nil
}

func validateTech(t render.TechConfig) error {
	if err := validateAccent("tech.accent", t.Accent); err != nil {
		return err
	}
	if err := validateText("tech.title", t.Title, maxShortText); err != nil {
		return err
	}
	if len(t.Groups) > maxTechGroup {
		return fmt.Errorf("tech.groups: at most %d groups (got %d)", maxTechGroup, len(t.Groups))
	}
	for i, g := range t.Groups {
		if err := validateText(fmt.Sprintf("tech.groups[%d].name", i), g.Name, maxShortText); err != nil {
			return err
		}
		if len(g.Items) > maxTechItems {
			return fmt.Errorf("tech.groups[%d].items: at most %d items (got %d)", i, maxTechItems, len(g.Items))
		}
		for j, it := range g.Items {
			if err := validateText(fmt.Sprintf("tech.groups[%d].items[%d]", i, j), it, maxShortText); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateAccent keeps colours to plain hex. The renderers interpolate accents
// straight into SVG attributes, so anything else would be markup injection into
// a file this service commits to a repository.
func validateAccent(field, v string) error {
	if v == "" || hexColour.MatchString(v) {
		return nil
	}
	return fmt.Errorf("%s: want a hex colour like #3fb950, got %q", field, v)
}

// validateText rejects over-long strings and control characters (which would
// corrupt the generated SVG and Markdown).
func validateText(field, v string, max int) error {
	if len([]rune(v)) > max {
		return fmt.Errorf("%s: at most %d characters (got %d)", field, max, len([]rune(v)))
	}
	if i := strings.IndexFunc(v, isControl); i >= 0 {
		return fmt.Errorf("%s: contains a control character at byte %d", field, i)
	}
	return nil
}

func isControl(r rune) bool {
	return r == '\r' || (r < 0x20 && r != '\n' && r != '\t') || r == 0x7f
}

func validateEnum(field, v string, allowed ...string) error {
	if slices.Contains(allowed, v) {
		return nil
	}
	return fmt.Errorf("%s: want one of %s, got %q", field, strings.Join(allowed, ", "), v)
}
