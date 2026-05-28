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
	"strings"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/config"
	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/marshaler"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/mcp"
	"github.com/g8e-ai/g8e/protocol"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestHTTPHandler(t *testing.T) (*HTTPHandler, *config.Config) {
	t.Helper()
	infra := setupTestInfrastructure(t, false)

	mcpGateway := mcp.NewGatewayService(mcp.Dependencies{
		Logger:          infra.Logger,
		Responder:       infra.Responder,
		SuspendedStore:  infra.DB,
		MaxPayloadBytes: infra.Cfg.Gateway.MaxPayloadBytes,
	})
	h := newHTTPHandler(HTTPHandlerDependencies{
		Cfg:               infra.Cfg,
		Logger:            infra.Logger,
		DB:                infra.DB,
		Pubsub:            infra.Pubsub,
		Auth:              infra.Auth,
		PKI:               infra.PKI,
		SessionSvc:        infra.SessionSvc,
		Reg:               infra.Reg,
		Passkey:           infra.Passkey,
		UserSvc:           infra.UserSvc,
		Responder:         infra.Responder,
		MCPGateway:        mcpGateway,
		IsReady:           func() bool { return true },
		IsGovernanceReady: func() bool { return true },
	})
	return h, infra.Cfg
}

func setupTestGatewayService(t *testing.T) (*GatewayService, *config.Config) {
	t.Helper()
	infra := setupTestInfrastructure(t, true)

	mcpGateway := mcp.NewGatewayService(mcp.Dependencies{
		Logger:          infra.Logger,
		Responder:       infra.Responder,
		SuspendedStore:  infra.DB,
		MaxPayloadBytes: infra.Cfg.Gateway.MaxPayloadBytes,
	})

	infra.Cfg.Gateway.BootstrapPort = constants.Ports.OperatorBootstrapHttps

	ls := &GatewayService{
		cfg:        infra.Cfg,
		logger:     infra.Logger,
		db:         infra.DB,
		pubsub:     infra.Pubsub,
		auth:       infra.Auth,
		pki:        infra.PKI,
		reg:        infra.Reg,
		passkey:    infra.Passkey,
		userSvc:    infra.UserSvc,
		sessionSvc: infra.SessionSvc,
		mcpGateway: mcpGateway,
		responder:  infra.Responder,
	}

	require.NoError(t, ls.initHandlersAndServers())

	return ls, infra.Cfg
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
