// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package tests

/*
TestUniversalGateway exercises the universal gateway with an in-process
GatewayFixture. This file retains only the behaviors unique to the universal
endpoint that are not covered by the dedicated per-protocol files:

  - Multi-protocol auto-detection on the universal HTTP endpoint
  - OOB suspension and querying suspended transactions

The MCP flow, A2A flow, governance envelope verification, downstream
integration, and canonical JSON wire format tests were removed: the first four
are covered by mcp_gateway_test.go, a2a_gateway_test.go, and
l2_consensus_integration_test.go with real assertions, and the canonical JSON
test had no meaningful assertions. The cross-protocol payload-variation and
error-case coverage lives in protocol_payload_test.go and
protocol_errors_test.go via the protocolAdapter pattern.
*/

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/v2/internal/config"
	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/g8e-ai/g8e/v2/internal/models"
	"github.com/g8e-ai/g8e/v2/internal/services/mcp"
	"github.com/g8e-ai/g8e/v2/internal/services/network"
	"github.com/g8e-ai/g8e/v2/test/fixtures"
)

// TestUniversalGateway_MultiProtocolAutoDetection verifies that the universal
// HTTP endpoint auto-detects MCP and A2A payloads by their JSON-RPC method and
// dispatches each to the correct protocol handler. Both protocols must return
// HTTP 200 with a well-formed JSON-RPC response body.
func TestUniversalGateway_MultiProtocolAutoDetection(t *testing.T) {
	f := fixtures.NewGatewayFixture(t, fixtures.GatewayFixtureOptions{
		TestName:          "universal-autodetect",
		Posture:           config.PostureNotary,
		AllowTestPortZero: true,
	})

	identity := fixtures.EnrollClientIdentity(t, f, "test-user", "test-org", "test-fingerprint", "test-host")
	apiClient := fixtures.CreateMTLSClient(t, f, identity)
	mtlsURL := network.LocalhostHTTPSURL(f.Service.GetHTTPSPort())

	t.Run("mcp_payload_detected_on_universal_endpoint", func(t *testing.T) {
		listReq := mcp.JSONRPCRequest{
			JSONRPC: constants.JSONRPCVersion,
			Method:  "tools/list",
			ID:      1,
		}
		reqBody, _ := json.Marshal(listReq)
		req, _ := http.NewRequest(http.MethodPost, mtlsURL+constants.APIPaths.MCPEndpoint, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(constants.HeaderAuthorization, "Bearer "+identity.OperatorSessionID)

		resp, err := apiClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var mcpResp struct {
			Result mcp.ToolsListResult `json:"result"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&mcpResp))
		require.NotNil(t, mcpResp.Result.Tools, "MCP tools/list must return a tools array")
		require.NotEmpty(t, mcpResp.Result.Tools, "MCP tools/list must return at least one tool from the downstream server")
	})

	t.Run("a2a_payload_detected_on_universal_endpoint", func(t *testing.T) {
		callReq := map[string]interface{}{
			"jsonrpc": constants.JSONRPCVersion,
			"method":  "a2a/call",
			"id":      1,
			"params": map[string]interface{}{
				"skill_name": "multi_skill",
				"payload":    map[string]string{"test": "data"},
			},
		}

		reqBody, _ := json.Marshal(callReq)
		req, _ := http.NewRequest(http.MethodPost, mtlsURL+constants.APIPaths.A2ACall, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(constants.HeaderAuthorization, "Bearer "+identity.OperatorSessionID)

		resp, err := apiClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var a2aRes struct {
			Result struct {
				Status      string `json:"status"`
				ApprovalURL string `json:"approval_url"`
			} `json:"result"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&a2aRes))
		require.Equal(t, string(constants.GatewayResponseStatusSuspended), a2aRes.Result.Status,
			"A2A call under notary posture must suspend for L3 approval")
		require.NotEmpty(t, a2aRes.Result.ApprovalURL, "Suspended A2A call must return an approval URL")
	})
}

// TestUniversalGateway_OOBSuspensionAndApproval verifies that a state-changing
// tool call suspends for L3 notary approval and that the suspended transaction
// is subsequently listable via the GET /api/v1/approvals endpoint filtered to
// the authenticated user.
func TestUniversalGateway_OOBSuspensionAndApproval(t *testing.T) {
	f := fixtures.NewGatewayFixture(t, fixtures.GatewayFixtureOptions{
		TestName:          "universal-suspension",
		Posture:           config.PostureNotary,
		AllowTestPortZero: true,
	})

	identity := fixtures.EnrollClientIdentity(t, f, "test-user", "test-org", "test-fingerprint", "test-host")
	apiClient := fixtures.CreateMTLSClient(t, f, identity)
	mtlsURL := network.LocalhostHTTPSURL(f.Service.GetHTTPSPort())

	t.Run("transaction_suspension_for_l3_approval", func(t *testing.T) {
		callReq := map[string]interface{}{
			"jsonrpc": constants.JSONRPCVersion,
			"method":  "tools/call",
			"id":      1,
			"params": map[string]interface{}{
				"name":      "sensitive_tool",
				"arguments": map[string]string{"action": "delete_file"},
			},
		}

		reqBody, _ := json.Marshal(callReq)
		req, _ := http.NewRequest(http.MethodPost, mtlsURL+constants.APIPaths.MCPEndpoint, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set(constants.HeaderAuthorization, "Bearer "+identity.OperatorSessionID)

		resp, err := apiClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var mcpRes struct {
			Result struct {
				Content []mcp.TextContent `json:"content"`
			} `json:"result"`
		}
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&mcpRes))
		require.NotEmpty(t, mcpRes.Result.Content, "Suspended MCP call must return content")
		require.Contains(t, mcpRes.Result.Content[0].Text, constants.MCPApprovalPausedPrefix,
			"Suspended MCP call content must start with the approval-paused prefix")
	})

	t.Run("query_suspended_transactions_lists_suspended_tx_for_user", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodGet, mtlsURL+constants.APIPaths.ApprovalsCLIList, nil)
		req.Header.Set(constants.HeaderAuthorization, "Bearer "+identity.OperatorSessionID)

		resp, err := apiClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var suspendedResp models.SuspendedTransactionsResponse
		require.NoError(t, json.NewDecoder(resp.Body).Decode(&suspendedResp))
		require.NotEmpty(t, suspendedResp.Transactions,
			"The transaction suspended in the previous subtest must be listable via GET /api/v1/approvals")
		require.Equal(t, identity.UserID, suspendedResp.Transactions[0].UserID,
			"Suspended transaction must be attributed to the authenticated user")
		require.Equal(t, "sensitive_tool", suspendedResp.Transactions[0].ToolName,
			"Suspended transaction must record the tool name from the suspended call")
	})
}
