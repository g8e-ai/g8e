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
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFileReadTool_Name(t *testing.T) {
	tool := &FileReadTool{}
	require.Equal(t, "read_file", tool.Name())
}

func TestFileReadTool_Description(t *testing.T) {
	tool := &FileReadTool{}
	require.NotEmpty(t, tool.Description())
	require.Contains(t, tool.Description(), "Reads")
	require.Contains(t, tool.Description(), "file")
}

func TestFileReadTool_InputSchema(t *testing.T) {
	tool := &FileReadTool{}
	schema := tool.InputSchema()

	require.Equal(t, "object", schema.Type)
	require.Contains(t, schema.Required, "path")

	properties := schema.Properties
	require.Contains(t, properties, "path")
	require.Equal(t, "string", properties["path"].Type)
	require.NotEmpty(t, properties["path"].Description)

	require.Contains(t, properties, "offset")
	require.Equal(t, "integer", properties["offset"].Type)
	require.NotEmpty(t, properties["offset"].Description)

	require.Contains(t, properties, "limit")
	require.Equal(t, "integer", properties["limit"].Type)
	require.NotEmpty(t, properties["limit"].Description)
}

func TestFileReadTool_Execute_InvalidJSON(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	_, err := tool.Execute(ctx, json.RawMessage(`{invalid json}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "unmarshal arguments")
}

func TestFileReadTool_Execute_EmptyPath(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	req := FileReadRequest{Path: ""}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	_, err = tool.Execute(ctx, args)
	require.Error(t, err)
	require.Contains(t, err.Error(), "path is required")
}

func TestFileReadTool_Execute_SensitiveSystemFile(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	dangerousPaths := []string{
		"/etc/shadow",
		"/etc/passwd",
		"/etc/gshadow",
		"/etc/sudoers",
	}

	for _, path := range dangerousPaths {
		t.Run(path, func(t *testing.T) {
			req := FileReadRequest{Path: path}
			args, err := json.Marshal(req)
			require.NoError(t, err)

			result, err := tool.Execute(ctx, args)
			require.NoError(t, err)
			require.Len(t, result.Content, 1)

			var fileResult FileReadResult
			err = json.Unmarshal([]byte(result.Content[0].Text), &fileResult)
			require.NoError(t, err)

			require.NotEmpty(t, fileResult.Error)
			// On Windows, the path resolves to D:\etc\shadow which doesn't exist,
			// so we get "failed to read file" instead of "access denied"
			// On Unix-like systems, we get "access denied"
			// Either error is acceptable as long as the file is not read
			isAccessDenied := strings.Contains(fileResult.Error, "access denied")
			isReadFailure := strings.Contains(fileResult.Error, "failed to read file")
			require.True(t, isAccessDenied || isReadFailure,
				"expected either access denied or read failure, got: %s", fileResult.Error)
		})
	}
}

func TestFileReadTool_Execute_SensitiveSSHKey(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	sensitiveKeys := []string{
		"id_rsa",
		"id_ed25519",
		"id_ecdsa",
	}

	for _, keyName := range sensitiveKeys {
		t.Run(keyName, func(t *testing.T) {
			keyPath := filepath.Join(tmpDir, keyName)
			err := os.WriteFile(keyPath, []byte("private key content"), 0600)
			require.NoError(t, err)

			req := FileReadRequest{Path: keyPath}
			args, err := json.Marshal(req)
			require.NoError(t, err)

			result, err := tool.Execute(ctx, args)
			require.NoError(t, err)
			require.Len(t, result.Content, 1)

			var fileResult FileReadResult
			err = json.Unmarshal([]byte(result.Content[0].Text), &fileResult)
			require.NoError(t, err)

			require.NotEmpty(t, fileResult.Error)
			require.Contains(t, fileResult.Error, "access denied")
			require.Contains(t, fileResult.Error, "sensitive file")
		})
	}
}

func TestFileReadTool_Execute_SafeSSHKeyWithExtension(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	safeKeyPath := filepath.Join(tmpDir, "id_rsa.pub")
	err := os.WriteFile(safeKeyPath, []byte("public key content"), 0644)
	require.NoError(t, err)

	req := FileReadRequest{Path: safeKeyPath}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var fileResult FileReadResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &fileResult)
	require.NoError(t, err)

	require.Empty(t, fileResult.Error)
	require.Equal(t, "public key content", fileResult.Content)
}

func TestFileReadTool_Execute_FileNotFound(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	req := FileReadRequest{Path: "/tmp/non-existent-file-12345.txt"}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var fileResult FileReadResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &fileResult)
	require.NoError(t, err)

	require.NotEmpty(t, fileResult.Error)
	require.Contains(t, fileResult.Error, "failed to read file")
}

func TestFileReadTool_Execute_Success(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "hello world\nline 2\nline 3"
	err := os.WriteFile(testFile, []byte(testContent), 0644)
	require.NoError(t, err)

	req := FileReadRequest{Path: testFile}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var fileResult FileReadResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &fileResult)
	require.NoError(t, err)

	require.Equal(t, testContent, fileResult.Content)
	require.Equal(t, testFile, fileResult.Path)
	require.Empty(t, fileResult.Error)
}

func TestFileReadTool_Execute_EmptyFile(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "empty.txt")
	err := os.WriteFile(testFile, []byte{}, 0644)
	require.NoError(t, err)

	req := FileReadRequest{Path: testFile}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var fileResult FileReadResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &fileResult)
	require.NoError(t, err)

	require.Empty(t, fileResult.Content)
	require.Empty(t, fileResult.Error)
}

func TestFileReadTool_Execute_WithOffset(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "line 1\nline 2\nline 3\nline 4\nline 5"
	err := os.WriteFile(testFile, []byte(testContent), 0644)
	require.NoError(t, err)

	req := FileReadRequest{Path: testFile, Offset: 3}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var fileResult FileReadResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &fileResult)
	require.NoError(t, err)

	expectedContent := "line 3\nline 4\nline 5"
	require.Equal(t, expectedContent, fileResult.Content)
	require.Equal(t, 3, fileResult.Offset)
}

func TestFileReadTool_Execute_WithLimit(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "line 1\nline 2\nline 3\nline 4\nline 5"
	err := os.WriteFile(testFile, []byte(testContent), 0644)
	require.NoError(t, err)

	req := FileReadRequest{Path: testFile, Limit: 2}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var fileResult FileReadResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &fileResult)
	require.NoError(t, err)

	expectedContent := "line 1\nline 2"
	require.Equal(t, expectedContent, fileResult.Content)
	require.Equal(t, 2, fileResult.Limit)
}

func TestFileReadTool_Execute_WithOffsetAndLimit(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "line 1\nline 2\nline 3\nline 4\nline 5"
	err := os.WriteFile(testFile, []byte(testContent), 0644)
	require.NoError(t, err)

	req := FileReadRequest{Path: testFile, Offset: 2, Limit: 2}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var fileResult FileReadResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &fileResult)
	require.NoError(t, err)

	expectedContent := "line 2\nline 3"
	require.Equal(t, expectedContent, fileResult.Content)
	require.Equal(t, 2, fileResult.Offset)
	require.Equal(t, 2, fileResult.Limit)
}

func TestFileReadTool_Execute_OffsetBeyondFile(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "line 1\nline 2\nline 3"
	err := os.WriteFile(testFile, []byte(testContent), 0644)
	require.NoError(t, err)

	req := FileReadRequest{Path: testFile, Offset: 10}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var fileResult FileReadResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &fileResult)
	require.NoError(t, err)

	require.Empty(t, fileResult.Content)
}

func TestFileReadTool_Execute_OffsetZero(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "line 1\nline 2\nline 3"
	err := os.WriteFile(testFile, []byte(testContent), 0644)
	require.NoError(t, err)

	req := FileReadRequest{Path: testFile, Offset: 0}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var fileResult FileReadResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &fileResult)
	require.NoError(t, err)

	require.Equal(t, testContent, fileResult.Content)
}

func TestFileReadTool_Execute_LimitZero(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "line 1\nline 2\nline 3"
	err := os.WriteFile(testFile, []byte(testContent), 0644)
	require.NoError(t, err)

	req := FileReadRequest{Path: testFile, Limit: 0}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var fileResult FileReadResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &fileResult)
	require.NoError(t, err)

	require.Equal(t, testContent, fileResult.Content)
}

func TestFileReadTool_Execute_StartAfterEnd(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := "line 1\nline 2\nline 3"
	err := os.WriteFile(testFile, []byte(testContent), 0644)
	require.NoError(t, err)

	req := FileReadRequest{Path: testFile, Offset: 5, Limit: 1}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var fileResult FileReadResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &fileResult)
	require.NoError(t, err)

	require.Empty(t, fileResult.Content)
}

func TestFileReadTool_Execute_SingleLine(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "single.txt")
	testContent := "single line"
	err := os.WriteFile(testFile, []byte(testContent), 0644)
	require.NoError(t, err)

	req := FileReadRequest{Path: testFile}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var fileResult FileReadResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &fileResult)
	require.NoError(t, err)

	require.Equal(t, testContent, fileResult.Content)
}

func TestFileReadTool_Execute_BinaryFile(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "binary.bin")
	// Use valid UTF-8 bytes that are still "binary-like" (null bytes, control chars)
	binaryContent := []byte{0x00, 0x01, 0x02, 0x03, 0x1F}
	err := os.WriteFile(testFile, binaryContent, 0644)
	require.NoError(t, err)

	req := FileReadRequest{Path: testFile}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var fileResult FileReadResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &fileResult)
	require.NoError(t, err)

	// Compare raw bytes to avoid encoding issues
	require.Equal(t, binaryContent, []byte(fileResult.Content))
}

func TestFileReadTool_Execute_UnicodeContent(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "unicode.txt")
	unicodeContent := "Hello 世界 🌍 Ñoño café"
	err := os.WriteFile(testFile, []byte(unicodeContent), 0644)
	require.NoError(t, err)

	req := FileReadRequest{Path: testFile}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var fileResult FileReadResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &fileResult)
	require.NoError(t, err)

	require.Equal(t, unicodeContent, fileResult.Content)
}

func TestFileReadTool_Execute_LargeFile(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "large.txt")

	// Create a file with many lines
	var lines []string
	for i := 0; i < 1000; i++ {
		lines = append(lines, "line "+string(rune(i)))
	}
	largeContent := strings.Join(lines, "\n")
	err := os.WriteFile(testFile, []byte(largeContent), 0644)
	require.NoError(t, err)

	req := FileReadRequest{Path: testFile, Offset: 100, Limit: 10}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var fileResult FileReadResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &fileResult)
	require.NoError(t, err)

	// Verify we got exactly 10 lines
	resultLines := strings.Split(fileResult.Content, "\n")
	require.Len(t, resultLines, 10)
}

func TestFileReadTool_Execute_AbsolutePath(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "absolute.txt")
	testContent := "test content"
	err := os.WriteFile(testFile, []byte(testContent), 0644)
	require.NoError(t, err)

	absPath, err := filepath.Abs(testFile)
	require.NoError(t, err)

	req := FileReadRequest{Path: absPath}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var fileResult FileReadResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &fileResult)
	require.NoError(t, err)

	require.Equal(t, testContent, fileResult.Content)
	require.Equal(t, absPath, fileResult.Path)
}

func TestFileReadTool_Execute_RelativePath(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalDir)

	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	testFile := "relative.txt"
	testContent := "test content"
	err = os.WriteFile(testFile, []byte(testContent), 0644)
	require.NoError(t, err)

	req := FileReadRequest{Path: testFile}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var fileResult FileReadResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &fileResult)
	require.NoError(t, err)

	require.Equal(t, testContent, fileResult.Content)
}

func TestFileReadTool_Execute_DirectoryInsteadOfFile(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()

	req := FileReadRequest{Path: tmpDir}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var fileResult FileReadResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &fileResult)
	require.NoError(t, err)

	require.NotEmpty(t, fileResult.Error)
	require.Contains(t, fileResult.Error, "failed to read file")
}

func TestFileReadTool_Execute_TrailingNewline(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "trailing.txt")
	testContent := "line 1\nline 2\n"
	err := os.WriteFile(testFile, []byte(testContent), 0644)
	require.NoError(t, err)

	req := FileReadRequest{Path: testFile}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var fileResult FileReadResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &fileResult)
	require.NoError(t, err)

	require.Equal(t, testContent, fileResult.Content)
}

func TestFileReadTool_Execute_OnlyNewlines(t *testing.T) {
	tool := &FileReadTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "newlines.txt")
	testContent := "\n\n\n"
	err := os.WriteFile(testFile, []byte(testContent), 0644)
	require.NoError(t, err)

	req := FileReadRequest{Path: testFile}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var fileResult FileReadResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &fileResult)
	require.NoError(t, err)

	require.Equal(t, testContent, fileResult.Content)
}
