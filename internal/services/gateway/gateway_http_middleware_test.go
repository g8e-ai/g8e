// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/response"
)

// newMiddlewareTestHandler creates a minimal HTTPHandler with only the fields
// required by the middleware functions under test. No DB, PKI, or external deps.
func newMiddlewareTestHandler(t *testing.T) *HTTPHandler {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelError}))
	resp := response.NewWriter(logger)
	cfg := &config.Config{}

	return &HTTPHandler{
		cfg:             cfg,
		logger:          logger,
		responder:       resp,
		limiters:        make(map[string]*tokenBucket),
		limiterLastUsed: make(map[string]time.Time),
	}
}

// --- containsTraversal ---

func TestContainsTraversal_DetectsTraversal(t *testing.T) {
	h := &HTTPHandler{}

	paths := []string{
		"../",
		"../etc/passwd",
		"/a/../b",
		"/foo/../../bar",
		"/..",
		"/../secret",
		"/api/../config",
		"..",
	}

	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			assert.True(t, h.containsTraversal(p), "expected traversal in %q", p)
		})
	}
}

func TestContainsTraversal_NoTraversal(t *testing.T) {
	h := &HTTPHandler{}

	paths := []string{
		"/",
		"/api/v1/users",
		"/db/settings/platform",
		"/a/b/c",
		"",
		"/normal-path",
		"/end-with-slash/",
		"/double//slash",
	}

	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			assert.False(t, h.containsTraversal(p), "expected no traversal in %q", p)
		})
	}
}

func TestContainsTraversal_EncodedDotsNotDetected(t *testing.T) {
	h := &HTTPHandler{}

	// containsTraversal operates on the raw string, not URL-decoded.
	// %2e%2e is the encoded form of ".." and should NOT be detected
	// by this function (that's the responsibility of the HTTP layer).
	paths := []string{
		"/%2e%2e/secret",
		"/%2e%2e%2e/secret",
		"/foo/%2e%2e/bar",
	}

	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			assert.False(t, h.containsTraversal(p), "encoded dots should not be detected by containsTraversal")
		})
	}
}

func TestContainsTraversal_DotSegmentsOnly(t *testing.T) {
	h := &HTTPHandler{}

	// Single dots and dotfiles should not be flagged as traversal
	paths := []string{
		"/.hidden",
		"/.env",
		"/path/./to/file",
		"/file.txt",
		"/.gitignore",
	}

	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			assert.False(t, h.containsTraversal(p), "single dots and dotfiles should not be traversal")
		})
	}
}

// --- pathTraversalGuard ---

func TestPathTraversalGuard_RejectsTraversalPaths(t *testing.T) {
	h := newMiddlewareTestHandler(t)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := h.pathTraversalGuard(next)

	paths := []string{
		"/db/users/../u1",
		"/../etc/passwd",
		"/api/../../config",
		"/..",
		"/../secret",
	}

	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, p, nil)
			rr := httptest.NewRecorder()
			mw.ServeHTTP(rr, req)
			assert.Equal(t, http.StatusBadRequest, rr.Code, "path %q should be rejected", p)
		})
	}
}

func TestPathTraversalGuard_AllowsValidPaths(t *testing.T) {
	h := newMiddlewareTestHandler(t)
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})
	mw := h.pathTraversalGuard(next)

	paths := []string{
		"/",
		"/db/users/u1",
		"/api/v1/health",
		"/kv/some-key",
		"/db/settings/platform_settings",
	}

	for _, p := range paths {
		t.Run(p, func(t *testing.T) {
			nextCalled = false
			req := httptest.NewRequest(http.MethodGet, p, nil)
			rr := httptest.NewRecorder()
			mw.ServeHTTP(rr, req)
			assert.True(t, nextCalled, "next handler should be called for %q", p)
			assert.Equal(t, http.StatusOK, rr.Code)
		})
	}
}

func TestPathTraversalGuard_AllowsTrailingSlashNormalization(t *testing.T) {
	h := newMiddlewareTestHandler(t)
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})
	mw := h.pathTraversalGuard(next)

	// A path like /api/ is cleaned to /api by filepath.Clean, but should
	// still be allowed (the cleaned form + "/" matches the original).
	req := httptest.NewRequest(http.MethodGet, "/api/", nil)
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)
	assert.True(t, nextCalled, "trailing slash should not be blocked")
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestPathTraversalGuard_AllowsDoubleSlashNormalization(t *testing.T) {
	h := newMiddlewareTestHandler(t)
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})
	mw := h.pathTraversalGuard(next)

	// Double slashes are cleaned by filepath.Clean but should not be
	// rejected as long as no ".." segment is present.
	req := httptest.NewRequest(http.MethodGet, "/api//v1", nil)
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)
	assert.True(t, nextCalled, "double slash should not be blocked")
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestPathTraversalGuard_RejectsEncodedTraversal(t *testing.T) {
	h := newMiddlewareTestHandler(t)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := h.pathTraversalGuard(next)

	// %2e%2e is URL-decoded to ".." by httptest.NewRequest, so the
	// guard should catch it.
	req := httptest.NewRequest(http.MethodGet, "/db/users/%2e%2e/u1", nil)
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusBadRequest, rr.Code, "encoded traversal should be rejected")
}

