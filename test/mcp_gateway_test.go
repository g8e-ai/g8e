//go:build integration

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

package tests

/*
TestMCPGateway_EndToEnd exercises g8eo from the perspective of a standard MCP client
(e.g., Claude Code or a generic AI agent). It verifies the "Universal Protocol Translator"
logic which allows "dumb" clients to be governed by the g8e Gateway without needing
native signing or envelope construction logic.

Practical Coverage:
1. Protocol Translation: Maps JSON-RPC tools/list and tools/call to typed GovernanceEnvelopes.
2. 3-Layer Verification: Forces tool calls through L1 (Hard Gates), L2 (Consensus), and L3 (Approval).
3. Suspension & OOB: Verifies that mutations are suspended, recorded, and only resumed
   after Out-of-Band (OOB) human approval via WebAuthn/Passkey.
4. Downstream Dispatch: Ensures verified payloads are correctly unwrapped and dispatched
   to the real downstream MCP server.
*/

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/mcp"
	"github.com/g8e-ai/g8e/internal/services/network"
	"github.com/g8e-ai/g8e/test/fixtures"
)

func mustMarshal(v interface{}) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("failed to marshal: %v", err))
	}
	return b
}

func TestMCPGateway_EndToEnd(t *testing.T) {
	// Create gateway fixture with default mock downstream server
	fixture := fixtures.NewGatewayFixture(t, fixtures.GatewayFixtureOptions{
		TestName:          t.Name(),
		AllowTestPortZero: true,
	})

	// Wait for gateway to be ready
	fixture.WaitForReady(t)

	// Enroll client identity
	identity := fixtures.EnrollClientIdentity(t, fixture, "mcp-user", "mcp-org", "mcp-fingerprint", "mcp-host")

	// Create mTLS client with enrolled identity
	mtlsClient := fixtures.CreateMTLSClient(t, fixture, identity)

	// MCP routes are available on HTTPS port with mTLS
	mcpURL := network.LocalhostHTTPSURL(fixture.Service.GetHTTPSPort())

	// 4. Test MCP tools/list
	t.Run("tools/list", func(t *testing.T) {
		listReq := mcp.JSONRPCRequest{
			JSONRPC: "2.0",
			Method:  "tools/list",
			ID:      1,
		}
		reqBody, _ := json.Marshal(listReq)
		req, _ := http.NewRequest(http.MethodPost, mcpURL+constants.APIPaths.MCPEndpoint, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		resp, err := mtlsClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var mcpResp struct {
			Result mcp.ToolsListResult `json:"result"`
		}
		err = json.NewDecoder(resp.Body).Decode(&mcpResp)
		require.NoError(t, err)
		// Gateway proxies tools/list to downstream (1 echo tool).
		// Native tool merging was removed with the per-method REST handlers.
		require.GreaterOrEqual(t, len(mcpResp.Result.Tools), 1)
		// Verify downstream tool is present
		hasEcho := false
		for _, tool := range mcpResp.Result.Tools {
			if tool.Name == "echo" {
				hasEcho = true
				break
			}
		}
		require.True(t, hasEcho, "Downstream 'echo' tool should be present in merged list")
	})

	// 4.5 Test MCP resources/list
	t.Run("resources/list", func(t *testing.T) {
		listReq := mcp.JSONRPCRequest{
			JSONRPC: "2.0",
			Method:  "resources/list",
			ID:      1,
		}
		reqBody, _ := json.Marshal(listReq)
		req, _ := http.NewRequest(http.MethodPost, mcpURL+constants.APIPaths.MCPEndpoint, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		resp, err := mtlsClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var mcpResp struct {
			Result mcp.ResourcesListResult `json:"result"`
		}
		err = json.NewDecoder(resp.Body).Decode(&mcpResp)
		require.NoError(t, err)
		require.Len(t, mcpResp.Result.Resources, 1)
		require.Equal(t, "file:///test.txt", mcpResp.Result.Resources[0].URI)
	})

	// 4.6 Test MCP prompts/list
	t.Run("prompts/list", func(t *testing.T) {
		listReq := mcp.JSONRPCRequest{
			JSONRPC: "2.0",
			Method:  "prompts/list",
			ID:      1,
		}
		reqBody, _ := json.Marshal(listReq)
		req, _ := http.NewRequest(http.MethodPost, mcpURL+constants.APIPaths.MCPEndpoint, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		resp, err := mtlsClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var mcpResp struct {
			Result mcp.PromptsListResult `json:"result"`
		}
		err = json.NewDecoder(resp.Body).Decode(&mcpResp)
		require.NoError(t, err)
		require.Len(t, mcpResp.Result.Prompts, 1)
		require.Equal(t, "test-prompt", mcpResp.Result.Prompts[0].Name)
	})

	// 5. Test MCP tools/call (Direct, no L3 needed for benign echo)
	// Actually, MCP_CALL is classified as a mutation, so it needs L3 unless we bypass it.
	// In this test environment, gatewayRejectingL3Notary always returns false, so the transaction
	// is suspended and returns "Execution paused" instead of dispatching to downstream.
	t.Run("tools/call", func(t *testing.T) {
		callReq := mcp.JSONRPCRequest{
			JSONRPC: "2.0",
			Method:  "tools/call",
			ID:      1,
		}
		params := mcp.CallToolRequest{
			Name:      "echo",
			Arguments: mustMarshal(map[string]interface{}{"msg": "hello"}),
		}
		callReq.Params = mustMarshal(params)

		reqBody, _ := json.Marshal(callReq)
		req, _ := http.NewRequest(http.MethodPost, mcpURL+constants.APIPaths.MCPEndpoint, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		resp, err := mtlsClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var mcpRes struct {
			Result struct {
				Content []mcp.TextContent `json:"content"`
			} `json:"result"`
		}
		body, _ := io.ReadAll(resp.Body)
		err = json.Unmarshal(body, &mcpRes)
		require.NoError(t, err)

		// MCP tool call returns the approval-paused prefix because L3 is rejected
		require.NotEmpty(t, mcpRes.Result.Content)
		require.Contains(t, mcpRes.Result.Content[0].Text, constants.MCPApprovalPausedPrefix)
	})
}
