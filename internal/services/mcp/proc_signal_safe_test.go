// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
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
		assert.True(t, errors.Is(err, constants.ErrMCPUnmarshalArguments))
	})

	t.Run("missing pid", func(t *testing.T) {
		args := json.RawMessage(`{"signal": "SIGTERM"}`)
		_, err := tool.Execute(ctx, args)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, constants.ErrMCPProcSignalRequired))
	})

	t.Run("missing signal", func(t *testing.T) {
		args := json.RawMessage(`{"pid": 1234}`)
		_, err := tool.Execute(ctx, args)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, constants.ErrMCPProcSignalRequired))
	})

	t.Run("invalid pid", func(t *testing.T) {
		args := json.RawMessage(`{"pid": -1, "signal": "SIGTERM"}`)
		_, err := tool.Execute(ctx, args)
		assert.Error(t, err)
		assert.True(t, errors.Is(err, constants.ErrMCPProcSignalRequired))
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
