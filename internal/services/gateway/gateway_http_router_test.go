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

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
)

// --- isSafeHost ---

func TestIsSafeHost_NilConfig(t *testing.T) {
	cases := []struct {
		name     string
		host     string
		expected bool
	}{
		{"localhost with nil config", "localhost", true},
		{"loopback IPv4 with nil config", "127.0.0.1", true},
		{"loopback IPv6 with nil config", "::1", true},
		{"private IP 10.x with nil config", "10.0.0.1", true},
		{"private IP 172.16.x with nil config", "172.16.0.1", true},
		{"private IP 192.168.x with nil config", "192.168.1.1", true},
		{"public IP with nil config", "8.8.8.8", false},
		{"arbitrary hostname with nil config", "example.com", false},
		{"empty host with nil config", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, isSafeHost(tc.host, nil))
		})
	}
}

func TestIsSafeHost_EmptyConfig(t *testing.T) {
	cfg := &config.Config{}

	cases := []struct {
		name     string
		host     string
		expected bool
	}{
		{"localhost", "localhost", true},
		{"loopback IPv4", "127.0.0.1", true},
		{"private IP", "10.0.0.1", true},
		{"public IP", "8.8.8.8", false},
		{"hostname not in config", "my-host", false},
		{"empty", "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.expected, isSafeHost(tc.host, cfg))
		})
	}
}

func TestIsSafeHost_ConfigEndpoint(t *testing.T) {
	t.Run("endpoint without port", func(t *testing.T) {
		cfg := &config.Config{Endpoint: "g8e.local"}
		assert.True(t, isSafeHost("g8e.local", cfg))
		assert.True(t, isSafeHost("G8E.LOCAL", cfg))
		assert.False(t, isSafeHost("other.local", cfg))
	})

	t.Run("endpoint with port strips port before comparison", func(t *testing.T) {
		cfg := &config.Config{Endpoint: "g8e.local:8443"}
		assert.True(t, isSafeHost("g8e.local", cfg))
		assert.True(t, isSafeHost("G8E.LOCAL", cfg))
		assert.False(t, isSafeHost("g8e.local:8443", cfg), "host with port should not match since isSafeHost receives already-stripped hosts")
	})

	t.Run("endpoint with IPv6 address", func(t *testing.T) {
		cfg := &config.Config{Endpoint: "[::1]:8443"}
		assert.True(t, isSafeHost("::1", cfg))
	})
}

func TestIsSafeHost_PublicBaseURL(t *testing.T) {
	t.Run("PublicBaseURL with scheme and port", func(t *testing.T) {
		cfg := &config.Config{
			Gateway: config.GatewayConfig{
				PublicBaseURL: "https://g8e-public.com:8443",
			},
		}
		assert.True(t, isSafeHost("g8e-public.com", cfg))
		assert.True(t, isSafeHost("G8E-PUBLIC.COM", cfg))
		assert.False(t, isSafeHost("g8e-public.com:8443", cfg))
		assert.False(t, isSafeHost("other.com", cfg))
	})

	t.Run("PublicBaseURL without port", func(t *testing.T) {
		cfg := &config.Config{
			Gateway: config.GatewayConfig{
				PublicBaseURL: "https://g8e.example.org",
			},
		}
		assert.True(t, isSafeHost("g8e.example.org", cfg))
	})

	t.Run("PublicBaseURL with path ignores path", func(t *testing.T) {
		cfg := &config.Config{
			Gateway: config.GatewayConfig{
				PublicBaseURL: "https://g8e.example.org/gateway",
			},
		}
		assert.True(t, isSafeHost("g8e.example.org", cfg))
	})

	t.Run("malformed PublicBaseURL does not panic", func(t *testing.T) {
		cfg := &config.Config{
			Gateway: config.GatewayConfig{
				PublicBaseURL: "://not-a-url",
			},
		}
		assert.False(t, isSafeHost("g8e.example.org", cfg))
	})
}

func TestIsSafeHost_InvalidCharacters(t *testing.T) {
	cfg := &config.Config{Endpoint: "g8e.local"}

	invalidHosts := []string{
		"evil.com;sh",
		"evil.com/path",
		"evil.com?param=value",
		"host with space",
		"host_under_score",
		"host`backtick",
		"host$var",
		"host|pipe",
		"host\nnewline",
		"host\x00null",
	}

	for _, host := range invalidHosts {
		t.Run(host, func(t *testing.T) {
			assert.False(t, isSafeHost(host, cfg), "host %q should be rejected", host)
		})
	}
}

func TestIsSafeHost_ValidSpecialCharacters(t *testing.T) {
	// These special characters are allowed by the character validation:
	// . - [ ] :
	// They won't all produce a safe host, but they should pass the character check
	// (i.e., not be rejected purely for having invalid characters).
	cfg := &config.Config{Endpoint: "g8e.local"}

	// "." and "-" are common in hostnames — tested via "g8e.local" above.
	// ":" is allowed (IPv6 addresses use it).
	assert.True(t, isSafeHost("::1", cfg), "::1 should be safe (loopback IPv6)")
	// "[" and "]" are allowed but [::1] is not a valid IP for net.ParseIP.
	assert.False(t, isSafeHost("[::1]", cfg), "[::1] should not be safe (brackets not parsed by net.ParseIP)")
}

func TestIsSafeHost_IPv6Loopback(t *testing.T) {
	assert.True(t, isSafeHost("::1", nil), "::1 should be safe (loopback)")
	assert.False(t, isSafeHost("::2", nil), "::2 should not be safe (not loopback, not private)")
}

func TestIsSafeHost_PublicIPs(t *testing.T) {
	cfg := &config.Config{Endpoint: "g8e.local"}

	publicIPs := []string{
		"8.8.8.8",
		"1.1.1.1",
		"172.32.0.1",
		"172.15.255.255",
		"192.169.0.1",
		"11.0.0.1",
		"0.0.0.0",
		"169.254.1.1",
	}

	for _, ip := range publicIPs {
		t.Run(ip, func(t *testing.T) {
			assert.False(t, isSafeHost(ip, cfg), "public IP %s should not be safe", ip)
		})
	}
}

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
