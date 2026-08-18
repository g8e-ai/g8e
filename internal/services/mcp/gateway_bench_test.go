// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package mcp

import (
	"encoding/json"
	"testing"

	"github.com/g8e-ai/g8e/internal/response"
)

// BenchmarkJSONRPCUnmarshal benchmarks JSON-RPC request parsing, which happens
// on every inbound MCP/A2A request through the gateway.
func BenchmarkJSONRPCUnmarshal(b *testing.B) {
	body := []byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"fs_read","arguments":{"path":"/etc/hosts"}}}`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var req JSONRPCRequest
		_ = json.Unmarshal(body, &req)
	}
}

// BenchmarkJSONRPCMarshal benchmarks JSON-RPC response serialization.
func BenchmarkJSONRPCMarshal(b *testing.B) {
	resp := response.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      1,
		Result:  json.RawMessage(`{"content":[{"type":"text","text":"127.0.0.1 localhost"}]}`),
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(resp)
	}
}

// BenchmarkCallToolRequestUnmarshal benchmarks the tool call params parsing,
// which is the inner unmarshal after the JSON-RPC envelope is decoded.
func BenchmarkCallToolRequestUnmarshal(b *testing.B) {
	params := json.RawMessage(`{"name":"fs_read","arguments":{"path":"/etc/hosts","offset":0,"limit":100}}`)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var req CallToolRequest
		_ = json.Unmarshal(params, &req)
	}
}

// BenchmarkCallToolResultMarshal benchmarks tool result serialization.
func BenchmarkCallToolResultMarshal(b *testing.B) {
	result := CallToolResult{
		Content: []TextContent{
			{Type: "text", Text: "127.0.0.1 localhost\n::1 localhost"},
		},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = json.Marshal(result)
	}
}
