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

package listen

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/g8e-ai/g8e/components/g8eo/config"
	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
	"github.com/g8e-ai/g8e/components/g8eo/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestHTTPHandler(t *testing.T) (*HTTPHandler, *config.Config) {
	t.Helper()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	// Clear environment to avoid interfering with tests
	tokenEnv := string(constants.EnvVar.InternalAuthToken)
	origToken := os.Getenv(tokenEnv)
	if origToken != "" {
		os.Unsetenv(tokenEnv)
		t.Cleanup(func() {
			os.Setenv(tokenEnv, origToken)
		})
	}

	dbDir := t.TempDir()
	sslDir := t.TempDir()
	db, err := NewListenDBService(dbDir, sslDir, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	pubsub := NewPubSubBroker(logger)
	t.Cleanup(func() { pubsub.Close() })

	auth := NewAuthService(db, logger)
	h := newHTTPHandler(cfg, logger, db, pubsub, auth, func() bool { return true })
	return h, cfg
}

func setupTestListenService(t *testing.T) (*ListenService, *config.Config) {
	t.Helper()
	cfg := testutil.NewTestConfig(t)
	logger := testutil.NewTestLogger()

	// Clear environment to avoid interfering with tests
	origToken := os.Getenv(string(constants.EnvVar.InternalAuthToken))
	os.Unsetenv(string(constants.EnvVar.InternalAuthToken))
	t.Cleanup(func() {
		if origToken != "" {
			os.Setenv(string(constants.EnvVar.InternalAuthToken), origToken)
		}
	})

	// Create a real DB service for the tests to use
	dbDir := t.TempDir()
	sslDir := t.TempDir()
	db, err := NewListenDBService(dbDir, sslDir, logger)
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	pubsub := NewPubSubBroker(logger)
	t.Cleanup(func() { pubsub.Close() })

	ls := newListenServiceFromComponents(cfg, logger, db, pubsub)
	return ls, cfg
}

func TestReadBody(t *testing.T) {
	content := []byte("test body content")
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(content))

	body, err := readBody(req)
	require.NoError(t, err)
	assert.Equal(t, content, body)
}