func TestPathTraversalGuard_PreservesQueryParams(t *testing.T) {
	h := newMiddlewareTestHandler(t)
	nextCalled := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})
	mw := h.pathTraversalGuard(next)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users?limit=10&offset=5", nil)
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)
	assert.True(t, nextCalled, "valid path with query params should pass through")
	assert.Equal(t, http.StatusOK, rr.Code)
}

// --- rateLimitMiddleware ---

func TestRateLimitMiddleware_DisabledPassesThrough(t *testing.T) {
	h := newMiddlewareTestHandler(t)
	h.cfg.Gateway.RateLimitRPS = 0
	h.cfg.Gateway.RateLimitBurst = 0

	nextCalled := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled++
		w.WriteHeader(http.StatusOK)
	})
	mw := h.rateLimitMiddleware(next)

	for i := 0; i < 50; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code, "request %d should pass through", i)
	}
	assert.Equal(t, 50, nextCalled, "all requests should pass through when rate limiting is disabled")
}

func TestRateLimitMiddleware_NegativeRPSPassesThrough(t *testing.T) {
	h := newMiddlewareTestHandler(t)
	h.cfg.Gateway.RateLimitRPS = -1

	nextCalled := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled++
		w.WriteHeader(http.StatusOK)
	})
	mw := h.rateLimitMiddleware(next)

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)
	assert.Equal(t, 1, nextCalled)
	assert.Equal(t, http.StatusOK, rr.Code)
}

func TestRateLimitMiddleware_RequestsWithinBurstAllowed(t *testing.T) {
	h := newMiddlewareTestHandler(t)
	h.cfg.Gateway.RateLimitRPS = 100 // high rate so tokens replenish fast
	h.cfg.Gateway.RateLimitBurst = 5

	nextCalled := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled++
		w.WriteHeader(http.StatusOK)
	})
	mw := h.rateLimitMiddleware(next)

	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = "10.0.0.1:9999"
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code, "request %d within burst should be allowed", i)
	}
	assert.Equal(t, 5, nextCalled, "all 5 burst requests should be allowed")
}

func TestRateLimitMiddleware_ExceedingBurstReturns429(t *testing.T) {
	h := newMiddlewareTestHandler(t)
	h.cfg.Gateway.RateLimitRPS = 0.01 // very slow refill
	h.cfg.Gateway.RateLimitBurst = 2

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := h.rateLimitMiddleware(next)

	addr := "10.0.0.2:8888"

	// First 2 requests consume the burst
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = addr
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code, "request %d should be allowed", i)
	}

	// 3rd request should be rate limited
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = addr
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusTooManyRequests, rr.Code, "request exceeding burst should get 429")
	assert.JSONEq(t, `{"error":"rate limit exceeded"}`, rr.Body.String())
}

func TestRateLimitMiddleware_DifferentIPsTrackedSeparately(t *testing.T) {
	h := newMiddlewareTestHandler(t)
	h.cfg.Gateway.RateLimitRPS = 0.01
	h.cfg.Gateway.RateLimitBurst = 1

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := h.rateLimitMiddleware(next)

	// IP1 exhausts its burst
	req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req1.RemoteAddr = "10.0.0.10:1234"
	rr1 := httptest.NewRecorder()
	mw.ServeHTTP(rr1, req1)
	assert.Equal(t, http.StatusOK, rr1.Code)

	// IP1 second request should be rate limited
	req1b := httptest.NewRequest(http.MethodGet, "/test", nil)
	req1b.RemoteAddr = "10.0.0.10:1234"
	rr1b := httptest.NewRecorder()
	mw.ServeHTTP(rr1b, req1b)
	assert.Equal(t, http.StatusTooManyRequests, rr1b.Code, "IP1 should be rate limited after burst")

	// IP2 should still be allowed (separate limiter)
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.RemoteAddr = "10.0.0.20:5678"
	rr2 := httptest.NewRecorder()
	mw.ServeHTTP(rr2, req2)
	assert.Equal(t, http.StatusOK, rr2.Code, "IP2 should have its own limiter")
}

