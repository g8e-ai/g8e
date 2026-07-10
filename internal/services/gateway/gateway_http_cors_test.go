// Copyright (c) 2026 Lateralus Labs, LLC.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gateway

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
)

func newCORSHTTPHandler(allowedOrigins []string) *HTTPHandler {
	return &HTTPHandler{
		cfg: &config.Config{
			Gateway: config.GatewayConfig{
				AllowedOrigins: allowedOrigins,
			},
		},
	}
}

func TestCORSMiddleware_PassThroughWhenNoAllowedOrigins(t *testing.T) {
	h := newCORSHTTPHandler(nil)
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	mw := h.corsMiddleware(next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://example.com")
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	assert.True(t, called, "next handler must be called when AllowedOrigins is empty")
	assert.Empty(t, rr.Header().Get(constants.HeaderAccessControlAllowOrigin))
	assert.Empty(t, rr.Header().Get(constants.HeaderVary))
}

func TestCORSMiddleware_AllowedOriginSetsHeaders(t *testing.T) {
	h := newCORSHTTPHandler([]string{"https://lovable.dev"})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := h.corsMiddleware(next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://lovable.dev")
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	assert.Equal(t, "https://lovable.dev", rr.Header().Get(constants.HeaderAccessControlAllowOrigin))
	assert.Equal(t, "true", rr.Header().Get(constants.HeaderAccessControlAllowCredentials))
	assert.Equal(t, "Origin", rr.Header().Get(constants.HeaderVary))
}

func TestCORSMiddleware_DisallowedOriginDoesNotSetAllowOrigin(t *testing.T) {
	h := newCORSHTTPHandler([]string{"https://lovable.dev"})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := h.corsMiddleware(next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://evil.com")
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	assert.Empty(t, rr.Header().Get(constants.HeaderAccessControlAllowOrigin))
	assert.Empty(t, rr.Header().Get(constants.HeaderAccessControlAllowCredentials))
	assert.Equal(t, "Origin", rr.Header().Get(constants.HeaderVary), "Vary: Origin must always be set when middleware is active")
}

func TestCORSMiddleware_NoOriginHeaderDoesNotSetAllowOrigin(t *testing.T) {
	h := newCORSHTTPHandler([]string{"https://lovable.dev"})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := h.corsMiddleware(next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	assert.Empty(t, rr.Header().Get(constants.HeaderAccessControlAllowOrigin))
	assert.Equal(t, "Origin", rr.Header().Get(constants.HeaderVary))
}

func TestCORSMiddleware_PreflightAllowedOriginReturns204(t *testing.T) {
	h := newCORSHTTPHandler([]string{"https://lovable.dev"})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("next handler must not be called for preflight")
	})

	mw := h.corsMiddleware(next)
	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "https://lovable.dev")
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNoContent, rr.Code)
	assert.Equal(t, "https://lovable.dev", rr.Header().Get(constants.HeaderAccessControlAllowOrigin))
	assert.Equal(t, "true", rr.Header().Get(constants.HeaderAccessControlAllowCredentials))
	assert.NotEmpty(t, rr.Header().Get(constants.HeaderAccessControlAllowMethods))
	assert.NotEmpty(t, rr.Header().Get(constants.HeaderAccessControlAllowHeaders))
	assert.Equal(t, constants.HeaderValueCORSPreflightMaxAge, rr.Header().Get(constants.HeaderAccessControlMaxAge))
}

func TestCORSMiddleware_PreflightDisallowedOriginDoesNotShortCircuit(t *testing.T) {
	h := newCORSHTTPHandler([]string{"https://lovable.dev"})
	called := false
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	mw := h.corsMiddleware(next)
	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "https://evil.com")
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	assert.True(t, called, "next handler must be called for disallowed-origin OPTIONS")
	assert.Empty(t, rr.Header().Get(constants.HeaderAccessControlAllowOrigin))
	assert.Empty(t, rr.Header().Get(constants.HeaderAccessControlAllowMethods))
}

func TestCORSMiddleware_OriginMatchingIsCaseInsensitive(t *testing.T) {
	h := newCORSHTTPHandler([]string{"https://Lovable.Dev"})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := h.corsMiddleware(next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://lovable.dev")
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	assert.Equal(t, "https://lovable.dev", rr.Header().Get(constants.HeaderAccessControlAllowOrigin))
}

func TestCORSMiddleware_OriginMatchingTrimsTrailingSlash(t *testing.T) {
	h := newCORSHTTPHandler([]string{"https://lovable.dev/"})
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	mw := h.corsMiddleware(next)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Origin", "https://lovable.dev")
	rr := httptest.NewRecorder()
	mw.ServeHTTP(rr, req)

	assert.Equal(t, "https://lovable.dev", rr.Header().Get(constants.HeaderAccessControlAllowOrigin))
}
