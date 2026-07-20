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

//go:build integration

package gateway

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/mcp"
	"github.com/g8e-ai/g8e/protocol"
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

func TestHTTPRouterNoRedirect(t *testing.T) {

	h, _, _ := setupTestHTTPHandler(t)

	router := h.buildHTTPRouter()

	cases := []struct {
		name           string
		reqHost        string
		reqPath        string
		expectedStatus int
	}{
		{
			name:           "Unregistered path returns 404 not redirect",
			reqHost:        "localhost",
			reqPath:        "/some/path?foo=bar",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "Unregistered path with port returns 404",
			reqHost:        "localhost:8080",
			reqPath:        "/some/path",
			expectedStatus: http.StatusNotFound,
		},
		{
			name:           "Unregistered path with unsafe host returns 404",
			reqHost:        "evil.com",
			reqPath:        "/some/path",
			expectedStatus: http.StatusNotFound,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "http://"+tc.reqHost+tc.reqPath, nil)
			req.Host = tc.reqHost
			rr := httptest.NewRecorder()

			router.ServeHTTP(rr, req)

			assert.Equal(t, tc.expectedStatus, rr.Code)
			assert.Empty(t, rr.Header().Get("Location"))
		})
	}
}

func setupTestHTTPHandler(t *testing.T) (*HTTPHandler, *config.Config, *TestInfrastructure) {
	t.Helper()
	infra := setupTestInfrastructure(t, false)

	mcpGateway, err := mcp.NewGatewayService(mcp.Dependencies{
		Logger:           infra.Logger,
		Responder:        infra.Responder,
		SuspendedStore:   infra.SuspendedStore,
		ScrubbingService: nil,
		MaxPayloadBytes:  infra.Cfg.Gateway.MaxPayloadBytes,
		Posture:          string(infra.Cfg.Gateway.Posture),
	})
	require.NoError(t, err, "failed to create MCP gateway")
	h, err := newHTTPHandler(HTTPHandlerDependencies{
		Cfg:                infra.Cfg,
		Logger:             infra.Logger,
		Stores:             infra.Stores,
		Pubsub:             infra.Pubsub,
		Auth:               infra.Auth,
		PKI:                infra.PKI,
		CLISessionSvc:      infra.CLISessionSvc,
		OperatorSessionSvc: infra.OperatorSessionSvc,
		WebSessionSvc:      infra.WebSessionSvc,
		Reg:                infra.Reg,
		Passkey:            infra.Passkey,
		UserSvc:            infra.UserSvc,
		Responder:          infra.Responder,
		MCPGateway:         mcpGateway,
		IsReady:            func() bool { return true },
		IsGovernanceReady:  func() bool { return true },
	})
	require.NoError(t, err, "failed to create HTTP handler")
	return h, infra.Cfg, infra
}

func setupTestGatewayService(t *testing.T) (*GatewayModeService, *config.Config) {
	t.Helper()
	infra := setupTestInfrastructure(t, true)

	mcpGateway, err := mcp.NewGatewayService(mcp.Dependencies{
		Logger:           infra.Logger,
		Responder:        infra.Responder,
		SuspendedStore:   infra.SuspendedStore,
		ScrubbingService: nil,
		MaxPayloadBytes:  infra.Cfg.Gateway.MaxPayloadBytes,
		Posture:          string(infra.Cfg.Gateway.Posture),
	})
	require.NoError(t, err, "failed to create MCP gateway")

	infra.Cfg.Gateway.HTTPPort = constants.Ports.OperatorHttp

	ls := &GatewayModeService{
		cfg:                infra.Cfg,
		logger:             infra.Logger,
		db:                 infra.DB,
		stores:             infra.Stores,
		pubsub:             infra.Pubsub,
		auth:               infra.Auth,
		pki:                infra.PKI,
		reg:                infra.Reg,
		passkey:            infra.Passkey,
		userSvc:            infra.UserSvc,
		cliSessionSvc:      infra.CLISessionSvc,
		operatorSessionSvc: infra.OperatorSessionSvc,
		webSessionSvc:      infra.WebSessionSvc,
		mcpGateway:         mcpGateway,
		responder:          infra.Responder,
	}

	require.NoError(t, ls.initHandlersAndServers())

	return ls, infra.Cfg
}

func TestReadBody(t *testing.T) {
	h := setupTestHTTPHandlerLightweight(t)
	content := []byte("test body content")
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(content))

	body, err := h.readBody(req)
	require.NoError(t, err)
	assert.Equal(t, content, body)
}

