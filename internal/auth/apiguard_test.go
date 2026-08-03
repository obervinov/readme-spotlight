package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const testToken = "0123456789abcdef0123456789abcdef" // 32 chars

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

func TestNewAPIGuardDisabledWhenTokenEmpty(t *testing.T) {
	g, err := NewAPIGuard("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g != nil {
		t.Fatal("want a nil guard (API disabled) for an empty token")
	}
}

func TestNewAPIGuardRejectsShortToken(t *testing.T) {
	if _, err := NewAPIGuard(strings.Repeat("a", MinAPITokenLength-1)); err == nil {
		t.Fatal("want an error for a token below the minimum length")
	}
}

func TestWrapAuthorization(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   int
	}{
		{"valid bearer", "Bearer " + testToken, http.StatusOK},
		{"case-insensitive scheme", "bearer " + testToken, http.StatusOK},
		{"wrong token", "Bearer " + strings.Repeat("b", 32), http.StatusUnauthorized},
		{"no header", "", http.StatusUnauthorized},
		{"wrong scheme", "Basic " + testToken, http.StatusUnauthorized},
		{"token without scheme", testToken, http.StatusUnauthorized},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			g, err := NewAPIGuard(testToken)
			if err != nil {
				t.Fatal(err)
			}
			req := httptest.NewRequest(http.MethodGet, "/api/content", nil)
			if tc.header != "" {
				req.Header.Set("Authorization", tc.header)
			}
			rec := httptest.NewRecorder()
			g.Wrap(okHandler()).ServeHTTP(rec, req)
			if rec.Code != tc.want {
				t.Fatalf("status = %d, want %d", rec.Code, tc.want)
			}
		})
	}
}

func TestWrapRejectsTokenInQueryString(t *testing.T) {
	g, err := NewAPIGuard(testToken)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/content?token="+testToken, nil)
	rec := httptest.NewRecorder()
	g.Wrap(okHandler()).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 — the token must not be accepted from the URL", rec.Code)
	}
}

// callWith drives the guard with a given bearer token and returns the status.
func callWith(h http.Handler, token string) int {
	req := httptest.NewRequest(http.MethodGet, "/api/content", nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec.Code
}

func TestWrapRateLimitsAuthorizedBurst(t *testing.T) {
	g, err := NewAPIGuard(testToken)
	if err != nil {
		t.Fatal(err)
	}
	h := g.Wrap(okHandler())
	for i := range apiOKBurst {
		if code := callWith(h, testToken); code != http.StatusOK {
			t.Fatalf("request %d: status = %d, want 200 within the burst", i+1, code)
		}
	}
	if code := callWith(h, testToken); code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 once the burst is spent", code)
	}
}

// A flood of anonymous requests must not consume the authorised caller's budget:
// otherwise anyone able to reach the endpoint could starve the agent without
// knowing the token.
func TestRejectedRequestsDoNotStarveAuthorizedCaller(t *testing.T) {
	g, err := NewAPIGuard(testToken)
	if err != nil {
		t.Fatal(err)
	}
	h := g.Wrap(okHandler())
	for range apiBadBurst {
		if code := callWith(h, "wrong-token-wrong-token-wrong-tok"); code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401 while the failure budget lasts", code)
		}
	}
	// Further anonymous attempts are throttled...
	if code := callWith(h, ""); code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 once the failure budget is spent", code)
	}
	// ...and the real caller still gets through.
	if code := callWith(h, testToken); code != http.StatusOK {
		t.Fatalf("status = %d, want 200 for the authorised caller", code)
	}
}

func TestWrapLocksOutAfterRepeatedFailures(t *testing.T) {
	g, err := NewAPIGuard(testToken)
	if err != nil {
		t.Fatal(err)
	}
	h := g.Wrap(okHandler())
	// The failure budget is smaller than the lockout threshold, so refill it
	// between attempts to isolate the lockout from the rate limit.
	for range apiMaxFailure {
		g.bad.mu.Lock()
		g.bad.allowance = g.bad.burst
		g.bad.mu.Unlock()
		if code := callWith(h, "wrong-token-wrong-token-wrong-tok"); code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401 while failures accumulate", code)
		}
	}
	// Even the correct token is refused during the lockout window.
	if code := callWith(h, testToken); code != http.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 during lockout", code)
	}
}

func TestBucketRefills(t *testing.T) {
	b := newBucket(1, 10*time.Millisecond)
	if _, ok := b.take(); !ok {
		t.Fatal("the first take must succeed")
	}
	if _, ok := b.take(); ok {
		t.Fatal("the second take must be refused")
	}
	// Rewind the clock reference instead of sleeping.
	b.mu.Lock()
	b.last = b.last.Add(-time.Second)
	b.mu.Unlock()
	if _, ok := b.take(); !ok {
		t.Fatal("the bucket must refill over time")
	}
}

func TestFromEnvReadsAPIToken(t *testing.T) {
	t.Setenv("RS_API_TOKEN", testToken)
	if got := FromEnv().APIToken; got != testToken {
		t.Fatalf("APIToken = %q, want the RS_API_TOKEN value", got)
	}
}
