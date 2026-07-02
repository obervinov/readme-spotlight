// Package auth guards the web UI. Three modes, selected by RS_AUTH_MODE:
//
//	none  (default) — no authentication, the UI is open
//	basic          — HTTP Basic auth against RS_BASIC_USER / RS_BASIC_PASSWORD
//	oidc           — OpenID Connect (e.g. Keycloak) authorization-code login
//
// It is off by default so the tool runs out of the box; deployments that expose
// the UI enable a mode explicitly.
package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

// Config is the authentication configuration, read from the environment.
type Config struct {
	Mode string // "none", "basic" or "oidc"

	BasicUser string
	BasicPass string

	OIDCIssuer       string
	OIDCClientID     string
	OIDCClientSecret string
	OIDCRedirectURL  string
	SessionSecret    string
}

// FromEnv reads the auth configuration from RS_* environment variables.
func FromEnv() Config {
	return Config{
		Mode:             strings.ToLower(envOr("RS_AUTH_MODE", "none")),
		BasicUser:        os.Getenv("RS_BASIC_USER"),
		BasicPass:        os.Getenv("RS_BASIC_PASSWORD"),
		OIDCIssuer:       os.Getenv("RS_OIDC_ISSUER_URL"),
		OIDCClientID:     os.Getenv("RS_OIDC_CLIENT_ID"),
		OIDCClientSecret: os.Getenv("RS_OIDC_CLIENT_SECRET"),
		OIDCRedirectURL:  os.Getenv("RS_OIDC_REDIRECT_URL"),
		SessionSecret:    os.Getenv("RS_SESSION_SECRET"),
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Authenticator protects HTTP handlers.
type Authenticator interface {
	// Wrap returns a handler that enforces authentication before delegating.
	Wrap(http.Handler) http.Handler
	// Routes registers any auth endpoints (login/callback/logout) that must be
	// reachable without authentication.
	Routes(mux *http.ServeMux)
}

// New builds the authenticator for the configured mode.
func New(ctx context.Context, cfg Config) (Authenticator, error) {
	switch cfg.Mode {
	case "", "none":
		return noop{}, nil
	case "basic":
		if cfg.BasicUser == "" || cfg.BasicPass == "" {
			return nil, fmt.Errorf("auth mode basic requires RS_BASIC_USER and RS_BASIC_PASSWORD")
		}
		return &basic{user: cfg.BasicUser, pass: cfg.BasicPass}, nil
	case "oidc":
		return newOIDC(ctx, cfg)
	default:
		return nil, fmt.Errorf("unknown RS_AUTH_MODE %q (want none, basic or oidc)", cfg.Mode)
	}
}

// noop lets every request through.
type noop struct{}

func (noop) Wrap(next http.Handler) http.Handler { return next }
func (noop) Routes(*http.ServeMux)               {}

// basic enforces HTTP Basic auth.
type basic struct{ user, pass string }

func (b *basic) Routes(*http.ServeMux) {}

func (b *basic) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		userOK := subtle.ConstantTimeCompare([]byte(u), []byte(b.user)) == 1
		passOK := subtle.ConstantTimeCompare([]byte(p), []byte(b.pass)) == 1
		if !ok || !userOK || !passOK {
			w.Header().Set("WWW-Authenticate", `Basic realm="readme-spotlight"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// oidcAuth performs an OpenID Connect authorization-code flow and keeps a signed
// session cookie.
type oidcAuth struct {
	oauth    oauth2.Config
	verifier *oidc.IDTokenVerifier
	provider *oidc.Provider
	secret   []byte
}

const (
	sessionCookie = "rs_session"
	stateCookie   = "rs_oauth_state"
	sessionTTL    = 12 * time.Hour
)

func newOIDC(ctx context.Context, cfg Config) (*oidcAuth, error) {
	for name, v := range map[string]string{
		"RS_OIDC_ISSUER_URL":    cfg.OIDCIssuer,
		"RS_OIDC_CLIENT_ID":     cfg.OIDCClientID,
		"RS_OIDC_CLIENT_SECRET": cfg.OIDCClientSecret,
		"RS_OIDC_REDIRECT_URL":  cfg.OIDCRedirectURL,
		"RS_SESSION_SECRET":     cfg.SessionSecret,
	} {
		if v == "" {
			return nil, fmt.Errorf("auth mode oidc requires %s", name)
		}
	}
	provider, err := oidc.NewProvider(ctx, cfg.OIDCIssuer)
	if err != nil {
		return nil, fmt.Errorf("oidc discovery: %w", err)
	}
	return &oidcAuth{
		oauth: oauth2.Config{
			ClientID:     cfg.OIDCClientID,
			ClientSecret: cfg.OIDCClientSecret,
			RedirectURL:  cfg.OIDCRedirectURL,
			Endpoint:     provider.Endpoint(),
			Scopes:       []string{oidc.ScopeOpenID, "profile", "email"},
		},
		verifier: provider.Verifier(&oidc.Config{ClientID: cfg.OIDCClientID}),
		provider: provider,
		secret:   []byte(cfg.SessionSecret),
	}, nil
}

func (o *oidcAuth) Routes(mux *http.ServeMux) {
	mux.HandleFunc("GET /auth/login", o.login)
	mux.HandleFunc("GET /auth/callback", o.callback)
	mux.HandleFunc("GET /auth/logout", o.logout)
}

func (o *oidcAuth) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if o.validSession(r) {
			next.ServeHTTP(w, r)
			return
		}
		http.Redirect(w, r, "/auth/login", http.StatusSeeOther)
	})
}

func (o *oidcAuth) login(w http.ResponseWriter, r *http.Request) {
	state := randomToken()
	http.SetCookie(w, &http.Cookie{
		Name: stateCookie, Value: state, Path: "/", HttpOnly: true,
		Secure: true, SameSite: http.SameSiteLaxMode, MaxAge: 600,
	})
	http.Redirect(w, r, o.oauth.AuthCodeURL(state), http.StatusSeeOther)
}

func (o *oidcAuth) callback(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(stateCookie)
	if err != nil || c.Value == "" || c.Value != r.URL.Query().Get("state") {
		http.Error(w, "invalid oauth state", http.StatusBadRequest)
		return
	}
	token, err := o.oauth.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		http.Error(w, "token exchange failed", http.StatusBadGateway)
		return
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		http.Error(w, "no id_token in response", http.StatusBadGateway)
		return
	}
	idToken, err := o.verifier.Verify(r.Context(), raw)
	if err != nil {
		http.Error(w, "id_token verification failed", http.StatusUnauthorized)
		return
	}
	o.setSession(w, idToken.Subject)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (o *oidcAuth) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1})
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// setSession issues a signed "subject|expiry" cookie.
func (o *oidcAuth) setSession(w http.ResponseWriter, subject string) {
	payload := subject + "|" + strconv.FormatInt(time.Now().Add(sessionTTL).Unix(), 10)
	value := base64.RawURLEncoding.EncodeToString([]byte(payload)) + "." + o.sign(payload)
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: value, Path: "/", HttpOnly: true,
		Secure: true, SameSite: http.SameSiteLaxMode, MaxAge: int(sessionTTL.Seconds()),
	})
}

func (o *oidcAuth) validSession(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	b64, mac, ok := strings.Cut(c.Value, ".")
	if !ok {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(b64)
	if err != nil {
		return false
	}
	payload := string(raw)
	if subtle.ConstantTimeCompare([]byte(mac), []byte(o.sign(payload))) != 1 {
		return false
	}
	_, expStr, ok := strings.Cut(payload, "|")
	if !ok {
		return false
	}
	exp, err := strconv.ParseInt(expStr, 10, 64)
	return err == nil && time.Now().Unix() < exp
}

func (o *oidcAuth) sign(payload string) string {
	m := hmac.New(sha256.New, o.secret)
	m.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func randomToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