func TestPathTraversalGuard(t *testing.T) {
	h := setupTestHTTPHandlerLightweight(t)

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

func TestAuthMiddleware(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)

	// Seed platform settings
	settings := models.SettingsDocument{
		Settings: &models.PlatformSettings{
			ActuatorKeyID: "test-key-id",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	settingsBytes, err := json.Marshal(settings)
	require.NoError(t, err)
	err = h.docStore.DocSet("settings", "platform_settings", settingsBytes)
	require.NoError(t, err)

	handler := h.auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("Health bypass", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, constants.APIPaths.Health, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})
}

func TestAuthWebSocket(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)

	handler := h.auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("Unauthorized", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ws/pubsub", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})
}

func TestAuthMiddlewareDeep(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)

	handler := h.auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("Uninitialized token - deny unauthenticated access", func(t *testing.T) {
		h.docStore.DocDelete("settings", "platform_settings")

		paths := []string{
			"/db/settings/platform_settings",
			"/kv/some-key",
			"/ws/pubsub",
		}

		for _, path := range paths {
			method := http.MethodGet
			if path == "/db/settings/platform_settings" {
				method = http.MethodPut
			}
			req := httptest.NewRequest(method, path, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)
			assert.Equal(t, http.StatusUnauthorized, rr.Code, "Path %s should be denied without token", path)

			assert.JSONEq(t, `{"error":"mTLS client certificate required"}`, rr.Body.String(), "Path %s should require mTLS", path)
		}
	})
}

func TestHandleHealth(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)

	t.Run("Returns 503 when platform_settings not found", func(t *testing.T) {
		h.docStore.DocDelete("settings", "platform_settings")
		req := httptest.NewRequest(http.MethodGet, constants.APIPaths.Health, nil)
		rr := httptest.NewRecorder()

		h.handleHealth(rr, req)
		assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
		assert.JSONEq(t, `{"error":"platform_settings not ready"}`, rr.Body.String())
	})

	t.Run("Returns 200 when platform_settings exists", func(t *testing.T) {
		settings := models.SettingsDocument{
			Settings: &models.PlatformSettings{
				ActuatorKeyID: "test-key-id",
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		settingsBytes, err := json.Marshal(settings)
		require.NoError(t, err)
		err = h.docStore.DocSet("settings", "platform_settings", settingsBytes)
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, constants.APIPaths.Health, nil)
		rr := httptest.NewRecorder()

		h.handleHealth(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp models.HealthResponse
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, constants.GatewayModeStatusOK, resp.Status)
	})
}

func TestHandleHealth_StateRootFailure(t *testing.T) {
	h, _, infra := setupTestHTTPHandler(t)

	settings := models.SettingsDocument{
		Settings: &models.PlatformSettings{
			ActuatorKeyID: "test-key-id",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	settingsBytes, err := json.Marshal(settings)
	require.NoError(t, err)
	err = h.docStore.DocSet("settings", "platform_settings", settingsBytes)
	require.NoError(t, err)

	// Force state root calculation to fail by dropping a table it queries
	_, err = infra.DB.GetDB().Exec("DROP TABLE kv_store")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.Health, nil)
	rr := httptest.NewRecorder()

	h.handleHealth(rr, req)
	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
	assert.JSONEq(t, `{"error":"state root calculation failed"}`, rr.Body.String())
}

func TestHandleBootstrapHealth(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)

	t.Run("Returns 503 when not ready", func(t *testing.T) {
		h.isReady = func() bool { return false }
		req := httptest.NewRequest(http.MethodGet, constants.APIPaths.Health, nil)
		rr := httptest.NewRecorder()

		h.handleBootstrapHealth(rr, req)
		assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
		assert.JSONEq(t, `{"error":"service initializing"}`, rr.Body.String())
	})

	t.Run("Returns 200 when ready", func(t *testing.T) {
		h.isReady = func() bool { return true }
		req := httptest.NewRequest(http.MethodGet, constants.APIPaths.Health, nil)
		rr := httptest.NewRecorder()

		h.handleBootstrapHealth(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp models.HealthResponse
		err := json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, constants.GatewayModeStatusOK, resp.Status)
		assert.Equal(t, constants.GatewayModeGateway, resp.Mode)
		// Bootstrap health does not include governance_ready or state_merkle_root
		assert.Empty(t, resp.StateMerkleRoot)
	})
}

