// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build integration

package tests

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

// protocolAdapter abstracts the wire-level differences between the MCP and A2A
// gateway protocols so that payload-variation and error-case tests can be
// written once as table-driven cases and exercised against both protocols.
//
// The interface is intentionally minimal (three methods): it returns the
// canonical endpoint path, builds a call request body for a given tool/skill
// name and payload, and extracts the suspended-status signal from the
// response body. New cross-protocol payload/error tests extend the shared
// tables in protocol_payload_test.go and protocol_errors_test.go rather than
// adding per-protocol test functions.
type protocolAdapter interface {
	// name is the short identifier used in subtest names ("mcp", "a2a").
	name() string
	// endpoint returns the canonical API path the adapter posts to.
	endpoint() string
	// callMethod returns the JSON-RPC method used to invoke a tool/skill
	// ("tools/call" for MCP, "a2a/call" for A2A). Used by the error-case
	// tests to build malformed request bodies.
	callMethod() string
	// nameParamKey returns the JSON params field holding the tool/skill
	// name ("name" for MCP, "skill_name" for A2A).
	nameParamKey() string
	// payloadParamKey returns the JSON params field holding the payload
	// ("arguments" for MCP, "payload" for A2A).
	payloadParamKey() string
	// makeCallBody builds a JSON-RPC request body that invokes the named
	// tool/skill with the supplied payload. payload is any here because the
	// payload-variation tests deliberately exercise arbitrary shapes (nested,
	// unicode, large, empty, null); this is the documented exception to the
	// "no Any types for known shapes" rule in devs.md.
	makeCallBody(name string, payload any) []byte
	// parseSuspendedStatus extracts the suspended-status signal from the
	// response body and returns it for assertion against the typed
	// constants.GatewayResponseStatusSuspended constant. For MCP the signal
	// is the first content text; for A2A it is the result.status field.
	parseSuspendedStatus(t *testing.T, body []byte) string
}

// mcpAdapter implements protocolAdapter for the MCP protocol. MCP tool calls
// return a CallToolResult whose first TextContent starts with
// constants.MCPApprovalPausedPrefix when the transaction is suspended.
type mcpAdapter struct{}

func (mcpAdapter) name() string { return "mcp" }

func (mcpAdapter) endpoint() string { return constants.APIPaths.MCPEndpoint }

func (mcpAdapter) callMethod() string      { return "tools/call" }
func (mcpAdapter) nameParamKey() string    { return "name" }
func (mcpAdapter) payloadParamKey() string { return "arguments" }

func (mcpAdapter) makeCallBody(name string, payload any) []byte {
	callReq := mcp.JSONRPCRequest{
		JSONRPC: constants.JSONRPCVersion,
		Method:  "tools/call",
		ID:      1,
	}
	params := mcp.CallToolRequest{
		Name:      name,
		Arguments: mustMarshal(payload),
	}
	callReq.Params = mustMarshal(params)
	body, err := json.Marshal(callReq)
	if err != nil {
		panic(fmt.Sprintf("mcpAdapter: marshal call body: %v", err))
	}
	return body
}

func (mcpAdapter) parseSuspendedStatus(t *testing.T, body []byte) string {
	var res struct {
		Result struct {
			Content []mcp.TextContent `json:"content"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(body, &res))
	require.NotEmpty(t, res.Result.Content, "MCP response should have content")
	return res.Result.Content[0].Text
}

// a2aAdapter implements protocolAdapter for the A2A protocol. A2A skill calls
// return an A2ASuspensionResponse whose result.status field equals
// constants.GatewayResponseStatusSuspended when the transaction is suspended.
type a2aAdapter struct{}

func (a2aAdapter) name() string { return "a2a" }

func (a2aAdapter) endpoint() string { return constants.APIPaths.A2ACall }

func (a2aAdapter) callMethod() string      { return "a2a/call" }
func (a2aAdapter) nameParamKey() string    { return "skill_name" }
func (a2aAdapter) payloadParamKey() string { return "payload" }

func (a2aAdapter) makeCallBody(name string, payload any) []byte {
	callReq := map[string]interface{}{
		"jsonrpc": constants.JSONRPCVersion,
		"method":  "a2a/call",
		"id":      1,
		"params": map[string]interface{}{
			"skill_name": name,
			"payload":    payload,
		},
	}
	body, err := json.Marshal(callReq)
	if err != nil {
		panic(fmt.Sprintf("a2aAdapter: marshal call body: %v", err))
	}
	return body
}

func (a2aAdapter) parseSuspendedStatus(t *testing.T, body []byte) string {
	var res struct {
		Result struct {
			Status string `json:"status"`
		} `json:"result"`
	}
	require.NoError(t, json.Unmarshal(body, &res))
	return res.Result.Status
}

// bothAdapters returns the two protocol adapters used by the consolidated
// cross-protocol tests. New protocols are added here.
func bothAdapters() []protocolAdapter {
	return []protocolAdapter{mcpAdapter{}, a2aAdapter{}}
}

// adapterFixture bundles a gateway fixture with the per-adapter enrolled
// identity and mTLS client. The MCP and A2A protocols share a single gateway
// fixture per test (the gateway serves both endpoints), but each protocol
// enrolls its own client identity to keep the test isolation clear.
type adapterFixture struct {
	fixture  *fixtures.GatewayFixture
	mtlsURL  string
	identity *fixtures.ClientIdentity
	client   *http.Client
}

// newAdapterFixture creates a gateway fixture configured for both MCP and A2A
// downstream servers. The downstreamURL is used for both protocols; pass an
// empty string to use the fixture's default mock downstream.
func newAdapterFixture(t *testing.T, testName, downstreamURL string) adapterFixture {
	t.Helper()
	opts := fixtures.GatewayFixtureOptions{
		TestName:          testName,
		A2ADownstreamURL:  downstreamURL,
		AllowTestPortZero: true,
	}
	f := fixtures.NewGatewayFixture(t, opts)
	f.WaitForReady(t)
	identity := fixtures.EnrollClientIdentity(t, f, testName+"-user", testName+"-org", testName+"-fp", testName+"-host")
	client := fixtures.CreateMTLSClient(t, f, identity)
	return adapterFixture{
		fixture:  f,
		mtlsURL:  network.LocalhostHTTPSURL(f.Service.GetHTTPSPort()),
		identity: identity,
		client:   client,
	}
}

// postAdapter sends a raw request body to the adapter's endpoint with the
// enrolled identity's bearer token and returns the response body. Used by the
// error-case tests that need to send malformed/raw bodies.
func (af adapterFixture) postAdapter(t *testing.T, adapter protocolAdapter, body []byte) []byte {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, af.mtlsURL+adapter.endpoint(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+af.identity.OperatorSessionID)
	resp, err := af.client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return out
}

// postAdapterWithStatus is postAdapter that also returns the HTTP status code,
// for tests that assert on the status code.
func (af adapterFixture) postAdapterWithStatus(t *testing.T, adapter protocolAdapter, body []byte) (int, []byte) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, af.mtlsURL+adapter.endpoint(), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+af.identity.OperatorSessionID)
	resp, err := af.client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	out, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, out
}
