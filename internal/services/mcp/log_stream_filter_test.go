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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestLogStreamFilterTool_Name(t *testing.T) {
	tool := &LogStreamFilterTool{}
	require.Equal(t, "log_stream_filter", tool.Name())
}

func TestLogStreamFilterTool_Description(t *testing.T) {
	tool := &LogStreamFilterTool{}
	require.NotEmpty(t, tool.Description())
	require.Contains(t, tool.Description(), "log")
	require.Contains(t, tool.Description(), "filter")
}

func TestLogStreamFilterTool_InputSchema(t *testing.T) {
	tool := &LogStreamFilterTool{}
	schema := tool.InputSchema()

	require.Equal(t, "object", schema.Type)
	require.Contains(t, schema.Required, "log_path")
	require.Contains(t, schema.Required, "pattern")

	properties := schema.Properties
	require.Contains(t, properties, "log_path")
	require.Contains(t, properties, "pattern")
	require.Contains(t, properties, "limit")

	require.Equal(t, "string", properties["log_path"].Type)
	require.Equal(t, "string", properties["pattern"].Type)
	require.Equal(t, "integer", properties["limit"].Type)
}

func TestLogStreamFilterTool_Execute_InvalidJSON(t *testing.T) {
	tool := &LogStreamFilterTool{}
	ctx := context.Background()

	_, err := tool.Execute(ctx, []byte("invalid json"))
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid arguments")
}

