package auth

import (
	"crypto/sha256"
	"crypto/subtle"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// MinAPITokenLength is the shortest RS_API_TOKEN the service will start with.
// The token authorises writes that end up in a Git repository under the user's
// name, so a hand-typed password is not acceptable — generate one, e.g.
// `openssl rand -hex 32`.
const MinAPITokenLength = 32

// Rate limiting is GLOBAL rather than per-IP: behind an ingress the only source
// of a client address is X-Forwarded-For, which the caller controls, so a per-IP
// budget would be bypassed by rotating the header.
//
// Authorised and rejected requests draw on separate budgets. Sharing one would
// let anyone who can reach the endpoint starve the legitimate caller with a flood
// of anonymous requests, without ever knowing the token.
const (
	apiOKBurst    = 20
	apiOKRefill   = time.Second // 60/min sustained for the real caller
	apiBadBurst   = 5
	apiBadRefill  = 6 * time.Second // 10/min for rejected attempts
	apiMaxFailure = 10              // consecutive failures before lockout
	apiLockout    = 5 * time.Minute
)

// APIGuard authenticates machine callers with a static bearer token, rate-limits
// them and locks out after repeated authentication failures.
//
// It is mounted outside the interactive authenticator so an agent never has to
// complete a browser login. That makes it a second front door, so it stays closed
// unless RS_API_TOKEN is set and holds full strength on its own.
type APIGuard struct {
	hash [sha256.Size]byte

	ok  *bucket // budget for authorised requests
	bad *bucket // budget for rejected requests

	mu        sync.Mutex
	failures  int
	lockedTil time.Time
}

// NewAPIGuard builds the guard for the configured token. A blank token returns
// (nil, nil), meaning the API stays disabled — the default.
func NewAPIGuard(token string) (*APIGuard, error) {
	if token == "" {
		return nil, nil
	}
	if len(token) < MinAPITokenLength {
		return nil, fmt.Errorf("RS_API_TOKEN must be at least %d characters (got %d)", MinAPITokenLength, len(token))
	}
	return &APIGuard{
		hash: sha256.Sum256([]byte(token)),
		ok:   newBucket(apiOKBurst, apiOKRefill),
		bad:  newBucket(apiBadBurst, apiBadRefill),
	}, nil
}

// Wrap authenticates the caller, then applies the budget that matches the
// outcome, before delegating.
func (g *APIGuard) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if retry, locked := g.lockedFor(); locked {
			refuse(w, retry)
			return
		}
		if !g.authorized(r) {
			g.recordFailure()
			if retry, ok := g.bad.take(); !ok {
				refuse(w, retry)
				return
			}
			w.Header().Set("WWW-Authenticate", `Bearer realm="readme-spotlight"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if retry, ok := g.ok.take(); !ok {
			refuse(w, retry)
			return
		}
		g.recordSuccess()
		next.ServeHTTP(w, r)
	})
}

// authorized compares the bearer token in constant time. Both sides are hashed
// first so the comparison is over fixed-length input and cannot leak the token
// length. The token is accepted from the Authorization header only — a query
// parameter would end up in proxy access logs.
func (g *APIGuard) authorized(r *http.Request) bool {
	scheme, value, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "bearer") {
		return false
	}
	got := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return subtle.ConstantTimeCompare(got[:], g.hash[:]) == 1
}

// lockedFor reports the remaining lockout window, if any.
func (g *APIGuard) lockedFor() (time.Duration, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if d := time.Until(g.lockedTil); d > 0 {
		return d, true
	}
	return 0, false
}

func (g *APIGuard) recordFailure() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.failures++
	if g.failures >= apiMaxFailure {
		g.failures = 0
		g.lockedTil = time.Now().Add(apiLockout)
	}
}

func (g *APIGuard) recordSuccess() {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.failures = 0
}

func refuse(w http.ResponseWriter, retry time.Duration) {
	w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
	http.Error(w, "too many requests", http.StatusTooManyRequests)
}

// bucket is a token bucket: burst requests are available at once, then one more
// every refill interval.
type bucket struct {
	burst  float64
	refill time.Duration

	mu        sync.Mutex
	allowance float64
	last      time.Time
}

func newBucket(burst int, refill time.Duration) *bucket {
	return &bucket{
		burst:     float64(burst),
		refill:    refill,
		allowance: float64(burst),
		last:      time.Now(),
	}
}

// take consumes one token, reporting how long to wait when none is available.
func (b *bucket) take() (time.Duration, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	b.allowance = min(b.allowance+now.Sub(b.last).Seconds()/b.refill.Seconds(), b.burst)
	b.last = now
	if b.allowance < 1 {
		return b.refill, false
	}
	b.allowance--
	return 0, true
}
