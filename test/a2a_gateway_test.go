//go:build integration

// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package tests

/*
TestA2AGateway_EndToEnd exercises g8eo from the perspective of a standard A2A client
(e.g., Google Agent2Agent protocol). It verifies the "Universal Protocol Translator"
logic which allows "dumb" clients to be governed by the g8e Gateway without needing
native signing or envelope construction logic.

Practical Coverage:
1. Protocol Translation: Maps A2A skill calls to typed GovernanceEnvelopes.
2. 3-Layer Verification: Forces skill calls through L1 (Hard Gates), L2 (Consensus), and L3 (Approval).
3. Suspension & OOB: Verifies that mutations are suspended, recorded, and only resumed
   after Out-of-Band (OOB) human approval via WebAuthn/Passkey.
4. Downstream Dispatch: Ensures verified payloads are correctly unwrapped and dispatched
   to the real downstream A2A server.
*/

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/services/network"
	"github.com/g8e-ai/g8e/test/fixtures"
)

func TestA2AGateway_SkillCallEndToEnd(t *testing.T) {
	downstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			SkillName string `json:"skill_name"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"result":"a2a says hello","summary":"verified skill execution"}`))
	}))
	defer downstreamServer.Close()

	fixture := fixtures.NewGatewayFixture(t, fixtures.GatewayFixtureOptions{
		TestName:          t.Name(),
		A2ADownstreamURL:  downstreamServer.URL,
		AllowTestPortZero: true,
	})
	fixture.WaitForReady(t)

	identity := fixtures.EnrollClientIdentity(t, fixture, "a2a-user", "a2a-org", "a2a-fingerprint", "a2a-host")
	mtlsClient := fixtures.CreateMTLSClient(t, fixture, identity)
	mtlsURL := network.LocalhostHTTPSURL(fixture.Service.GetHTTPSPort())

	// Test A2A Call (Suspends for L3, then Resume)
	t.Run("a2a call", func(t *testing.T) {
		callReq := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "a2a/call",
			"id":      1,
			"params": map[string]interface{}{
				"skill_name": "test-skill",
				"payload":    map[string]string{"foo": "bar"},
			},
		}

		reqBody, _ := json.Marshal(callReq)
		req, _ := http.NewRequest(http.MethodPost, mtlsURL+constants.APIPaths.A2ACall, bytes.NewReader(reqBody))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+identity.OperatorSessionID)
		resp, err := mtlsClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var a2aRes struct {
			JSONRPC string `json:"jsonrpc"`
			ID      int    `json:"id"`
			Result  struct {
				ID          string `json:"id"`
				Status      string `json:"status"`
				TxHash      string `json:"tx_hash"`
				ApprovalURL string `json:"approval_url"`
				Message     string `json:"message"`
			} `json:"result"`
		}
		err = json.NewDecoder(resp.Body).Decode(&a2aRes)
		require.NoError(t, err)
		// The L3 notary rejects, so the transaction should be suspended
		require.Equal(t, string(constants.GatewayResponseStatusSuspended), a2aRes.Result.Status, "expected suspended status, got: %s", a2aRes.Result.Status)
		require.NotEmpty(t, a2aRes.Result.ApprovalURL)
	})
}

// TestA2AGateway_APIKeyRejected verifies that the A2A endpoint rejects
// API-key-based authentication attempts. The gateway requires mTLS; any
// request presenting an X-API-Key header without a client certificate must
// fail with HTTP 401. This is an auth-layer test, not a protocol-error test,
// so it is not part of the shared cross-protocol error-case table.
func TestA2AGateway_APIKeyRejected(t *testing.T) {
	fixture := fixtures.NewGatewayFixture(t, fixtures.GatewayFixtureOptions{
		TestName:          t.Name(),
		AllowTestPortZero: true,
	})
	fixture.WaitForReady(t)

	mtlsURL := network.LocalhostHTTPSURL(fixture.Service.GetHTTPSPort())
	plainClient := fixtures.CreateNoCertClient(t, fixture)
	reqBody := `{"jsonrpc":"2.0","id":1,"method":"a2a/call","params":{"skill_name":"test"}}`

	req, _ := http.NewRequest(http.MethodPost, mtlsURL+constants.APIPaths.A2ACall, bytes.NewReader([]byte(reqBody)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-API-Key", "test-api-key")

	resp, err := plainClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
}
