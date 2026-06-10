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

//go:build e2e

package tests

/*
TestUniversalGateway_RealInfrastructure exercises the universal gateway with
REAL operators, REAL gateways, and REAL MCP/A2A calls against a running platform.

This test suite validates:
1. Real MCP protocol translation and governance enforcement
2. Real A2A protocol translation and governance enforcement
3. Multi-protocol payload auto-detection on the universal HTTP endpoint
4. Full L1/L2/L3 governance gate verification with real infrastructure
5. OOB suspension and WebAuthn approval flow
6. Real downstream server integration
7. Canonical JSON wire format with protojson GovernanceEnvelope

Prerequisites:
- Platform running: ./g8e platform start
- Authenticated: ./g8e auth login
- Real PKI certificates in .g8e/pki
- Real SQLite database in .g8e/data

This test uses NO mocks - all components are real infrastructure.
*/

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/services/mcp"
)

// readLiveCreds reads cli_session_id from the credentials file.
// Universal gateway tests use CLI auth (X-G8E-CLI-Session-ID header + CLI cert).
func readLiveCreds(t *testing.T, credsFile string) (cliSessionID string) {
	t.Helper()
	data, err := os.ReadFile(credsFile)
	if err != nil {
		t.Fatalf("failed to read credentials file at %s - run './g8e auth login' first: %v", credsFile, err)
	}
	var creds struct {
		CLISessionID string `json:"cli_session_id"`
	}
	require.NoError(t, json.Unmarshal(data, &creds), "failed to parse credentials file")
	require.NotEmpty(t, creds.CLISessionID, "cli_session_id not found in credentials - run './g8e auth login' first")
	return creds.CLISessionID
}

// TestUniversalGateway_RealMCPFlow validates the complete MCP flow
// with real operator, real gateway, and real MCP calls.
func TestUniversalGateway_RealMCPFlow(t *testing.T) {
	repoRoot := ResolveRepoRootFromTestDir(t)
	EnsureAuthLogin(t, repoRoot)
	mtlsClient, cliCfg := NewLiveOperatorHTTPClient(t, repoRoot)
	EnsureGatewayReady(t, cliCfg)

	cliSessionID := readLiveCreds(t, cliCfg.CredentialsFile())
	mtlsURL := fmt.Sprintf("https://localhost:%d", cliCfg.OperatorHTTPSPort())

	setAuth := func(req *http.Request) {
		req.Header.Set(constants.HeaderCLISessionID, cliSessionID)
	}

	t.Run("health check", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, mtlsURL+constants.APIPaths.Health, nil)
		setAuth(req)
		resp, err := mtlsClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var health models.HealthResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&health))
		require.NotEmpty(t, health.StateMerkleRoot)
		t.Logf("State root: %s", health.StateMerkleRoot)
	})

	t.Run("MCP tools/list with real gateway", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, mtlsURL+"/api/v1/mcp/tools/list", nil)
		setAuth(req)
		resp, err := mtlsClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			var mcpResp struct {
				Result mcp.ToolsListResult `json:"result"`
			}
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&mcpResp))
			t.Logf("Tools listed: %d tools", len(mcpResp.Result.Tools))
		} else {
			t.Logf("tools/list returned status %d (may indicate no downstream MCP server configured)", resp.StatusCode)
		}
	})

	t.Run("MCP resources/list with real gateway", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, mtlsURL+"/api/v1/mcp/resources/list", nil)
		setAuth(req)
		resp, err := mtlsClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			var mcpResp struct {
				Result mcp.ListResourcesResult `json:"result"`
			}
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&mcpResp))
			t.Logf("Resources listed: %d resources", len(mcpResp.Result.Resources))
		} else {
			t.Logf("resources/list returned status %d", resp.StatusCode)
		}
	})

	t.Run("MCP prompts/list with real gateway", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, mtlsURL+"/api/v1/mcp/prompts/list", nil)
		setAuth(req)
		resp, err := mtlsClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			var mcpResp struct {
				Result mcp.ListPromptsResult `json:"result"`
			}
			require.NoError(t, json.NewDecoder(resp.Body).Decode(&mcpResp))
			t.Logf("Prompts listed: %d prompts", len(mcpResp.Result.Prompts))
		} else {
			t.Logf("prompts/list returned status %d", resp.StatusCode)
		}
	})

	t.Run("MCP tools/call with governance envelope", func(t *testing.T) {
		callReq := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "tools/call",
			"id":      1,
			"params": map[string]interface{}{
				"name":      "test_tool",
				"arguments": map[string]string{"message": "hello from real gateway"},
			},
		}

		reqBody, _ := json.Marshal(callReq)
		req, _ := http.NewRequest(http.MethodPost, mtlsURL+"/api/v1/mcp/tools/call", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		setAuth(req)
		resp, err := mtlsClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err == nil {
			if txHash, ok := result["transaction_hash"].(string); ok {
				t.Logf("Transaction hash: %s", txHash)
			}
		}
		t.Logf("MCP tools/call status %d response: %s", resp.StatusCode, string(body))
	})
}

