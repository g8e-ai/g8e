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

	"github.com/g8e-ai/g8e/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestFSDiskProfileTool_Name(t *testing.T) {
	tool := &FSDiskProfileTool{}
	require.Equal(t, "fs_disk_profile", tool.Name())
}

func TestFSDiskProfileTool_Description(t *testing.T) {
	tool := &FSDiskProfileTool{}
	require.NotEmpty(t, tool.Description())
	require.Contains(t, tool.Description(), "directory")
	require.Contains(t, tool.Description(), "disk")
}

func TestFSDiskProfileTool_InputSchema(t *testing.T) {
	tool := &FSDiskProfileTool{}
	schema := tool.InputSchema()

	require.Equal(t, "object", schema.Type)
	require.Contains(t, schema.Required, "path")

	properties := schema.Properties
	require.Contains(t, properties, "path")
	require.Contains(t, properties, "max_depth")

	require.Equal(t, "string", properties["path"].Type)
	require.Equal(t, "integer", properties["max_depth"].Type)
	require.Contains(t, properties["path"].Description, "Path")
	require.Contains(t, properties["max_depth"].Description, "depth")
}

func TestFSDiskProfileTool_Execute_InvalidJSON(t *testing.T) {
	tool := &FSDiskProfileTool{}
	ctx := context.Background()

	_, err := tool.Execute(ctx, json.RawMessage(`{invalid json`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid arguments")
}

func TestFSDiskProfileTool_Execute_MissingPath(t *testing.T) {
	tool := &FSDiskProfileTool{}
	ctx := context.Background()

	req := FSDiskProfileRequest{}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	_, err = tool.Execute(ctx, args)
	require.Error(t, err)
	require.Contains(t, err.Error(), "path required")
}

func TestFSDiskProfileTool_Execute_EmptyPath(t *testing.T) {
	tool := &FSDiskProfileTool{}
	ctx := context.Background()

	req := FSDiskProfileRequest{Path: ""}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	_, err = tool.Execute(ctx, args)
	require.Error(t, err)
	require.Contains(t, err.Error(), "path required")
}

func TestFSDiskProfileTool_Execute_NonExistentPath(t *testing.T) {
	tool := &FSDiskProfileTool{}
	ctx := context.Background()

	req := FSDiskProfileRequest{Path: "/non-existent-path-12345"}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	_, err = tool.Execute(ctx, args)
	require.Error(t, err)
	require.Contains(t, err.Error(), "error accessing path")
}

func TestFSDiskProfileTool_Execute_SingleFile(t *testing.T) {
	tool := &FSDiskProfileTool{}
	ctx := context.Background()

	tmpDir := testutil.TempDir(t)
	testFile := filepath.Join(tmpDir, "test.txt")
	err := os.WriteFile(testFile, []byte("hello world"), 0644)
	require.NoError(t, err)

	req := FSDiskProfileRequest{Path: tmpDir}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var profileResult FSDiskProfileResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &profileResult)
	require.NoError(t, err)

	require.Greater(t, len(profileResult.Entries), 0)

	// Find the root entry (should be ".")
	var rootEntry *DirEntry
	for i := range profileResult.Entries {
		if profileResult.Entries[i].Path == "." {
			rootEntry = &profileResult.Entries[i]
			break
		}
	}
	require.NotNil(t, rootEntry)
	require.True(t, rootEntry.IsDir)
}

func TestFSDiskProfileTool_Execute_DirectoryStructure(t *testing.T) {
	tool := &FSDiskProfileTool{}
	ctx := context.Background()

	tmpDir := testutil.TempDir(t)

	// Create directory structure
	subDir1 := filepath.Join(tmpDir, "subdir1")
	subDir2 := filepath.Join(tmpDir, "subdir2")
	err := os.MkdirAll(subDir1, 0755)
	require.NoError(t, err)
	err = os.MkdirAll(subDir2, 0755)
	require.NoError(t, err)

	// Create files
	file1 := filepath.Join(subDir1, "file1.txt")
	file2 := filepath.Join(subDir2, "file2.txt")
	err = os.WriteFile(file1, []byte("content1"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(file2, []byte("content2"), 0644)
	require.NoError(t, err)

	req := FSDiskProfileRequest{Path: tmpDir}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var profileResult FSDiskProfileResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &profileResult)
	require.NoError(t, err)

	// Should have entries for root, both subdirs, and both files
	require.GreaterOrEqual(t, len(profileResult.Entries), 5)

	// Verify we have directory entries
	hasSubDir1 := false
	hasSubDir2 := false
	for _, entry := range profileResult.Entries {
		if entry.Path == "subdir1" {
			hasSubDir1 = true
			require.True(t, entry.IsDir)
		}
		if entry.Path == "subdir2" {
			hasSubDir2 = true
			require.True(t, entry.IsDir)
		}
	}
	require.True(t, hasSubDir1)
	require.True(t, hasSubDir2)
}

func TestFSDiskProfileTool_Execute_MaxDepth(t *testing.T) {
	tool := &FSDiskProfileTool{}
	ctx := context.Background()

	tmpDir := testutil.TempDir(t)

	// Create nested structure: tmpDir/subdir1/subdir2/subdir3/file.txt
	subDir1 := filepath.Join(tmpDir, "subdir1")
	subDir2 := filepath.Join(subDir1, "subdir2")
	subDir3 := filepath.Join(subDir2, "subdir3")
	err := os.MkdirAll(subDir3, 0755)
	require.NoError(t, err)

	file := filepath.Join(subDir3, "file.txt")
	err = os.WriteFile(file, []byte("deep content"), 0644)
	require.NoError(t, err)

	// Test with max_depth=1 (should see up to depth 2: root, subdir1, subdir2)
	req := FSDiskProfileRequest{Path: tmpDir, MaxDepth: 1}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var profileResult FSDiskProfileResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &profileResult)
	require.NoError(t, err)

	// Should see subdir2 but not subdir3 or file.txt
	hasSubDir2 := false
	hasSubDir3 := false
	hasDeepFile := false
	for _, entry := range profileResult.Entries {
		if entry.Path == filepath.Join("subdir1", "subdir2") {
			hasSubDir2 = true
		}
		if entry.Path == filepath.Join("subdir1", "subdir2", "subdir3") {
			hasSubDir3 = true
		}
		if entry.Path == filepath.Join("subdir1", "subdir2", "subdir3", "file.txt") {
			hasDeepFile = true
		}
	}
	require.True(t, hasSubDir2, "Should see subdir2 with max_depth=1")
	require.False(t, hasSubDir3, "Should not see subdir3 with max_depth=1")
	require.False(t, hasDeepFile, "Should not see deep file with max_depth=1")
}

func TestFSDiskProfileTool_Execute_DefaultMaxDepth(t *testing.T) {
	tool := &FSDiskProfileTool{}
	ctx := context.Background()

	tmpDir := testutil.TempDir(t)

	// Create nested structure
	subDir1 := filepath.Join(tmpDir, "subdir1")
	subDir2 := filepath.Join(subDir1, "subdir2")
	err := os.MkdirAll(subDir2, 0755)
	require.NoError(t, err)

	file := filepath.Join(subDir2, "file.txt")
	err = os.WriteFile(file, []byte("content"), 0644)
	require.NoError(t, err)

	// Test without max_depth (should use default of 2)
	req := FSDiskProfileRequest{Path: tmpDir}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var profileResult FSDiskProfileResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &profileResult)
	require.NoError(t, err)

	// With default depth of 2, should see subdir2 but not necessarily its contents
	hasSubDir2 := false
	for _, entry := range profileResult.Entries {
		if entry.Path == filepath.Join("subdir1", "subdir2") {
			hasSubDir2 = true
			break
		}
	}
	require.True(t, hasSubDir2, "Should see subdir2 with default max_depth=2")
}