func TestLogStreamFilterTool_Execute_MissingRequiredFields(t *testing.T) {
	tool := &LogStreamFilterTool{}
	ctx := context.Background()

	tests := []struct {
		name    string
		req     LogStreamFilterRequest
		wantErr string
	}{
		{
			name: "missing log_path",
			req: LogStreamFilterRequest{
				Pattern: "error",
			},
			wantErr: "log_path and pattern required",
		},
		{
			name: "missing pattern",
			req: LogStreamFilterRequest{
				LogPath: "/tmp/test.log",
			},
			wantErr: "log_path and pattern required",
		},
		{
			name: "both empty",
			req: LogStreamFilterRequest{
				LogPath: "",
				Pattern: "",
			},
			wantErr: "log_path and pattern required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			args, err := json.Marshal(tt.req)
			require.NoError(t, err)

			_, err = tool.Execute(ctx, args)
			require.Error(t, err)
			require.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestLogStreamFilterTool_Execute_PathTraversal(t *testing.T) {
	tool := &LogStreamFilterTool{}
	ctx := context.Background()

	req := LogStreamFilterRequest{
		LogPath: "../../../etc/passwd",
		Pattern: "test",
	}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	_, err = tool.Execute(ctx, args)
	require.Error(t, err)
	require.Contains(t, err.Error(), "path traversal")
}

func TestLogStreamFilterTool_Execute_FileNotFound(t *testing.T) {
	tool := &LogStreamFilterTool{}
	ctx := context.Background()

	req := LogStreamFilterRequest{
		LogPath: "/tmp/non-existent-log-file-12345.log",
		Pattern: "error",
	}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	_, err = tool.Execute(ctx, args)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to open log file")
}

func TestLogStreamFilterTool_Execute_InvalidRegex(t *testing.T) {
	tool := &LogStreamFilterTool{}
	ctx := context.Background()

	// Create a temporary log file
	tmpDir := testutil.TempDir(t)
	logFile := filepath.Join(tmpDir, "test.log")
	err := os.WriteFile(logFile, []byte("test line\n"), 0644)
	require.NoError(t, err)

	req := LogStreamFilterRequest{
		LogPath: logFile,
		Pattern: "[invalid-regex",
	}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	_, err = tool.Execute(ctx, args)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid regex pattern")
}

func TestLogStreamFilterTool_Execute_Success(t *testing.T) {
	tool := &LogStreamFilterTool{}
	ctx := context.Background()

	// Create a temporary log file with sample content
	tmpDir := testutil.TempDir(t)
	logFile := filepath.Join(tmpDir, "test.log")
	content := `2024-01-01 10:00:00 INFO Application started
2024-01-01 10:00:01 ERROR Failed to connect to database
2024-01-01 10:00:02 INFO Retrying connection
2024-01-01 10:00:03 ERROR Connection timeout
2024-01-01 10:00:04 INFO Connection established
`
	err := os.WriteFile(logFile, []byte(content), 0644)
	require.NoError(t, err)

	req := LogStreamFilterRequest{
		LogPath: logFile,
		Pattern: "ERROR",
	}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var filterResult LogStreamFilterResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &filterResult)
	require.NoError(t, err)

	require.Equal(t, 2, filterResult.Count)
	require.Len(t, filterResult.Lines, 2)
	require.Contains(t, filterResult.Lines[0], "ERROR")
	require.Contains(t, filterResult.Lines[1], "ERROR")
}

func TestLogStreamFilterTool_Execute_NoMatches(t *testing.T) {
	tool := &LogStreamFilterTool{}
	ctx := context.Background()

	// Create a temporary log file
	tmpDir := testutil.TempDir(t)
	logFile := filepath.Join(tmpDir, "test.log")
	content := `2024-01-01 10:00:00 INFO Application started
2024-01-01 10:00:01 INFO Processing request
`
	err := os.WriteFile(logFile, []byte(content), 0644)
	require.NoError(t, err)

	req := LogStreamFilterRequest{
		LogPath: logFile,
		Pattern: "ERROR",
	}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var filterResult LogStreamFilterResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &filterResult)
	require.NoError(t, err)

	require.Equal(t, 0, filterResult.Count)
	require.Empty(t, filterResult.Lines)
}

func TestLogStreamFilterTool_Execute_Limit(t *testing.T) {
	tool := &LogStreamFilterTool{}
	ctx := context.Background()

	// Create a temporary log file with many matching lines
	tmpDir := testutil.TempDir(t)
	logFile := filepath.Join(tmpDir, "test.log")
	var content strings.Builder
	for i := 0; i < 50; i++ {
		content.WriteString("2024-01-01 10:00:0")
		content.WriteString(string(rune('0' + i%10)))
		content.WriteString(" ERROR Test error ")
		content.WriteString(string(rune('0' + i)))
		content.WriteString("\n")
	}
	err := os.WriteFile(logFile, []byte(content.String()), 0644)
	require.NoError(t, err)

	tests := []struct {
		name          string
		limit         int
		expectedCount int
	}{
		{
			name:          "limit 5",
			limit:         5,
			expectedCount: 5,
		},
		{
			name:          "limit 0 (use default)",
			limit:         0,
			expectedCount: 50, // All lines since default is 100
		},
		{
			name:          "limit negative (use default)",
			limit:         -1,
			expectedCount: 50,
		},
		{
			name:          "limit greater than matches",
			limit:         100,
			expectedCount: 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := LogStreamFilterRequest{
				LogPath: logFile,
				Pattern: "ERROR",
				Limit:   tt.limit,
			}
			args, err := json.Marshal(req)
			require.NoError(t, err)

			result, err := tool.Execute(ctx, args)
			require.NoError(t, err)

			var filterResult LogStreamFilterResult
			err = json.Unmarshal([]byte(result.Content[0].Text), &filterResult)
			require.NoError(t, err)

			require.Equal(t, tt.expectedCount, filterResult.Count)
		})
	}
}

func TestLogStreamFilterTool_Execute_ContextCancellation(t *testing.T) {
	tool := &LogStreamFilterTool{}

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Create a temporary log file
	tmpDir := testutil.TempDir(t)
	logFile := filepath.Join(tmpDir, "test.log")
	content := `2024-01-01 10:00:00 INFO Application started
2024-01-01 10:00:01 ERROR Failed to connect
`
	err := os.WriteFile(logFile, []byte(content), 0644)
	require.NoError(t, err)

	req := LogStreamFilterRequest{
		LogPath: logFile,
		Pattern: "ERROR",
	}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	_, err = tool.Execute(ctx, args)
	require.Error(t, err)
	require.Equal(t, context.Canceled, err)
}