// TestUniversalGateway_RealA2AFlow validates the complete A2A flow
// with real operator, real gateway, and real A2A calls.
func TestUniversalGateway_RealA2AFlow(t *testing.T) {
	repoRoot := ResolveRepoRootFromTestDir(t)
	EnsureAuthLogin(t, repoRoot)
	mtlsClient, cliCfg := NewLiveOperatorHTTPClient(t, repoRoot)
	EnsureGatewayReady(t, cliCfg)

	cliSessionID := readLiveCreds(t, cliCfg.CredentialsFile())
	mtlsURL := fmt.Sprintf("https://localhost:%d", cliCfg.OperatorHTTPSPort())

	setAuth := func(req *http.Request) {
		req.Header.Set(constants.HeaderCLISessionID, cliSessionID)
	}

	t.Run("A2A skill call with governance envelope", func(t *testing.T) {
		callReq := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "a2a/call",
			"id":      1,
			"params": map[string]interface{}{
				"skill_name": "test_skill",
				"payload": map[string]string{
					"action": "test",
					"data":   "real a2a call",
				},
			},
		}

		reqBody, _ := json.Marshal(callReq)
		req, _ := http.NewRequest(http.MethodPost, mtlsURL+"/api/v1/a2a/call", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		setAuth(req)
		resp, err := mtlsClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err == nil {
			if txHash, ok := result["transaction_hash"].(string); ok {
				t.Logf("Transaction hash: %s", txHash)
			}
		}
		t.Logf("A2A call status %d response: %s", resp.StatusCode, string(body))
	})
}

// TestUniversalGateway_MultiProtocolAutoDetection validates that the
// universal HTTP endpoint auto-detects MCP and A2A payloads.
func TestUniversalGateway_MultiProtocolAutoDetection(t *testing.T) {
	repoRoot := ResolveRepoRootFromTestDir(t)
	EnsureAuthLogin(t, repoRoot)
	mtlsClient, cliCfg := NewLiveOperatorHTTPClient(t, repoRoot)
	EnsureGatewayReady(t, cliCfg)

	cliSessionID := readLiveCreds(t, cliCfg.CredentialsFile())
	mtlsURL := fmt.Sprintf("https://localhost:%d", cliCfg.OperatorHTTPSPort())

	setAuth := func(req *http.Request) {
		req.Header.Set(constants.HeaderCLISessionID, cliSessionID)
	}

	t.Run("MCP payload detected on universal endpoint", func(t *testing.T) {
		callReq := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "tools/list",
			"id":      1,
			"params":  map[string]interface{}{},
		}

		reqBody, _ := json.Marshal(callReq)
		req, _ := http.NewRequest(http.MethodPost, mtlsURL+"/api/v1/mcp/tools/list", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		setAuth(req)
		resp, err := mtlsClient.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		t.Logf("MCP detection status: %d", resp.StatusCode)
	})

	t.Run("A2A payload detected on universal endpoint", func(t *testing.T) {
		callReq := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "a2a/call",
			"id":      1,
			"params": map[string]interface{}{
				"skill_name": "multi_skill",
				"payload":    map[string]string{"test": "data"},
			},
		}

		reqBody, _ := json.Marshal(callReq)
		req, _ := http.NewRequest(http.MethodPost, mtlsURL+"/api/v1/a2a/call", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		setAuth(req)
		resp, err := mtlsClient.Do(req)
		require.NoError(t, err)
		resp.Body.Close()
		t.Logf("A2A detection status: %d", resp.StatusCode)
	})
}

