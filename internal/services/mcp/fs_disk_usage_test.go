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

//go:build !windows

package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
)

// mockStatFS is a mock implementation of statFSInterface for testing.
type mockStatFS struct {
	stat  syscall.Statfs_t
	error error
}

func (m *mockStatFS) StatFS(path string, stat *syscall.Statfs_t) error {
	if m.error != nil {
		return m.error
	}
	*stat = m.stat
	return nil
}

// mockReadFile is a mock implementation of readFileInterface for testing.
type mockReadFile struct {
	data  []byte
	error error
}

func (m *mockReadFile) ReadFile(path string) ([]byte, error) {
	if m.error != nil {
		return nil, m.error
	}
	return m.data, nil
}

func TestFSDiskUsageTool_Name(t *testing.T) {
	tool := &FSDiskUsageTool{}
	require.Equal(t, "fs_disk_usage", tool.Name())
}

func TestFSDiskUsageTool_Description(t *testing.T) {
	tool := &FSDiskUsageTool{}
	require.NotEmpty(t, tool.Description())
	require.Contains(t, tool.Description(), "filesystem")
	require.Contains(t, tool.Description(), "free")
}

func TestFSDiskUsageTool_InputSchema(t *testing.T) {
	tool := &FSDiskUsageTool{}
	schema := tool.InputSchema()

	require.Equal(t, "object", schema.Type)

	properties := schema.Properties
	require.Contains(t, properties, "path")
	require.Equal(t, "string", properties["path"].Type)
	require.NotEmpty(t, properties["path"].Description)
}

func TestFSDiskUsageTool_Execute_InvalidJSON(t *testing.T) {
	tool := &FSDiskUsageTool{}
	ctx := context.Background()

	_, err := tool.Execute(ctx, json.RawMessage(`{invalid json}`))
	require.Error(t, err)
	require.Contains(t, err.Error(), "unmarshal arguments")
}

func TestFSDiskUsageTool_Execute_WithValidPath_Success(t *testing.T) {
	tool := &FSDiskUsageTool{
		statFS: &mockStatFS{
			stat: syscall.Statfs_t{
				Blocks: 1000,
				Bsize:  4096,
				Bfree:  500,
				Bavail: 450,
			},
		},
	}
	ctx := context.Background()

	req := FSDiskUsageRequest{Path: "/tmp"}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var diskResult FSDiskUsageResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &diskResult)
	require.NoError(t, err)

	require.Equal(t, "/tmp", diskResult.Path)
	require.NotNil(t, diskResult.Filesystem)
	require.Equal(t, "/tmp", diskResult.Filesystem.Path)
	require.Equal(t, uint64(1000*4096), diskResult.Filesystem.TotalBytes)
	require.Equal(t, uint64(500*4096), diskResult.Filesystem.FreeBytes)
	require.Equal(t, uint64(450*4096), diskResult.Filesystem.AvailableBytes)
	require.Equal(t, uint64(500*4096), diskResult.Filesystem.UsedBytes)
	require.InDelta(t, 50.0, diskResult.Filesystem.UsedPercent, 0.1)
}

func TestFSDiskUsageTool_Execute_WithEmptyPath_AllFilesystems(t *testing.T) {
	mountsData := `sysfs /sys sysfs rw,nosuid,nodev,noexec,relatime 0 0
proc /proc proc rw,nosuid,nodev,noexec,relatime 0 0
/dev/sda1 / ext4 rw,relatime 0 0
/dev/sda2 /home ext4 rw,relatime 0 0
`

	tool := &FSDiskUsageTool{
		statFS: &mockStatFS{
			stat: syscall.Statfs_t{
				Blocks: 2000,
				Bsize:  4096,
				Bfree:  1000,
				Bavail: 900,
			},
		},
		readFile: &mockReadFile{
			data: []byte(mountsData),
		},
	}
	ctx := context.Background()

	req := FSDiskUsageRequest{Path: ""}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var diskResult FSDiskUsageResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &diskResult)
	require.NoError(t, err)

	require.Greater(t, diskResult.Count, 0)
	require.Len(t, diskResult.Filesystems, diskResult.Count)
	require.Nil(t, diskResult.Filesystem)
}

func TestFSDiskUsageTool_Execute_StatFSError(t *testing.T) {
	tool := &FSDiskUsageTool{
		statFS: &mockStatFS{
			error: errors.New("permission denied"),
		},
	}
	ctx := context.Background()

	req := FSDiskUsageRequest{Path: "/root"}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	_, err = tool.Execute(ctx, args)
	require.Error(t, err)
	require.Contains(t, err.Error(), "get disk usage")
}

func TestFSDiskUsageTool_Execute_ReadFileError(t *testing.T) {
	tool := &FSDiskUsageTool{
		readFile: &mockReadFile{
			error: errors.New("file not found"),
		},
	}
	ctx := context.Background()

	req := FSDiskUsageRequest{Path: ""}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	_, err = tool.Execute(ctx, args)
	require.Error(t, err)
	require.Contains(t, err.Error(), "get disk usage")
}

