//go:build integration
// +build integration

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
	"os/exec"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcSignalSafeTool_Execute_Integration(t *testing.T) {
	tool := &ProcSignalSafeTool{}
	ctx := context.Background()

	// Start a long-running subprocess
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("timeout", "10")
	} else {
		cmd = exec.Command("sleep", "10")
	}

	err := cmd.Start()
	require.NoError(t, err)
	defer func() {
		_ = cmd.Process.Kill()
	}()

	pid := cmd.Process.Pid

	t.Run("SIGTERM", func(t *testing.T) {
		args := json.RawMessage(string(mustMarshal(t, map[string]interface{}{
			"pid":    pid,
			"signal": "SIGTERM",
		})))
		result, err := tool.Execute(ctx, args)
		require.NoError(t, err)

		var res ProcSignalSafeResult
		err = json.Unmarshal([]byte(result.Content[0].Text), &res)
		require.NoError(t, err)
		assert.True(t, res.Sent)
		assert.Equal(t, pid, res.PID)
		assert.Empty(t, res.Error)

		// Give it a moment to exit
		time.Sleep(100 * time.Millisecond)
	})

	t.Run("SIGINT", func(t *testing.T) {
		// Start a new process
		var cmd2 *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd2 = exec.Command("timeout", "10")
		} else {
			cmd2 = exec.Command("sleep", "10")
		}
		err := cmd2.Start()
		require.NoError(t, err)
		defer func() { _ = cmd2.Process.Kill() }()
		pid2 := cmd2.Process.Pid

		args := json.RawMessage(string(mustMarshal(t, map[string]interface{}{
			"pid":    pid2,
			"signal": "SIGINT",
		})))
		result, err := tool.Execute(ctx, args)
		require.NoError(t, err)

		var res ProcSignalSafeResult
		err = json.Unmarshal([]byte(result.Content[0].Text), &res)
		require.NoError(t, err)
		assert.True(t, res.Sent)
	})

	t.Run("SIGKILL", func(t *testing.T) {
		// Start a new process
		var cmd3 *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd3 = exec.Command("timeout", "10")
		} else {
			cmd3 = exec.Command("sleep", "10")
		}
		err := cmd3.Start()
		require.NoError(t, err)
		defer func() { _ = cmd3.Process.Kill() }()
		pid3 := cmd3.Process.Pid

		args := json.RawMessage(string(mustMarshal(t, map[string]interface{}{
			"pid":    pid3,
			"signal": "SIGKILL",
		})))
		result, err := tool.Execute(ctx, args)
		require.NoError(t, err)

		var res ProcSignalSafeResult
		err = json.Unmarshal([]byte(result.Content[0].Text), &res)
		require.NoError(t, err)
		assert.True(t, res.Sent)
	})

	t.Run("non-existent PID", func(t *testing.T) {
		// Use a very large PID that likely doesn't exist
		nonExistentPID := 999999
		args := json.RawMessage(string(mustMarshal(t, map[string]interface{}{
			"pid":    nonExistentPID,
			"signal": "SIGTERM",
		})))
		result, err := tool.Execute(ctx, args)
		require.NoError(t, err)

		var res ProcSignalSafeResult
		err = json.Unmarshal([]byte(result.Content[0].Text), &res)
		require.NoError(t, err)
		// On some OSes, FindProcess doesn't fail, but Signal might.
		// If it failed, res.Sent should be false.
		if !res.Sent {
			assert.NotEmpty(t, res.Error)
		}
	})
}