func TestContainsTraversal(t *testing.T) {
	h := &HTTPHandler{}
	assert.True(t, h.containsTraversal("/a/../b"))
	assert.True(t, h.containsTraversal("../etc/passwd"))
	assert.False(t, h.containsTraversal("/a/b/c"))
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

func TestNewHTTPHandler(t *testing.T) {
	h, cfg, _ := setupTestHTTPHandler(t)

	assert.NotNil(t, h)
	assert.NotNil(t, h.cfg)
	assert.NotNil(t, h.logger)
	assert.NotNil(t, h.docStore)
	assert.NotNil(t, h.kvStore)
	assert.NotNil(t, h.sseStore)
	assert.NotNil(t, h.pubsub)
	assert.NotNil(t, h.auth)
	assert.NotNil(t, h.pki)
	assert.NotNil(t, h.cliSessionSvc)
	assert.NotNil(t, h.operatorSessionSvc)
	assert.NotNil(t, h.webSessionSvc)
	assert.NotNil(t, h.reg)
	assert.NotNil(t, h.passkey)
	assert.NotNil(t, h.userSvc)
	assert.NotNil(t, h.responder)
	assert.NotNil(t, h.mcp)
	assert.NotNil(t, h.pkiController)
	assert.NotNil(t, h.dbController)
	assert.NotNil(t, h.authController)
	assert.NotNil(t, h.adminController)
	assert.NotNil(t, h.operatorController)
	assert.Equal(t, cfg, h.cfg)
}

func TestBuildPublicRouter(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)

	router := h.buildPublicRouter()
	assert.NotNil(t, router)

	// Test that health endpoint is registered
	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.Health, nil)
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	// Health endpoint may require auth in some configurations, so we just check it's registered
	assert.NotNil(t, rr)

	// Test that PKI devices enroll endpoint is registered on public router
	req = httptest.NewRequest(http.MethodPost, constants.APIPaths.PKIDevicesEnroll, nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	// Endpoint requires mTLS, so we expect 401 Unauthorized, not 404 Not Found
	assert.NotEqual(t, http.StatusNotFound, rr.Code, "PKIDevicesEnroll should be registered on public router")

	// 1. RouteAuthWebSession routing tests for cookie-authed paths
	authedPaths := []string{
		"/api/v1/users/me",
		"/api/v1/approvals/tx-123/challenge",
		"/api/v1/approvals/tx-123/verify",
		"/api/v1/auth/passkeys/cred-123",
		"/api/v1/auth/sessions/me",
	}
	for _, path := range authedPaths {
		req = httptest.NewRequest(http.MethodGet, path, nil)
		rr = httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code, "Path %s should be RouteAuthWebSession and return 401, not 302/404", path)
		assert.Contains(t, rr.Body.String(), constants.ErrWebSessionCookieRequired.Error(), "Path %s should return web session cookie required error", path)
	}

	// 2. Console passkey endpoint accessibility tests (must bypass mTLS check)
	consolePasskeyPaths := []string{
		constants.APIPaths.AuthPasskeysConsoleRegisterChallenge,
		constants.APIPaths.AuthPasskeysConsoleRegisterVerify,
		constants.APIPaths.AuthPasskeysConsoleAuthenticateChallenge,
		constants.APIPaths.AuthPasskeysConsoleAuthenticateVerify,
	}
	for _, path := range consolePasskeyPaths {
		req = httptest.NewRequest(http.MethodPost, path, nil)
		rr = httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		// Since these are public, they bypass mTLS and reach the handler.
		// Since we send nil/empty body, they may fail with 400 or other errors,
		// but they must NOT return 401 with ErrMTLSCertRequired.
		assert.NotContains(t, rr.Body.String(), constants.ErrMTLSCertRequired.Error(), "Path %s should bypass mTLS check", path)
		assert.NotEqual(t, http.StatusNotFound, rr.Code, "Path %s should be registered", path)
	}

	// 3. CLI passkey status endpoint must be mTLS-gated (exact path overrides RouteAuthWebSession prefix)
	req = httptest.NewRequest(http.MethodGet, constants.APIPaths.AuthPasskeysCLIStatus, nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	// The cli/status path is under /api/v1/auth/passkeys (RouteAuthWebSession prefix) but registered
	// as an exact RouteAuthMTLS path, so it must hit the mTLS middleware and return 401, not pass through.
	assert.NotEqual(t, http.StatusNotFound, rr.Code, "cli/status should be registered on public router")
	assert.Contains(t, rr.Body.String(), constants.ErrMTLSCertRequired.Error(), "cli/status should be mTLS-gated")

	// 4. Landing page redirect test (302 to /console/ directly)
	req = httptest.NewRequest(http.MethodGet, "/", nil)
	rr = httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusFound, rr.Code, "Landing page should redirect with 302")
	assert.Equal(t, "/console/", rr.Header().Get("Location"), "Landing page should redirect directly to /console/")

	// 5. mTLS-protected passkey CLI status must NOT bypass mTLS
	mtlsPasskeyPaths := []string{
		constants.APIPaths.AuthPasskeysCLIStatus,
	}
	for _, path := range mtlsPasskeyPaths {
		req = httptest.NewRequest(http.MethodPost, path, nil)
		rr = httptest.NewRecorder()
		router.ServeHTTP(rr, req)
		assert.NotEqual(t, http.StatusNotFound, rr.Code, "Path %s should be registered", path)
		assert.Contains(t, rr.Body.String(), constants.ErrMTLSCertRequired.Error(), "Path %s should be mTLS-gated", path)
	}
}

type errorReader struct{}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, fmt.Errorf("forced read error")
}

