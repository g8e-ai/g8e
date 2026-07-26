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
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
)

// --- handleSwaggerUI ---

func TestHandleSwaggerUI_ReturnsHTMLContentType(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/swagger/", nil)

	handleSwaggerUI(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "text/html; charset=utf-8", rr.Header().Get("Content-Type"))
}

func TestHandleSwaggerUI_BodyContainsSwaggerUIElements(t *testing.T) {
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/swagger/index.html", nil)

	handleSwaggerUI(rr, req)

	body := rr.Body.String()
	assert.Contains(t, body, "swagger-ui")
	assert.Contains(t, body, "SwaggerUIBundle")
	assert.Contains(t, body, "/swagger/doc.json")
}

func TestHandleSwaggerUI_IgnoresMethod(t *testing.T) {
	methods := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(method, "/swagger/", nil)

			handleSwaggerUI(rr, req)

			assert.Equal(t, http.StatusOK, rr.Code)
			assert.Equal(t, "text/html; charset=utf-8", rr.Header().Get("Content-Type"))
		})
	}
}

// --- registerMCPRoutes ---

func TestRegisterMCPRoutes_RegistersMCPAndA2APaths(t *testing.T) {
	mux := http.NewServeMux()
	handlerCalled := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled++
		w.WriteHeader(http.StatusOK)
	})

	registerMCPRoutes(mux, handler)

	paths := []string{constants.APIPaths.MCPEndpoint, constants.APIPaths.A2ACall}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			handlerCalled = 0
			req := httptest.NewRequest(http.MethodPost, path, nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			assert.Equal(t, http.StatusOK, rr.Code, "path %s should be registered and handled", path)
			assert.Equal(t, 1, handlerCalled, "handler should be called once for %s", path)
		})
	}
}

func TestRegisterMCPRoutes_UnregisteredPathsReturn404(t *testing.T) {
	mux := http.NewServeMux()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	registerMCPRoutes(mux, handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/nonexistent", nil)
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusNotFound, rr.Code, "unregistered path should return 404")
}

func TestRegisterMCPRoutes_BothPathsDeferToSameHandler(t *testing.T) {
	mux := http.NewServeMux()
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Test-Handler", "called")
		w.WriteHeader(http.StatusOK)
	})

	registerMCPRoutes(mux, handler)

	for _, path := range []string{constants.APIPaths.MCPEndpoint, constants.APIPaths.A2ACall} {
		t.Run(path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodPost, path, nil)
			rr := httptest.NewRecorder()
			mux.ServeHTTP(rr, req)

			require.Equal(t, http.StatusOK, rr.Code)
			assert.Equal(t, "called", rr.Header().Get("X-Test-Handler"), "handler should be the same for %s", path)
		})
	}
}