func TestPathTraversalGuard(t *testing.T) {
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
			handler := pathTraversalGuard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	h, _ := setupTestHTTPHandler(t)
	token := "test-token"

	// Seed token in DB
	err := h.db.DocSet("settings", "platform_settings", mustDocJSON(t, map[string]interface{}{
		"internal_auth_token": token,
	}))
	require.NoError(t, err)

	handler := h.auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("Valid token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/db/users/u1", nil)
		req.Header.Set(constants.HeaderInternalAuth, token)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("Invalid token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/db/users/u1", nil)
		req.Header.Set(constants.HeaderInternalAuth, "wrong")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("Missing token", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/db/users/u1", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})

	t.Run("Health bypass", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("WebSocket token in query", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ws/pubsub?token="+token, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})
}

func TestAuthWebSocket(t *testing.T) {
	h, _ := setupTestHTTPHandler(t)
	token := "ws-token"

	err := h.db.DocSet("settings", "platform_settings", mustDocJSON(t, map[string]interface{}{
		"internal_auth_token": token,
	}))
	require.NoError(t, err)

	handler := h.auth.WebSocketAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("Token in header", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ws/pubsub", nil)
		req.Header.Set(constants.HeaderInternalAuth, token)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("Token in query", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ws/pubsub?token="+token, nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("Unauthorized", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/ws/pubsub", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
	})
}

func TestGetInternalAuthToken(t *testing.T) {
	h, _ := setupTestHTTPHandler(t)
	token := "env-token"

	// Ensure we start clean
	os.Unsetenv(string(constants.EnvVar.InternalAuthToken))
	h.db.DocDelete("settings", "platform_settings")

	t.Run("From environment", func(t *testing.T) {
		os.Setenv(string(constants.EnvVar.InternalAuthToken), token)
		defer os.Unsetenv(string(constants.EnvVar.InternalAuthToken))
		assert.Equal(t, token, h.auth.GetInternalAuthToken())
	})

	t.Run("From DB", func(t *testing.T) {
		dbToken := "db-token"
		// DocSet uses json.RawMessage, so we must provide valid JSON
		err := h.db.DocSet("settings", "platform_settings", mustDocJSON(t, map[string]interface{}{
			"internal_auth_token": dbToken,
		}))
		require.NoError(t, err)
		assert.Equal(t, dbToken, h.auth.GetInternalAuthToken())
	})

	t.Run("Environment takes precedence over DB", func(t *testing.T) {
		os.Setenv(string(constants.EnvVar.InternalAuthToken), "env-priority")
		defer os.Unsetenv(string(constants.EnvVar.InternalAuthToken))

		err := h.db.DocSet("settings", "platform_settings", mustDocJSON(t, map[string]interface{}{
			"internal_auth_token": "db-token",
		}))
		require.NoError(t, err)

		assert.Equal(t, "env-priority", h.auth.GetInternalAuthToken())
	})

	t.Run("Malformed DB settings returns empty", func(t *testing.T) {
		// Set a document that doesn't match the expected structure (internal_auth_token is not a string)
		err := h.db.DocSet("settings", "platform_settings", mustDocJSON(t, map[string]interface{}{
			"internal_auth_token": 123,
		}))
		require.NoError(t, err)
		assert.Equal(t, "", h.auth.GetInternalAuthToken())
	})

	t.Run("Missing DB settings returns empty", func(t *testing.T) {
		h.db.DocDelete("settings", "platform_settings")
		assert.Equal(t, "", h.auth.GetInternalAuthToken())
	})
}

func TestAuthMiddlewareDeep(t *testing.T) {
	h, _ := setupTestHTTPHandler(t)
	token := "secret-token"

	handler := h.auth.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	t.Run("Uninitialized token - allow platform_settings GET", func(t *testing.T) {
		os.Unsetenv(string(constants.EnvVar.InternalAuthToken))
		h.db.DocDelete("settings", "platform_settings")

		req := httptest.NewRequest(http.MethodGet, "/db/settings/platform_settings", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("Uninitialized token - allow platform_settings PUT", func(t *testing.T) {
		os.Unsetenv(string(constants.EnvVar.InternalAuthToken))
		h.db.DocDelete("settings", "platform_settings")

		req := httptest.NewRequest(http.MethodPut, "/db/settings/platform_settings", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("Uninitialized token - deny other paths", func(t *testing.T) {
		os.Unsetenv(string(constants.EnvVar.InternalAuthToken))
		h.db.DocDelete("settings", "platform_settings")

		req := httptest.NewRequest(http.MethodGet, "/db/users/u1", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.Contains(t, rr.Body.String(), "internal auth token not initialized")
	})

	t.Run("Initialized token - Valid", func(t *testing.T) {
		os.Setenv(string(constants.EnvVar.InternalAuthToken), token)
		defer os.Unsetenv(string(constants.EnvVar.InternalAuthToken))

		req := httptest.NewRequest(http.MethodGet, "/db/users/u1", nil)
		req.Header.Set(constants.HeaderInternalAuth, token)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("Initialized token - Invalid", func(t *testing.T) {
		os.Setenv(string(constants.EnvVar.InternalAuthToken), token)
		defer os.Unsetenv(string(constants.EnvVar.InternalAuthToken))

		req := httptest.NewRequest(http.MethodGet, "/db/users/u1", nil)
		req.Header.Set(constants.HeaderInternalAuth, "wrong-token")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		assert.Equal(t, http.StatusUnauthorized, rr.Code)
		assert.Contains(t, rr.Body.String(), "invalid internal auth token")
	})
}

func TestHandleHealth(t *testing.T) {
	h, _ := setupTestHTTPHandler(t)

	t.Run("Returns 503 when platform_settings not found", func(t *testing.T) {
		h.db.DocDelete("settings", "platform_settings")
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rr := httptest.NewRecorder()

		h.handleHealth(rr, req)
		assert.Equal(t, http.StatusServiceUnavailable, rr.Code)
		assert.Contains(t, rr.Body.String(), "platform_settings not ready")
	})

	t.Run("Returns 200 when platform_settings exists", func(t *testing.T) {
		err := h.db.DocSet("settings", "platform_settings", mustDocJSON(t, map[string]interface{}{
			"internal_auth_token": "test-token",
		}))
		require.NoError(t, err)

		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rr := httptest.NewRecorder()

		h.handleHealth(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)

		var resp models.HealthResponse
		err = json.Unmarshal(rr.Body.Bytes(), &resp)
		require.NoError(t, err)
		assert.Equal(t, constants.Status.ListenMode.StatusOK, resp.Status)
	})
}

func TestHandleDB(t *testing.T) {
	h, _ := setupTestHTTPHandler(t)

	t.Run("BadRequest - no collection", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/db/", nil)
		rr := httptest.NewRecorder()
		h.handleDB(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("BadRequest - no ID", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/db/users/", nil)
		rr := httptest.NewRecorder()
		h.handleDB(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("PUT and GET", func(t *testing.T) {
		data := map[string]string{"name": "alice"}
		reqPut := httptest.NewRequest(http.MethodPut, "/db/users/u1", bytes.NewReader(mustDocJSON(t, data)))
		rrPut := httptest.NewRecorder()
		h.handleDB(rrPut, reqPut)
		assert.Equal(t, http.StatusOK, rrPut.Code)

		reqGet := httptest.NewRequest(http.MethodGet, "/db/users/u1", nil)
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
		reqPatch := httptest.NewRequest(http.MethodPatch, "/db/users/u1", bytes.NewReader(mustDocJSON(t, patch)))
		rrPatch := httptest.NewRecorder()
		h.handleDB(rrPatch, reqPatch)
		assert.Equal(t, http.StatusOK, rrPatch.Code)

		reqGet := httptest.NewRequest(http.MethodGet, "/db/users/u1", nil)
		rrGet := httptest.NewRecorder()
		h.handleDB(rrGet, reqGet)
		var doc map[string]interface{}
		json.Unmarshal(rrGet.Body.Bytes(), &doc)
		assert.Equal(t, "alice", doc["name"])
		assert.Equal(t, "admin", doc["role"])
	})

	t.Run("DELETE", func(t *testing.T) {
		reqDel := httptest.NewRequest(http.MethodDelete, "/db/users/u1", nil)
		rrDel := httptest.NewRecorder()
		h.handleDB(rrDel, reqDel)
		assert.Equal(t, http.StatusOK, rrDel.Code)

		reqGet := httptest.NewRequest(http.MethodGet, "/db/users/u1", nil)
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
		req := httptest.NewRequest(http.MethodPut, "/db/users/u1", strings.NewReader("{invalid-json}"))
		rr := httptest.NewRecorder()
		h.handleDB(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "invalid JSON body")
	})

	t.Run("PATCH not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPatch, "/db/users/nonexistent", bytes.NewReader(mustDocJSON(t, map[string]string{"foo": "bar"})))
		rr := httptest.NewRecorder()
		h.handleDB(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("DELETE not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/db/users/nonexistent", nil)
		rr := httptest.NewRecorder()
		h.handleDB(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("Method Not Allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/db/users/u1", nil)
		rr := httptest.NewRecorder()
		h.handleDB(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Query validation", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/db/items/_query", strings.NewReader("{invalid}"))
		rr := httptest.NewRecorder()
		h.handleDBQuery(rr, req, "items")
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("SSE Events count", func(t *testing.T) {
		h.db.SSEEventsAppend("s1", "T", "{}")
		req := httptest.NewRequest(http.MethodGet, "/db/_sse_events/count", nil)
		rr := httptest.NewRecorder()
		h.handleSSEEvents(rr, req, "count")
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("SSE Events wipe", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/db/_sse_events", nil)
		rr := httptest.NewRecorder()
		h.handleSSEEvents(rr, req, "")
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("SSE Events invalid", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/db/_sse_events/invalid", nil)
		rr := httptest.NewRecorder()
		h.handleSSEEvents(rr, req, "invalid")
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
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
		req := httptest.NewRequest(http.MethodPut, "/kv/k1", strings.NewReader("{invalid-json}"))
		rr := httptest.NewRecorder()
		h.handleKV(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("TTL required for expire", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/kv/k1/_expire", strings.NewReader(`{"ttl":0}`))
		rr := httptest.NewRecorder()
		h.handleKV(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("KV Keys", func(t *testing.T) {
		h.db.KVSet("key1", "val1", 0)
		req := httptest.NewRequest(http.MethodPost, "/kv/_keys", strings.NewReader(`{"pattern":"key*"}`))
		rr := httptest.NewRecorder()
		h.handleKVKeys(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
		assert.Contains(t, rr.Body.String(), "key1")
	})

	t.Run("KV Keys Invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/kv/_keys", strings.NewReader(`{invalid}`))
		rr := httptest.NewRecorder()
		h.handleKVKeys(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("KV Scan Invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/kv/_scan", strings.NewReader(`{invalid}`))
		rr := httptest.NewRecorder()
		h.handleKVScan(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("KV Delete Pattern Missing Pattern", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/kv/_delete_pattern", strings.NewReader(`{}`))
		rr := httptest.NewRecorder()
		h.handleKVDeletePattern(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("KV Delete Pattern Invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/kv/_delete_pattern", strings.NewReader(`{invalid}`))
		rr := httptest.NewRecorder()
		h.handleKVDeletePattern(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("KV Method Not Allowed", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/kv/key", nil)
		rr := httptest.NewRecorder()
		h.handleKV(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})
}

func TestHandleBlob(t *testing.T) {
	h, _ := setupTestHTTPHandler(t)

	t.Run("PUT and GET", func(t *testing.T) {
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
		reqMeta := httptest.NewRequest(http.MethodGet, "/blob/ns1/b1/meta", nil)
		rrMeta := httptest.NewRecorder()
		h.handleBlob(rrMeta, reqMeta)
		assert.Equal(t, http.StatusOK, rrMeta.Code)
		assert.Contains(t, rrMeta.Body.String(), `"id":"b1"`)
	})

	t.Run("Too Large", func(t *testing.T) {
		largeBody := make([]byte, maxBlobBodySize+1)
		req := httptest.NewRequest(http.MethodPut, "/blob/ns1/large", bytes.NewReader(largeBody))
		req.Header.Set("Content-Type", "application/octet-stream")
		rr := httptest.NewRecorder()
		h.handleBlob(rr, req)
		assert.Equal(t, http.StatusRequestEntityTooLarge, rr.Code)
	})

	t.Run("Missing Content-Type", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/blob/ns1/b1", strings.NewReader("data"))
		rr := httptest.NewRecorder()
		h.handleBlob(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "Content-Type header required")
	})

	t.Run("Invalid namespace", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodDelete, "/blob/../invalid", nil)
		rr := httptest.NewRecorder()
		h.handleBlob(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Namespace delete not allowed method", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/blob/ns1", nil)
		rr := httptest.NewRecorder()
		h.handleBlob(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Blob id invalid", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/blob/ns1/..", nil)
		rr := httptest.NewRecorder()
		h.handleBlob(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("Blob meta not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/blob/ns1/nonexistent/meta", nil)
		rr := httptest.NewRecorder()
		h.handleBlob(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("Blob get not found", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/blob/ns1/nonexistent", nil)
		rr := httptest.NewRecorder()
		h.handleBlob(rr, req)
		assert.Equal(t, http.StatusNotFound, rr.Code)
	})

	t.Run("Blob PUT empty body", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/blob/ns1/empty", strings.NewReader(""))
		req.Header.Set("Content-Type", "text/plain")
		rr := httptest.NewRecorder()
		h.handleBlob(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
		assert.Contains(t, rr.Body.String(), "body must not be empty")
	})

	t.Run("Blob PUT read error", func(t *testing.T) {
		// Create a request with a body that returns an error
		req := httptest.NewRequest(http.MethodPut, "/blob/ns1/error", &errorReader{})
		req.Header.Set("Content-Type", "text/plain")
		rr := httptest.NewRecorder()
		h.handleBlob(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})

	t.Run("X-Blob-TTL valid", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/blob/ns1/ttl-test", strings.NewReader("data"))
		req.Header.Set("Content-Type", "text/plain")
		req.Header.Set("X-Blob-TTL", "3600")
		rr := httptest.NewRecorder()
		h.handleBlob(rr, req)
		assert.Equal(t, http.StatusOK, rr.Code)
	})

	t.Run("X-Blob-TTL invalid", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPut, "/blob/ns1/ttl-fail", strings.NewReader("data"))
		req.Header.Set("Content-Type", "text/plain")
		req.Header.Set("X-Blob-TTL", "-1")
		rr := httptest.NewRecorder()
		h.handleBlob(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
}

func TestWebSocketAuthIntegration(t *testing.T) {
	h, _ := setupTestHTTPHandler(t)
	token := "ws-auth-token"

	// Seed token in DB
	err := h.db.DocSet("settings", "platform_settings", mustDocJSON(t, map[string]interface{}{
		"internal_auth_token": token,
	}))
	require.NoError(t, err)

	server := httptest.NewServer(h.buildRouter())
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/pubsub"

	t.Run("Valid token in query", func(t *testing.T) {
		urlWithToken := wsURL + "?token=" + token
		ws, resp, err := websocket.DefaultDialer.Dial(urlWithToken, nil)
		require.NoError(t, err)
		defer ws.Close()
		assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)

		// Verify we can send/receive
		subMsg := PubSubMessage{Action: constants.PubSubActionSubscribe, Channel: "test-chan"}
		err = ws.WriteJSON(subMsg)
		require.NoError(t, err)

		var ack map[string]interface{}
		err = ws.SetReadDeadline(time.Now().Add(2 * time.Second))
		require.NoError(t, err)
		err = ws.ReadJSON(&ack)
		require.NoError(t, err)
		assert.Equal(t, constants.PubSubEventSubscribed, ack["type"])
	})

	t.Run("Valid token in header", func(t *testing.T) {
		header := http.Header{}
		header.Set(constants.HeaderInternalAuth, token)
		ws, resp, err := websocket.DefaultDialer.Dial(wsURL, header)
		require.NoError(t, err)
		defer ws.Close()
		assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	})

	t.Run("Invalid token", func(t *testing.T) {
		urlWithToken := wsURL + "?token=wrong"
		ws, resp, err := websocket.DefaultDialer.Dial(urlWithToken, nil)
		assert.Error(t, err)
		if ws != nil {
			ws.Close()
		}
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("Missing token", func(t *testing.T) {
		ws, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
		assert.Error(t, err)
		if ws != nil {
			ws.Close()
		}
		assert.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})

	t.Run("Bootstrap bypass", func(t *testing.T) {
		// Remove token from DB and Env
		h.db.DocDelete("settings", "platform_settings")
		os.Unsetenv(string(constants.EnvVar.InternalAuthToken))

		ws, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
		require.NoError(t, err)
		defer ws.Close()
		assert.Equal(t, http.StatusSwitchingProtocols, resp.StatusCode)
	})
}

func TestHandlePubSubPublish(t *testing.T) {
	h, _ := setupTestHTTPHandler(t)

	t.Run("Publish valid", func(t *testing.T) {
		pubReq := models.PubSubPublishRequest{
			Channel: "test-chan",
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
		req := httptest.NewRequest(http.MethodGet, "/pubsub/publish", nil)
		rr := httptest.NewRecorder()
		h.handlePubSubPublish(rr, req)
		assert.Equal(t, http.StatusMethodNotAllowed, rr.Code)
	})

	t.Run("Invalid JSON", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/pubsub/publish", strings.NewReader("{invalid}"))
		rr := httptest.NewRecorder()
		h.handlePubSubPublish(rr, req)
		assert.Equal(t, http.StatusBadRequest, rr.Code)
	})
}

func TestContainsTraversal(t *testing.T) {
	assert.True(t, containsTraversal("/a/../b"))
	assert.True(t, containsTraversal("../etc/passwd"))
	assert.False(t, containsTraversal("/a/b/c"))
}

func TestBlobSegmentValid(t *testing.T) {
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