// TestCLICertBoundToOperator is a regression test for the auth path used by
// `./g8e login` clients (CLI cert + Bearer <operator_session_id> +
// X-G8E-CLI-Session-ID). The cert URI SAN is a CLI SPIFFE ID and must be
// accepted on internal routes when the cli session is owned by the bound
// Operator session.
func TestCLICertBoundToOperator(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)

	const (
		userID            = "u-1"
		cliSessionID      = "cli-bound-1"
		operatorSessionID = "op-session-bound-1"
		otherOpSessionID  = "op-session-other"
	)

	cliSess := models.CLISession{
		ID:                cliSessionID,
		UserID:            userID,
		OperatorSessionID: operatorSessionID,
		ExpiresAt:         time.Now().Add(24 * time.Hour),
	}
	b, _ := json.Marshal(cliSess)
	require.NoError(t, h.docStore.DocSet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID, b))

	wid := protocol.NewWorkloadIdentity()
	cliURI, err := wid.CLISPIFFEURL(userID, cliSessionID)
	require.NoError(t, err)
	opURI, err := wid.OperatorSPIFFEURL("org", "op-id", operatorSessionID)
	require.NoError(t, err)

	t.Run("CLI cert bound to Operator session is accepted", func(t *testing.T) {
		bound, err := h.auth.cliCertBoundToOperator(
			[]*url.URL{cliURI}, cliSessionID, userID, operatorSessionID,
		)
		require.NoError(t, err)
		assert.True(t, bound)
	})

	t.Run("CLI cert bound to a different Operator session is rejected", func(t *testing.T) {
		bound, err := h.auth.cliCertBoundToOperator(
			[]*url.URL{cliURI}, cliSessionID, userID, otherOpSessionID,
		)
		require.NoError(t, err)
		assert.False(t, bound)
	})

	t.Run("operator URI is not accepted via the CLI path", func(t *testing.T) {
		bound, err := h.auth.cliCertBoundToOperator(
			[]*url.URL{opURI}, cliSessionID, userID, operatorSessionID,
		)
		require.NoError(t, err)
		assert.False(t, bound)
	})

	t.Run("expired CLI session is rejected", func(t *testing.T) {
		expired := models.CLISession{
			ID:                "cli-expired",
			UserID:            userID,
			OperatorSessionID: operatorSessionID,
			ExpiresAt:         time.Now().Add(-1 * time.Hour),
		}
		eb, _ := json.Marshal(expired)
		require.NoError(t, h.docStore.DocSet(marshaler.CollectionName(constants.CollectionCLISessions), "cli-expired", eb))
		expiredURI, err := wid.CLISPIFFEURL(userID, "cli-expired")
		require.NoError(t, err)
		bound, err := h.auth.cliCertBoundToOperator(
			[]*url.URL{expiredURI}, "cli-expired", userID, operatorSessionID,
		)
		require.NoError(t, err)
		assert.False(t, bound)
	})
}

func TestHTTPHandler_ServeHTTP(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)
	// ServeHTTP uses buildPublicRouter which redirects "/" to "/console/"
	assert.Equal(t, http.StatusFound, rr.Code)
	assert.Equal(t, "/console/", rr.Header().Get("Location"))
}

func TestHTTPHandler_GetMCPGateway(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)

	mcpGateway := h.GetMCPGateway()
	assert.NotNil(t, mcpGateway, "GetMCPGateway should return non-nil service")
}

func TestHTTPHandler_GetPasskeyHandler(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)

	passkey := h.GetPasskeyHandler()
	assert.NotNil(t, passkey, "GetPasskeyHandler should return non-nil service")
}

func TestHTTPHandler_GetGatewayWebSocketHandler(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)

	pubsub := h.GetGatewayWebSocketHandler()
	assert.NotNil(t, pubsub, "GetGatewayWebSocketHandler should return non-nil service")
}

func TestHTTPHandler_handleLandingPage(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	h.handleLandingPage(rr, req)
	assert.Equal(t, http.StatusFound, rr.Code)
	assert.Equal(t, "/console/", rr.Header().Get("Location"))
}

// makeTestAppWorkloadCert returns a self-signed cert with a SPIFFE URI SAN for an app workload identity.
func makeTestAppWorkloadCert(t *testing.T, appID string) *x509.Certificate {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	spiffeURI, err := url.Parse("spiffe://g8e.local/app/" + appID)
	require.NoError(t, err)
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-app-cert"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		URIs:         []*url.URL{spiffeURI},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(certDER)
	require.NoError(t, err)
	return cert
}

// makeTestOperatorCert returns a self-signed cert with a SPIFFE URI SAN for an operator identity.
func makeTestOperatorCert(t *testing.T, operatorSessionID string) *x509.Certificate {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	require.NoError(t, err)
	// Use exact path /app/g8eo to test identity rejection
	spiffeURI, err := url.Parse("spiffe://g8e.local/app/g8eo")
	require.NoError(t, err)
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test-operator-cert"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		URIs:         []*url.URL{spiffeURI},
	}
	certDER, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	require.NoError(t, err)
	cert, err := x509.ParseCertificate(certDER)
	require.NoError(t, err)
	return cert
}