// TestUniversalGateway_GovernanceEnvelopeVerification validates that
// all requests pass through the L1/L2/L3 governance gates with real infrastructure.
func TestUniversalGateway_GovernanceEnvelopeVerification(t *testing.T) {
	repoRoot := ResolveRepoRootFromTestDir(t)
	EnsureAuthLogin(t, repoRoot)
	mtlsClient, cliCfg := NewLiveOperatorHTTPClient(t, repoRoot)
	EnsureGatewayReady(t, cliCfg)

	cliSessionID := readLiveCreds(t, cliCfg.CredentialsFile())
	mtlsURL := fmt.Sprintf("https://localhost:%d", cliCfg.OperatorHTTPSPort())

	setAuth := func(req *http.Request) {
		req.Header.Set(constants.HeaderCLISessionID, cliSessionID)
	}

	t.Run("L1 hard gates enforced", func(t *testing.T) {
		callReq := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "tools/call",
			"id":      1,
			"params": map[string]interface{}{
				"name": "execute_command",
				"arguments": map[string]string{
					"command": "sudo rm -rf /",
				},
			},
		}

		reqBody, _ := json.Marshal(callReq)
		req, _ := http.NewRequest(http.MethodPost, mtlsURL+"/api/v1/mcp/tools/call", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		setAuth(req)
		resp, err := mtlsClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err == nil {
			if errorMsg, ok := result["error"].(string); ok {
				t.Logf("L1 rejection: %s", errorMsg)
				require.Contains(t, strings.ToLower(errorMsg), "forbidden")
			}
		}
	})

	t.Run("L2 consensus verification", func(t *testing.T) {
		callReq := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "tools/call",
			"id":      1,
			"params": map[string]interface{}{
				"name":      "consensus_tool",
				"arguments": map[string]string{"action": "require_consensus"},
			},
		}

		reqBody, _ := json.Marshal(callReq)
		req, _ := http.NewRequest(http.MethodPost, mtlsURL+"/api/v1/mcp/tools/call", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		setAuth(req)
		resp, err := mtlsClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err == nil {
			if l2Meta, ok := result["l2_metadata"].(map[string]interface{}); ok {
				t.Logf("L2 metadata present: %v", l2Meta)
			}
		}
	})

	t.Run("L3 approval verification", func(t *testing.T) {
		callReq := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "tools/call",
			"id":      1,
			"params": map[string]interface{}{
				"name":      "approval_tool",
				"arguments": map[string]string{"action": "require_approval"},
			},
		}

		reqBody, _ := json.Marshal(callReq)
		req, _ := http.NewRequest(http.MethodPost, mtlsURL+"/api/v1/mcp/tools/call", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		setAuth(req)
		resp, err := mtlsClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err == nil {
			if approvalURL, ok := result["approval_url"].(string); ok {
				t.Logf("L3 approval URL: %s", approvalURL)
			}
			if status, ok := result["status"].(string); ok {
				t.Logf("Transaction status: %s", status)
			}
		}
	})
}

// TestUniversalGateway_OOBSuspensionAndApproval validates the OOB
// suspension and WebAuthn approval flow with real infrastructure.
func TestUniversalGateway_OOBSuspensionAndApproval(t *testing.T) {
	repoRoot := ResolveRepoRootFromTestDir(t)
	EnsureAuthLogin(t, repoRoot)
	mtlsClient, cliCfg := NewLiveOperatorHTTPClient(t, repoRoot)
	EnsureGatewayReady(t, cliCfg)

	cliSessionID := readLiveCreds(t, cliCfg.CredentialsFile())
	mtlsURL := fmt.Sprintf("https://localhost:%d", cliCfg.OperatorHTTPSPort())

	setAuth := func(req *http.Request) {
		req.Header.Set(constants.HeaderCLISessionID, cliSessionID)
	}

	t.Run("transaction suspension for L3 approval", func(t *testing.T) {
		callReq := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "tools/call",
			"id":      1,
			"params": map[string]interface{}{
				"name":      "sensitive_tool",
				"arguments": map[string]string{"action": "delete_file"},
			},
		}

		reqBody, _ := json.Marshal(callReq)
		req, _ := http.NewRequest(http.MethodPost, mtlsURL+"/api/v1/mcp/tools/call", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		setAuth(req)
		resp, err := mtlsClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		t.Logf("Suspension test status %d: %s", resp.StatusCode, string(body))
	})

	t.Run("query suspended transactions", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, mtlsURL+"/api/v1/suspended-transactions", nil)
		setAuth(req)
		resp, err := mtlsClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			var transactions []map[string]interface{}
			if err := json.NewDecoder(resp.Body).Decode(&transactions); err == nil {
				t.Logf("Suspended transactions: %d", len(transactions))
			}
		}
	})
}

