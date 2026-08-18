// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package response

import (
	"encoding/json"
	"testing"
)

// FuzzJSONRPCRequestDecoding tests JSON-RPC request decoding with random inputs
// to catch edge-case panics and JSON parsing errors.
func FuzzJSONRPCRequestDecoding(f *testing.F) {
	// Add seed corpus with valid and edge-case inputs
	f.Add(`{"jsonrpc":"2.0","method":"tools/call","params":{"name":"test"},"id":1}`)
	f.Add(`{"jsonrpc":"2.0","method":"tools/list","id":2}`)
	f.Add(`{"jsonrpc":"2.0","method":"unknown","params":{},"id":3}`)
	f.Add(`{"jsonrpc":"1.0","method":"test","id":4}`)
	f.Add(`{"method":"test","id":5}`)
	f.Add(`{"jsonrpc":"2.0","id":6}`)
	f.Add(`invalid json`)
	f.Add(``)
	f.Add(`{"jsonrpc":"2.0","method":"test","params":"not an object","id":7}`)
	f.Add(`{"jsonrpc":"2.0","method":"test","params":null,"id":8}`)
	f.Add(`{"jsonrpc":"2.0","method":"test","params":[1,2,3],"id":9}`)
	f.Add(`{"jsonrpc":"2.0","method":"test","params":{"nested":{"deep":"value"}},"id":10}`)
	f.Add(`{"jsonrpc":"2.0","method":"test","params":"` + string(make([]byte, 10000)) + `","id":11}`)
	f.Add(`{"jsonrpc":"2.0","method":"test","id":null}`)
	f.Add(`{"jsonrpc":"2.0","method":"test","id":0}`)
	f.Add(`{"jsonrpc":"2.0","method":"test","id":-1}`)
	f.Add(`{"jsonrpc":"2.0","method":"test","id":"string-id"}`)

	f.Fuzz(func(t *testing.T, data string) {
		// This should never panic - JSON decoding must handle all inputs gracefully
		var req JSONRPCRequest
		_ = json.Unmarshal([]byte(data), &req)
	})
}

// FuzzJSONRPCResponseDecoding tests JSON-RPC response decoding with random inputs
// to catch edge-case panics and JSON parsing errors.
func FuzzJSONRPCResponseDecoding(f *testing.F) {
	// Add seed corpus with valid and edge-case inputs
	f.Add(`{"jsonrpc":"2.0","result":{"content":"test"},"id":1}`)
	f.Add(`{"jsonrpc":"2.0","error":{"code":-32600,"message":"Invalid request"},"id":2}`)
	f.Add(`{"jsonrpc":"2.0","result":null,"id":3}`)
	f.Add(`{"jsonrpc":"2.0","error":null,"id":4}`)
	f.Add(`{"jsonrpc":"2.0","result":{},"id":5}`)
	f.Add(`{"jsonrpc":"2.0","error":{},"id":6}`)
	f.Add(`{"jsonrpc":"2.0","result":[1,2,3],"id":7}`)
	f.Add(`{"jsonrpc":"2.0","error":{"code":-32700,"message":"Parse error","data":"extra"},"id":8}`)
	f.Add(`{"jsonrpc":"2.0","id":9}`)
	f.Add(`{"jsonrpc":"2.0","result":"test","error":null,"id":10}`)
	f.Add(`{"jsonrpc":"2.0","result":"test","error":{"code":-32000},"id":11}`)
	f.Add(`invalid json`)
	f.Add(``)
	f.Add(`{"jsonrpc":"2.0","result":"` + string(make([]byte, 10000)) + `","id":12}`)

	f.Fuzz(func(t *testing.T, data string) {
		// This should never panic - JSON decoding must handle all inputs gracefully
		var resp JSONRPCResponse
		_ = json.Unmarshal([]byte(data), &resp)
	})
}
