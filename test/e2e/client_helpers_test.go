// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package e2e

import (
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseCAPool(t *testing.T) {
	tests := []struct {
		name          string
		caBundle      []byte
		expectError   bool
		errorContains string
	}{
		{
			name:          "empty bundle",
			caBundle:      []byte(""),
			expectError:   true,
			errorContains: "CA bundle is empty",
		},
		{
			name:          "invalid PEM (no valid certs)",
			caBundle:      []byte("not a certificate"),
			expectError:   true,
			errorContains: "CA bundle contains no valid PEM certificates",
		},
		{
			name:          "whitespace only",
			caBundle:      []byte("   \n\t  "),
			expectError:   true,
			errorContains: "CA bundle contains no valid PEM certificates",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pool, err := parseCAPool(tt.caBundle)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
				assert.Nil(t, pool)
			} else {
				require.NoError(t, err)
				assert.NotNil(t, pool)
				assert.IsType(t, &x509.CertPool{}, pool)
			}
		})
	}
}

func TestExtractServerName(t *testing.T) {
	tests := []struct {
		name          string
		inputURL      string
		expectedHost  string
		expectError   bool
		errorContains string
	}{
		{
			name:         "standard HTTPS URL",
			inputURL:     "https://gateway.example.com:8443",
			expectedHost: "gateway.example.com",
			expectError:  false,
		},
		{
			name:         "HTTPS URL with port",
			inputURL:     "https://192.168.1.100:8443",
			expectedHost: "192.168.1.100",
			expectError:  false,
		},
		{
			name:         "HTTPS URL without port",
			inputURL:     "https://localhost",
			expectedHost: "localhost",
			expectError:  false,
		},
		{
			name:          "HTTP scheme (not HTTPS)",
			inputURL:      "http://localhost:8443",
			expectError:   true,
			errorContains: "expected https scheme",
		},
		{
			name:          "FTP scheme",
			inputURL:      "ftp://localhost:8443",
			expectError:   true,
			errorContains: "expected https scheme",
		},
		{
			name:          "empty hostname",
			inputURL:      "https://:8443",
			expectError:   true,
			errorContains: "empty hostname",
		},
		{
			name:          "malformed URL",
			inputURL:      "not-a-url",
			expectError:   true,
			errorContains: "expected https scheme",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, err := extractServerName(tt.inputURL)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedHost, host)
			}
		})
	}
}

func TestTruncateBody(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "empty body",
			input:    []byte(""),
			expected: "",
		},
		{
			name:     "short body under limit",
			input:    []byte("hello world"),
			expected: "hello world",
		},
		{
			name:     "body exactly at limit (512)",
			input:    []byte(strings.Repeat("a", 512)),
			expected: strings.Repeat("a", 512),
		},
		{
			name:     "body over limit",
			input:    []byte(strings.Repeat("b", 600)),
			expected: strings.Repeat("b", 512) + "...(truncated)",
		},
		{
			name:     "body with special characters",
			input:    []byte("hello\nworld\t\r"),
			expected: "hello\nworld\t\r",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := truncateBody(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

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
			name:        "successful 200 response",
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

func TestIsEnsembleHealthy(t *testing.T) {
	tests := []struct {
		name     string
		body     []byte
		expected bool
	}{
		{
			name:     "healthy status ok",
			body:     []byte(`{"status": "ok"}`),
			expected: true,
		},
		{
			name:     "healthy status OK (uppercase)",
			body:     []byte(`{"status": "OK"}`),
			expected: true,
		},
		{
			name:     "healthy status Ok (mixed case)",
			body:     []byte(`{"status": "Ok"}`),
			expected: true,
		},
		{
			name:     "unhealthy status",
			body:     []byte(`{"status": "degraded"}`),
			expected: false,
		},
		{
			name:     "missing status field",
			body:     []byte(`{"other": "value"}`),
			expected: false,
		},
		{
			name:     "malformed JSON",
			body:     []byte(`{invalid json`),
			expected: false,
		},
		{
			name:     "empty body",
			body:     []byte(""),
			expected: false,
		},
		{
			name:     "null body",
			body:     nil,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isEnsembleHealthy(tt.body)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMaxResponseBytesConstant(t *testing.T) {
	// Verify the constant has the expected value (1 MiB)
	assert.Equal(t, 1<<20, maxResponseBytes)
}

func TestDefaultClientTimeoutConstant(t *testing.T) {
	// Verify the constant has the expected value (30 seconds)
	assert.Equal(t, 30*time.Second, defaultClientTimeout)
}

// TestDecodeJSON covers the single typed decode path used by all E2EClient
// response handlers. It runs as a Tier 1 unit test with no network or platform
// dependency.
func TestDecodeJSON(t *testing.T) {
	type widget struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}

	tests := []struct {
		name          string
		body          []byte
		label         string
		expected      widget
		expectError   bool
		errorContains string
	}{
		{
			name:  "valid JSON decodes into typed value",
			body:  []byte(`{"name":"alpha","count":7}`),
			label: "widget",
			expected: widget{
				Name:  "alpha",
				Count: 7,
			},
			expectError: false,
		},
		{
			name:          "malformed JSON returns wrapped error",
			body:          []byte(`{invalid json`),
			label:         "widget",
			expectError:   true,
			errorContains: "decode widget",
		},
		{
			name:          "empty body returns wrapped error",
			body:          []byte(``),
			label:         "widget",
			expectError:   true,
			errorContains: "decode widget",
		},
		{
			name:          "null body returns wrapped error",
			body:          nil,
			label:         "widget",
			expectError:   true,
			errorContains: "decode widget",
		},
		{
			name:  "type mismatch returns wrapped error",
			body:  []byte(`{"name":"alpha","count":"not-a-number"}`),
			label: "widget",
			expectError:   true,
			errorContains: "decode widget",
		},
		{
			name:  "extra fields ignored by tolerant decode",
			body:  []byte(`{"name":"alpha","count":7,"extra":"ignored"}`),
			label: "widget",
			expected: widget{
				Name:  "alpha",
				Count: 7,
			},
			expectError: false,
		},
		{
			name:  "label appears in error message",
			body:  []byte(`{`),
			label: "health response",
			expectError:   true,
			errorContains: "decode health response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := decodeJSON[widget](tt.body, tt.label)
			if tt.expectError {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errorContains)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}