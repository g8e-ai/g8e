// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package e2e

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoRequest(t *testing.T) {
	tests := []struct {
		name           string
		setupServer    func() *httptest.Server
		expectedStatus int
		expectError    bool
		errorContains  string
		checkBody      func(t *testing.T, body []byte)
		checkStatus    int // the actual status code we expect from the server
	}{
		{
			name: "successful 200 response",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(`{"status": "ok"}`))
				}))
			},
			expectedStatus: http.StatusOK,
			expectError:    false,
			checkBody: func(t *testing.T, body []byte) {
				assert.Contains(t, string(body), "status")
			},
			checkStatus: http.StatusOK,
		},
		{
			name: "non-200 response returns error",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusNotFound)
					w.Write([]byte(`{"error": "not found"}`))
				}))
			},
			expectedStatus: http.StatusOK,
			expectError:    true,
			errorContains:  "status 404, expected 200",
			checkBody:      nil,
			checkStatus:    http.StatusNotFound,
		},
		{
			name: "response exceeding maxResponseBytes",
			setupServer: func() *httptest.Server {
				largeBody := strings.Repeat("x", maxResponseBytes+100)
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusOK)
					w.Write([]byte(largeBody))
				}))
			},
			expectedStatus: http.StatusOK,
			expectError:    true,
			errorContains:  "response exceeds",
			checkBody:      nil,
			checkStatus:    http.StatusOK,
		},
		{
			name: "500 internal server error",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusInternalServerError)
					w.Write([]byte(`{"error": "internal error"}`))
				}))
			},
			expectedStatus: http.StatusOK,
			expectError:    true,
			errorContains:  "status 500, expected 200",
			checkBody:      nil,
			checkStatus:    http.StatusInternalServerError,
		},
		{
			name: "401 unauthorized",
			setupServer: func() *httptest.Server {
				return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.WriteHeader(http.StatusUnauthorized)
					w.Write([]byte(`{"error": "unauthorized"}`))
				}))
			},
			expectedStatus: http.StatusOK,
			expectError:    true,
			errorContains:  "status 401, expected 200",
			checkBody:      nil,
			checkStatus:    http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := tt.setupServer()
			defer srv.Close()

			client := &http.Client{Timeout: 5 * time.Second}
			req, err := http.NewRequest(http.MethodGet, srv.URL, nil)
			require.NoError(t, err)

			body, statusCode, err := doRequest(client, req, tt.expectedStatus)

			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
				assert.Equal(t, tt.checkStatus, statusCode)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.checkStatus, statusCode)
				if tt.checkBody != nil {
					tt.checkBody(t, body)
				}
			}
		})
	}
}

func TestDoRequest_NetworkError(t *testing.T) {
	// Test that network errors (e.g., connection refused) are properly wrapped
	client := &http.Client{Timeout: 100 * time.Millisecond}
	req, err := http.NewRequest(http.MethodGet, "http://127.0.0.1:9999", nil)
	require.NoError(t, err)

	_, _, err = doRequest(client, req, http.StatusOK)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "execute request")
}
