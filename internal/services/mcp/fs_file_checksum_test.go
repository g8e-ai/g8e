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
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFSFileChecksumTool_Name(t *testing.T) {
	tool := &FSFileChecksumTool{}
	require.Equal(t, "fs_file_checksum", tool.Name())
}

func TestFSFileChecksumTool_Description(t *testing.T) {
	tool := &FSFileChecksumTool{}
	require.NotEmpty(t, tool.Description())
	require.Contains(t, tool.Description(), "checksum")
}

func TestFSFileChecksumTool_InputSchema(t *testing.T) {
	tool := &FSFileChecksumTool{}
	schema := tool.InputSchema()

	require.Equal(t, "object", schema.Type)
	require.Contains(t, schema.Required, "file_path")

	properties := schema.Properties
	require.Contains(t, properties, "file_path")
	require.Equal(t, "string", properties["file_path"].Type)
	require.NotEmpty(t, properties["file_path"].Description)
}

func TestFSFileChecksumTool_Execute_InvalidJSON(t *testing.T) {
	tool := &FSFileChecksumTool{}
	ctx := context.Background()

	_, err := tool.Execute(ctx, json.RawMessage(`{invalid json}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "unmarshal arguments")
}

func TestFSFileChecksumTool_Execute_EmptyFilePath(t *testing.T) {
	tool := &FSFileChecksumTool{}
	ctx := context.Background()

	req := FSFileChecksumRequest{FilePath: ""}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	_, err = tool.Execute(ctx, args)
	require.Error(t, err)
	require.Contains(t, err.Error(), "file_path required")
}

func TestFSFileChecksumTool_Execute_InvalidPath_Whitespace(t *testing.T) {
	tool := &FSFileChecksumTool{}
	ctx := context.Background()

	req := FSFileChecksumRequest{FilePath: "  /tmp/file.txt  "}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	_, err = tool.Execute(ctx, args)
	require.Error(t, err)
	require.Contains(t, err.Error(), "must not contain leading/trailing whitespace")
}

func TestFSFileChecksumTool_Execute_InvalidPath_PathTraversal(t *testing.T) {
	tool := &FSFileChecksumTool{}
	ctx := context.Background()

	tests := []struct {
		name string
		path string
	}{
		{"parent dir reference", "../etc/passwd"},
		{"mid path traversal", "/tmp/../etc/passwd"},
		{"double parent", "../../etc/passwd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := FSFileChecksumRequest{FilePath: tt.path}
			args, err := json.Marshal(req)
			require.NoError(t, err)

			_, err = tool.Execute(ctx, args)
			require.Error(t, err)
			require.Contains(t, err.Error(), "parent directory references")
		})
	}
}

func TestFSFileChecksumTool_Execute_InvalidPath_NullByte(t *testing.T) {
	tool := &FSFileChecksumTool{}
	ctx := context.Background()

	req := FSFileChecksumRequest{FilePath: "/tmp/file\x00.txt"}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	_, err = tool.Execute(ctx, args)
	require.Error(t, err)
	require.Contains(t, err.Error(), "null bytes")
}

func TestFSFileChecksumTool_Execute_FileNotFound(t *testing.T) {
	tool := &FSFileChecksumTool{}
	ctx := context.Background()

	req := FSFileChecksumRequest{FilePath: "/tmp/non-existent-file-12345.txt"}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	_, err = tool.Execute(ctx, args)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to read file")
}

func TestFSFileChecksumTool_Execute_Success(t *testing.T) {
	tool := &FSFileChecksumTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "test.txt")
	testContent := []byte("hello world")
	err := os.WriteFile(testFile, testContent, 0644)
	require.NoError(t, err)

	req := FSFileChecksumRequest{FilePath: testFile}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var checksumResult FSFileChecksumResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &checksumResult)
	require.NoError(t, err)

	require.Equal(t, testFile, checksumResult.FilePath)
	require.Equal(t, "sha256", checksumResult.Algorithm)
	require.Equal(t, int64(len(testContent)), checksumResult.SizeBytes)

	// Verify the checksum is correct
	expectedHash := sha256.Sum256(testContent)
	expectedChecksum := hex.EncodeToString(expectedHash[:])
	require.Equal(t, expectedChecksum, checksumResult.Checksum)
}

func TestFSFileChecksumTool_Execute_EmptyFile(t *testing.T) {
	tool := &FSFileChecksumTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "empty.txt")
	err := os.WriteFile(testFile, []byte{}, 0644)
	require.NoError(t, err)

	req := FSFileChecksumRequest{FilePath: testFile}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var checksumResult FSFileChecksumResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &checksumResult)
	require.NoError(t, err)

	require.Equal(t, int64(0), checksumResult.SizeBytes)
	// SHA256 of empty string
	require.Equal(t, "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", checksumResult.Checksum)
}

func TestFSFileChecksumTool_Execute_LargeFile(t *testing.T) {
	tool := &FSFileChecksumTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "large.bin")
	
	// Create a 1MB file
	largeContent := make([]byte, 1024*1024)
	for i := range largeContent {
		largeContent[i] = byte(i % 256)
	}
	err := os.WriteFile(testFile, largeContent, 0644)
	require.NoError(t, err)

	req := FSFileChecksumRequest{FilePath: testFile}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var checksumResult FSFileChecksumResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &checksumResult)
	require.NoError(t, err)

	require.Equal(t, int64(len(largeContent)), checksumResult.SizeBytes)

	// Verify the checksum is correct
	expectedHash := sha256.Sum256(largeContent)
	expectedChecksum := hex.EncodeToString(expectedHash[:])
	require.Equal(t, expectedChecksum, checksumResult.Checksum)
}

func TestFSFileChecksumTool_Execute_BinaryFile(t *testing.T) {
	tool := &FSFileChecksumTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "binary.bin")
	
	// Create binary content with null bytes and other special characters
	binaryContent := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0x80, 0x81}
	err := os.WriteFile(testFile, binaryContent, 0644)
	require.NoError(t, err)

	req := FSFileChecksumRequest{FilePath: testFile}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var checksumResult FSFileChecksumResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &checksumResult)
	require.NoError(t, err)

	require.Equal(t, int64(len(binaryContent)), checksumResult.SizeBytes)

	// Verify the checksum is correct
	expectedHash := sha256.Sum256(binaryContent)
	expectedChecksum := hex.EncodeToString(expectedHash[:])
	require.Equal(t, expectedChecksum, checksumResult.Checksum)
}

func TestFSFileChecksumTool_Execute_UnicodeContent(t *testing.T) {
	tool := &FSFileChecksumTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "unicode.txt")
	
	// Create content with various Unicode characters
	unicodeContent := []byte("Hello 世界 🌍 Ñoño café")
	err := os.WriteFile(testFile, unicodeContent, 0644)
	require.NoError(t, err)

	req := FSFileChecksumRequest{FilePath: testFile}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var checksumResult FSFileChecksumResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &checksumResult)
	require.NoError(t, err)

	// Verify the checksum is correct
	expectedHash := sha256.Sum256(unicodeContent)
	expectedChecksum := hex.EncodeToString(expectedHash[:])
	require.Equal(t, expectedChecksum, checksumResult.Checksum)
}

func TestFSFileChecksumTool_Execute_DirectoryInsteadOfFile(t *testing.T) {
	tool := &FSFileChecksumTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	
	req := FSFileChecksumRequest{FilePath: tmpDir}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	_, err = tool.Execute(ctx, args)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to read file")
}

func TestFSFileChecksumTool_Execute_AbsolutePath(t *testing.T) {
	tool := &FSFileChecksumTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "absolute.txt")
	err := os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	// Use absolute path
	absPath, err := filepath.Abs(testFile)
	require.NoError(t, err)

	req := FSFileChecksumRequest{FilePath: absPath}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var checksumResult FSFileChecksumResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &checksumResult)
	require.NoError(t, err)

	require.Equal(t, absPath, checksumResult.FilePath)
}

func TestFSFileChecksumTool_Execute_RelativePath(t *testing.T) {
	tool := &FSFileChecksumTool{}
	ctx := context.Background()

	// Change to temp directory to test relative paths
	tmpDir := t.TempDir()
	originalDir, err := os.Getwd()
	require.NoError(t, err)
	defer os.Chdir(originalDir)
	
	err = os.Chdir(tmpDir)
	require.NoError(t, err)

	testFile := "relative.txt"
	err = os.WriteFile(testFile, []byte("test"), 0644)
	require.NoError(t, err)

	req := FSFileChecksumRequest{FilePath: testFile}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var checksumResult FSFileChecksumResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &checksumResult)
	require.NoError(t, err)

	require.Equal(t, testFile, checksumResult.FilePath)
}

func TestFSFileChecksumTool_Execute_MultipleFiles(t *testing.T) {
	tool := &FSFileChecksumTool{}
	ctx := context.Background()

	tmpDir := t.TempDir()

	files := map[string]string{
		"file1.txt": "content1",
		"file2.txt": "content2",
		"file3.txt": "content3",
	}

	for name, content := range files {
		path := filepath.Join(tmpDir, name)
		err := os.WriteFile(path, []byte(content), 0644)
		require.NoError(t, err)

		req := FSFileChecksumRequest{FilePath: path}
		args, err := json.Marshal(req)
		require.NoError(t, err)

		result, err := tool.Execute(ctx, args)
		require.NoError(t, err)

		var checksumResult FSFileChecksumResult
		err = json.Unmarshal([]byte(result.Content[0].Text), &checksumResult)
		require.NoError(t, err)

		// Verify each file has a unique checksum
		expectedHash := sha256.Sum256([]byte(content))
		expectedChecksum := hex.EncodeToString(expectedHash[:])
		require.Equal(t, expectedChecksum, checksumResult.Checksum)
	}
}
