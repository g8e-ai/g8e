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

//go:build integration

package tests

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
)

// payloadCase describes a single payload-variation scenario exercised across
// both MCP and A2A protocols. The payload field is any because these tests
// deliberately exercise arbitrary shapes (nested, unicode, large, empty,
// null); this is the documented exception to the "no Any types for known
// shapes" rule in devs.md.
type payloadCase struct {
	name    string
	tool    string
	payload any
}

// payloadCases is the shared table of payload variations. Both protocols
// assert the same outcome: the transaction is suspended awaiting L3 notary
// approval. New payload shapes are added here and automatically covered for
// both protocols.
var payloadCases = []payloadCase{
	{
		name: "nested_object_structure",
		tool: "nested_tool",
		payload: map[string]interface{}{
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
	{
		name: "unicode_and_special_characters",
		tool: "unicode_tool",
		payload: map[string]interface{}{
			"text":  "Hello 世界 🌍 \n\t\r\"'\\",
			"emoji": []string{"😀", "🎉", "🚀"},
		},
	},
	{
		name:    "large_payload_100kb",
		tool:    "large_tool",
		payload: map[string]interface{}{"data": strings.Repeat("x", 100000)},
	},
	{
		name:    "empty_payload",
		tool:    "empty_tool",
		payload: map[string]interface{}{},
	},
	{
		name:    "null_payload",
		tool:    "null_tool",
		payload: nil,
	},
}

// TestGatewayProtocols_PayloadVariationsSuspendExecution verifies that both
// the MCP and A2A gateways suspend transactions across a matrix of payload
// shapes (nested, unicode, large, empty, null). The dedicated per-protocol
// payload-variation tests were replaced by this single table-driven test,
// which iterates over both protocol adapters and the shared payloadCases
// table. Each subtest asserts the typed suspended-status constant.
func TestGatewayProtocols_PayloadVariationsSuspendExecution(t *testing.T) {
	downstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"result":"ok","summary":"verified skill execution"}`))
	}))
	defer downstreamServer.Close()

	af := newAdapterFixture(t, "payload-variations", downstreamServer.URL)

	for _, adapter := range bothAdapters() {
		adapter := adapter
		for _, tc := range payloadCases {
			tc := tc
			t.Run(adapter.name()+"/"+tc.name, func(t *testing.T) {
				body := adapter.makeCallBody(tc.tool, tc.payload)
				status, respBody := af.postAdapterWithStatus(t, adapter, body)
				require.Equal(t, http.StatusOK, status)

				signal := adapter.parseSuspendedStatus(t, respBody)
				switch a := adapter.(type) {
				case mcpAdapter:
					require.Contains(t, signal, constants.MCPApprovalPausedPrefix,
						"MCP response content should start with the approval-paused prefix")
				case a2aAdapter:
					require.Equal(t, string(constants.GatewayResponseStatusSuspended), signal,
						"A2A response status should be the typed suspended constant")
				default:
					t.Fatalf("unhandled adapter type %T", a)
				}
			})
		}
	}
}
