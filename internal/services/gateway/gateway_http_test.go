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
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/responder"
	"github.com/g8e-ai/g8e/internal/services/keystore"
	"github.com/g8e-ai/g8e/internal/services/mcp"
	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/g8e-ai/g8e/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestHTTPHandler(t *testing.T) (*HTTPHandler, *config.Config) {
	t.Helper()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	dbDir := t.TempDir()
	pkiDir := t.TempDir()
	secretsDir := t.TempDir()
	db, err := OpenGatewayDBService(dbDir, secretsDir, logger, true)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	// Remove the secrets directory written by InitAppSettings
	os.RemoveAll(secretsDir)
	os.MkdirAll(secretsDir, 0755)

	pubsub := NewPubSubBroker(logger)
	t.Cleanup(func() { pubsub.Close() })

	// Initialize SecretManager with test backend for keystore operations
	backend, err := keystore.NewTestBackend()
	require.NoError(t, err)
	ks, err := keystore.NewWithBackend(t.TempDir(), logger, backend)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())
	require.NoError(t, ks.EnsurePermissions())
	sm := &SecretManager{
		db:         db.db,
		secretsDir: t.TempDir(),
		logger:     logger,
		keystore:   ks,
	}

	pki := newPKIAuthority(dbDir, pkiDir, db, sm, logger)
	err = pki.EnsurePKI(nil)
	require.NoError(t, err)

	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	resp := responder.New(logger)
	auth := NewAuthService(db, pki, logger, userSvc, personaSvc, resp, secretsDir, nil, "")
	sessionSvc := NewSessionService(db, logger)
	reg := NewRegistrationService(db, pki, logger, userSvc, sessionSvc, &cfg.Gateway)
	apiKeySvc := NewAPIKeyService(db, logger)
	passkey, _ := NewPasskeyService(db, logger, &PasskeyConfig{RpID: "localhost", RpName: "g8e"})
	mcpGateway := mcp.NewGatewayService(mcp.Dependencies{
		Logger:          logger,
		Responder:       resp,
		SuspendedStore:  db,
		MaxPayloadBytes: cfg.Gateway.MaxPayloadBytes,
	})
	h := newHTTPHandler(HTTPHandlerDependencies{
		Cfg:               cfg,
		Logger:            logger,
		DB:                db,
		Pubsub:            pubsub,
		Auth:              auth,
		PKI:               pki,
		SessionSvc:        sessionSvc,
		Reg:               reg,
		Passkey:           passkey,
		UserSvc:           userSvc,
		APIKey:            apiKeySvc,
		Responder:         resp,
		MCPGateway:        mcpGateway,
		IsReady:           func() bool { return true },
		IsGovernanceReady: func() bool { return true },
	})
	return h, cfg
}

func setupTestGatewayService(t *testing.T) (*GatewayService, *config.Config) {
	t.Helper()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	// Create a real DB service for the tests to use
	dbDir := t.TempDir()
	pkiDir := t.TempDir()
	secretsDir := t.TempDir()
	db, err := OpenGatewayDBService(dbDir, secretsDir, logger, true)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	// Remove the secrets directory written by InitAppSettings
	os.RemoveAll(secretsDir)
	os.MkdirAll(secretsDir, 0755)

	pubsub := NewPubSubBroker(logger)
	t.Cleanup(func() { pubsub.Close() })

	// Reset test storage before creating keystore (similar to keystore_test.go pattern)
	keystore.ResetTestStorage()

	// Initialize SecretManager with test backend for complete test environment
	// Use separate temp dir for keystore to avoid conflicts with secretsDir
	keystoreDir := t.TempDir()
	backend, err := keystore.NewTestBackend()
	require.NoError(t, err)
	ks, err := keystore.NewWithBackend(keystoreDir, logger, backend)
	require.NoError(t, err)
	require.NoError(t, ks.Initialize())
	require.NoError(t, ks.EnsurePermissions())
	sm := &SecretManager{
		db:         db.db,
		secretsDir: secretsDir,
		logger:     logger,
		keystore:   ks,
	}

	pki := newPKIAuthority(dbDir, pkiDir, db, sm, logger)
	err = pki.EnsurePKI(nil)
	require.NoError(t, err)

	// Build all services with the initialized PKI
	userSvc := NewUserService(db, logger)
	personaSvc := NewPersonaService(db, logger)
	resp := responder.New(logger)
	auth := NewAuthService(db, pki, logger, userSvc, personaSvc, resp, secretsDir, nil, "")
	sessionSvc := NewSessionService(db, logger)
	reg := NewRegistrationService(db, pki, logger, userSvc, sessionSvc, &cfg.Gateway)
	apiKeySvc := NewAPIKeyService(db, logger)
	passkey, _ := NewPasskeyService(db, logger, &PasskeyConfig{RpID: "localhost", RpName: "g8e"})
	mcpGateway := mcp.NewGatewayService(mcp.Dependencies{
		Logger:          logger,
		Responder:       resp,
		SuspendedStore:  db,
		MaxPayloadBytes: cfg.Gateway.MaxPayloadBytes,
	})

	cfg.Gateway.BootstrapPort = constants.Ports.OperatorBootstrapHttps

	// Build GatewayService directly with initialized components
	ls := &GatewayService{
		cfg:        cfg,
		logger:     logger,
		db:         db,
		pubsub:     pubsub,
		auth:       auth,
		pki:        pki,
		reg:        reg,
		passkey:    passkey,
		userSvc:    userSvc,
		sessionSvc: sessionSvc,
		apiKeySvc:  apiKeySvc,
		mcpGateway: mcpGateway,
		responder:  resp,
	}

	if err := ls.initHandlersAndServers(); err != nil {
		require.NoError(t, err)
	}

	return ls, cfg
}

func TestReadBody(t *testing.T) {
	t.Parallel()
	h, _ := setupTestHTTPHandler(t)
	content := []byte("test body content")
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(content))

	body, err := h.readBody(req)
	require.NoError(t, err)
	assert.Equal(t, content, body)
}