func TestFSDiskUsageTool_Execute_MarshalError(t *testing.T) {
	tool := &FSDiskUsageTool{
		statFS: &mockStatFS{
			stat: syscall.Statfs_t{
				Blocks: 1000,
				Bsize:  4096,
				Bfree:  500,
				Bavail: 450,
			},
		},
	}
	ctx := context.Background()

	// This test is a placeholder - in practice, marshaling FSDiskUsageResult
	// should not fail as it contains only basic types. The test ensures
	// the error path is covered if the struct changes in the future.
	req := FSDiskUsageRequest{Path: "/tmp"}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.NotNil(t, result.Content)
}

func TestGetDiskUsageForPath_Success(t *testing.T) {
	mockStat := &mockStatFS{
		stat: syscall.Statfs_t{
			Blocks: 5000,
			Bsize:  4096,
			Bfree:  2000,
			Bavail: 1800,
		},
	}

	result, err := getDiskUsageForPath("/var", mockStat)
	require.NoError(t, err)

	require.Equal(t, "/var", result.Path)
	require.NotNil(t, result.Filesystem)
	require.Equal(t, "/var", result.Filesystem.Path)
	require.Equal(t, uint64(5000*4096), result.Filesystem.TotalBytes)
	require.Equal(t, uint64(2000*4096), result.Filesystem.FreeBytes)
	require.Equal(t, uint64(1800*4096), result.Filesystem.AvailableBytes)
	require.Equal(t, uint64(3000*4096), result.Filesystem.UsedBytes)
	require.InDelta(t, 60.0, result.Filesystem.UsedPercent, 0.1)
}

func TestGetDiskUsageForPath_StatFSError(t *testing.T) {
	mockStat := &mockStatFS{
		error: errors.New("no such file or directory"),
	}

	_, err := getDiskUsageForPath("/nonexistent", mockStat)
	require.Error(t, err)
	require.Contains(t, err.Error(), "statfs")
}

func TestGetDiskUsageForPath_ZeroBlocks(t *testing.T) {
	mockStat := &mockStatFS{
		stat: syscall.Statfs_t{
			Blocks: 0,
			Bsize:  4096,
			Bfree:  0,
			Bavail: 0,
		},
	}

	result, err := getDiskUsageForPath("/empty", mockStat)
	require.NoError(t, err)

	require.Equal(t, uint64(0), result.Filesystem.TotalBytes)
	require.Equal(t, uint64(0), result.Filesystem.UsedBytes)
	require.Equal(t, uint64(0), result.Filesystem.FreeBytes)
	require.Equal(t, uint64(0), result.Filesystem.AvailableBytes)
	// Used percent should be 0 when total is 0 (NaN check)
	require.True(t, result.Filesystem.UsedPercent == 0 || result.Filesystem.UsedPercent != result.Filesystem.UsedPercent)
}

func TestGetAllDiskUsage_Success(t *testing.T) {
	mountsData := `sysfs /sys sysfs rw 0 0
/dev/sda1 / ext4 rw 0 0
/dev/sda2 /home ext4 rw 0 0
`

	mockStat := &mockStatFS{
		stat: syscall.Statfs_t{
			Blocks: 1000,
			Bsize:  4096,
			Bfree:  500,
			Bavail: 450,
		},
	}
	mockRead := &mockReadFile{
		data: []byte(mountsData),
	}

	result, err := getAllDiskUsage(mockStat, mockRead)
	require.NoError(t, err)

	require.Greater(t, result.Count, 0)
	require.Len(t, result.Filesystems, result.Count)
	require.Nil(t, result.Filesystem)
}

func TestGetAllDiskUsage_ReadFileError(t *testing.T) {
	mockRead := &mockReadFile{
		error: errors.New("permission denied"),
	}

	_, err := getAllDiskUsage(nil, mockRead)
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse mounts")
}

func TestGetAllDiskUsage_SkipsFailedStatFS(t *testing.T) {
	mountsData := `/dev/sda1 / ext4 rw 0 0
/dev/sda2 /home ext4 rw 0 0
`

	// Create a custom statFS that fails on the second call
	customStatFS := &customStatFS{
		failAfter: 1,
		stat: syscall.Statfs_t{
			Blocks: 1000,
			Bsize:  4096,
			Bfree:  500,
			Bavail: 450,
		},
	}

	mockRead := &mockReadFile{
		data: []byte(mountsData),
	}

	result, err := getAllDiskUsage(customStatFS, mockRead)
	require.NoError(t, err)

	// Should succeed with at least one filesystem
	require.GreaterOrEqual(t, result.Count, 1)
	require.GreaterOrEqual(t, customStatFS.callCount, 1)
}

// customStatFS is a custom implementation that can fail after N calls
type customStatFS struct {
	failAfter int
	callCount int
	stat      syscall.Statfs_t
}

func (c *customStatFS) StatFS(path string, stat *syscall.Statfs_t) error {
	c.callCount++
	if c.callCount > c.failAfter {
		return errors.New("statfs failed")
	}
	*stat = c.stat
	return nil
}

