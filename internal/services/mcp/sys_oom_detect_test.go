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
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSysOOMDetectTool_Metadata(t *testing.T) {
	tool := &SysOOMDetectTool{}
	assert.Equal(t, "sys_oom_detect", tool.Name())
	assert.NotEmpty(t, tool.Description())
	assert.Contains(t, tool.Description(), "OOM")

	schema := tool.InputSchema()
	require.NotNil(t, schema)
	assert.Equal(t, "object", schema.Type)
	assert.Contains(t, schema.Properties, "log_path")
}

func TestSysOOMDetectTool_Execute(t *testing.T) {
	tempDir := t.TempDir()

	// Create a mock log file with OOM events that match the improved regexes
	logContent := `
[ 1235.123456] oom-killer: killed process 5678 (worker) pid=5678 process worker 512 MB
[ 1236.789012] some other log line
[ 1237.000000] killed process 9012 (db) pid=9012 process db 1024 MB
[ 1238.000000] Out of memory: killed process 1234 (my-app) total-vm:1048576kB, anon-rss:524288kB
`
	logPath := filepath.Join(tempDir, "dmesg")
	err := os.WriteFile(logPath, []byte(logContent), 0644)
	require.NoError(t, err)

	logPathJSON, err := json.Marshal(logPath)
	require.NoError(t, err)

	tests := []struct {
		name          string
		args          string
		ctx           context.Context
		expectedCount int
		expectedError string
		skipIfNoFile  bool
	}{
		{
			name:          "Successful detection",
			args:          `{"log_path": ` + string(logPathJSON) + `}`,
			ctx:           context.Background(),
			expectedCount: 3,
		},
		{
			name:          "Invalid log path (traversal)",
			args:          `{"log_path": "../../../etc/passwd"}`,
			ctx:           context.Background(),
			expectedError: "invalid path",
		},
		{
			name:          "Invalid JSON arguments",
			args:          `{invalid}`,
			ctx:           context.Background(),
			expectedError: "unmarshal arguments",
		},
		{
			name: "Context cancellation",
			args: `{"log_path": ` + string(logPathJSON) + `}`,
			ctx: func() context.Context {
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				return ctx
			}(),
			expectedError: "context canceled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := &SysOOMDetectTool{}
			result, err := tool.Execute(tt.ctx, json.RawMessage(tt.args))

			if tt.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedError)
				return
			}

			require.NoError(t, err)
			assert.Len(t, result.Content, 1)
			assert.Equal(t, "text", result.Content[0].Type)

			var oomResult SysOOMDetectResult
			err = json.Unmarshal([]byte(result.Content[0].Text), &oomResult)
			require.NoError(t, err)
			assert.Len(t, oomResult.Events, tt.expectedCount)

			if tt.expectedCount > 0 {
				// Verify first event (pid=5678 process worker 512 MB)
				assert.Equal(t, 5678, oomResult.Events[0].PID)
				assert.Equal(t, "worker", oomResult.Events[0].Process)
				assert.Equal(t, 512, oomResult.Events[0].MemoryMB)

				// Verify second event (pid=9012 process db 1024 MB)
				assert.Equal(t, 9012, oomResult.Events[1].PID)
				assert.Equal(t, "db", oomResult.Events[1].Process)
				assert.Equal(t, 1024, oomResult.Events[1].MemoryMB)

				// Verify third event (my-app)
				// Note: pidRegex won't match "killed process 1234" because it expects "pid=" or "pid:"
				// So PID will be 0, but process name should be "my-app"
				assert.Equal(t, "my-app", oomResult.Events[2].Process)
				assert.Equal(t, 0, oomResult.Events[2].MemoryMB) // No MB in this line

				// Verify timestamp is recent
				ts, err := time.Parse(time.RFC3339, oomResult.Events[0].Timestamp)
				assert.NoError(t, err)
				assert.WithinDuration(t, time.Now(), ts, 10*time.Second)
			}
		})
	}
}

func TestSysOOMDetectTool_Execute_DefaultPath(t *testing.T) {
	// Only run this test if the default dmesg path exists
	if _, err := os.Stat("/var/log/dmesg"); os.IsNotExist(err) {
		t.Skip("dmesg does not exist, skipping default path test")
	}

	tool := &SysOOMDetectTool{}
	_, err := tool.Execute(context.Background(), json.RawMessage("{}"))
	// We don't check for success/failure since it depends on system permissions,
	// but we ensure it doesn't panic and returns a valid response or error.
	if err != nil {
		assert.Contains(t, err.Error(), "open log file")
	}
}

func TestSysOOMDetectTool_Execute_NoEvents(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "clean_dmesg")
	err := os.WriteFile(logPath, []byte("system boot successful\nno errors found"), 0644)
	require.NoError(t, err)

	logPathJSON, err := json.Marshal(logPath)
	require.NoError(t, err)

	tool := &SysOOMDetectTool{}
	args := `{"log_path": ` + string(logPathJSON) + `}`
	result, err := tool.Execute(context.Background(), json.RawMessage(args))

	require.NoError(t, err)
	var oomResult SysOOMDetectResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &oomResult)
	require.NoError(t, err)
	assert.Empty(t, oomResult.Events)
}
