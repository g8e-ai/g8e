// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package console

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHandler_ServesStaticContent(t *testing.T) {
	handler := Handler()

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
			name:       "nonexistent file returns 404",
			path:       "/nonexistent.html",
			wantStatus: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			assert.Equal(t, tt.wantStatus, rr.Code)
			if tt.wantBody != "" {
				assert.Contains(t, rr.Header().Get("Content-Type"), "text/html")
				assert.Contains(t, rr.Body.String(), tt.wantBody)
			}
		})
	}
}