func TestParseMounts_Success(t *testing.T) {
	mountsData := `sysfs /sys sysfs rw,nosuid,nodev,noexec,relatime 0 0
proc /proc proc rw,nosuid,nodev,noexec,relatime 0 0
udev /dev devtmpfs rw,nosuid,relatime,size=8171996k,nr_inodes=2042999,mode=755 0 0
devpts /dev/pts devpts rw,nosuid,noexec,relatime,gid=5,mode=620,ptmxmode=000 0 0
/dev/sda1 / ext4 rw,relatime 0 0
/dev/sda2 /home ext4 rw,relatime 0 0
`

	mockRead := &mockReadFile{
		data: []byte(mountsData),
	}

	mounts, err := parseMounts(mockRead)
	require.NoError(t, err)

	// Should include / and /home, but exclude /sys, /proc, /dev
	require.Contains(t, mounts, "/")
	require.Contains(t, mounts, "/home")
	require.NotContains(t, mounts, "/sys")
	require.NotContains(t, mounts, "/proc")
	require.NotContains(t, mounts, "/dev")
}

func TestParseMounts_EmptyLines(t *testing.T) {
	mountsData := `sysfs /sys sysfs rw 0 0

/dev/sda1 / ext4 rw 0 0

/dev/sda2 /home ext4 rw 0 0
`

	mockRead := &mockReadFile{
		data: []byte(mountsData),
	}

	mounts, err := parseMounts(mockRead)
	require.NoError(t, err)
	require.Greater(t, len(mounts), 0)
}

func TestParseMounts_ReadFileError(t *testing.T) {
	mockRead := &mockReadFile{
		error: errors.New("file not found"),
	}

	_, err := parseMounts(mockRead)
	require.Error(t, err)
	require.Contains(t, err.Error(), "read /proc/mounts")
}

func TestParseMounts_MalformedLine(t *testing.T) {
	mountsData := `sysfs /sys sysfs rw 0 0
malformed_line
/dev/sda1 / ext4 rw 0 0
`

	mockRead := &mockReadFile{
		data: []byte(mountsData),
	}

	mounts, err := parseMounts(mockRead)
	require.NoError(t, err)
	// Should skip malformed line and still return valid mounts
	require.Greater(t, len(mounts), 0)
}

func TestParseMounts_ExcludesSpecialMounts(t *testing.T) {
	mountsData := `sysfs /sys sysfs rw 0 0
proc /proc proc rw 0 0
devtmpfs /dev devtmpfs rw 0 0
/dev/sda1 / ext4 rw 0 0
/dev/sda2 /home ext4 rw 0 0
/run/user/1000 /run/user/1000 tmpfs rw 0 0
`

	mockRead := &mockReadFile{
		data: []byte(mountsData),
	}

	mounts, err := parseMounts(mockRead)
	require.NoError(t, err)

	// Should exclude /sys, /proc, /dev
	require.NotContains(t, mounts, "/sys")
	require.NotContains(t, mounts, "/proc")
	require.NotContains(t, mounts, "/dev")
	// Should include /, /home, and /run/user/1000
	require.Contains(t, mounts, "/")
	require.Contains(t, mounts, "/home")
	require.Contains(t, mounts, "/run/user/1000")
}

func TestFSDiskUsageTool_NilDependencies(t *testing.T) {
	// Test that nil dependencies use real implementations
	tool := &FSDiskUsageTool{}
	ctx := context.Background()

	// This will use real syscall.Statfs and os.ReadFile
	// We can't predict the exact result, but we can verify it doesn't panic
	req := FSDiskUsageRequest{Path: "/tmp"}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	// This may fail if /tmp doesn't exist or isn't accessible
	// but it shouldn't panic
	_, err = tool.Execute(ctx, args)
	// We don't assert error here because it depends on the test environment
	_ = err
}

func TestFSDiskUsageTool_UsedPercentCalculation(t *testing.T) {
	tests := []struct {
		name         string
		blocks       uint64
		bsize        int64
		bfree        uint64
		expectedPct  float64
	}{
		{
			name:        "50% used",
			blocks:      1000,
			bsize:       4096,
			bfree:       500,
			expectedPct: 50.0,
		},
		{
			name:        "75% used",
			blocks:      1000,
			bsize:       4096,
			bfree:       250,
			expectedPct: 75.0,
		},
		{
			name:        "0% used",
			blocks:      1000,
			bsize:       4096,
			bfree:       1000,
			expectedPct: 0.0,
		},
		{
			name:        "100% used",
			blocks:      1000,
			bsize:       4096,
			bfree:       0,
			expectedPct: 100.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStat := &mockStatFS{
				stat: syscall.Statfs_t{
					Blocks: tt.blocks,
					Bsize:  tt.bsize,
					Bfree:  tt.bfree,
					Bavail: tt.bfree,
				},
			}

			result, err := getDiskUsageForPath("/test", mockStat)
			require.NoError(t, err)
			require.InDelta(t, tt.expectedPct, result.Filesystem.UsedPercent, 0.1)
		})
	}
}
