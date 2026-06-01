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

package mcp

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/internal/responder"
	"github.com/stretchr/testify/require"
)

func TestHandleMCP_InitializeHandshake(t *testing.T) {
	t.Parallel()
	logger := slog.Default()
	resp := responder.New(logger)

	g := &GatewayService{
		logger:          logger,
		responder:       resp,
		maxPayloadBytes: 10 * 1024 * 1024,
	}

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`
	r := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleMCP(w, r)

	require.Equal(t, http.StatusOK, w.Code)

	var respBody map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &respBody)
	require.NoError(t, err)

	require.Equal(t, float64(1), respBody["id"])
	result := respBody["result"].(map[string]interface{})
	require.Equal(t, "2025-06-18", result["protocolVersion"])

	caps := result["capabilities"].(map[string]interface{})
	require.Contains(t, caps, "tools")
	require.Contains(t, caps, "resources")
	require.Contains(t, caps, "prompts")

	serverInfo := result["serverInfo"].(map[string]interface{})
	require.Equal(t, "g8e-gateway", serverInfo["name"])
	require.Equal(t, "1.0", serverInfo["version"])
}

func TestHandleMCP_InitializeEchoesProtocolVersion(t *testing.T) {
	t.Parallel()
	logger := slog.Default()
	resp := responder.New(logger)

	g := &GatewayService{
		logger:          logger,
		responder:       resp,
		maxPayloadBytes: 10 * 1024 * 1024,
	}

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`
	r := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleMCP(w, r)

	require.Equal(t, http.StatusOK, w.Code)

	var respBody map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &respBody)
	require.NoError(t, err)

	result := respBody["result"].(map[string]interface{})
	require.Equal(t, "2024-11-05", result["protocolVersion"])
}