func TestPathTraversalGuard(t *testing.T) {
	t.Parallel()
	h, _ := setupTestHTTPHandler(t)

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
	err := h.db.DocSet("settings", "platform_settings", mustDocJSON(t, map[string]interface{}{
		"session_encryption_key": "test-key",
	}))
	require.NoError(t, err)

	handler := h.auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("Health bypass", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
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

	t.Run("Uninitialized token - Native Registration Path - allow without token", func(t *testing.T) {
		t.Parallel()
		h.db.DocDelete("settings", "platform_settings")

		req := httptest.NewRequest(http.MethodPost, "/api/auth/device-link/register", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		// We expect OK here because our mock handler just returns 200 if it passes middleware.
		// RegistrationService logic is not called because we are testing the middleware layer.
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("Uninitialized token - deny unauthenticated access", func(t *testing.T) {
		t.Parallel()
		h.db.DocDelete("settings", "platform_settings")

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

	t.Run("Blob endpoint with valid device-link token", func(t *testing.T) {
		t.Parallel()
		// Create a device-link token
		token := "dlk_test12345678901234567890"
		linkData := map[string]interface{}{
			"token":      token,
			"user_id":    "test-user",
			"status":     "active",
			"created_at": time.Now().UTC().Format(time.RFC3339),
			"expires_at": time.Now().Add(1 * time.Hour).UTC().Format(time.RFC3339),
		}
		linkBytes, err := json.Marshal(linkData)
		require.NoError(t, err)
		err = h.db.KVSet("g8e:device-link:"+token, string(linkBytes), 3600)
		require.NoError(t, err)

		// Put a blob in the store
		err = h.db.BlobPut("operator-binary", "linux-amd64", []byte("test-binary"), "application/octet-stream", 0)
		require.NoError(t, err)

		// Use the actual router with blob handler
		router := h.buildRouter()
		req := httptest.NewRequest(http.MethodGet, "/blob/operator-binary/linux-amd64", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		// Should succeed without mTLS since device-link token is valid
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Equal(t, []byte("test-binary"), rr.Body.Bytes())
	})

	t.Run("Blob endpoint with invalid device-link token", func(t *testing.T) {
		t.Parallel()
		// Use the actual router with blob handler
		router := h.buildRouter()
		req := httptest.NewRequest(http.MethodGet, "/blob/operator-binary/linux-amd64", nil)
		req.Header.Set("Authorization", "Bearer dlk_invalid")
		rr := httptest.NewRecorder()
		router.ServeHTTP(rr, req)

		// Should require mTLS since token is invalid
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.JSONEq(t, `{"error":"mTLS client certificate required"}`, rr.Body.String())
	})
}

func TestHandleHealth(t *testing.T) {
	t.Parallel()
	h, _ := setupTestHTTPHandler(t)

	t.Run("Returns 503 when platform_settings not found", func(t *testing.T) {
		t.Parallel()
		h.db.DocDelete("settings", "platform_settings")
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rr := httptest.NewRecorder()

		h.handleHealth(rr, req)
		assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
		assert.JSONEq(t, `{"error":"platform_settings not ready"}`, rr.Body.String())
	})

	t.Run("Returns 200 when platform_settings exists", func(t *testing.T) {
		t.Parallel()
		err := h.db.DocSet("settings", "platform_settings", mustDocJSON(t, map[string]interface{}{
			"session_encryption_key": "test-key",
		}))
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rr := httptest.NewRecorder()

		h.handleHealth(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp models.HealthResponse
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, constants.GatewayModeStatusOK, resp.Status)
	})
}

func TestHandleRotateAPIKeyDoesNotReturnSecret(t *testing.T) {
	t.Parallel()
	h, _ := setupTestHTTPHandler(t)
	operatorID := "op-1"
	userID := "user-1"
	oldKey := "g8e_old_key_123"

	op := &models.OperatorDocumentGo{
		ID:             operatorID,
		UserID:         userID,
		OperatorAPIKey: oldKey,
		Status:         constants.OperatorStatusOffline,
	}
	require.NoError(t, h.db.DocSet("operators", operatorID, mustDocJSON(t, op)))

	body := mustDocJSON(t, models.RotateAPIKeyRequest{OperatorID: operatorID})
	req := httptest.NewRequest(http.MethodPost, "/api/operators/rotate-api-key?user_id="+userID, bytes.NewReader(body))
	rr := httptest.NewRecorder()

	h.handleRotateAPIKey(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	assert.NotContains(t, rr.Body.String(), "api_key")
	assert.NotContains(t, rr.Body.String(), oldKey)

	var resp map[string]interface{}
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
	assert.Equal(t, true, resp["success"])
	assert.NotContains(t, resp, "api_key")

	doc, err := h.db.DocGet("operators", operatorID)
	require.NoError(t, err)
	newKey := docFieldString(t, doc, "operator_api_key")
	require.NotEmpty(t, newKey)
	assert.NotEqual(t, oldKey, newKey)
	assert.NotContains(t, rr.Body.String(), newKey)
}

func TestHandleDB(t *testing.T) {
	h, _ := setupTestHTTPHandler(t)

	t.Run("BadRequest - no collection", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/db/", nil)
		rr := httptest.NewRecorder()
		h.handleDB(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("BadRequest - no ID", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/db/users/", nil)
		rr := httptest.NewRecorder()
		h.handleDB(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("PUT and GET", func(t *testing.T) {
		data := map[string]string{"name": "alice"}
		reqPut := httptest.NewRequest(http.MethodPut, "/db/settings/u1", bytes.NewReader(mustDocJSON(t, data)))
		rrPut := httptest.NewRecorder()
		h.handleDB(rrPut, reqPut)
		assert.Equal(t, http.StatusOK, rrPut.Code)

		reqGet := httptest.NewRequest(http.MethodGet, "/db/settings/u1", nil)
		rrGet := httptest.NewRecorder()
		h.handleDB(rrGet, reqGet)
		assert.Equal(t, http.StatusOK, rrGet.Code)

		var doc map[string]interface{}
		err := json.Unmarshal(rrGet.Body.Bytes(), &doc)
		require.NoError(t, err)
		assert.Equal(t, "alice", doc["name"])
	})

	t.Run("PATCH", func(t *testing.T) {
		patch := map[string]string{"role": "admin"}
		reqPatch := httptest.NewRequest(http.MethodPatch, "/db/settings/u1", bytes.NewReader(mustDocJSON(t, patch)))
		rrPatch := httptest.NewRecorder()
		h.handleDB(rrPatch, reqPatch)
		assert.Equal(t, http.StatusOK, rrPatch.Code)

		reqGet := httptest.NewRequest(http.MethodGet, "/db/settings/u1", nil)
		rrGet := httptest.NewRecorder()
		h.handleDB(rrGet, reqGet)
		var doc map[string]interface{}
		json.Unmarshal(rrGet.Body.Bytes(), &doc)
		assert.Equal(t, "alice", doc["name"])
		assert.Equal(t, "admin", doc["role"])
	})

	t.Run("DELETE", func(t *testing.T) {
		reqDel := httptest.NewRequest(http.MethodDelete, "/db/settings/u1", nil)
		rrDel := httptest.NewRecorder()
		h.handleDB(rrDel, reqDel)
		assert.Equal(t, http.StatusOK, rrDel.Code)

		reqGet := httptest.NewRequest(http.MethodGet, "/db/settings/u1", nil)
		rrGet := httptest.NewRecorder()
		h.handleDB(rrGet, reqGet)
		assert.Equal(t, http.StatusNotFound, rrGet.Code)
	})

	t.Run("Query", func(t *testing.T) {
		h.db.DocSet("items", "i1", mustDocJSON(t, map[string]int{"val": 10}))
		h.db.DocSet("items", "i2", mustDocJSON(t, map[string]int{"val": 20}))

		query := models.DocQueryRequest{
			Limit: 1,
		}
		body, _ := json.Marshal(query)
		reqQuery := httptest.NewRequest(http.MethodPost, "/db/items/_query", bytes.NewReader(body))
		rrQuery := httptest.NewRecorder()
		h.handleDB(rrQuery, reqQuery)
		assert.Equal(t, http.StatusOK, rrQuery.Code)

		var results []map[string]interface{}
		err := json.Unmarshal(rrQuery.Body.Bytes(), &results)
		require.NoError(t, err)
		assert.Len(t, results, 1)
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPut, "/db/settings/u1", strings.NewReader("{invalid-json}"))
		rr := httptest.NewRecorder()
		h.handleDB(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.JSONEq(t, `{"error":"invalid JSON body"}`, rr.Body.String())
	})

	t.Run("PATCH not found", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPatch, "/db/settings/nonexistent", bytes.NewReader(mustDocJSON(t, map[string]string{"foo": "bar"})))
		rr := httptest.NewRecorder()
		h.handleDB(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("DELETE not found", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodDelete, "/db/users/nonexistent", nil)
		rr := httptest.NewRecorder()
		h.handleDB(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("Method Not Allowed", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/db/users/u1", nil)
		rr := httptest.NewRecorder()
		h.handleDB(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Non-bootstrap mutations redirect to governance envelope", func(t *testing.T) {
		t.Parallel()
		tests := []struct {
			method string
			body   []byte
		}{
			{method: http.MethodPut, body: mustDocJSON(t, map[string]string{"name": "alice"})},
			{method: http.MethodPatch, body: mustDocJSON(t, map[string]string{"role": "admin"})},
			{method: http.MethodDelete},
		}

		for _, tt := range tests {
			req := httptest.NewRequest(tt.method, "/db/items/i1", bytes.NewReader(tt.body))
			rr := httptest.NewRecorder()
			h.handleDB(rr, req)
			assert.Equal(t, http.StatusConflict, rr.Code, "method=%s", tt.method)
			assert.JSONEq(t, `{"error":"submit via POST /api/governance/envelope"}`, rr.Body.String())
		}
	})

	t.Run("Query validation", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/db/items/_query", strings.NewReader("{invalid}"))
		rr := httptest.NewRecorder()
		h.handleDBQuery(rr, req, "items")
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("SSE Events count", func(t *testing.T) {
		t.Parallel()
		h.db.SSEEventsAppend(SSERoute{WebSessionID: "s1"}, "T", "{}")
		req := httptest.NewRequest(http.MethodGet, "/db/_sse_events/count", nil)
		rr := httptest.NewRecorder()
		h.handleSSEEvents(rr, req, "count")
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("SSE Events wipe", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodDelete, "/db/_sse_events", nil)
		rr := httptest.NewRecorder()
		h.handleSSEEvents(rr, req, "")
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("SSE Events invalid", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/db/_sse_events/invalid", nil)
		rr := httptest.NewRecorder()
		h.handleSSEEvents(rr, req, "invalid")
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
}

// Regression: g8e-compatible agentic ensembles push typed events via /api/internal/sse/push and
// CLI/dashboard consumers poll /api/internal/sse/events with exactly one of
// web_session_id, cli_session_id, or user_id set. The Gateway persists each
// event under a typed routing column so CLI (BYO frontend) and web sessions
// occupy disjoint routing namespaces and never receive each other's events.
func TestInternalSSEBridge(t *testing.T) {
	h, _ := setupTestHTTPHandler(t)
	_, _ = h.db.SSEEventsWipe()

	// Seed platform settings required for SSE push
	err := h.db.DocSet("settings", "platform_settings", mustDocJSON(t, map[string]interface{}{
		"session_encryption_key": "test-key",
	}))
	require.NoError(t, err)

	push := func(body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/api/internal/sse/push", strings.NewReader(body))
		req.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{
				{
					URIs: []*url.URL{
						{Scheme: "spiffe", Host: "g8e.local", Path: "/app/example-ensemble"},
					},
				},
			},
		}
		rr := httptest.NewRecorder()
		h.handleInternalSSEPush(rr, req)
		return rr
	}

	t.Run("push requires mTLS certificate", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/internal/sse/push", strings.NewReader(`{"web_session_id":"ws-1","event":{"type":"ai.text","data":{}}}`))
		rr := httptest.NewRecorder()
		h.handleInternalSSEPush(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.JSONEq(t, `{"error":"mTLS client certificate required"}`, rr.Body.String())
	})

	t.Run("push rejects operator identity", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/internal/sse/push", strings.NewReader(`{"web_session_id":"ws-1","event":{"type":"ai.text","data":{}}}`))
		req.TLS = &tls.ConnectionState{
			PeerCertificates: []*x509.Certificate{
				{
					URIs: []*url.URL{
						{Scheme: "spiffe", Host: "g8e.local", Path: "/app/g8eo"},
					},
				},
			},
		}
		rr := httptest.NewRecorder()
		h.handleInternalSSEPush(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.JSONEq(t, `{"error":"unauthorized client identity"}`, rr.Body.String())
	})

	seedCLISession := func(cliSessionID, operatorSessionID string) {
		cliSess := models.CLISession{
			ID:                cliSessionID,
			UserID:            "u-1",
			OperatorSessionID: operatorSessionID,
			ExpiresAt:         time.Now().Add(24 * time.Hour),
		}
		b, _ := json.Marshal(cliSess)
		err := h.db.DocSet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID, b)
		require.NoError(t, err)
	}

	t.Run("web session event is persisted and replayable", func(t *testing.T) {
		body := `{"web_session_id":"ws-1","event":{"type":"ai.text","data":{"chunk":"hello"}}}`
		rr := push(body)
		assert.Equal(t, http.StatusOK, rr.Code)

		// Set up the operator->web binding for authorization
		h.db.KVSet(sessionOperatorBindKey("op-session-1"), "ws-1", 0)

		req := httptest.NewRequest(http.MethodGet, "/api/internal/sse/events?web_session_id=ws-1&since_id=0", nil)
		req.Header.Set(constants.HeaderAuthorization, "Bearer op-session-1")
		rr = httptest.NewRecorder()
		h.handleInternalSSEEvents(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), `"event_type":"ai.text"`)
		assert.Contains(t, rr.Body.String(), `\"chunk\":\"hello\"`)
	})

	t.Run("cli session event is persisted and replayable as a first-class type", func(t *testing.T) {
		body := `{"cli_session_id":"cli-1","event":{"type":"ai.text","data":{"chunk":"byo"}}}`
		rr := push(body)
		assert.Equal(t, http.StatusOK, rr.Code)

		// Set up the cli->operator binding for authorization
		seedCLISession("cli-1", "op-session-1")

		req := httptest.NewRequest(http.MethodGet, "/api/internal/sse/events?cli_session_id=cli-1&since_id=0", nil)
		req.Header.Set(constants.HeaderAuthorization, "Bearer op-session-1")
		rr = httptest.NewRecorder()
		h.handleInternalSSEEvents(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), `"event_type":"ai.text"`)
		assert.Contains(t, rr.Body.String(), `\"chunk\":\"byo\"`)
	})

	t.Run("cli and web with colliding ids do not cross namespaces", func(t *testing.T) {
		_, _ = h.db.SSEEventsWipe()
		rr := push(`{"web_session_id":"shared-id","event":{"type":"web.only","data":{}}}`)
		assert.Equal(t, http.StatusOK, rr.Code)
		rr = push(`{"cli_session_id":"shared-id","event":{"type":"cli.only","data":{}}}`)
		assert.Equal(t, http.StatusOK, rr.Code)

		// Set up the cli->operator binding for authorization
		seedCLISession("shared-id", "op-session-1")

		req := httptest.NewRequest(http.MethodGet, "/api/internal/sse/events?cli_session_id=shared-id&since_id=0", nil)
		req.Header.Set(constants.HeaderAuthorization, "Bearer op-session-1")
		rr = httptest.NewRecorder()
		h.handleInternalSSEEvents(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), `"event_type":"cli.only"`)
		assert.NotContains(t, rr.Body.String(), `"event_type":"web.only"`)
	})

	t.Run("web and cli session ids are mutually exclusive on push", func(t *testing.T) {
		rr := push(`{"web_session_id":"w","cli_session_id":"c","event":{"type":"x"}}`)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("background event routes by user_id", func(t *testing.T) {
		body := `{"user_id":"u-2","event":{"type":"system.notice","data":{}}}`
		rr := push(body)
		assert.Equal(t, http.StatusOK, rr.Code)

		// Create a mock operator session bound to user u-2
		opDoc := map[string]interface{}{
			"id":                  "op-u2",
			"user_id":             "u-2",
			"operator_session_id": "op-session-u2",
			"status":              constants.OperatorStatusActive,
		}
		opBytes, _ := json.Marshal(opDoc)
		h.db.DocSet(marshaler.CollectionName(constants.CollectionOperators), "op-u2", opBytes)

		req := httptest.NewRequest(http.MethodGet, "/api/internal/sse/events?user_id=u-2&since_id=0", nil)
		req.Header.Set(constants.HeaderAuthorization, "Bearer op-session-u2")
		rr = httptest.NewRecorder()
		h.handleInternalSSEEvents(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), `"event_type":"system.notice"`)
	})

	t.Run("missing routing key is rejected", func(t *testing.T) {
		rr := push(`{"event":{"type":"x"}}`)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("missing event field is rejected", func(t *testing.T) {
		rr := push(`{"web_session_id":"ws-3"}`)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("events GET requires exactly one routing key", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/internal/sse/events?since_id=0", nil)
		req.Header.Set(constants.HeaderAuthorization, "Bearer op-session-1")
		rr := httptest.NewRecorder()
		h.handleInternalSSEEvents(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("since_id replays only newer events", func(t *testing.T) {
		_, _ = h.db.SSEEventsWipe()
		_ = h.db.SSEEventsAppend(SSERoute{WebSessionID: "ws-x"}, "a", `{"event":{"type":"a"}}`)
		_ = h.db.SSEEventsAppend(SSERoute{WebSessionID: "ws-x"}, "b", `{"event":{"type":"b"}}`)

		// Set up the operator->web binding for authorization
		h.db.KVSet(sessionOperatorBindKey("op-session-1"), "ws-x", 0)

		req := httptest.NewRequest(http.MethodGet, "/api/internal/sse/events?web_session_id=ws-x&since_id=0&limit=1", nil)
		req.Header.Set(constants.HeaderAuthorization, "Bearer op-session-1")
		rr := httptest.NewRecorder()
		h.handleInternalSSEEvents(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), `"count":1`)
		assert.Contains(t, rr.Body.String(), `"event_type":"a"`)
	})

	t.Run("authorization: operator cannot access unbound cli_session_id", func(t *testing.T) {
		_ = h.db.SSEEventsAppend(SSERoute{CLISessionID: "cli-unbound"}, "test", `{"event":{"type":"x"}}`)
		req := httptest.NewRequest(http.MethodGet, "/api/internal/sse/events?cli_session_id=cli-unbound&since_id=0", nil)
		req.Header.Set(constants.HeaderAuthorization, "Bearer op-session-1")
		rr := httptest.NewRecorder()
		h.handleInternalSSEEvents(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.JSONEq(t, `{"error":"cli session not found"}`, rr.Body.String())
	})

	t.Run("authorization: operator cannot access cli_session_id owned by different operator", func(t *testing.T) {
		// Bind cli-owned to op-session-1
		seedCLISession("cli-owned", "op-session-1")
		_ = h.db.SSEEventsAppend(SSERoute{CLISessionID: "cli-owned"}, "test", `{"event":{"type":"x"}}`)

		// Try to access with op-session-2
		req := httptest.NewRequest(http.MethodGet, "/api/internal/sse/events?cli_session_id=cli-owned&since_id=0", nil)
		req.Header.Set(constants.HeaderAuthorization, "Bearer op-session-2")
		rr := httptest.NewRecorder()
		h.handleInternalSSEEvents(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.JSONEq(t, `{"error":"operator session does not own this cli session"}`, rr.Body.String())
	})

	t.Run("authorization: operator can access own cli_session_id", func(t *testing.T) {
		// Bind cli-mine to op-session-1
		seedCLISession("cli-mine", "op-session-1")
		_ = h.db.SSEEventsAppend(SSERoute{CLISessionID: "cli-mine"}, "x", `{"event":{"type":"x"}}`)

		req := httptest.NewRequest(http.MethodGet, "/api/internal/sse/events?cli_session_id=cli-mine&since_id=0", nil)
		req.Header.Set(constants.HeaderAuthorization, "Bearer op-session-1")
		rr := httptest.NewRecorder()
		h.handleInternalSSEEvents(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), `"event_type":"x"`)
	})

	t.Run("authorization: operator cannot access web_session_id not bound to them", func(t *testing.T) {
		_ = h.db.SSEEventsAppend(SSERoute{WebSessionID: "ws-other"}, "test", `{"event":{"type":"x"}}`)

		req := httptest.NewRequest(http.MethodGet, "/api/internal/sse/events?web_session_id=ws-other&since_id=0", nil)
		req.Header.Set(constants.HeaderAuthorization, "Bearer op-session-1")
		rr := httptest.NewRecorder()
		h.handleInternalSSEEvents(rr, req)
		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.JSONEq(t, `{"error":"operator session does not own this web session"}`, rr.Body.String())
	})

	t.Run("authorization: operator cannot access user_id they don't belong to", func(t *testing.T) {
		_ = h.db.SSEEventsAppend(SSERoute{UserID: "user-other"}, "test", `{"event":{"type":"x"}}`)

		req := httptest.NewRequest(http.MethodGet, "/api/internal/sse/events?user_id=user-other&since_id=0", nil)
		req.Header.Set(constants.HeaderAuthorization, "Bearer op-session-1")
		rr := httptest.NewRecorder()
		h.handleInternalSSEEvents(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.JSONEq(t, `{"error":"invalid operator session"}`, rr.Body.String())
	})

	t.Run("authorization: missing operator session id is rejected", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/internal/sse/events?cli_session_id=cli-1&since_id=0", nil)
		rr := httptest.NewRecorder()
		h.handleInternalSSEEvents(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.JSONEq(t, `{"error":"missing operator session id"}`, rr.Body.String())
	})

	t.Run("stream endpoint and Last-Event-ID", func(t *testing.T) {
		seedCLISession("cli-stream", "op-session-1")
		_ = h.db.SSEEventsAppend(SSERoute{CLISessionID: "cli-stream"}, "stream-init", `{"event":{"type":"stream-init"}}`)

		req := httptest.NewRequest(http.MethodGet, "/api/internal/sse/stream?cli_session_id=cli-stream", nil)
		req.Header.Set(constants.HeaderAuthorization, "Bearer op-session-1")
		req.Header.Set("Last-Event-ID", "1") // Replays since ID 1

		rr := httptest.NewRecorder()

		ctx, cancel := context.WithCancel(context.Background())
		req = req.WithContext(ctx)

		go func() {
			require.Eventually(t, func() bool {
				time.Sleep(100 * time.Millisecond)
				return true
			}, 200*time.Millisecond, 10*time.Millisecond)
			cancel()
		}()

		h.handleInternalSSEStream(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Header().Get("Content-Type"), "text/event-stream")
		assert.Contains(t, rr.Body.String(), "event: stream-init")
	})
}

func TestHandleKV(t *testing.T) {
	h, _ := setupTestHTTPHandler(t)

	t.Run("PUT and GET", func(t *testing.T) {
		reqPut := httptest.NewRequest(http.MethodPut, "/kv/k1", bytes.NewReader(mustDocJSON(t, models.KVSetRequest{Value: "g8e"})))
		rrPut := httptest.NewRecorder()
		h.handleKV(rrPut, reqPut)
		assert.Equal(t, http.StatusOK, rrPut.Code)

		reqGet := httptest.NewRequest(http.MethodGet, "/kv/k1", nil)
		rrGet := httptest.NewRecorder()
		h.handleKV(rrGet, reqGet)
		assert.Equal(t, http.StatusOK, rrGet.Code)
		assert.Contains(t, rrGet.Body.String(), `"value":"g8e"`)
	})

	t.Run("TTL and Expire", func(t *testing.T) {
		reqTtl := httptest.NewRequest(http.MethodGet, "/kv/k1/_ttl", nil)
		rrTtl := httptest.NewRecorder()
		h.handleKV(rrTtl, reqTtl)
		assert.Equal(t, http.StatusOK, rrTtl.Code)

		reqExp := httptest.NewRequest(http.MethodPut, "/kv/k1/_expire", bytes.NewReader(mustDocJSON(t, models.KVExpireRequest{TTL: 100})))
		rrExp := httptest.NewRecorder()
		h.handleKV(rrExp, reqExp)
		assert.Equal(t, http.StatusOK, rrExp.Code)
	})

	t.Run("Scan and DeletePattern", func(t *testing.T) {
		h.db.KVSet("pref:1", "a", 0)
		h.db.KVSet("pref:2", "b", 0)

		reqScan := httptest.NewRequest(http.MethodPost, "/kv/_scan", bytes.NewReader(mustDocJSON(t, models.KVPatternRequest{Pattern: "pref:*"})))
		rrScan := httptest.NewRecorder()
		h.handleKV(rrScan, reqScan)
		assert.Equal(t, http.StatusOK, rrScan.Code)
		assert.Contains(t, rrScan.Body.String(), "pref:1")

		reqDel := httptest.NewRequest(http.MethodPost, "/kv/_delete_pattern", bytes.NewReader(mustDocJSON(t, models.KVPatternRequest{Pattern: "pref:*"})))
		rrDel := httptest.NewRecorder()
		h.handleKV(rrDel, reqDel)
		assert.Equal(t, http.StatusOK, rrDel.Code)
		assert.Contains(t, rrDel.Body.String(), `"deleted":2`)
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPut, "/kv/k1", strings.NewReader("{invalid-json}"))
		rr := httptest.NewRecorder()
		h.handleKV(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("TTL required for expire", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPut, "/kv/k1/_expire", strings.NewReader(`{"ttl":0}`))
		rr := httptest.NewRecorder()
		h.handleKV(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("KV Keys", func(t *testing.T) {
		t.Parallel()
		h.db.KVSet("key1", "val1", 0)
		req := httptest.NewRequest(http.MethodPost, "/kv/_keys", strings.NewReader(`{"pattern":"key*"}`))
		rr := httptest.NewRecorder()
		h.handleKVKeys(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), "key1")
	})

	t.Run("KV Keys Invalid JSON", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/kv/_keys", strings.NewReader(`{invalid}`))
		rr := httptest.NewRecorder()
		h.handleKVKeys(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("KV Scan Invalid JSON", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/kv/_scan", strings.NewReader(`{invalid}`))
		rr := httptest.NewRecorder()
		h.handleKVScan(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("KV Delete Pattern Missing Pattern", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/kv/_delete_pattern", strings.NewReader(`{}`))
		rr := httptest.NewRecorder()
		h.handleKVDeletePattern(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("KV Delete Pattern Invalid JSON", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/kv/_delete_pattern", strings.NewReader(`{invalid}`))
		rr := httptest.NewRecorder()
		h.handleKVDeletePattern(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("KV Method Not Allowed", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/kv/key", nil)
		rr := httptest.NewRecorder()
		h.handleKV(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
}

func TestHandleBlob(t *testing.T) {
	t.Parallel()
	h, _ := setupTestHTTPHandler(t)

	t.Run("PUT and GET", func(t *testing.T) {
		t.Parallel()
		content := []byte("blob-data")
		reqPut := httptest.NewRequest(http.MethodPut, "/blob/ns1/b1", bytes.NewReader(content))
		reqPut.Header.Set("Content-Type", "text/plain")
		rrPut := httptest.NewRecorder()
		h.handleBlob(rrPut, reqPut)
		assert.Equal(t, http.StatusOK, rrPut.Code)

		reqGet := httptest.NewRequest(http.MethodGet, "/blob/ns1/b1", nil)
		rrGet := httptest.NewRecorder()
		h.handleBlob(rrGet, reqGet)
		assert.Equal(t, http.StatusOK, rrGet.Code)
		assert.Equal(t, content, rrGet.Body.Bytes())
		assert.Equal(t, "text/plain", rrGet.Header().Get("Content-Type"))
	})

	t.Run("Metadata", func(t *testing.T) {
		t.Parallel()
		// Create a blob first
		content := []byte("blob-data")
		reqPut := httptest.NewRequest(http.MethodPut, "/blob/ns2/b2", bytes.NewReader(content))
		reqPut.Header.Set("Content-Type", "text/plain")
		rrPut := httptest.NewRecorder()
		h.handleBlob(rrPut, reqPut)
		assert.Equal(t, http.StatusOK, rrPut.Code)

		// Then get metadata
		reqMeta := httptest.NewRequest(http.MethodGet, "/blob/ns2/b2/meta", nil)
		rrMeta := httptest.NewRecorder()
		h.handleBlob(rrMeta, reqMeta)
		assert.Equal(t, http.StatusOK, rrMeta.Code)
		assert.Contains(t, rrMeta.Body.String(), `"id":"b2"`)
	})

	t.Run("Too Large", func(t *testing.T) {
		t.Parallel()
		largeBody := make([]byte, maxBlobBodySize+1)
		req := httptest.NewRequest(http.MethodPut, "/blob/ns1/large", bytes.NewReader(largeBody))
		req.Header.Set("Content-Type", "application/octet-stream")
		rr := httptest.NewRecorder()
		h.handleBlob(rr, req)
		assert.Equal(t, http.StatusRequestEntityTooLarge, rr.Code)
	})

	t.Run("Missing Content-Type", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPut, "/blob/ns1/b1", strings.NewReader("data"))
		rr := httptest.NewRecorder()
		h.handleBlob(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.JSONEq(t, `{"error":"Content-Type header required"}`, rr.Body.String())
	})

	t.Run("Invalid namespace", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodDelete, "/blob/../invalid", nil)
		rr := httptest.NewRecorder()
		h.handleBlob(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Namespace delete not allowed method", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/blob/ns1", nil)
		rr := httptest.NewRecorder()
		h.handleBlob(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Blob id invalid", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/blob/ns1/..", nil)
		rr := httptest.NewRecorder()
		h.handleBlob(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Blob meta not found", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/blob/ns1/nonexistent/meta", nil)
		rr := httptest.NewRecorder()
		h.handleBlob(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("Blob get not found", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/blob/ns1/nonexistent", nil)
		rr := httptest.NewRecorder()
		h.handleBlob(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("Blob PUT empty body", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPut, "/blob/ns1/empty", strings.NewReader(""))
		req.Header.Set("Content-Type", "text/plain")
		rr := httptest.NewRecorder()
		h.handleBlob(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.JSONEq(t, `{"error":"body must not be empty"}`, rr.Body.String())
	})

	t.Run("Blob PUT read error", func(t *testing.T) {
		t.Parallel()
		// Create a request with a body that returns an error
		req := httptest.NewRequest(http.MethodPut, "/blob/ns1/error", &errorReader{})
		req.Header.Set("Content-Type", "text/plain")
		rr := httptest.NewRecorder()
		h.handleBlob(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("X-Blob-TTL valid", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPut, "/blob/ns1/ttl-test", strings.NewReader("data"))
		req.Header.Set("Content-Type", "text/plain")
		req.Header.Set("X-Blob-TTL", "3600")
		rr := httptest.NewRecorder()
		h.handleBlob(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("X-Blob-TTL invalid", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPut, "/blob/ns1/ttl-fail", strings.NewReader("data"))
		req.Header.Set("Content-Type", "text/plain")
		req.Header.Set("X-Blob-TTL", "-1")
		rr := httptest.NewRecorder()
		h.handleBlob(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
}

func TestWebSocketAuthIntegration(t *testing.T) {
	t.Parallel()
	h, _ := setupTestHTTPHandler(t)

	server := httptest.NewServer(h.buildRouter())
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/pubsub"

	t.Run("Missing token", func(t *testing.T) {
		ws, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
		assert.Error(t, err)
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		if ws != nil {
			ws.Close()
		}
		require.NotNil(t, resp, "response should not be nil even on dial error")
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
}

func TestHandlePubSubPublish(t *testing.T) {
	t.Parallel()
	h, _ := setupTestHTTPHandler(t)

	t.Run("Publish valid", func(t *testing.T) {
		t.Parallel()
		pubReq := models.PubSubPublishRequest{
			Channel: constants.ResultsChannel("op-1", "session-1"),
			Data:    mustDocJSON(t, map[string]string{"foo": "bar"}),
		}
		body, _ := json.Marshal(pubReq)
		req := httptest.NewRequest(http.MethodPost, "/pubsub/publish", bytes.NewReader(body))
		rr := httptest.NewRecorder()
		h.handlePubSubPublish(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), `"receivers":0`)
	})

	t.Run("Method Not Allowed", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodGet, "/pubsub/publish", nil)
		rr := httptest.NewRecorder()
		h.handlePubSubPublish(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		t.Parallel()
		req := httptest.NewRequest(http.MethodPost, "/pubsub/publish", strings.NewReader("{invalid}"))
		rr := httptest.NewRecorder()
		h.handlePubSubPublish(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Reject mutation channels", func(t *testing.T) {
		t.Parallel()
		for _, channel := range []string{constants.CmdChannel("op-1", "session-1"), "auditor:op-1:session-1"} {
			pubReq := models.PubSubPublishRequest{
				Channel: channel,
				Data:    mustDocJSON(t, map[string]string{"foo": "bar"}),
			}
			body, _ := json.Marshal(pubReq)
			req := httptest.NewRequest(http.MethodPost, "/pubsub/publish", bytes.NewReader(body))
			rr := httptest.NewRecorder()
			h.handlePubSubPublish(rr, req)
			assert.Equal(t, http.StatusConflict, rr.Code, "channel=%s", channel)
			assert.JSONEq(t, `{"error":"submit via POST /api/governance/envelope"}`, rr.Body.String())
		}
	})
}

func TestHandleBootstrap(t *testing.T) {
	t.Parallel()
	h, _ := setupTestHTTPHandler(t)

	t.Run("Success - Bootstrap with CSR over loopback", func(t *testing.T) {
		t.Parallel()
		csr := generateTestCSR(t)
		cliCsr := generateTestCSR(t)
		body := map[string]string{
			"name":               "Owner",
			"csr_pem":            csr,
			"cli_csr_pem":        cliCsr,
			"system_fingerprint": "test-fp",
		}
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/auth/bootstrap", bytes.NewReader(b))
		req.RemoteAddr = "127.0.0.1:12345"
		rr := httptest.NewRecorder()

		h.handleBootstrap(rr, req)

		assert.Equal(t, http.StatusCreated, rr.Code)
		var resp map[string]interface{}
		err := json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp["success"].(bool))
		assert.NotEmpty(t, resp["operator_cert"])
		assert.NotEmpty(t, resp["operator_cert_chain"])
		assert.NotEmpty(t, resp["hub_trust_bundle"])
		assert.NotEmpty(t, resp["operator_session_id"])
		assert.NotEmpty(t, resp["cli_session_id"])
		assert.NotEqual(t, resp["operator_session_id"], resp["cli_session_id"],
			"cli_session_id MUST be a distinct identifier from operator_session_id")
	})

	t.Run("Failure - Non-loopback CSR request rejected", func(t *testing.T) {
		t.Parallel()
		// Create a fresh handler to ensure no users exist
		h2, _ := setupTestHTTPHandler(t)
		csr := generateTestCSR(t)
		cliCsr := generateTestCSR(t)
		body := map[string]string{
			"name":               "Owner",
			"csr_pem":            csr,
			"cli_csr_pem":        cliCsr,
			"system_fingerprint": "test-fp",
		}
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/auth/bootstrap", bytes.NewReader(b))
		req.RemoteAddr = "192.168.1.1:12345"
		rr := httptest.NewRecorder()

		h2.handleBootstrap(rr, req)

		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.JSONEq(t, `{"error":"CSR auto-issue only available over loopback"}`, rr.Body.String())
	})

	t.Run("Success - Rotation for existing bootstrap user", func(t *testing.T) {
		t.Parallel()
		hRot, _ := setupTestHTTPHandler(t)
		// Create a bootstrap user first
		user, err := hRot.userSvc.CreateBootstrapUser()
		require.NoError(t, err)
		require.NotNil(t, user)

		csr := generateTestCSR(t)
		cliCsr := generateTestCSR(t)
		body := map[string]string{
			"name":               "Owner",
			"csr_pem":            csr,
			"cli_csr_pem":        cliCsr,
			"system_fingerprint": "rotated-fp",
		}
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/auth/bootstrap", bytes.NewReader(b))
		req.RemoteAddr = "127.0.0.1:12345"
		rr := httptest.NewRecorder()

		hRot.handleBootstrap(rr, req)

		assert.Equal(t, http.StatusCreated, rr.Code)
		var resp map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.True(t, resp["success"].(bool))
		assert.NotEmpty(t, resp["operator_cert"])
	})

	t.Run("Failure - Rotation fails for disabled bootstrap user", func(t *testing.T) {
		t.Parallel()
		h3, _ := setupTestHTTPHandler(t)
		// 1. Bootstrap
		user, _ := h3.userSvc.CreateBootstrapUser()
		// 2. Disable
		h3.userSvc.Disable(user.ID, "retired", "actor", "op")

		// 3. Attempt rotation
		csr := generateTestCSR(t)
		cliCsr := generateTestCSR(t)
		body := map[string]string{
			"name":               "Owner",
			"csr_pem":            csr,
			"cli_csr_pem":        cliCsr,
			"system_fingerprint": "fail-fp",
		}
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/auth/bootstrap", bytes.NewReader(b))
		req.RemoteAddr = "127.0.0.1:12345"
		rr := httptest.NewRecorder()

		h3.handleBootstrap(rr, req)

		assert.Equal(t, http.StatusConflict, rr.Code)
		assert.JSONEq(t, `{"error":"bootstrap user is disabled, cannot rotate"}`, rr.Body.String())
	})

	t.Run("Failure - Rejects bootstrap if ANY other users exist", func(t *testing.T) {
		t.Parallel()
		h4, _ := setupTestHTTPHandler(t)
		// Create a regular user first
		h4.userSvc.CreateUser()

		body := map[string]string{
			"name": "Superadmin",
		}
		b, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/api/auth/bootstrap", bytes.NewReader(b))
		rr := httptest.NewRecorder()

		h4.handleBootstrap(rr, req)

		assert.Equal(t, http.StatusForbidden, rr.Code)
		assert.JSONEq(t, `{"error":"bootstrap only available for initial setup"}`, rr.Body.String())
	})
}

func TestHandleBootstrapStatus(t *testing.T) {
	t.Run("Initially not bootstrapped", func(t *testing.T) {
		t.Parallel()
		h, _ := setupTestHTTPHandler(t)
		req := httptest.NewRequest(http.MethodGet, "/api/auth/bootstrap/status", nil)
		rr := httptest.NewRecorder()
		h.handleBootstrapStatus(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]interface{}
		err := json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, false, resp["bootstrapped"])
	})

	t.Run("Bootstrapped after creating a user", func(t *testing.T) {
		t.Parallel()
		h, _ := setupTestHTTPHandler(t)
		_, err := h.userSvc.CreateUser()
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/api/auth/bootstrap/status", nil)
		rr := httptest.NewRecorder()
		h.handleBootstrapStatus(rr, req)

		assert.Equal(t, http.StatusOK, rr.Code)
		var resp map[string]interface{}
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, true, resp["bootstrapped"])
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

type errorReader struct{}

func (e *errorReader) Read(p []byte) (n int, err error) {
	return 0, fmt.Errorf("forced read error")
}

// TestCLICertBoundToOperator is a regression test for the auth path used by
// `./g8e login` clients (CLI cert + Bearer <operator_session_id> +
// X-G8E-CLI-Session-ID). The cert URI SAN is a CLI SPIFFE ID and must be
// accepted on internal routes when the cli session is owned by the bound
// operator session.
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
	require.NoError(t, h.db.DocSet(marshaler.CollectionName(constants.CollectionCLISessions), cliSessionID, b))

	wid := protocol.NewWorkloadIdentity()
	cliURI, err := wid.CLISPIFFEURL(userID, cliSessionID)
	require.NoError(t, err)
	opURI, err := wid.OperatorSPIFFEURL("org", "op-id", operatorSessionID)
	require.NoError(t, err)

	t.Run("CLI cert bound to operator session is accepted", func(t *testing.T) {
		t.Parallel()
		assert.True(t, h.auth.cliCertBoundToOperator(
			[]*url.URL{cliURI}, cliSessionID, userID, operatorSessionID,
		))
	})

	t.Run("CLI cert bound to a different operator session is rejected", func(t *testing.T) {
		t.Parallel()
		assert.False(t, h.auth.cliCertBoundToOperator(
			[]*url.URL{cliURI}, cliSessionID, userID, otherOpSessionID,
		))
	})

	t.Run("operator URI is not accepted via the CLI path", func(t *testing.T) {
		t.Parallel()
		assert.False(t, h.auth.cliCertBoundToOperator(
			[]*url.URL{opURI}, cliSessionID, userID, operatorSessionID,
		))
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
		require.NoError(t, h.db.DocSet(marshaler.CollectionName(constants.CollectionCLISessions), "cli-expired", eb))
		expiredURI, err := wid.CLISPIFFEURL(userID, "cli-expired")
		require.NoError(t, err)
		assert.False(t, h.auth.cliCertBoundToOperator(
			[]*url.URL{expiredURI}, "cli-expired", userID, operatorSessionID,
		))
	})
}
