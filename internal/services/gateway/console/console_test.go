// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package console

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assetReference matches the asset URLs the Vite build writes into index.html.
// The console is built with base /console/, which the router strips.
var assetReference = regexp.MustCompile(`(?:src|href)="/console/(assets/[^"]+)"`)

func serve(t *testing.T, path string) *httptest.ResponseRecorder {
	t.Helper()
	handler, err := Handler()
	require.NoError(t, err)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
	return rr
}

func TestHandler_ServesStaticContent(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		wantStatus int
		wantBody   string
	}{
		{
			name:       "root path serves index HTML",
			path:       "/",
			wantStatus: http.StatusOK,
			wantBody:   "<title>g8e Console</title>",
		},
		{
			name:       "selection query still serves index HTML",
			path:       "/?case=c1&investigation=i1",
			wantStatus: http.StatusOK,
			wantBody:   "<title>g8e Console</title>",
		},
		{
			name:       "nonexistent file returns 404",
			path:       "/nonexistent.html",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rr := serve(t, tt.path)

			assert.Equal(t, tt.wantStatus, rr.Code)
			if tt.wantBody != "" {
				assert.Contains(t, rr.Header().Get("Content-Type"), "text/html")
				assert.Contains(t, rr.Body.String(), tt.wantBody)
			}
		})
	}
}

func TestHandler_SetsSecurityHeaders(t *testing.T) {
	rr := serve(t, "/")

	csp := rr.Header().Get("Content-Security-Policy")
	assert.Contains(t, csp, "script-src 'self'")
	assert.Contains(t, csp, "frame-ancestors 'none'")
	assert.NotContains(t, csp, "unsafe-inline")
	assert.Equal(t, "DENY", rr.Header().Get("X-Frame-Options"))
	assert.Equal(t, "nosniff", rr.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "no-referrer", rr.Header().Get("Referrer-Policy"))
	assert.Equal(t, "no-cache", rr.Header().Get("Cache-Control"))
}

// TestEmbeddedBuild_IndexReferencesEmbeddedAssets guards against a stale or
// partial embed: every asset index.html loads must be present and non-empty,
// and hashed assets are served as immutable.
func TestEmbeddedBuild_IndexReferencesEmbeddedAssets(t *testing.T) {
	static, err := fs.Sub(staticFS, "static")
	require.NoError(t, err)
	index, err := fs.ReadFile(static, "index.html")
	require.NoError(t, err)

	assert.NotRegexp(t, `<script>[^<]`, string(index), "index.html must not contain inline scripts (CSP script-src 'self')")
	references := assetReference.FindAllStringSubmatch(string(index), -1)
	require.NotEmpty(t, references, "index.html should load at least its script and stylesheet; run `make console-embed`")
	for _, match := range references {
		info, err := fs.Stat(static, match[1])
		require.NoError(t, err, "index.html references %s which is not embedded; run `make console-embed`", match[1])
		assert.NotZero(t, info.Size(), match[1])

		rr := serve(t, "/"+match[1])
		assert.Equal(t, http.StatusOK, rr.Code, match[1])
		assert.Equal(t, "public, max-age=31536000, immutable", rr.Header().Get("Cache-Control"), match[1])
	}
}
