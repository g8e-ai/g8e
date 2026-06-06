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
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/g8e-ai/g8e/internal/response"
	"github.com/stretchr/testify/require"
)

// endpointTestOption is a functional option for configuring a test GatewayService for endpoint tests.
type endpointTestOption func(*GatewayService)

// withEndpointSigningKey sets custom signing key and keyID for the test GatewayService.
func withEndpointSigningKey(privKey ed25519.PrivateKey, keyID string) endpointTestOption {
	return func(g *GatewayService) {
		g.signingKey = privKey
		g.keyID = keyID
	}
}

// newEndpointTestGatewayService creates a GatewayService with sensible defaults for endpoint testing.
// Options can be provided to override specific fields.
func newEndpointTestGatewayService(opts ...endpointTestOption) *GatewayService {
	logger := slog.Default()
	g := &GatewayService{
		logger:          logger,
		responder:       response.NewWriter(logger),
		maxPayloadBytes: 10 * 1024 * 1024,
	}

	// Apply options
	for _, opt := range opts {
		opt(g)
	}

	return g
}

func TestHandleMCP_InitializeHandshake(t *testing.T) {
	t.Parallel()
	g := newEndpointTestGatewayService()

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
	g := newEndpointTestGatewayService()

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
	g := newEndpointTestGatewayService()

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
	g := newEndpointTestGatewayService()

	reqBody := `{"jsonrpc":"2.0","method":"notifications/initialized"}`
	r := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleMCP(w, r)

	require.Equal(t, http.StatusAccepted, w.Code)
	require.Empty(t, w.Body.Bytes())
}

func TestHandleMCP_Ping(t *testing.T) {
	t.Parallel()
	g := newEndpointTestGatewayService()

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
	g := newEndpointTestGatewayService()

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
	g := newEndpointTestGatewayService()

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
	g := newEndpointTestGatewayService()

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
	g := newEndpointTestGatewayService()

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
	g := newEndpointTestGatewayService()

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
	g := newEndpointTestGatewayService()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	r := httptest.NewRequest(http.MethodGet, "/mcp", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	g.HandleMCP(w, r)

	require.Equal(t, http.StatusMethodNotAllowed, w.Code)
	require.Equal(t, "POST", w.Header().Get("Allow"))
}

func TestHandleMCP_InvalidJSON(t *testing.T) {
	t.Parallel()
	g := newEndpointTestGatewayService()

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
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	_ = pubKey

	g := newEndpointTestGatewayService(withEndpointSigningKey(privKey, "test-key"))

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
	g := newEndpointTestGatewayService()

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
	g := newEndpointTestGatewayService()

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
	g := newEndpointTestGatewayService()

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
