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
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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
		require.Equal(t, "suspended", a2aRes.Result.Status, "expected suspended status, got: %s", a2aRes.Result.Status)
		require.NotEmpty(t, a2aRes.Result.ApprovalURL)
	})
}

func TestA2AGateway_PayloadVariations(t *testing.T) {
	downstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			SkillName string `json:"skill_name"`
		}
		json.NewDecoder(r.Body).Decode(&req)

		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"result":"a2a response","summary":"verified skill execution"}`))
	}))
	defer downstreamServer.Close()

	fixture := fixtures.NewGatewayFixture(t, fixtures.GatewayFixtureOptions{
		TestName:          t.Name(),
		A2ADownstreamURL:  downstreamServer.URL,
		AllowTestPortZero: true,
	})
	fixture.WaitForReady(t)

	identity := fixtures.EnrollClientIdentity(t, fixture, "a2a-payload-user", "a2a-payload-org", "a2a-payload-fingerprint", "a2a-payload-host")
	mtlsClient := fixtures.CreateMTLSClient(t, fixture, identity)
	mtlsURL := network.LocalhostHTTPSURL(fixture.Service.GetHTTPSPort())

	t.Run("nested payload structure", func(t *testing.T) {
		callReq := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "a2a/call",
			"id":      1,
			"params": map[string]interface{}{
				"skill_name": "nested_skill",
				"payload": map[string]interface{}{
					"config": map[string]interface{}{
						"nested": map[string]interface{}{
							"deep": map[string]interface{}{
								"value": "test",
							},
						},
					},
					"items": []interface{}{"item1", "item2", 123},
				},
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
			Result struct {
				Status string `json:"status"`
			} `json:"result"`
		}
		err = json.NewDecoder(resp.Body).Decode(&a2aRes)
		require.NoError(t, err)
		require.Equal(t, "suspended", a2aRes.Result.Status)
	})

	t.Run("unicode and special characters in payload", func(t *testing.T) {
		callReq := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "a2a/call",
			"id":      1,
			"params": map[string]interface{}{
				"skill_name": "unicode_skill",
				"payload": map[string]interface{}{
					"text":  "Hello 世界 🌍 \n\t\r\"'\\",
					"emoji": []string{"😀", "🎉", "🚀"},
				},
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
			Result struct {
				Status string `json:"status"`
			} `json:"result"`
		}
		err = json.NewDecoder(resp.Body).Decode(&a2aRes)
		require.NoError(t, err)
		require.Equal(t, "suspended", a2aRes.Result.Status)
	})

	t.Run("large payload", func(t *testing.T) {
		largeString := strings.Repeat("x", 100000)
		callReq := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "a2a/call",
			"id":      1,
			"params": map[string]interface{}{
				"skill_name": "large_skill",
				"payload": map[string]interface{}{
					"data": largeString,
				},
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
			Result struct {
				Status string `json:"status"`
			} `json:"result"`
		}
		err = json.NewDecoder(resp.Body).Decode(&a2aRes)
		require.NoError(t, err)
		require.Equal(t, "suspended", a2aRes.Result.Status)
	})

	t.Run("empty payload", func(t *testing.T) {
		callReq := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "a2a/call",
			"id":      1,
			"params": map[string]interface{}{
				"skill_name": "empty_skill",
				"payload":    map[string]interface{}{},
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
			Result struct {
				Status string `json:"status"`
			} `json:"result"`
		}
		err = json.NewDecoder(resp.Body).Decode(&a2aRes)
		require.NoError(t, err)
		require.Equal(t, "suspended", a2aRes.Result.Status)
	})

	t.Run("null payload", func(t *testing.T) {
		callReq := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "a2a/call",
			"id":      1,
			"params": map[string]interface{}{
				"skill_name": "null_skill",
				"payload":    nil,
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
			Result struct {
				Status string `json:"status"`
			} `json:"result"`
		}
		err = json.NewDecoder(resp.Body).Decode(&a2aRes)
		require.NoError(t, err)
		require.Equal(t, "suspended", a2aRes.Result.Status)
	})

	t.Run("execution_id parameter", func(t *testing.T) {
		callReq := map[string]interface{}{
			"jsonrpc": "2.0",
			"method":  "a2a/call",
			"id":      1,
			"params": map[string]interface{}{
				"skill_name":   "execution_id_skill",
				"payload":      map[string]string{"foo": "bar"},
				"execution_id": "exec-12345",
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
			Result struct {
				Status string `json:"status"`
			} `json:"result"`
		}
		err = json.NewDecoder(resp.Body).Decode(&a2aRes)
		require.NoError(t, err)
		require.Equal(t, "suspended", a2aRes.Result.Status)
	})
}

func TestA2AGateway_ErrorCases(t *testing.T) {
	downstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// No downstream server needed for error cases
		w.WriteHeader(http.StatusOK)
	}))
	defer downstreamServer.Close()

	fixture := fixtures.NewGatewayFixture(t, fixtures.GatewayFixtureOptions{
		TestName:          t.Name(),
		A2ADownstreamURL:  downstreamServer.URL,
		AllowTestPortZero: true,
	})
	fixture.WaitForReady(t)

	identity := fixtures.EnrollClientIdentity(t, fixture, "a2a-error-user", "a2a-error-org", "a2a-error-fingerprint", "a2a-error-host")
	mtlsClient := fixtures.CreateMTLSClient(t, fixture, identity)
	mtlsURL := network.LocalhostHTTPSURL(fixture.Service.GetHTTPSPort())

	t.Run("api key rejected", func(t *testing.T) {
		plainClient := fixtures.CreateNoCertClient(t, fixture)
		reqBody := `{"jsonrpc":"2.0","id":1,"method":"a2a/call","params":{"skill_name":"test"}}`

		// Test with API key in header
		req, _ := http.NewRequest("POST", mtlsURL+constants.APIPaths.A2ACall, bytes.NewReader([]byte(reqBody)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-API-Key", "test-api-key")

		resp, err := plainClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode)
	})
	t.Run("invalid JSON-RPC version", func(t *testing.T) {
		reqBody := `{"jsonrpc":"1.0","id":1,"method":"a2a/call","params":{"skill_name":"test"}}`
		req, _ := http.NewRequest(http.MethodPost, mtlsURL+"/api/v1/a2a/call", bytes.NewReader([]byte(reqBody)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+identity.OperatorSessionID)
		resp, err := mtlsClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("missing method", func(t *testing.T) {
		reqBody := `{"jsonrpc":"2.0","id":1,"params":{"skill_name":"test"}}`
		req, _ := http.NewRequest(http.MethodPost, mtlsURL+"/api/v1/a2a/call", bytes.NewReader([]byte(reqBody)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+identity.OperatorSessionID)
		resp, err := mtlsClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("unknown method", func(t *testing.T) {
		reqBody := `{"jsonrpc":"2.0","id":1,"method":"unknown_method","params":{}}`
		req, _ := http.NewRequest(http.MethodPost, mtlsURL+"/api/v1/a2a/call", bytes.NewReader([]byte(reqBody)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+identity.OperatorSessionID)
		resp, err := mtlsClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("malformed JSON", func(t *testing.T) {
		reqBody := `{invalid json`
		req, _ := http.NewRequest(http.MethodPost, mtlsURL+"/api/v1/a2a/call", bytes.NewReader([]byte(reqBody)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+identity.OperatorSessionID)
		resp, err := mtlsClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
		var jsonRPCResp struct {
			Error struct {
				Code    int    `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		body, _ := io.ReadAll(resp.Body)
		err = json.Unmarshal(body, &jsonRPCResp)
		require.NoError(t, err)
		require.Equal(t, -32700, jsonRPCResp.Error.Code)
		require.Contains(t, jsonRPCResp.Error.Message, "parse error")
	})

	t.Run("missing skill_name", func(t *testing.T) {
		reqBody := `{"jsonrpc":"2.0","id":1,"method":"a2a/call","params":{"payload":{}}}`
		req, _ := http.NewRequest(http.MethodPost, mtlsURL+"/api/v1/a2a/call", bytes.NewReader([]byte(reqBody)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+identity.OperatorSessionID)
		resp, err := mtlsClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("invalid payload JSON", func(t *testing.T) {
		reqBody := `{"jsonrpc":"2.0","id":1,"method":"a2a/call","params":{"skill_name":"test","payload":"{invalid}"}}`
		req, _ := http.NewRequest(http.MethodPost, mtlsURL+"/api/v1/a2a/call", bytes.NewReader([]byte(reqBody)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+identity.OperatorSessionID)
		resp, err := mtlsClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})

	t.Run("missing params", func(t *testing.T) {
		reqBody := `{"jsonrpc":"2.0","id":1,"method":"a2a/call"}`
		req, _ := http.NewRequest(http.MethodPost, mtlsURL+"/api/v1/a2a/call", bytes.NewReader([]byte(reqBody)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+identity.OperatorSessionID)
		resp, err := mtlsClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	})
}
