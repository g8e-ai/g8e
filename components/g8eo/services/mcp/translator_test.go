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

package mcp

import (
	"encoding/json"
	"testing"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
	"github.com/stretchr/testify/assert"
)

func TestMCPToolNameToEventType(t *testing.T) {
	assert.Equal(t, constants.Event.Operator.Command.Requested, MCPToolNameToEventType("run_commands_with_operator"))
	assert.Equal(t, "", MCPToolNameToEventType("unknown"))
}

func TestTranslateToolCallToCommand(t *testing.T) {
	// 1. Valid request
	req := &JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      "test-id",
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"run_commands_with_operator","arguments":{"command":"ls -la"}}`),
	}
	g8eMsg, err := TranslateToolCallToCommand(req)
	assert.NoError(t, err)
	assert.Equal(t, "test-id", g8eMsg.ID)
	assert.Equal(t, constants.Event.Operator.Command.Requested, g8eMsg.EventType)
	assert.Contains(t, string(g8eMsg.Payload), "ls -la")

	// 2. Unsupported method
	req.Method = "unknown"
	_, err = TranslateToolCallToCommand(req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "method not found")

	// 3. Unsupported tool
	req.Method = "tools/call"
	req.Params = json.RawMessage(`{"name":"unknown","arguments":{}}`)
	_, err = TranslateToolCallToCommand(req)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported tool")
}

func TestTranslateResultToMCP(t *testing.T) {
	requestID := "mcp-req-123"

	t.Run("ExecutionResultsPayload", func(t *testing.T) {
		payload := models.ExecutionResultsPayload{
			ExecutionID: "mcp-req-123",
			Stdout:      "Hello MCP",
			Stderr:      "",
		}

		resp, err := TranslateResultToMCP(requestID, payload.ExecutionID, constants.Event.Operator.Command.Completed, payload)
		assert.NoError(t, err)
		assert.Equal(t, "2.0", resp.JSONRPC)
		assert.Equal(t, requestID, resp.ID)

		var result CallToolResult
		err = json.Unmarshal(resp.Result, &result)
		assert.NoError(t, err)
		assert.Len(t, result.Content, 1)
		assert.Equal(t, "text", result.Content[0].Type)
		assert.Equal(t, "Hello MCP", result.Content[0].Text)
		// Metadata should be nil for ExecutionResultsPayload (standard output)
		assert.Nil(t, result.Metadata)
	})

	t.Run("PortCheckResultPayload", func(t *testing.T) {
		payload := models.PortCheckResultPayload{
			ExecutionID: "mcp-req-123",
			Results: []models.PortCheckEntry{
				{Host: "localhost", Port: 80, Open: true},
			},
		}
		eventType := constants.Event.Operator.PortCheck.Completed
		resp, err := TranslateResultToMCP(requestID, payload.ExecutionID, eventType, payload)
		assert.NoError(t, err)

		var result CallToolResult
		err = json.Unmarshal(resp.Result, &result)
		assert.NoError(t, err)

		assert.NotNil(t, result.Metadata)
		assert.Equal(t, eventType, result.Metadata.EventType)
		assert.Equal(t, payload.ExecutionID, result.Metadata.ExecutionID)

		originalPayload := result.Metadata.OriginalPayload.(map[string]interface{})
		results := originalPayload["results"].([]interface{})
		assert.Len(t, results, 1)
		entry := results[0].(map[string]interface{})
		assert.Equal(t, "localhost", entry["host"])
		assert.Equal(t, float64(80), entry["port"])
		assert.Equal(t, true, entry["open"])
	})

	t.Run("FsListResultPayload", func(t *testing.T) {
		payload := models.FsListResultPayload{
			ExecutionID: "mcp-req-123",
			Entries: []models.FsListEntry{
				{Name: "file.txt", IsDir: false, Size: 123},
			},
		}
		eventType := constants.Event.Operator.FsList.Completed
		resp, err := TranslateResultToMCP(requestID, payload.ExecutionID, eventType, payload)
		assert.NoError(t, err)

		var result CallToolResult
		err = json.Unmarshal(resp.Result, &result)
		assert.NoError(t, err)

		assert.NotNil(t, result.Metadata)
		assert.Equal(t, eventType, result.Metadata.EventType)
		assert.Equal(t, payload.ExecutionID, result.Metadata.ExecutionID)

		originalPayload := result.Metadata.OriginalPayload.(map[string]interface{})
		entries := originalPayload["entries"].([]interface{})
		assert.Len(t, entries, 1)
		entry := entries[0].(map[string]interface{})
		assert.Equal(t, "file.txt", entry["name"])
		assert.Equal(t, false, entry["is_dir"])
	})
}
