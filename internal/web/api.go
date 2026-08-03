package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/obervinov/readme-spotlight/internal/config"
	"github.com/obervinov/readme-spotlight/internal/logs"
	"github.com/obervinov/readme-spotlight/internal/publish"
)

// The machine API lets an agent keep the README's wording in sync with an
// external source (a CV, for instance) without driving the browser UI.
//
// Its surface is deliberately narrow, because it authenticates with a static
// token and the service holds a GitHub token that can write to repositories:
//
//	GET   /api/content  — read the editable content
//	PATCH /api/content  — replace some content fields (nothing else is accepted)
//	POST  /api/publish  — publish, always as a pull request
//
// Everything that decides *where* and *how* publishing happens (target repo,
// branch, README path, markers, publish mode, schedule) is reachable only from
// the authenticated UI. See config.Content for the reasoning.
const (
	apiMaxBody      = 64 << 10 // 64 KiB is far more than a content patch needs
	apiRunTimeout   = 2 * time.Minute
	apiContentRoute = "/api/content"
)

// apiMux routes the machine API. The guard is applied by the caller.
func (s *Server) apiMux() http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET "+apiContentRoute, s.apiGetContent)
	m.HandleFunc("PATCH "+apiContentRoute, s.apiPatchContent)
	m.HandleFunc("POST /api/publish", s.apiPublish)
	return m
}

func (s *Server) apiGetContent(w http.ResponseWriter, _ *http.Request) {
	cfg, _, err := s.store.GetConfig()
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, config.ContentOf(cfg))
}

func (s *Server) apiPatchContent(w http.ResponseWriter, r *http.Request) {
	if !isJSON(r) {
		apiError(w, http.StatusUnsupportedMediaType, "expected Content-Type: application/json")
		return
	}

	var patch config.ContentPatch
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, apiMaxBody))
	// Unknown fields are an error rather than a silent no-op: a caller trying to
	// set target_repo or publish_mode gets told those are out of reach instead of
	// believing the write landed.
	dec.DisallowUnknownFields()
	if err := dec.Decode(&patch); err != nil {
		apiError(w, http.StatusBadRequest, "invalid body: "+err.Error())
		return
	}

	cfg, _, err := s.store.GetConfig()
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	updated, changed, err := patch.Apply(cfg)
	if err != nil {
		apiError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := s.store.SetConfig(updated); err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}
	logs.Infof("api: content updated — fields=%s", strings.Join(changed, ","))
	writeJSON(w, http.StatusOK, config.ContentOf(updated))
}

func (s *Server) apiPublish(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), apiRunTimeout)
	defer cancel()

	var res publish.Result
	_, err, busy := s.runSync(ctx, "api-publish", func(ctx context.Context) (string, error) {
		var err error
		res, err = s.run.PublishPR(ctx)
		if err != nil {
			return "", err
		}
		return publishMessage(res), nil
	})
	switch {
	case busy:
		apiError(w, http.StatusConflict, "a run is already in progress")
	case err != nil:
		apiError(w, http.StatusBadGateway, err.Error())
	default:
		writeJSON(w, http.StatusOK, res)
	}
}

// isJSON rejects anything but a JSON body, which also keeps browser form posts
// from reaching the API.
func isJSON(r *http.Request) bool {
	ct := r.Header.Get("Content-Type")
	base, _, _ := strings.Cut(ct, ";")
	return strings.EqualFold(strings.TrimSpace(base), "application/json")
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		logs.Infof("api: writing response failed — %v", err)
	}
}

func apiError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// errAPIDisabled documents the closed-by-default state in one place.
var errAPIDisabled = errors.New("machine API is disabled (RS_API_TOKEN is not set)")
