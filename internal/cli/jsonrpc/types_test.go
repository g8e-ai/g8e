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

package jsonrpc

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRequest_Validate(t *testing.T) {
	t.Run("valid request", func(t *testing.T) {
		req := Request{
			JSONRPC: "2.0",
			Method:  "tools/list",
			ID:      1,
		}
		err := req.Validate()
		assert.NoError(t, err)
	})

	t.Run("missing jsonrpc version", func(t *testing.T) {
		req := Request{
			Method: "tools/list",
			ID:     1,
		}
		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid jsonrpc version")
	})

	t.Run("invalid jsonrpc version", func(t *testing.T) {
		req := Request{
			JSONRPC: "1.0",
			Method:  "tools/list",
			ID:      1,
		}
		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "invalid jsonrpc version")
	})

	t.Run("missing method", func(t *testing.T) {
		req := Request{
			JSONRPC: "2.0",
			ID:      1,
		}
		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "method is required")
	})

	t.Run("missing id", func(t *testing.T) {
		req := Request{
			JSONRPC: "2.0",
			Method:  "tools/list",
		}
		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "id is required")
	})

	t.Run("nil id", func(t *testing.T) {
		req := Request{
			JSONRPC: "2.0",
			Method:  "tools/list",
			ID:      nil,
		}
		err := req.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "id is required")
	})
}

func TestRequest_Unmarshal(t *testing.T) {
	t.Run("valid tools/list request", func(t *testing.T) {
		jsonStr := `{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`
		var req Request
		err := json.Unmarshal([]byte(jsonStr), &req)
		require.NoError(t, err)
		assert.Equal(t, "2.0", req.JSONRPC)
		assert.Equal(t, "tools/list", req.Method)
		assert.InEpsilon(t, 1, req.ID, 0.0)
		assert.NotNil(t, req.Params)
	})

	t.Run("valid tools/call request with params", func(t *testing.T) {
		jsonStr := `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"test_tool","arguments":{}}}`
		var req Request
		err := json.Unmarshal([]byte(jsonStr), &req)
		require.NoError(t, err)
		assert.Equal(t, "tools/call", req.Method)
		assert.NotNil(t, req.Params)
	})

	t.Run("request with string id", func(t *testing.T) {
		jsonStr := `{"jsonrpc":"2.0","id":"req-1","method":"tools/list"}`
		var req Request
		err := json.Unmarshal([]byte(jsonStr), &req)
		require.NoError(t, err)
		assert.Equal(t, "req-1", req.ID)
	})

	t.Run("invalid JSON", func(t *testing.T) {
		jsonStr := `invalid json`
		var req Request
		err := json.Unmarshal([]byte(jsonStr), &req)
		assert.Error(t, err)
	})
}

func TestNewSuccessResponse(t *testing.T) {
	t.Run("success response with result", func(t *testing.T) {
		result := json.RawMessage(`{"tools":[]}`)
		resp := NewSuccessResponse(1, result)

		assert.Equal(t, "2.0", resp.JSONRPC)
		assert.Equal(t, 1, resp.ID)
		assert.Nil(t, resp.Error)
		assert.Equal(t, result, resp.Result)
	})

	t.Run("success response marshals correctly", func(t *testing.T) {
		result := json.RawMessage(`{"tools":[]}`)
		resp := NewSuccessResponse(1, result)

		bytes, err := json.Marshal(resp)
		require.NoError(t, err)

		var unmarshaled map[string]interface{}
		err = json.Unmarshal(bytes, &unmarshaled)
		require.NoError(t, err)

		assert.Equal(t, "2.0", unmarshaled["jsonrpc"])
		assert.InEpsilon(t, 1, unmarshaled["id"], 0.0)
		assert.Nil(t, unmarshaled["error"])
		assert.NotNil(t, unmarshaled["result"])
	})
}