func TestFSDiskProfileTool_Execute_ZeroMaxDepth(t *testing.T) {
	tool := &FSDiskProfileTool{}
	ctx := context.Background()

	tmpDir := testutil.TempDir(t)

	// Create simple structure
	subDir := filepath.Join(tmpDir, "subdir")
	err := os.Mkdir(subDir, 0755)
	require.NoError(t, err)

	file := filepath.Join(tmpDir, "file.txt")
	err = os.WriteFile(file, []byte("content"), 0644)
	require.NoError(t, err)

	// Test with max_depth=0 (should use default)
	req := FSDiskProfileRequest{Path: tmpDir, MaxDepth: 0}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var profileResult FSDiskProfileResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &profileResult)
	require.NoError(t, err)

	// Should see entries (default depth applies)
	require.Greater(t, len(profileResult.Entries), 0)
}

func TestFSDiskProfileTool_Execute_NegativeMaxDepth(t *testing.T) {
	tool := &FSDiskProfileTool{}
	ctx := context.Background()

	tmpDir := testutil.TempDir(t)

	file := filepath.Join(tmpDir, "file.txt")
	err := os.WriteFile(file, []byte("content"), 0644)
	require.NoError(t, err)

	// Test with negative max_depth (should use default)
	req := FSDiskProfileRequest{Path: tmpDir, MaxDepth: -1}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var profileResult FSDiskProfileResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &profileResult)
	require.NoError(t, err)

	require.Greater(t, len(profileResult.Entries), 0)
}