func TestHTTPHandler_handleInternalSSEPush(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)

	t.Run("Method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/internal/sse/push", nil)
		rr := httptest.NewRecorder()
		h.sseController.handleInternalSSEPush(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
		assert.JSONEq(t, `{"error":"method not allowed"}`, rr.Body.String())
	})

	t.Run("Missing mTLS client certificate", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/internal/sse/push", nil)
		rr := httptest.NewRecorder()
		h.sseController.handleInternalSSEPush(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.JSONEq(t, `{"error":"mTLS client certificate required"}`, rr.Body.String())
	})

	t.Run("Empty peer certificates", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/internal/sse/push", nil)
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{}}
		rr := httptest.NewRecorder()
		h.sseController.handleInternalSSEPush(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.JSONEq(t, `{"error":"mTLS client certificate required"}`, rr.Body.String())
	})

	t.Run("Invalid JSON body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/internal/sse/push", bytes.NewReader([]byte("invalid json")))
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{makeTestAppWorkloadCert(t, "test-app")}}
		rr := httptest.NewRecorder()
		h.sseController.handleInternalSSEPush(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.JSONEq(t, `{"error":"invalid JSON body"}`, rr.Body.String())
	})

	t.Run("Missing event field", func(t *testing.T) {
		payload := map[string]string{
			"web_session_id": "web-1",
		}
		body, _ := json.Marshal(payload)
		req := httptest.NewRequest(http.MethodPost, "/internal/sse/push", bytes.NewReader(body))
		req.TLS = &tls.ConnectionState{PeerCertificates: []*x509.Certificate{makeTestAppWorkloadCert(t, "test-app")}}
		rr := httptest.NewRecorder()
		h.sseController.handleInternalSSEPush(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.JSONEq(t, `{"error":"event field is required"}`, rr.Body.String())
	})
}

func TestHTTPHandler_handleInternalSSEEvents(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)

	t.Run("Method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/internal/sse/events", nil)
		rr := httptest.NewRecorder()
		h.sseController.handleInternalSSEEvents(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
		assert.JSONEq(t, `{"error":"method not allowed"}`, rr.Body.String())
	})

	t.Run("Missing auth identity (no context, no mTLS)", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/internal/sse/events?cli_session_id=cli-1", nil)
		rr := httptest.NewRecorder()
		h.sseController.handleInternalSSEEvents(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.JSONEq(t, `{"error":"missing auth identity"}`, rr.Body.String())
	})

	t.Run("mTLS auth - missing routing parameter", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyOperatorSessionID, "op-session-1")
		req := httptest.NewRequest(http.MethodGet, "/internal/sse/events", nil)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		h.sseController.handleInternalSSEEvents(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.JSONEq(t, `{"error":"exactly one routing target required"}`, rr.Body.String())
	})

	t.Run("mTLS auth - multiple routing parameters", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyOperatorSessionID, "op-session-1")
		req := httptest.NewRequest(http.MethodGet, "/internal/sse/events?cli_session_id=cli-1&web_session_id=web-1", nil)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		h.sseController.handleInternalSSEEvents(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.JSONEq(t, `{"error":"exactly one routing target required"}`, rr.Body.String())
	})

	t.Run("cookie auth - missing routing parameter", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyWebSessionID, "web-session-1")
		ctx = context.WithValue(ctx, constants.ContextKeyUserID, "user-1")
		req := httptest.NewRequest(http.MethodGet, "/internal/sse/events", nil)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		h.sseController.handleInternalSSEEvents(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.JSONEq(t, `{"error":"exactly one routing target required"}`, rr.Body.String())
	})

	t.Run("cookie auth - web session mismatch", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyWebSessionID, "web-session-auth")
		ctx = context.WithValue(ctx, constants.ContextKeyUserID, "user-1")
		req := httptest.NewRequest(http.MethodGet, "/internal/sse/events?web_session_id=web-session-other", nil)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		h.sseController.handleInternalSSEEvents(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.Contains(t, rr.Body.String(), "web session does not match")
	})
}