func TestRateLimitMiddleware_RemoteAddrWithoutPort(t *testing.T) {
	h := newMiddlewareTestHandler(t)
	h.cfg.Gateway.RateLimitRPS = 100
	h.cfg.Gateway.RateLimitBurst = 1

	nextCalled := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled++
		w.WriteHeader(http.StatusOK)
	})
	mw := h.rateLimitMiddleware(next)

	// RemoteAddr without port — net.SplitHostPort will fail, so the
	// full string is used as the IP key.
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "10.0.0.30"
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, 1, nextCalled)

	// Verify the limiter was stored under the raw RemoteAddr
	h.muLimiters.Lock()
	_, ok := h.limiters["10.0.0.30"]
	h.muLimiters.Unlock()
	assert.True(t, ok, "limiter should be keyed by raw RemoteAddr when SplitHostPort fails")
}

func TestRateLimitMiddleware_StaleLimiterCleanup(t *testing.T) {
	h := newMiddlewareTestHandler(t)
	h.cfg.Gateway.RateLimitRPS = 10
	h.cfg.Gateway.RateLimitBurst = 10

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := h.rateLimitMiddleware(next)

	// Create limiters for 3 IPs
	ips := []string{"10.0.0.40:1", "10.0.0.41:1", "10.0.0.42:1"}
	for _, addr := range ips {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.RemoteAddr = addr
		rr := httptest.NewRecorder()
		mw.ServeHTTP(rr, req)
	}

	h.muLimiters.Lock()
	assert.Len(t, h.limiters, 3, "should have 3 limiters")
	// Age out all limiters by setting lastUsed to 10 minutes ago
	for ip := range h.limiters {
		h.limiterLastUsed[ip] = time.Now().Add(-10 * time.Minute)
	}
	h.muLimiters.Unlock()

	// A new request from a 4th IP should trigger cleanup
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = "10.0.0.43:1"
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	h.muLimiters.Lock()
	// Only the new IP's limiter should remain
	assert.Len(t, h.limiters, 1, "stale limiters should be cleaned up, only the new one should remain")
	_, ok := h.limiters["10.0.0.43"]
	h.muLimiters.Unlock()
	assert.True(t, ok, "the new IP's limiter should exist")
}

func TestRateLimitMiddleware_ReusesExistingLimiter(t *testing.T) {
	h := newMiddlewareTestHandler(t)
	h.cfg.Gateway.RateLimitRPS = 100
	h.cfg.Gateway.RateLimitBurst = 10

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := h.rateLimitMiddleware(next)

	addr := "10.0.0.50:1234"

	// First request creates the limiter
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = addr
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	h.muLimiters.Lock()
	limiter1 := h.limiters["10.0.0.50"]
	h.muLimiters.Unlock()
	require.NotNil(t, limiter1)

	// Second request should reuse the same limiter
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.RemoteAddr = addr
	rr2 := httptest.NewRecorder()
	mw.ServeHTTP(rr2, req2)

	h.muLimiters.Lock()
	limiter2 := h.limiters["10.0.0.50"]
	h.muLimiters.Unlock()
	assert.Same(t, limiter1, limiter2, "should reuse the same limiter for the same IP")
}

func TestRateLimitMiddleware_ConcurrentRequestsSafe(t *testing.T) {
	h := newMiddlewareTestHandler(t)
	h.cfg.Gateway.RateLimitRPS = 1000
	h.cfg.Gateway.RateLimitBurst = 100

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := h.rateLimitMiddleware(next)

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.RemoteAddr = "10.0.0.60:1234"
			rr := httptest.NewRecorder()
			mw.ServeHTTP(rr, req)
			// Status should be either 200 (within burst) or 429 (rate limited)
			assert.LessOrEqual(t, rr.Code, 429)
		}(i)
	}
	wg.Wait()

	// If we get here without a race detector failure, the test passes
	h.muLimiters.Lock()
	assert.NotEmpty(t, h.limiters, "should have at least one limiter after concurrent requests")
	h.muLimiters.Unlock()
}

func TestRateLimitMiddleware_UpdatesLimiterLastUsed(t *testing.T) {
	h := newMiddlewareTestHandler(t)
	h.cfg.Gateway.RateLimitRPS = 100
	h.cfg.Gateway.RateLimitBurst = 10

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mw := h.rateLimitMiddleware(next)

	addr := "10.0.0.70:1234"

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.RemoteAddr = addr
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	h.muLimiters.Lock()
	lastUsed, ok := h.limiterLastUsed["10.0.0.70"]
	h.muLimiters.Unlock()
	require.True(t, ok, "limiterLastUsed should have an entry for the IP")
	assert.WithinDuration(t, time.Now(), lastUsed, 2*time.Second, "lastUsed should be recent")
}
