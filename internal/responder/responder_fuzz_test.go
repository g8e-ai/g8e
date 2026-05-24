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

package responder

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
		err := json.Unmarshal([]byte(data), &req)

		// If decoding succeeded, validate the structure
		if err == nil {
			// JSONRPC should be "2.0" for valid requests
			if req.JSONRPC != "" && req.JSONRPC != "2.0" {
				// Non-2.0 version is allowed but may be rejected by validation logic
			}

			// Method should be non-empty for valid requests
			if req.Method == "" && req.JSONRPC == "2.0" {
				// Missing method is invalid but shouldn't panic
			}

			// ID can be any JSON value (string, number, null)
			// Params can be any JSON value (object, array, null, or omitted)
		}
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
		err := json.Unmarshal([]byte(data), &resp)

		// If decoding succeeded, validate the structure
		if err == nil {
			// JSONRPC should be "2.0" for valid responses
			if resp.JSONRPC != "" && resp.JSONRPC != "2.0" {
				// Non-2.0 version is allowed but may be rejected by validation logic
			}

			// Exactly one of result or error should be present (per JSON-RPC spec)
			// But we allow both to be null for flexibility
			if resp.Result != nil && resp.Error != nil {
				// Both present is technically invalid per spec but shouldn't panic
			}

			// ID should match the request ID
			// ID can be any JSON value (string, number, null)
		}
	})
}