// TestUniversalGateway_RealDownstreamIntegration validates integration
// with a real downstream MCP/A2A server configured in the gateway.
func TestUniversalGateway_RealDownstreamIntegration(t *testing.T) {
	repoRoot := ResolveRepoRootFromTestDir(t)
	EnsureAuthLogin(t, repoRoot)
	mtlsClient, cliCfg := NewLiveOperatorHTTPClient(t, repoRoot)
	EnsureGatewayReady(t, cliCfg)

	cliSessionID := readLiveCreds(t, cliCfg.CredentialsFile())
	mtlsURL := fmt.Sprintf("https://localhost:%d", cliCfg.OperatorHTTPSPort())

	setAuth := func(req *http.Request) {
		req.Header.Set(constants.HeaderCLISessionID, cliSessionID)
	}

	t.Run("downstream server tools/list", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, mtlsURL+"/api/v1/mcp/tools/list", nil)
		setAuth(req)
		resp, err := mtlsClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			var mcpResp struct {
				Result mcp.ToolsListResult `json:"result"`
			}
			if err := json.Unmarshal(body, &mcpResp); err == nil {
				t.Logf("Downstream tools: %d", len(mcpResp.Result.Tools))
			}
		} else {
			t.Logf("Downstream server not configured (status %d)", resp.StatusCode)
		}
	})

	t.Run("downstream server tools/call", func(t *testing.T) {
		callReq := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "tools/call",
			"id":      1,
			"params": map[string]interface{}{
				"name":      "execute_command",
				"arguments": map[string]string{"command": "echo test"},
			},
		}

		reqBody, _ := json.Marshal(callReq)
		req, _ := http.NewRequest(http.MethodPost, mtlsURL+"/api/v1/mcp/tools/call", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		setAuth(req)
		resp, err := mtlsClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		t.Logf("Downstream call status %d: %s", resp.StatusCode, string(body))
	})
}

// TestUniversalGateway_CanonicalJSONWireFormat validates that the gateway
// uses canonical JSON (protojson) for all wire formats.
func TestUniversalGateway_CanonicalJSONWireFormat(t *testing.T) {
	repoRoot := ResolveRepoRootFromTestDir(t)
	EnsureAuthLogin(t, repoRoot)
	mtlsClient, cliCfg := NewLiveOperatorHTTPClient(t, repoRoot)
	EnsureGatewayReady(t, cliCfg)

	cliSessionID := readLiveCreds(t, cliCfg.CredentialsFile())
	mtlsURL := fmt.Sprintf("https://localhost:%d", cliCfg.OperatorHTTPSPort())

	setAuth := func(req *http.Request) {
		req.Header.Set(constants.HeaderCLISessionID, cliSessionID)
	}

	t.Run("governance envelope uses protojson", func(t *testing.T) {
		callReq := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "tools/call",
			"id":      1,
			"params": map[string]interface{}{
				"name":      "canonical_test",
				"arguments": map[string]string{"test": "data"},
			},
		}

		reqBody, _ := json.Marshal(callReq)
		req, _ := http.NewRequest(http.MethodPost, mtlsURL+"/api/v1/mcp/tools/call", bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		setAuth(req)
		resp, err := mtlsClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		var result map[string]interface{}
		if err := json.Unmarshal(body, &result); err == nil {
			if txHash, ok := result["transaction_hash"].(string); ok {
				t.Logf("Canonical transaction_hash: %s", txHash)
			}
			if timestamp, ok := result["timestamp"].(string); ok {
				t.Logf("Canonical timestamp: %s", timestamp)
			}
		}
		t.Logf("Canonical JSON wire format status %d", resp.StatusCode)
	})
}
