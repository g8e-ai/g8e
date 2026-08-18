// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package tests

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/g8e-ai/g8e/internal/constants"
)

// errorCase describes a single malformed-request scenario exercised across
// both MCP and A2A protocols. buildBody receives the adapter so it can
// construct protocol-specific malformed JSON (e.g. the right method name and
// param field names). wantCode is the JSON-RPC error code to assert (0 means
// no error-code assertion, only HTTP 200); wantMsgContains is a substring the
// error message must contain (empty means no message assertion).
type errorCase struct {
	name            string
	buildBody       func(adapter protocolAdapter) []byte
	wantCode        int
	wantMsgContains string
}

// errorCases is the shared table of malformed-request scenarios. Both
// protocols assert the same outcome: the gateway returns HTTP 200 with a
// JSON-RPC error body (per the JSON-RPC 2.0 spec, errors are transported in
// the response body, not via HTTP status). New error scenarios are added
// here and automatically covered for both protocols.
var errorCases = []errorCase{
	{
		name: "invalid_jsonrpc_version",
		buildBody: func(a protocolAdapter) []byte {
			return jsonRaw(map[string]interface{}{
				"jsonrpc": "1.0",
				"id":      1,
				"method":  a.callMethod(),
				"params": map[string]interface{}{
					a.nameParamKey():    "test",
					a.payloadParamKey(): map[string]interface{}{},
				},
			})
		},
	},
	{
		name: "missing_method",
		buildBody: func(a protocolAdapter) []byte {
			return jsonRaw(map[string]interface{}{
				"jsonrpc": constants.JSONRPCVersion,
				"id":      1,
				"params": map[string]interface{}{
					a.nameParamKey():    "test",
					a.payloadParamKey(): map[string]interface{}{},
				},
			})
		},
	},
	{
		name: "unknown_method",
		buildBody: func(a protocolAdapter) []byte {
			return jsonRaw(map[string]interface{}{
				"jsonrpc": constants.JSONRPCVersion,
				"id":      1,
				"method":  "unknown_method",
				"params":  map[string]interface{}{},
			})
		},
	},
	{
		name: "malformed_json",
		buildBody: func(protocolAdapter) []byte {
			return []byte(`{invalid json`)
		},
		wantCode:        constants.JSONRPCErrorCodeParseError,
		wantMsgContains: constants.JSONRPCErrorMessageParseError,
	},
	{
		name: "missing_tool_or_skill_name",
		buildBody: func(a protocolAdapter) []byte {
			return jsonRaw(map[string]interface{}{
				"jsonrpc": constants.JSONRPCVersion,
				"id":      1,
				"method":  a.callMethod(),
				"params": map[string]interface{}{
					a.payloadParamKey(): map[string]interface{}{},
				},
			})
		},
	},
	{
		name: "invalid_payload_json",
		buildBody: func(a protocolAdapter) []byte {
			return jsonRaw(map[string]interface{}{
				"jsonrpc": constants.JSONRPCVersion,
				"id":      1,
				"method":  a.callMethod(),
				"params": map[string]interface{}{
					a.nameParamKey():    "test",
					a.payloadParamKey(): "{invalid}",
				},
			})
		},
	},
	{
		name: "missing_params",
		buildBody: func(a protocolAdapter) []byte {
			return jsonRaw(map[string]interface{}{
				"jsonrpc": constants.JSONRPCVersion,
				"id":      1,
				"method":  a.callMethod(),
			})
		},
	},
}

// TestGatewayProtocols_MalformedRequestsReturnJSONRPCErrors verifies that
// both the MCP and A2A gateways return JSON-RPC error responses for a matrix
// of malformed requests (invalid version, missing method, unknown method,
// malformed JSON, missing name, invalid payload, missing params). The
// dedicated per-protocol error-case tests were replaced by this single
// table-driven test. Per the JSON-RPC 2.0 spec, errors are returned with
// HTTP 200 and the error in the response body. The malformed-JSON case
// asserts the typed parse-error code constant.
func TestGatewayProtocols_MalformedRequestsReturnJSONRPCErrors(t *testing.T) {
	downstreamServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer downstreamServer.Close()

	af := newAdapterFixture(t, "error-cases", downstreamServer.URL)

	for _, adapter := range bothAdapters() {
		adapter := adapter
		for _, tc := range errorCases {
			tc := tc
			t.Run(adapter.name()+"/"+tc.name, func(t *testing.T) {
				body := tc.buildBody(adapter)
				status, respBody := af.postAdapterWithStatus(t, adapter, body)
				require.Equal(t, http.StatusOK, status, "JSON-RPC errors return HTTP 200 with error in body")

				if tc.wantCode != 0 {
					var jsonRPCResp struct {
						Error struct {
							Code    int    `json:"code"`
							Message string `json:"message"`
						} `json:"error"`
					}
					require.NoError(t, json.Unmarshal(respBody, &jsonRPCResp))
					require.Equal(t, tc.wantCode, jsonRPCResp.Error.Code,
						"JSON-RPC error code should match the typed constant")
					if tc.wantMsgContains != "" {
						require.Contains(t, jsonRPCResp.Error.Message, tc.wantMsgContains,
							"JSON-RPC error message should contain the typed prefix")
					}
				}
			})
		}
	}
}

// jsonRaw marshals v to JSON bytes, panicking on error. Used by errorCase
// buildBody functions to construct malformed request bodies.
func jsonRaw(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("jsonRaw: marshal: %v", err))
	}
	return b
}