func TestNewErrorResponse(t *testing.T) {
	t.Run("error response with data", func(t *testing.T) {
		data := json.RawMessage(`{"details":"test"}`)
		resp := NewErrorResponse(1, CodeMethodNotFound, "Method not found", data)

		assert.Equal(t, "2.0", resp.JSONRPC)
		assert.Equal(t, 1, resp.ID)
		assert.Nil(t, resp.Result)
		assert.NotNil(t, resp.Error)
		assert.Equal(t, CodeMethodNotFound, resp.Error.Code)
		assert.Equal(t, "Method not found", resp.Error.Message)
		assert.Equal(t, data, resp.Error.Data)
	})

	t.Run("error response without data", func(t *testing.T) {
		resp := NewErrorResponse(1, CodeInternalError, "Internal error", nil)

		assert.Nil(t, resp.Error.Data)
	})
}

func TestStandardErrorResponses(t *testing.T) {
	t.Run("parse error response", func(t *testing.T) {
		resp := NewParseErrorResponse(1, assert.AnError)
		assert.Equal(t, CodeParseError, resp.Error.Code)
		assert.Equal(t, "Parse error", resp.Error.Message)
	})

	t.Run("invalid request response", func(t *testing.T) {
		resp := NewInvalidRequestResponse(1, "missing method")
		assert.Equal(t, CodeInvalidRequest, resp.Error.Code)
		assert.Equal(t, "Invalid Request: missing method", resp.Error.Message)
	})

	t.Run("method not found response", func(t *testing.T) {
		resp := NewMethodNotFoundResponse(1, "unknown_method")
		assert.Equal(t, CodeMethodNotFound, resp.Error.Code)
		assert.Equal(t, "Method not found: unknown_method", resp.Error.Message)
	})

	t.Run("invalid params response", func(t *testing.T) {
		resp := NewInvalidParamsResponse(1, "missing required field")
		assert.Equal(t, CodeInvalidParams, resp.Error.Code)
		assert.Equal(t, "Invalid params: missing required field", resp.Error.Message)
	})

	t.Run("internal error response", func(t *testing.T) {
		resp := NewInternalErrorResponse(1, assert.AnError)
		assert.Equal(t, CodeInternalError, resp.Error.Code)
		assert.Contains(t, resp.Error.Message, assert.AnError.Error())
	})
}

func TestResponse_Marshal(t *testing.T) {
	t.Run("error response marshals correctly", func(t *testing.T) {
		resp := NewErrorResponse(1, CodeMethodNotFound, "Method not found", nil)

		bytes, err := json.Marshal(resp)
		require.NoError(t, err)

		var unmarshaled map[string]interface{}
		err = json.Unmarshal(bytes, &unmarshaled)
		require.NoError(t, err)

		assert.Equal(t, "2.0", unmarshaled["jsonrpc"])
		assert.InEpsilon(t, 1, unmarshaled["id"], 0.0)
		assert.Nil(t, unmarshaled["result"])
		assert.NotNil(t, unmarshaled["error"])

		errorObj := unmarshaled["error"].(map[string]interface{})
		assert.InEpsilon(t, CodeMethodNotFound, errorObj["code"], 0.0)
		assert.Equal(t, "Method not found", errorObj["message"])
	})
}

func TestGatewayErrorCodes(t *testing.T) {
	t.Run("gateway error codes are in reserved range", func(t *testing.T) {
		assert.LessOrEqual(t, CodeInvalidEnvelope, -32000)
		assert.GreaterOrEqual(t, CodeInvalidEnvelope, -32099)
		assert.LessOrEqual(t, CodeGatewayNotReady, -32000)
		assert.GreaterOrEqual(t, CodeGatewayNotReady, -32099)
	})

	t.Run("gateway error codes are unique", func(t *testing.T) {
		codes := map[int]bool{
			CodeInvalidEnvelope:     true,
			CodeHashMismatch:        true,
			CodeExpired:             true,
			CodeReplay:              true,
			CodeStateMismatch:       true,
			CodeL1ValidationFailed:  true,
			CodeL2SignatureInvalid:  true,
			CodeL3ProofInvalid:      true,
			CodePayloadDecodeFailed: true,
			CodeResourceNotFound:    true,
			CodeGatewayNotReady:     true,
		}
		assert.Len(t, codes, 11)
	})
}