func TestHandleMCP_InitializeDefaultsProtocolVersion(t *testing.T) {
	t.Parallel()
	logger := slog.Default()
	resp := responder.New(logger)

	g := &GatewayService{
		logger:          logger,
		responder:       resp,
		maxPayloadBytes: 10 * 1024 * 1024,
	}

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"initialize"}`
	r := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleMCP(w, r)

	require.Equal(t, http.StatusOK, w.Code)

	var respBody map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &respBody)
	require.NoError(t, err)

	require.Equal(t, float64(1), respBody["id"])
	result := respBody["result"].(map[string]interface{})
	require.Equal(t, "2025-06-18", result["protocolVersion"])
}

func TestHandleMCP_NotificationInitialized(t *testing.T) {
	t.Parallel()
	logger := slog.Default()
	resp := responder.New(logger)

	g := &GatewayService{
		logger:          logger,
		responder:       resp,
		maxPayloadBytes: 10 * 1024 * 1024,
	}

	reqBody := `{"jsonrpc":"2.0","method":"notifications/initialized"}`
	r := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleMCP(w, r)

	require.Equal(t, http.StatusAccepted, w.Code)
	require.Empty(t, w.Body.Bytes())
}

func TestHandleMCP_Ping(t *testing.T) {
	t.Parallel()
	logger := slog.Default()
	resp := responder.New(logger)

	g := &GatewayService{
		logger:          logger,
		responder:       resp,
		maxPayloadBytes: 10 * 1024 * 1024,
	}

	reqBody := `{"jsonrpc":"2.0","id":42,"method":"ping"}`
	r := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleMCP(w, r)

	require.Equal(t, http.StatusOK, w.Code)

	var respBody map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &respBody)
	require.NoError(t, err)

	require.Equal(t, float64(42), respBody["id"])
	require.NotNil(t, respBody["result"])
	require.Empty(t, respBody["result"])
}

func TestHandleMCP_IDEchoing(t *testing.T) {
	t.Parallel()
	logger := slog.Default()
	resp := responder.New(logger)

	g := &GatewayService{
		logger:          logger,
		responder:       resp,
		maxPayloadBytes: 10 * 1024 * 1024,
	}

	testCases := []struct {
		id       string
		expected interface{}
	}{
		{"1", float64(1)},
		{"999", float64(999)},
		{`"abc-123"`, "abc-123"},
		{`"test-id"`, "test-id"},
	}
	for _, tc := range testCases {
		reqBody := `{"jsonrpc":"2.0","id":` + tc.id + `,"method":"ping"}`
		r := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
		w := httptest.NewRecorder()

		g.HandleMCP(w, r)

		require.Equal(t, http.StatusOK, w.Code)

		var respBody map[string]interface{}
		err := json.Unmarshal(w.Body.Bytes(), &respBody)
		require.NoError(t, err)

		require.Equal(t, tc.expected, respBody["id"])
	}
}

func TestHandleMCP_InvalidJSONRPCVersion(t *testing.T) {
	t.Parallel()
	logger := slog.Default()
	resp := responder.New(logger)

	g := &GatewayService{
		logger:          logger,
		responder:       resp,
		maxPayloadBytes: 10 * 1024 * 1024,
	}

	reqBody := `{"jsonrpc":"1.0","id":1,"method":"initialize"}`
	r := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleMCP(w, r)

	require.Equal(t, http.StatusOK, w.Code)

	var respBody map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &respBody)
	require.NoError(t, err)

	require.Contains(t, respBody, "error")
	errObj := respBody["error"].(map[string]interface{})
	require.Equal(t, float64(-32600), errObj["code"])
	require.Contains(t, errObj["message"].(string), "jsonrpc version must be 2.0")
}

func TestHandleMCP_MissingMethod(t *testing.T) {
	t.Parallel()
	logger := slog.Default()
	resp := responder.New(logger)

	g := &GatewayService{
		logger:          logger,
		responder:       resp,
		maxPayloadBytes: 10 * 1024 * 1024,
	}

	reqBody := `{"jsonrpc":"2.0","id":1}`
	r := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleMCP(w, r)

	require.Equal(t, http.StatusOK, w.Code)

	var respBody map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &respBody)
	require.NoError(t, err)

	require.Contains(t, respBody, "error")
	errObj := respBody["error"].(map[string]interface{})
	require.Equal(t, float64(-32600), errObj["code"])
	require.Contains(t, errObj["message"].(string), "method required")
}

func TestHandleMCP_UnknownMethod(t *testing.T) {
	t.Parallel()
	logger := slog.Default()
	resp := responder.New(logger)

	g := &GatewayService{
		logger:          logger,
		responder:       resp,
		maxPayloadBytes: 10 * 1024 * 1024,
	}

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"unknown_method"}`
	r := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleMCP(w, r)

	require.Equal(t, http.StatusOK, w.Code)

	var respBody map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &respBody)
	require.NoError(t, err)

	require.Contains(t, respBody, "error")
	errObj := respBody["error"].(map[string]interface{})
	require.Equal(t, float64(-32601), errObj["code"])
	require.Contains(t, errObj["message"].(string), "method not found")
}

func TestHandleMCP_BatchRequestRejected(t *testing.T) {
	t.Parallel()
	logger := slog.Default()
	resp := responder.New(logger)

	g := &GatewayService{
		logger:          logger,
		responder:       resp,
		maxPayloadBytes: 10 * 1024 * 1024,
	}

	reqBody := `[{"jsonrpc":"2.0","id":1,"method":"ping"}]`
	r := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleMCP(w, r)

	require.Equal(t, http.StatusOK, w.Code)

	var respBody map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &respBody)
	require.NoError(t, err)

	require.Contains(t, respBody, "error")
	errObj := respBody["error"].(map[string]interface{})
	require.Equal(t, float64(-32600), errObj["code"])
	require.Contains(t, errObj["message"].(string), "batch requests are not supported")
}

