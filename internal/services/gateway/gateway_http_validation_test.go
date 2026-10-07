// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package gateway

import (
	"net"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/config"
)

func TestIsPrivateIP(t *testing.T) {

	cases := []struct {
		ip       string
		expected bool
	}{
		// 10.0.0.0/8
		{"10.0.0.1", true},
		{"10.255.255.255", true},
		{"10.128.0.1", true},
		// 172.16.0.0/12
		{"172.16.0.1", true},
		{"172.31.255.255", true},
		{"172.20.0.1", true},
		{"172.17.0.1", true},
		{"172.30.255.255", true},
		// 192.168.0.0/16
		{"192.168.0.1", true},
		{"192.168.255.255", true},
		{"192.168.1.1", true},
		{"192.168.100.50", true},
		// Public IPs should be false
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"172.32.0.1", false},      // Outside 172.16.0.0/12
		{"172.15.255.255", false},  // Outside 172.16.0.0/12
		{"192.169.0.1", false},     // Outside 192.168.0.0/16
		{"11.0.0.1", false},        // Outside 10.0.0.0/8
		{"172.15.0.1", false},      // Just outside 172.16.0.0/12
		{"172.32.0.1", false},      // Just outside 172.16.0.0/12
		{"192.167.255.255", false}, // Just outside 192.168.0.0/16
		{"192.169.0.0", false},     // Just outside 192.168.0.0/16
		{"9.255.255.255", false},   // Just outside 10.0.0.0/8
		{"11.0.0.0", false},        // Just outside 10.0.0.0/8
		// IPv6 addresses (not handled by this function, should return false)
		{"::1", false},
		{"2001:db8::1", false},
		{"fe80::1", false},
	}

	for _, tc := range cases {
		t.Run(tc.ip, func(t *testing.T) {
			ip := net.ParseIP(tc.ip)
			require.NotNil(t, ip, "Failed to parse IP: %s", tc.ip)
			result := isPrivateIP(ip)
			assert.Equal(t, tc.expected, result, "IP %s should return %v", tc.ip, tc.expected)
		})
	}
}

func TestIsSafeHost(t *testing.T) {

	cfg := &config.Config{
		Endpoint: "g8e.local",
		Gateway: config.GatewayConfig{
			PublicBaseURL: "https://g8e-public.com:8443",
		},
	}

	cases := []struct {
		host     string
		expected bool
	}{
		// Local / Loopback
		{"localhost", true},
		{"127.0.0.1", true},
		{"::1", true},
		// RFC 1918 Private IPs
		{"192.168.1.1", true},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		// Configured Endpoint
		{"g8e.local", true},
		// Configured PublicBaseURL
		{"g8e-public.com", true},
		// Case insensitivity
		{"G8E.LOCAL", true},
		// Invalid / Malicious Characters
		{"evil.com;sh", false},
		{"evil.com/path", false},
		{"evil.com?param=value", false},
		{"evil.com", false},
		{"google.com", false},
		{"", false},
	}

	for _, tc := range cases {
		t.Run(tc.host, func(t *testing.T) {
			result := isSafeHost(tc.host, cfg)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestPathTraversalGuard(t *testing.T) {
	h := newMiddlewareTestHandler(t)

	tests := []struct {
		name       string
		path       string
		wantStatus int
	}{
		{"Valid path", "/db/users/u1", http.StatusOK},
		{"Traversal in path", "/db/users/../u1", http.StatusBadRequest},
		{"Encoded traversal in path", "/db/users/%2e%2e/u1", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := h.pathTraversalGuard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)
			assert.Equal(t, tt.wantStatus, rr.Code)
		})
	}
}

func TestBlobSegmentValid(t *testing.T) {
	assert.True(t, blobSegmentValid("valid-segment"))
	assert.False(t, blobSegmentValid(""))
	assert.False(t, blobSegmentValid(".."))
	assert.False(t, blobSegmentValid("path/traversal"))
	assert.False(t, blobSegmentValid("back\\slash"))
	assert.False(t, blobSegmentValid("null\x00byte"))
}

func TestIsMutationPubSubChannelAllowed(t *testing.T) {

	tests := []struct {
		name    string
		channel string
		allowed bool
	}{
		{"Heartbeat channel allowed", "heartbeat:operator-1", true},
		{"Results channel allowed", "results:cli-session-1", true},
		{"SSE channel allowed", "sse:sessions-1", true},
		{"WebSocket session channel allowed", "ws_session:conn-1", true},
		{"Internal channel allowed", "internal:system", true},
		{"Command channel not allowed", "cmd:execute", false},
		{"Governance channel not allowed", "governance:envelope", false},
		{"Empty channel not allowed", "", false},
		{"Random channel not allowed", "random:channel", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.allowed, isMutationPubSubChannelAllowed(tt.channel))
		})
	}
}

func TestHTTPHandler_handleLandingPage(t *testing.T) {
	controller := &HealthController{}

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	controller.handleLandingPage(rr, req)
	assert.Equal(t, http.StatusFound, rr.Code)
	assert.Equal(t, "/console/", rr.Header().Get("Location"))
}
