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
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcSignalSafeTool_Metadata(t *testing.T) {
	tool := &ProcSignalSafeTool{}
	assert.Equal(t, "proc_signal_safe", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.Contains(t, tool.Description(), "signal")

	schema := tool.InputSchema()
	assert.Equal(t, "object", schema.Type)
	assert.Contains(t, schema.Required, "pid")
	assert.Contains(t, schema.Required, "signal")
	assert.Equal(t, "integer", schema.Properties["pid"].Type)
	assert.Equal(t, "string", schema.Properties["signal"].Type)
}

func TestProcSignalSafeTool_Execute_InvalidArgs(t *testing.T) {
	tool := &ProcSignalSafeTool{}
	ctx := context.Background()

	t.Run("invalid json", func(t *testing.T) {
		_, err := tool.Execute(ctx, json.RawMessage(`{invalid`))
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid arguments")
	})

	t.Run("missing pid", func(t *testing.T) {
		args := json.RawMessage(`{"signal": "SIGTERM"}`)
		_, err := tool.Execute(ctx, args)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "pid and signal required")
	})

	t.Run("missing signal", func(t *testing.T) {
		args := json.RawMessage(`{"pid": 1234}`)
		_, err := tool.Execute(ctx, args)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "pid and signal required")
	})

	t.Run("invalid pid", func(t *testing.T) {
		args := json.RawMessage(`{"pid": -1, "signal": "SIGTERM"}`)
		_, err := tool.Execute(ctx, args)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "pid and signal required")
	})
}

func TestProcSignalSafeTool_Execute_Denylist(t *testing.T) {
	tool := &ProcSignalSafeTool{}
	ctx := context.Background()

	protectedPIDs := []int{1, 2}
	for _, pid := range protectedPIDs {
		args := json.RawMessage(string(mustMarshal(t, map[string]interface{}{
			"pid":    pid,
			"signal": "SIGTERM",
		})))
		result, err := tool.Execute(ctx, args)
		require.NoError(t, err)
		require.Len(t, result.Content, 1)

		var res ProcSignalSafeResult
		err = json.Unmarshal([]byte(result.Content[0].Text), &res)
		require.NoError(t, err)
		assert.False(t, res.Sent)
		assert.Equal(t, pid, res.PID)
		assert.Contains(t, res.Error, "protected")
	}
}

func TestProcSignalSafeTool_Execute_UnsupportedSignal(t *testing.T) {
	tool := &ProcSignalSafeTool{}
	ctx := context.Background()

	args := json.RawMessage(`{"pid": 99999, "signal": "SIGINVALID"}`)
	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var res ProcSignalSafeResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &res)
	require.NoError(t, err)
	assert.False(t, res.Sent)
	assert.Equal(t, "SIGINVALID", res.Signal)
	assert.Contains(t, res.Error, "unsupported signal")
}

func mustMarshal(t *testing.T, v interface{}) []byte {
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}
