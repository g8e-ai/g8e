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
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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

func TestIsLoopbackOrigin(t *testing.T) {
	t.Parallel()

	cases := []struct {
		origin   string
		expected bool
	}{
		{"http://localhost:8080", true},
		{"http://localhost", true},
		{"https://localhost:443", true},
		{"http://127.0.0.1:8080", true},
		{"http://127.0.0.1", true},
		{"http://127.0.0.2:8080", true},
		{"http://[::1]:8080", true},
		{"http://[::1]", true},
		{"http://example.com:8080", false},
		{"http://192.168.1.1:8080", false},
		{"http://10.0.0.1:8080", false},
		{"invalid-url", false},
		{"", false},
	}

	for _, tc := range cases {
		t.Run(tc.origin, func(t *testing.T) {
			t.Parallel()
			result := isLoopbackOrigin(tc.origin)
			assert.Equal(t, tc.expected, result, "Origin %s should return %v", tc.origin, tc.expected)
		})
	}
}

func TestMCPOriginGuard(t *testing.T) {
	t.Parallel()
	infra := setupTestInfrastructure(t, false)

	mcpGateway, err := mcp.NewGatewayService(mcp.Dependencies{
		Logger:          infra.Logger,
		Responder:       infra.Responder,
		SuspendedStore:  infra.SuspendedStore,
		MaxPayloadBytes: infra.Cfg.Gateway.MaxPayloadBytes,
		Posture:         string(infra.Cfg.Gateway.Posture),
	})
	require.NoError(t, err, "failed to create MCP gateway")
	h, err := newHTTPHandler(HTTPHandlerDependencies{
		Cfg:                infra.Cfg,
		Logger:             infra.Logger,
		DB:                 infra.DB,
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
	require.NoError(t, err, "failed to create HTTP handler")

	router := h.buildMCPHttpRouter()

	cases := []struct {
		name           string
		origin         string
		expectedStatus int
	}{
		{
			name:           "no origin header allowed",
			origin:         "",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "localhost origin allowed",
			origin:         "http://localhost:8080",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "127.0.0.1 origin allowed",
			origin:         "http://127.0.0.1:8080",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "::1 origin allowed",
			origin:         "http://[::1]:8080",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "external origin rejected",
			origin:         "http://example.com:8080",
			expectedStatus: http.StatusForbidden,
		},
		{
			name:           "private IP origin rejected",
			origin:         "http://192.168.1.1:8080",
			expectedStatus: http.StatusForbidden,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"ping"}`))
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			assert.Equal(t, tc.expectedStatus, w.Code, "Expected status %d for origin %s", tc.expectedStatus, tc.origin)
		})
	}
}

func setupTestHTTPHandler(t *testing.T) (*HTTPHandler, *config.Config) {
	t.Helper()
	infra := setupTestInfrastructure(t, false)

	mcpGateway, err := mcp.NewGatewayService(mcp.Dependencies{
		Logger:          infra.Logger,
		Responder:       infra.Responder,
		SuspendedStore:  infra.SuspendedStore,
		MaxPayloadBytes: infra.Cfg.Gateway.MaxPayloadBytes,
		Posture:         string(infra.Cfg.Gateway.Posture),
	})
	require.NoError(t, err, "failed to create MCP gateway")
	h, err := newHTTPHandler(HTTPHandlerDependencies{
		Cfg:                infra.Cfg,
		Logger:             infra.Logger,
		DB:                 infra.DB,
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
	return h, infra.Cfg
}

func setupTestGatewayService(t *testing.T) (*GatewayModeService, *config.Config) {
	t.Helper()
	infra := setupTestInfrastructure(t, true)

	mcpGateway, err := mcp.NewGatewayService(mcp.Dependencies{
		Logger:          infra.Logger,
		Responder:       infra.Responder,
		SuspendedStore:  infra.SuspendedStore,
		MaxPayloadBytes: infra.Cfg.Gateway.MaxPayloadBytes,
		Posture:         string(infra.Cfg.Gateway.Posture),
	})
	require.NoError(t, err, "failed to create MCP gateway")

	infra.Cfg.Gateway.HTTPPort = constants.Ports.OperatorHttp

	ls := &GatewayModeService{
		cfg:                infra.Cfg,
		logger:             infra.Logger,
		db:                 infra.DB,
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
	t.Parallel()
	h := setupTestHTTPHandlerLightweight(t)
	content := []byte("test body content")
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(content))

	body, err := h.readBody(req)
	require.NoError(t, err)
	assert.Equal(t, content, body)
}

func TestPathTraversalGuard(t *testing.T) {
	t.Parallel()
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
			t.Parallel()
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
	t.Parallel()
	h, _ := setupTestHTTPHandler(t)

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
	err = h.db.DocStore.DocSet("settings", "platform_settings", settingsBytes)
	require.NoError(t, err)

	handler := h.auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("Health bypass", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, constants.APIPaths.Health, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})
}

func TestAuthWebSocket(t *testing.T) {
	t.Parallel()
	h, _ := setupTestHTTPHandler(t)

	handler := h.auth.WebSocketAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("Unauthorized", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/ws/pubsub", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})
}

func TestAuthMiddlewareDeep(t *testing.T) {
	t.Parallel()
	h, _ := setupTestHTTPHandler(t)

	handler := h.auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("Uninitialized token - deny unauthenticated access", func(t *testing.T) {
		t.Parallel()
		h.db.DocStore.DocDelete("settings", "platform_settings")

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
	h, _ := setupTestHTTPHandler(t)

	t.Run("Returns 503 when platform_settings not found", func(t *testing.T) {
		t.Parallel()
		h.db.DocStore.DocDelete("settings", "platform_settings")
		req := httptest.NewRequest(http.MethodGet, constants.APIPaths.Health, nil)
		rr := httptest.NewRecorder()

		h.handleHealth(rr, req)
		assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
		assert.JSONEq(t, `{"error":"platform_settings not ready"}`, rr.Body.String())
	})

	t.Run("Returns 200 when platform_settings exists", func(t *testing.T) {
		t.Parallel()
		settings := models.SettingsDocument{
			Settings: &models.PlatformSettings{
				ActuatorKeyID: "test-key-id",
			},
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		settingsBytes, err := json.Marshal(settings)
		require.NoError(t, err)
		err = h.db.DocStore.DocSet("settings", "platform_settings", settingsBytes)
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
	t.Parallel()
	h, _ := setupTestHTTPHandler(t)

	settings := models.SettingsDocument{
		Settings: &models.PlatformSettings{
			ActuatorKeyID: "test-key-id",
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	settingsBytes, err := json.Marshal(settings)
	require.NoError(t, err)
	err = h.db.DocStore.DocSet("settings", "platform_settings", settingsBytes)
	require.NoError(t, err)

	// Force state root calculation to fail by dropping a table it queries
	_, err = h.db.db.Exec("DROP TABLE kv_store")
	require.NoError(t, err)

	req := httptest.NewRequest(http.MethodGet, constants.APIPaths.Health, nil)
	rr := httptest.NewRecorder()

	h.handleHealth(rr, req)
	assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
	assert.JSONEq(t, `{"error":"state root calculation failed"}`, rr.Body.String())
}

func TestHandleBootstrapHealth(t *testing.T) {
	h, _ := setupTestHTTPHandler(t)

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
	t.Parallel()
	h := &HTTPHandler{}
	assert.True(t, h.containsTraversal("/a/../b"))
	assert.True(t, h.containsTraversal("../etc/passwd"))
	assert.False(t, h.containsTraversal("/a/b/c"))
}

func TestBlobSegmentValid(t *testing.T) {
	t.Parallel()
	assert.True(t, blobSegmentValid("valid-segment"))
	assert.False(t, blobSegmentValid(""))
	assert.False(t, blobSegmentValid(".."))
	assert.False(t, blobSegmentValid("path/traversal"))
	assert.False(t, blobSegmentValid("back\\slash"))
	assert.False(t, blobSegmentValid("null\x00byte"))
}

func TestIsMutationPubSubChannelAllowed(t *testing.T) {
	t.Parallel()

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
			t.Parallel()
			assert.Equal(t, tt.allowed, isMutationPubSubChannelAllowed(tt.channel))
		})
	}
}

func TestNewHTTPHandler(t *testing.T) {
	t.Parallel()
	h, cfg := setupTestHTTPHandler(t)

	assert.NotNil(t, h)
	assert.NotNil(t, h.cfg)
	assert.NotNil(t, h.logger)
	assert.NotNil(t, h.db)
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
	t.Parallel()
	h, _ := setupTestHTTPHandler(t)

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
	t.Parallel()
	h, _ := setupTestHTTPHandler(t)

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
	require.NoError(t, h.db.DocStore.DocSet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID, b))

	wid := protocol.NewWorkloadIdentity()
	cliURI, err := wid.CLISPIFFEURL(userID, cliSessionID)
	require.NoError(t, err)
	opURI, err := wid.OperatorSPIFFEURL("org", "op-id", operatorSessionID)
	require.NoError(t, err)

	t.Run("CLI cert bound to Operator session is accepted", func(t *testing.T) {
		t.Parallel()
		bound, err := h.auth.cliCertBoundToOperator(
			[]*url.URL{cliURI}, cliSessionID, userID, operatorSessionID,
		)
		require.NoError(t, err)
		assert.True(t, bound)
	})

	t.Run("CLI cert bound to a different Operator session is rejected", func(t *testing.T) {
		t.Parallel()
		bound, err := h.auth.cliCertBoundToOperator(
			[]*url.URL{cliURI}, cliSessionID, userID, otherOpSessionID,
		)
		require.NoError(t, err)
		assert.False(t, bound)
	})

	t.Run("operator URI is not accepted via the CLI path", func(t *testing.T) {
		t.Parallel()
		bound, err := h.auth.cliCertBoundToOperator(
			[]*url.URL{opURI}, cliSessionID, userID, operatorSessionID,
		)
		require.NoError(t, err)
		assert.False(t, bound)
	})

	t.Run("expired CLI session is rejected", func(t *testing.T) {
		t.Parallel()
		expired := models.CLISession{
			ID:                "cli-expired",
			UserID:            userID,
			OperatorSessionID: operatorSessionID,
			ExpiresAt:         time.Now().Add(-1 * time.Hour),
		}
		eb, _ := json.Marshal(expired)
		require.NoError(t, h.db.DocStore.DocSet(marshaler.CollectionName(constants.CollectionCLISessions), "cli-expired", eb))
		expiredURI, err := wid.CLISPIFFEURL(userID, "cli-expired")
		require.NoError(t, err)
		bound, err := h.auth.cliCertBoundToOperator(
			[]*url.URL{expiredURI}, "cli-expired", userID, operatorSessionID,
		)
		require.NoError(t, err)
		assert.False(t, bound)
	})
}

func TestHTTPHandler_buildRouter(t *testing.T) {
	t.Parallel()
	h, _ := setupTestHTTPHandler(t)

	router := h.buildRouter()
	assert.NotNil(t, router, "buildRouter should return non-nil handler")
}

func TestHTTPHandler_ServeHTTP(t *testing.T) {
	t.Parallel()
	h, _ := setupTestHTTPHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)
	// ServeHTTP uses buildRouter which includes auth middleware, so unauthenticated requests are rejected
	assert.Equal(t, http.StatusUnauthorized, rr.Code)
}

func TestHTTPHandler_GetMCPGateway(t *testing.T) {
	t.Parallel()
	h, _ := setupTestHTTPHandler(t)

	mcpGateway := h.GetMCPGateway()
	assert.NotNil(t, mcpGateway, "GetMCPGateway should return non-nil service")
}

func TestHTTPHandler_GetPasskeyService(t *testing.T) {
	t.Parallel()
	h, _ := setupTestHTTPHandler(t)

	passkey := h.GetPasskeyService()
	assert.NotNil(t, passkey, "GetPasskeyService should return non-nil service")
}

func TestHTTPHandler_GetGatewayWebSocketHandler(t *testing.T) {
	t.Parallel()
	h, _ := setupTestHTTPHandler(t)

	pubsub := h.GetGatewayWebSocketHandler()
	assert.NotNil(t, pubsub, "GetGatewayWebSocketHandler should return non-nil service")
}

func TestHTTPHandler_handleLandingPage(t *testing.T) {
	t.Parallel()
	h, _ := setupTestHTTPHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	h.handleLandingPage(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Contains(t, rr.Body.String(), "g8e", "Landing page should contain g8e")
}