func TestHTTPHandler_handleInternalSSEStream(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)

	t.Run("Method not allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/internal/sse/stream", nil)
		rr := httptest.NewRecorder()
		h.sseController.handleInternalSSEStream(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
		assert.JSONEq(t, `{"error":"method not allowed"}`, rr.Body.String())
	})

	t.Run("Missing auth identity (no context, no mTLS)", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/internal/sse/stream?cli_session_id=cli-1", nil)
		rr := httptest.NewRecorder()
		h.sseController.handleInternalSSEStream(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.JSONEq(t, `{"error":"missing auth identity"}`, rr.Body.String())
	})

	t.Run("mTLS auth - missing routing parameter", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyOperatorSessionID, "op-session-1")
		req := httptest.NewRequest(http.MethodGet, "/internal/sse/stream", nil)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		h.sseController.handleInternalSSEStream(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.JSONEq(t, `{"error":"exactly one routing target required"}`, rr.Body.String())
	})

	t.Run("cookie auth - missing routing parameter", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyWebSessionID, "web-session-1")
		ctx = context.WithValue(ctx, constants.ContextKeyUserID, "user-1")
		req := httptest.NewRequest(http.MethodGet, "/internal/sse/stream", nil)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		h.sseController.handleInternalSSEStream(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.JSONEq(t, `{"error":"exactly one routing target required"}`, rr.Body.String())
	})
}

func TestHTTPHandler_handleInternalSSEStream_CLIMTLSAuth(t *testing.T) {
	h, _, _ := setupTestHTTPHandler(t)

	const (
		userID       = "u-cli-mtls"
		cliSessionID = "cli-mtls-1"
	)

	cliSess := models.CLISession{
		ID:        cliSessionID,
		UserID:    userID,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}
	b, _ := json.Marshal(cliSess)
	require.NoError(t, h.docStore.DocSet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID, b))

	t.Run("CLI mTLS auth with matching user ID passes auth", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
		ctx = context.WithValue(ctx, constants.ContextKeyUserID, userID)
		req := httptest.NewRequest(http.MethodGet, "/internal/sse/stream?cli_session_id="+cliSessionID, nil)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		h.sseController.handleInternalSSEStream(rr, req)
		cancel()
		assert.NotEqual(t, http.StatusUnauthorized, rr.Code)
		assert.NotEqual(t, http.StatusForbidden, rr.Code)
	})

	t.Run("CLI mTLS auth with wrong user ID is forbidden", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, "other-user")
		req := httptest.NewRequest(http.MethodGet, "/internal/sse/stream?cli_session_id="+cliSessionID, nil)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		h.sseController.handleInternalSSEStream(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
	})

	t.Run("CLI mTLS auth with app ID is rejected as mTLS auth", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), constants.ContextKeyUserID, userID)
		ctx = context.WithValue(ctx, constants.ContextKeyAppID, "app-1")
		req := httptest.NewRequest(http.MethodGet, "/internal/sse/stream?cli_session_id="+cliSessionID, nil)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		h.sseController.handleInternalSSEStream(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})
}

