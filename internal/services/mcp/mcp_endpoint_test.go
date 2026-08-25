// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

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

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/response"
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
	_, privKey, _ := ed25519.GenerateKey(rand.Reader)
	g := &GatewayService{
		logger:          logger,
		responder:       response.NewWriter(logger),
		maxPayloadBytes: 10 * 1024 * 1024,
		envProc:         &fakeEnvelopeProcessor{},
		signingKey:      privKey,
		keyID:           "endpoint-test-key",
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

	var resp JSONRPCResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Nil(t, resp.Error)

	require.InEpsilon(t, float64(1), resp.ID, 0.0)
	var result InitializeResult
	err = json.Unmarshal(resp.Result, &result)
	require.NoError(t, err)
	require.Equal(t, "2025-06-18", result.ProtocolVersion)
	require.Equal(t, ServerCapabilities{
		Tools:     ToolsCapability{ListChanged: true},
		Resources: ResourcesCapability{},
		Prompts:   PromptsCapability{},
	}, result.Capabilities)
	require.Equal(t, "g8e-gateway", result.ServerInfo.Name)
	require.Equal(t, "1.0", result.ServerInfo.Version)
}

func TestHandleMCP_InitializeEchoesProtocolVersion(t *testing.T) {
	t.Parallel()
	g := newEndpointTestGatewayService()

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05"}}`
	r := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleMCP(w, r)

	require.Equal(t, http.StatusOK, w.Code)

	var resp JSONRPCResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Nil(t, resp.Error)

	var result InitializeResult
	err = json.Unmarshal(resp.Result, &result)
	require.NoError(t, err)
	require.Equal(t, "2024-11-05", result.ProtocolVersion)
}

func TestHandleMCP_InitializeDefaultsProtocolVersion(t *testing.T) {
	t.Parallel()
	g := newEndpointTestGatewayService()

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"initialize"}`
	r := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleMCP(w, r)

	require.Equal(t, http.StatusOK, w.Code)

	var resp JSONRPCResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Nil(t, resp.Error)

	require.InEpsilon(t, float64(1), resp.ID, 0.0)
	var result InitializeResult
	err = json.Unmarshal(resp.Result, &result)
	require.NoError(t, err)
	require.Equal(t, "2025-06-18", result.ProtocolVersion)
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

	var resp JSONRPCResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Nil(t, resp.Error)

	require.InEpsilon(t, float64(42), resp.ID, 0.0)
	require.NotNil(t, resp.Result)
	require.JSONEq(t, "{}", string(resp.Result))
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

		var resp JSONRPCResponse
		err := json.Unmarshal(w.Body.Bytes(), &resp)
		require.NoError(t, err)

		require.Equal(t, tc.expected, resp.ID)
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

	var resp JSONRPCResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	require.NotNil(t, resp.Error)
	require.Equal(t, -32600, resp.Error.Code)
	require.Contains(t, resp.Error.Message, "jsonrpc version must be 2.0")
}

func TestHandleMCP_MissingMethod(t *testing.T) {
	t.Parallel()
	g := newEndpointTestGatewayService()

	reqBody := `{"jsonrpc":"2.0","id":1}`
	r := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleMCP(w, r)

	require.Equal(t, http.StatusOK, w.Code)

	var resp JSONRPCResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	require.NotNil(t, resp.Error)
	require.Equal(t, -32600, resp.Error.Code)
	require.Contains(t, resp.Error.Message, "method required")
}

func TestHandleMCP_UnknownMethod(t *testing.T) {
	t.Parallel()
	g := newEndpointTestGatewayService()

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"unknown_method"}`
	r := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleMCP(w, r)

	require.Equal(t, http.StatusOK, w.Code)

	var resp JSONRPCResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	require.NotNil(t, resp.Error)
	require.Equal(t, -32601, resp.Error.Code)
	require.Contains(t, resp.Error.Message, "method not found")
}

func TestHandleMCP_BatchRequestRejected(t *testing.T) {
	t.Parallel()
	g := newEndpointTestGatewayService()

	reqBody := `[{"jsonrpc":"2.0","id":1,"method":"ping"}]`
	r := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleMCP(w, r)

	require.Equal(t, http.StatusOK, w.Code)

	var resp JSONRPCResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	require.NotNil(t, resp.Error)
	require.Equal(t, -32600, resp.Error.Code)
	require.Contains(t, resp.Error.Message, "batch requests are not supported")
}

func TestHandleMCP_GETMethodNotAllowed(t *testing.T) {
	t.Parallel()
	g := newEndpointTestGatewayService()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	r := httptest.NewRequest(http.MethodDelete, "/mcp", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	g.HandleMCP(w, r)

	require.Equal(t, http.StatusMethodNotAllowed, w.Code)
	require.Equal(t, "POST, GET", w.Header().Get("Allow"))
}

func TestHandleMCP_InvalidJSON(t *testing.T) {
	t.Parallel()
	g := newEndpointTestGatewayService()

	reqBody := `{invalid json`
	r := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleMCP(w, r)

	require.Equal(t, http.StatusOK, w.Code)

	var resp JSONRPCResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)

	require.NotNil(t, resp.Error)
	require.Equal(t, constants.JSONRPCErrorCodeParseError, resp.Error.Code)
	require.Contains(t, resp.Error.Message, constants.JSONRPCErrorMessageParseError)
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

	var resp JSONRPCResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Nil(t, resp.Error)

	require.InEpsilon(t, float64(1), resp.ID, 0.0)
	var result ToolsListResult
	err = json.Unmarshal(resp.Result, &result)
	require.NoError(t, err)
	require.NotNil(t, result.Tools)
}

func TestHandleMCP_ResourcesList(t *testing.T) {
	t.Parallel()
	g := newEndpointTestGatewayService()

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"resources/list"}`
	r := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleMCP(w, r)

	require.Equal(t, http.StatusOK, w.Code)

	var resp JSONRPCResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Nil(t, resp.Error)

	require.InEpsilon(t, float64(1), resp.ID, 0.0)
	var result ResourcesListResult
	err = json.Unmarshal(resp.Result, &result)
	require.NoError(t, err)
	require.NotNil(t, result.Resources)
}

func TestHandleMCP_PromptsList(t *testing.T) {
	t.Parallel()
	g := newEndpointTestGatewayService()

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"prompts/list"}`
	r := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleMCP(w, r)

	require.Equal(t, http.StatusOK, w.Code)

	var resp JSONRPCResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Nil(t, resp.Error)

	require.InEpsilon(t, float64(1), resp.ID, 0.0)
	var result PromptsListResult
	err = json.Unmarshal(resp.Result, &result)
	require.NoError(t, err)
	require.NotNil(t, result.Prompts)
}

func TestHandleMCP_ResourcesTemplatesList(t *testing.T) {
	t.Parallel()
	g := newEndpointTestGatewayService()

	reqBody := `{"jsonrpc":"2.0","id":1,"method":"resources/templates/list"}`
	r := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(reqBody))
	w := httptest.NewRecorder()

	g.HandleMCP(w, r)

	require.Equal(t, http.StatusOK, w.Code)

	var resp JSONRPCResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Nil(t, resp.Error)

	require.InEpsilon(t, float64(1), resp.ID, 0.0)
	var result resourceTemplatesList
	err = json.Unmarshal(resp.Result, &result)
	require.NoError(t, err)
	require.Empty(t, result.ResourceTemplates)
}