func TestLogStreamFilterTool_Execute_Scrubbing(t *testing.T) {
	tool := &LogStreamFilterTool{}
	ctx := context.Background()

	// Create a temporary log file with sensitive data
	tmpDir := testutil.TempDir(t)
	logFile := filepath.Join(tmpDir, "test.log")
	content := `2024-01-01 10:00:00 ERROR password=secret123 authentication failed
2024-01-01 10:00:01 ERROR api_key=abc123def456 rate limit exceeded
2024-01-01 10:00:02 ERROR secret=mysecret token expired
2024-01-01 10:00:03 ERROR bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9 access denied
`
	err := os.WriteFile(logFile, []byte(content), 0644)
	require.NoError(t, err)

	req := LogStreamFilterRequest{
		LogPath: logFile,
		Pattern: "ERROR",
	}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var filterResult LogStreamFilterResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &filterResult)
	require.NoError(t, err)

	require.Equal(t, 4, filterResult.Count)

	// Verify scrubbing occurred
	for _, line := range filterResult.Lines {
		require.Contains(t, line, "REDACTED")
		require.NotContains(t, line, "secret123")
		require.NotContains(t, line, "abc123def456")
		require.NotContains(t, line, "mysecret")
		require.NotContains(t, line, "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9")
	}
}

func TestLogStreamFilterTool_Execute_ComplexRegex(t *testing.T) {
	tool := &LogStreamFilterTool{}
	ctx := context.Background()

	// Create a temporary log file
	tmpDir := testutil.TempDir(t)
	logFile := filepath.Join(tmpDir, "test.log")
	content := `2024-01-01 10:00:00 [INFO] Application started
2024-01-01 10:00:01 [WARN] Memory usage high
2024-01-01 10:00:02 [ERROR] Database connection failed
2024-01-01 10:00:03 [DEBUG] Query executed
2024-01-01 10:00:04 [ERROR] Timeout occurred
`
	err := os.WriteFile(logFile, []byte(content), 0644)
	require.NoError(t, err)

	req := LogStreamFilterRequest{
		LogPath: logFile,
		Pattern: `\[ERROR\].*`,
	}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var filterResult LogStreamFilterResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &filterResult)
	require.NoError(t, err)

	require.Equal(t, 2, filterResult.Count)
	for _, line := range filterResult.Lines {
		require.Contains(t, line, "[ERROR]")
	}
}

func TestLogStreamFilterTool_Execute_CaseInsensitive(t *testing.T) {
	tool := &LogStreamFilterTool{}
	ctx := context.Background()

	// Create a temporary log file with mixed case
	tmpDir := testutil.TempDir(t)
	logFile := filepath.Join(tmpDir, "test.log")
	content := `2024-01-01 10:00:00 error Application started
2024-01-01 10:00:01 ERROR Database failed
2024-01-01 10:00:02 Error Timeout
2024-01-01 10:00:03 info Processing
`
	err := os.WriteFile(logFile, []byte(content), 0644)
	require.NoError(t, err)

	req := LogStreamFilterRequest{
		LogPath: logFile,
		Pattern: "(?i)error",
	}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var filterResult LogStreamFilterResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &filterResult)
	require.NoError(t, err)

	require.Equal(t, 3, filterResult.Count)
}

func TestLogStreamFilterTool_Execute_EmptyFile(t *testing.T) {
	tool := &LogStreamFilterTool{}
	ctx := context.Background()

	// Create an empty temporary log file
	tmpDir := testutil.TempDir(t)
	logFile := filepath.Join(tmpDir, "test.log")
	err := os.WriteFile(logFile, []byte(""), 0644)
	require.NoError(t, err)

	req := LogStreamFilterRequest{
		LogPath: logFile,
		Pattern: "ERROR",
	}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var filterResult LogStreamFilterResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &filterResult)
	require.NoError(t, err)

	require.Equal(t, 0, filterResult.Count)
	require.Empty(t, filterResult.Lines)
}

func TestLogStreamFilterTool_Execute_AbsolutePath(t *testing.T) {
	tool := &LogStreamFilterTool{}
	ctx := context.Background()

	// Create a temporary log file
	tmpDir := testutil.TempDir(t)
	logFile := filepath.Join(tmpDir, "test.log")
	content := `2024-01-01 10:00:00 ERROR Test error
`
	err := os.WriteFile(logFile, []byte(content), 0644)
	require.NoError(t, err)

	// Use absolute path
	absPath, err := filepath.Abs(logFile)
	require.NoError(t, err)

	req := LogStreamFilterRequest{
		LogPath: absPath,
		Pattern: "ERROR",
	}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var filterResult LogStreamFilterResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &filterResult)
	require.NoError(t, err)

	require.Equal(t, 1, filterResult.Count)
}