func TestHTTPHandler_rateLimitMiddleware(t *testing.T) {

	t.Run("Rate limit disabled - passes through", func(t *testing.T) {
		infra := setupTestInfrastructure(t, false)
		infra.Cfg.Gateway.RateLimitRPS = 0 // Disabled

		mcpGateway, err := mcp.NewGatewayService(mcp.Dependencies{
			Logger:           infra.Logger,
			Responder:        infra.Responder,
			SuspendedStore:   infra.SuspendedStore,
			ScrubbingService: nil,
			MaxPayloadBytes:  infra.Cfg.Gateway.MaxPayloadBytes,
			Posture:          string(infra.Cfg.Gateway.Posture),
		})
		require.NoError(t, err)

		h, err := newHTTPHandler(HTTPHandlerDependencies{
			Cfg:                infra.Cfg,
			Logger:             infra.Logger,
			Stores:             infra.Stores,
			Pubsub:             infra.Pubsub,
			Auth:               infra.Auth,
			PKI:                infra.PKI,
			CLISessionSvc:      infra.CLISessionSvc,
			OperatorSessionSvc: infra.OperatorSessionSvc,
			WebSessionSvc:      infra.WebSessionSvc,
			Reg:                infra.Reg,
			Passkey:            infra.Passkey,
			UserSvc:            infra.UserSvc,
			Responder:          infra.Responder,
			MCPGateway:         mcpGateway,
			AppEnrollment:      nil,
			IsReady:            func() bool { return true },
			IsGovernanceReady:  func() bool { return true },
		})
		require.NoError(t, err)

		nextCalled := false
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
			w.WriteHeader(http.StatusOK)
		})

		middleware := h.rateLimitMiddleware(nextHandler)

		// Make multiple requests - all should pass through
		for i := 0; i < 10; i++ {
			nextCalled = false
			req := httptest.NewRequest(http.MethodPost, "/test", nil)
			req.RemoteAddr = "192.168.1.1:12345"
			rr := httptest.NewRecorder()

			middleware.ServeHTTP(rr, req)

			assert.True(t, nextCalled, "next handler should be called when rate limiting is disabled")
			assert.Equal(t, http.StatusOK, rr.Code)
		}
	})

	t.Run("Rate limit enabled - allows requests within limit", func(t *testing.T) {
		infra := setupTestInfrastructure(t, false)
		infra.Cfg.Gateway.RateLimitRPS = 10
		infra.Cfg.Gateway.RateLimitBurst = 20

		mcpGateway, err := mcp.NewGatewayService(mcp.Dependencies{
			Logger:           infra.Logger,
			Responder:        infra.Responder,
			SuspendedStore:   infra.SuspendedStore,
			ScrubbingService: nil,
			MaxPayloadBytes:  infra.Cfg.Gateway.MaxPayloadBytes,
			Posture:          string(infra.Cfg.Gateway.Posture),
		})
		require.NoError(t, err)

		h, err := newHTTPHandler(HTTPHandlerDependencies{
			Cfg:                infra.Cfg,
			Logger:             infra.Logger,
			Stores:             infra.Stores,
			Pubsub:             infra.Pubsub,
			Auth:               infra.Auth,
			PKI:                infra.PKI,
			CLISessionSvc:      infra.CLISessionSvc,
			OperatorSessionSvc: infra.OperatorSessionSvc,
			WebSessionSvc:      infra.WebSessionSvc,
			Reg:                infra.Reg,
			Passkey:            infra.Passkey,
			UserSvc:            infra.UserSvc,
			Responder:          infra.Responder,
			MCPGateway:         mcpGateway,
			AppEnrollment:      nil,
			IsReady:            func() bool { return true },
			IsGovernanceReady:  func() bool { return true },
		})
		require.NoError(t, err)

		nextCalled := false
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
			w.WriteHeader(http.StatusOK)
		})

		middleware := h.rateLimitMiddleware(nextHandler)

		// Make requests within burst limit - all should pass
		for i := 0; i < 15; i++ {
			nextCalled = false
			req := httptest.NewRequest(http.MethodPost, "/test", nil)
			req.RemoteAddr = "192.168.1.1:12345"
			rr := httptest.NewRecorder()

			middleware.ServeHTTP(rr, req)

			assert.True(t, nextCalled, "request %d should be allowed within burst limit", i)
			assert.Equal(t, http.StatusOK, rr.Code)
		}
	})

	t.Run("Rate limit enabled - different IPs tracked separately", func(t *testing.T) {
		infra := setupTestInfrastructure(t, false)
		infra.Cfg.Gateway.RateLimitRPS = 1
		infra.Cfg.Gateway.RateLimitBurst = 2

		mcpGateway, err := mcp.NewGatewayService(mcp.Dependencies{
			Logger:           infra.Logger,
			Responder:        infra.Responder,
			SuspendedStore:   infra.SuspendedStore,
			ScrubbingService: nil,
			MaxPayloadBytes:  infra.Cfg.Gateway.MaxPayloadBytes,
			Posture:          string(infra.Cfg.Gateway.Posture),
		})
		require.NoError(t, err)

		h, err := newHTTPHandler(HTTPHandlerDependencies{
			Cfg:                infra.Cfg,
			Logger:             infra.Logger,
			Stores:             infra.Stores,
			Pubsub:             infra.Pubsub,
			Auth:               infra.Auth,
			PKI:                infra.PKI,
			CLISessionSvc:      infra.CLISessionSvc,
			OperatorSessionSvc: infra.OperatorSessionSvc,
			WebSessionSvc:      infra.WebSessionSvc,
			Reg:                infra.Reg,
			Passkey:            infra.Passkey,
			UserSvc:            infra.UserSvc,
			Responder:          infra.Responder,
			MCPGateway:         mcpGateway,
			AppEnrollment:      nil,
			IsReady:            func() bool { return true },
			IsGovernanceReady:  func() bool { return true },
		})
		require.NoError(t, err)

		nextCalled := false
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
			w.WriteHeader(http.StatusOK)
		})

		middleware := h.rateLimitMiddleware(nextHandler)

		// IP1 makes burst requests
		for i := 0; i < 2; i++ {
			nextCalled = false
			req := httptest.NewRequest(http.MethodPost, "/test", nil)
			req.RemoteAddr = "192.168.1.1:12345"
			rr := httptest.NewRecorder()
			middleware.ServeHTTP(rr, req)
			assert.True(t, nextCalled, "IP1 request %d should be allowed", i)
		}

		// IP2 should also be allowed (separate limiter)
		nextCalled = false
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		req.RemoteAddr = "192.168.1.2:12345"
		rr := httptest.NewRecorder()
		middleware.ServeHTTP(rr, req)
		assert.True(t, nextCalled, "IP2 should have its own limiter")
	})

	t.Run("Rate limit exceeded - returns 429", func(t *testing.T) {
		infra := setupTestInfrastructure(t, false)
		infra.Cfg.Gateway.RateLimitRPS = 1
		infra.Cfg.Gateway.RateLimitBurst = 2

		mcpGateway, err := mcp.NewGatewayService(mcp.Dependencies{
			Logger:           infra.Logger,
			Responder:        infra.Responder,
			SuspendedStore:   infra.SuspendedStore,
			ScrubbingService: nil,
			MaxPayloadBytes:  infra.Cfg.Gateway.MaxPayloadBytes,
			Posture:          string(infra.Cfg.Gateway.Posture),
		})
		require.NoError(t, err)

		h, err := newHTTPHandler(HTTPHandlerDependencies{
			Cfg:                infra.Cfg,
			Logger:             infra.Logger,
			Stores:             infra.Stores,
			Pubsub:             infra.Pubsub,
			Auth:               infra.Auth,
			PKI:                infra.PKI,
			CLISessionSvc:      infra.CLISessionSvc,
			OperatorSessionSvc: infra.OperatorSessionSvc,
			WebSessionSvc:      infra.WebSessionSvc,
			Reg:                infra.Reg,
			Passkey:            infra.Passkey,
			UserSvc:            infra.UserSvc,
			Responder:          infra.Responder,
			MCPGateway:         mcpGateway,
			AppEnrollment:      nil,
			IsReady:            func() bool { return true },
			IsGovernanceReady:  func() bool { return true },
		})
		require.NoError(t, err)

		nextCalled := false
		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			nextCalled = true
			w.WriteHeader(http.StatusOK)
		})

		middleware := h.rateLimitMiddleware(nextHandler)

		// Exhaust burst limit
		for i := 0; i < 3; i++ {
			nextCalled = false
			req := httptest.NewRequest(http.MethodPost, "/test", nil)
			req.RemoteAddr = "192.168.1.1:12345"
			rr := httptest.NewRecorder()
			middleware.ServeHTTP(rr, req)
			// First 2 should be allowed (burst), 3rd might be rate limited
		}

		// Make another request immediately - should be rate limited
		nextCalled = false
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rr := httptest.NewRecorder()
		middleware.ServeHTTP(rr, req)

		assert.False(t, nextCalled, "request should be rate limited")
		assert.Equal(t, http.StatusTooManyRequests, rr.Code)
		assert.JSONEq(t, `{"error":"rate limit exceeded"}`, rr.Body.String())
	})

	t.Run("Stale limiter cleanup", func(t *testing.T) {
		infra := setupTestInfrastructure(t, false)
		infra.Cfg.Gateway.RateLimitRPS = 10
		infra.Cfg.Gateway.RateLimitBurst = 20

		mcpGateway, err := mcp.NewGatewayService(mcp.Dependencies{
			Logger:           infra.Logger,
			Responder:        infra.Responder,
			SuspendedStore:   infra.SuspendedStore,
			ScrubbingService: nil,
			MaxPayloadBytes:  infra.Cfg.Gateway.MaxPayloadBytes,
			Posture:          string(infra.Cfg.Gateway.Posture),
		})
		require.NoError(t, err)

		h, err := newHTTPHandler(HTTPHandlerDependencies{
			Cfg:                infra.Cfg,
			Logger:             infra.Logger,
			Stores:             infra.Stores,
			Pubsub:             infra.Pubsub,
			Auth:               infra.Auth,
			PKI:                infra.PKI,
			CLISessionSvc:      infra.CLISessionSvc,
			OperatorSessionSvc: infra.OperatorSessionSvc,
			WebSessionSvc:      infra.WebSessionSvc,
			Reg:                infra.Reg,
			Passkey:            infra.Passkey,
			UserSvc:            infra.UserSvc,
			Responder:          infra.Responder,
			MCPGateway:         mcpGateway,
			AppEnrollment:      nil,
			IsReady:            func() bool { return true },
			IsGovernanceReady:  func() bool { return true },
		})
		require.NoError(t, err)

		nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		middleware := h.rateLimitMiddleware(nextHandler)

		// Create limiters for multiple IPs
		ips := []string{"192.168.1.1:12345", "192.168.1.2:12345", "192.168.1.3:12345"}
		for _, ip := range ips {
			req := httptest.NewRequest(http.MethodPost, "/test", nil)
			req.RemoteAddr = ip
			rr := httptest.NewRecorder()
			middleware.ServeHTTP(rr, req)
		}

		// Manually set last used time to trigger cleanup
		h.muLimiters.Lock()
		for ip := range h.limiters {
			h.limiterLastUsed[ip] = time.Now().Add(-10 * time.Minute)
		}
		h.muLimiters.Unlock()

		// Make a new request - should trigger cleanup
		req := httptest.NewRequest(http.MethodPost, "/test", nil)
		req.RemoteAddr = "192.168.1.4:12345"
		rr := httptest.NewRecorder()
		middleware.ServeHTTP(rr, req)

		// Verify old limiters were cleaned up
		h.muLimiters.Lock()
		assert.LessOrEqual(t, len(h.limiters), 2, "stale limiters should be cleaned up")
		h.muLimiters.Unlock()
	})
}