func TestHandleMCP_GETMethodNotAllowed(t *testing.T) {
	t.Parallel()
	logger := slog.Default()
	resp := responder.New(logger)

	g := &GatewayService{
		logger:          logger,
		responder:       resp,
		maxPayloadBytes: 10 * 1024 * 1024,
	}

	r := httptest.NewRequest(http.MethodGet, "/mcp", nil)
	w := httptest.NewRecorder()

	g.HandleMCP(w, r)

	require.Equal(t, http.StatusMethodNotAllowed, w.Code)
	require.Equal(t, "POST", w.Header().Get("Allow"))
}

func TestHandleMCP_InvalidJSON(t *testing.T) {
	t.Parallel()
	logger := slog.Default()
	resp := responder.New(logger)

	g := &GatewayService{
		logger:          logger,
		responder:       resp,
		maxPayloadBytes: 10 * 1024 * 1024,
	}

	reqBody := `{invalid json`
	r := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleMCP(w, r)

	require.Equal(t, http.StatusOK, w.Code)

	var respBody map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &respBody)
	require.NoError(t, err)

	require.Contains(t, respBody, "error")
	errObj := respBody["error"].(map[string]interface{})
	require.Equal(t, float64(-32700), errObj["code"])
	require.Contains(t, errObj["message"].(string), "parse error")
}

func TestHandleMCP_ToolsList(t *testing.T) {
	t.Parallel()
	logger := slog.Default()
	resp := responder.New(logger)

	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	_ = pubKey

	g := &GatewayService{
		logger:          logger,
		responder:       resp,
		signingKey:      privKey,
		keyID:           "test-key",
		maxPayloadBytes: 10 * 1024 * 1024,
	}

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	r := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleMCP(w, r)

	require.Equal(t, http.StatusOK, w.Code)

	var respBody map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &respBody)
	require.NoError(t, err)

	require.Equal(t, float64(1), respBody["id"])
	result := respBody["result"].(map[string]interface{})
	require.Contains(t, result, "tools")
}

func TestHandleMCP_ResourcesList(t *testing.T) {
	t.Parallel()
	logger := slog.Default()
	resp := responder.New(logger)

	g := &GatewayService{
		logger:          logger,
		responder:       resp,
		maxPayloadBytes: 10 * 1024 * 1024,
	}

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"resources/list"}`
	r := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleMCP(w, r)

	require.Equal(t, http.StatusOK, w.Code)

	var respBody map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &respBody)
	require.NoError(t, err)

	require.Equal(t, float64(1), respBody["id"])
	result := respBody["result"].(map[string]interface{})
	require.Contains(t, result, "resources")
}

func TestHandleMCP_PromptsList(t *testing.T) {
	t.Parallel()
	logger := slog.Default()
	resp := responder.New(logger)

	g := &GatewayService{
		logger:          logger,
		responder:       resp,
		maxPayloadBytes: 10 * 1024 * 1024,
	}

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"prompts/list"}`
	r := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleMCP(w, r)

	require.Equal(t, http.StatusOK, w.Code)

	var respBody map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &respBody)
	require.NoError(t, err)

	require.Equal(t, float64(1), respBody["id"])
	result := respBody["result"].(map[string]interface{})
	require.Contains(t, result, "prompts")
}

func TestHandleMCP_ResourcesTemplatesList(t *testing.T) {
	t.Parallel()
	logger := slog.Default()
	resp := responder.New(logger)

	g := &GatewayService{
		logger:          logger,
		responder:       resp,
		maxPayloadBytes: 10 * 1024 * 1024,
	}

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"resources/templates/list"}`
	r := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleMCP(w, r)

	require.Equal(t, http.StatusOK, w.Code)

	var respBody map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &respBody)
	require.NoError(t, err)

	require.Equal(t, float64(1), respBody["id"])
	result := respBody["result"].(map[string]interface{})
	require.Contains(t, result, "resourceTemplates")
	templates := result["resourceTemplates"].([]interface{})
	require.Empty(t, templates)
}