func TestFSDiskProfileTool_Execute_ContextCancellation(t *testing.T) {
	tool := &FSDiskProfileTool{}

	// Create a context that's already cancelled
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	tmpDir := testutil.TempDir(t)

	// Create some structure
	subDir := filepath.Join(tmpDir, "subdir")
	err := os.Mkdir(subDir, 0755)
	require.NoError(t, err)

	req := FSDiskProfileRequest{Path: tmpDir}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	_, err = tool.Execute(ctx, args)
	require.Error(t, err)
	require.Contains(t, err.Error(), "context canceled")
}

func TestFSDiskProfileTool_Execute_SizeCalculation(t *testing.T) {
	tool := &FSDiskProfileTool{}
	ctx := context.Background()

	tmpDir := testutil.TempDir(t)

	// Create files with known sizes
	file1 := filepath.Join(tmpDir, "file1.txt")
	file2 := filepath.Join(tmpDir, "file2.txt")
	content1 := make([]byte, 1024*1024) // 1 MB
	content2 := make([]byte, 512*1024)  // 0.5 MB

	err := os.WriteFile(file1, content1, 0644)
	require.NoError(t, err)
	err = os.WriteFile(file2, content2, 0644)
	require.NoError(t, err)

	req := FSDiskProfileRequest{Path: tmpDir}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var profileResult FSDiskProfileResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &profileResult)
	require.NoError(t, err)

	// Total should be at least 1 MB (files may have filesystem overhead)
	require.GreaterOrEqual(t, profileResult.TotalMB, int64(1))
}

func TestFSDiskProfileTool_Execute_ModificationTime(t *testing.T) {
	tool := &FSDiskProfileTool{}
	ctx := context.Background()

	tmpDir := testutil.TempDir(t)

	file := filepath.Join(tmpDir, "file.txt")
	err := os.WriteFile(file, []byte("content"), 0644)
	require.NoError(t, err)

	// Get expected modification time
	info, err := os.Stat(file)
	require.NoError(t, err)
	expectedModTime := info.ModTime().Unix()

	req := FSDiskProfileRequest{Path: tmpDir}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var profileResult FSDiskProfileResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &profileResult)
	require.NoError(t, err)

	// Find the file entry
	var fileEntry *DirEntry
	for i := range profileResult.Entries {
		if profileResult.Entries[i].Path == "file.txt" {
			fileEntry = &profileResult.Entries[i]
			break
		}
	}
	require.NotNil(t, fileEntry)

	// Modification time should be close (within 1 second tolerance)
	require.InDelta(t, expectedModTime, fileEntry.Modified, 1)
}

func TestFSDiskProfileTool_Execute_EmptyDirectory(t *testing.T) {
	tool := &FSDiskProfileTool{}
	ctx := context.Background()

	tmpDir := testutil.TempDir(t)
	// Empty directory

	req := FSDiskProfileRequest{Path: tmpDir}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var profileResult FSDiskProfileResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &profileResult)
	require.NoError(t, err)

	// Should have at least the root entry
	require.GreaterOrEqual(t, len(profileResult.Entries), 1)

	// Total size should be 0 for empty directory
	require.Equal(t, int64(0), profileResult.TotalMB)
}

func TestFSDiskProfileTool_Execute_SymlinkSkip(t *testing.T) {
	tool := &FSDiskProfileTool{}
	ctx := context.Background()

	tmpDir := testutil.TempDir(t)

	// Create a file and a symlink to it
	file := filepath.Join(tmpDir, "file.txt")
	err := os.WriteFile(file, []byte("content"), 0644)
	require.NoError(t, err)

	symlink := filepath.Join(tmpDir, "link.txt")
	err = os.Symlink(file, symlink)
	if err != nil {
		t.Skipf("symlink creation not supported on this platform: %v", err)
	}

	req := FSDiskProfileRequest{Path: tmpDir}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var profileResult FSDiskProfileResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &profileResult)
	require.NoError(t, err)

	// Should have entries for both file and symlink
	// (symlinks are followed by filepath.Walk by default)
	require.GreaterOrEqual(t, len(profileResult.Entries), 2)
}

func TestFSDiskProfileTool_Execute_LargeMaxDepth(t *testing.T) {
	tool := &FSDiskProfileTool{}
	ctx := context.Background()

	tmpDir := testutil.TempDir(t)

	// Create a moderately deep structure
	current := tmpDir
	for i := 0; i < 5; i++ {
		current = filepath.Join(current, "level"+string(rune('0'+i)))
		err := os.Mkdir(current, 0755)
		require.NoError(t, err)
	}

	// Test with large max_depth
	req := FSDiskProfileRequest{Path: tmpDir, MaxDepth: 10}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var profileResult FSDiskProfileResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &profileResult)
	require.NoError(t, err)

	// Should see all levels
	require.GreaterOrEqual(t, len(profileResult.Entries), 5)
}
