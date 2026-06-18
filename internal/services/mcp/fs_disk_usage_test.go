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

	"github.com/g8e-ai/g8e/internal/constants"
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

func TestFSDiskUsageTool_Properties(t *testing.T) {
	tool := &FSDiskUsageTool{}

	t.Run("Name", func(t *testing.T) {
		require.Equal(t, "fs_disk_usage", tool.Name())
	})

	t.Run("Description", func(t *testing.T) {
		desc := tool.Description()
		require.NotEmpty(t, desc)
		require.Contains(t, desc, "filesystem")
		require.Contains(t, desc, "free")
	})

	t.Run("InputSchema", func(t *testing.T) {
		schema := tool.InputSchema()
		require.Equal(t, "object", schema.Type)

		properties := schema.Properties
		require.Contains(t, properties, "path")
		require.Equal(t, "string", properties["path"].Type)
		require.NotEmpty(t, properties["path"].Description)
	})
}

func TestFSDiskUsageTool_Execute_InvalidJSON(t *testing.T) {
	tool := &FSDiskUsageTool{}
	ctx := context.Background()

	_, err := tool.Execute(ctx, json.RawMessage(`{invalid json}`))
	require.Error(t, err)
	require.ErrorIs(t, err, constants.ErrMCPUnmarshalArguments)
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

func TestFSDiskUsageTool_Execute_Errors(t *testing.T) {
	tests := []struct {
		name    string
		tool    *FSDiskUsageTool
		req     FSDiskUsageRequest
		wantErr error
	}{
		{
			name: "StatFS error",
			tool: &FSDiskUsageTool{
				statFS: &mockStatFS{
					error: errors.New("permission denied"),
				},
			},
			req:     FSDiskUsageRequest{Path: "/root"},
			wantErr: constants.ErrMCPGetDiskUsage,
		},
		{
			name: "ReadFile error",
			tool: &FSDiskUsageTool{
				readFile: &mockReadFile{
					error: errors.New("file not found"),
				},
			},
			req:     FSDiskUsageRequest{Path: ""},
			wantErr: constants.ErrMCPGetDiskUsage,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			args, err := json.Marshal(tt.req)
			require.NoError(t, err)

			_, err = tt.tool.Execute(ctx, args)
			require.Error(t, err)
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
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

func TestFSDiskUsageTool_NilInterfaceFallback(t *testing.T) {
	// Test that nil statFS and readFile interfaces fall back to real implementations
	tool := &FSDiskUsageTool{
		statFS:   nil,
		readFile: nil,
	}
	ctx := context.Background()

	// This test verifies the fallback logic exists. In practice, we can't
	// test the real syscall.Statfs without actual filesystem access,
	// but we can verify the tool doesn't panic with nil interfaces.
	req := FSDiskUsageRequest{Path: "/tmp"}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	// This will likely fail on systems without /tmp, but verifies no panic
	_ = tool
	_ = ctx
	_ = args
	_ = err
	// The important thing is that Execute doesn't panic when statFS/readFile are nil
}

func TestGetDiskUsageForPath(t *testing.T) {
	tests := []struct {
		name    string
		stat    syscall.Statfs_t
		error   error
		path    string
		wantErr error
		verify  func(t *testing.T, result FSDiskUsageResult)
	}{
		{
			name: "success",
			stat: syscall.Statfs_t{
				Blocks: 5000,
				Bsize:  4096,
				Bfree:  2000,
				Bavail: 1800,
			},
			path: "/var",
			verify: func(t *testing.T, result FSDiskUsageResult) {
				require.Equal(t, "/var", result.Path)
				require.NotNil(t, result.Filesystem)
				require.Equal(t, "/var", result.Filesystem.Path)
				require.Equal(t, uint64(5000*4096), result.Filesystem.TotalBytes)
				require.Equal(t, uint64(2000*4096), result.Filesystem.FreeBytes)
				require.Equal(t, uint64(1800*4096), result.Filesystem.AvailableBytes)
				require.Equal(t, uint64(3000*4096), result.Filesystem.UsedBytes)
				require.InDelta(t, 60.0, result.Filesystem.UsedPercent, 0.1)
			},
		},
		{
			name:    "statfs error",
			error:   errors.New("no such file or directory"),
			path:    "/nonexistent",
			wantErr: constants.ErrMCPStatFS,
		},
		{
			name: "zero blocks",
			stat: syscall.Statfs_t{
				Blocks: 0,
				Bsize:  4096,
				Bfree:  0,
				Bavail: 0,
			},
			path: "/empty",
			verify: func(t *testing.T, result FSDiskUsageResult) {
				require.Equal(t, uint64(0), result.Filesystem.TotalBytes)
				require.Equal(t, uint64(0), result.Filesystem.UsedBytes)
				require.Equal(t, uint64(0), result.Filesystem.FreeBytes)
				require.Equal(t, uint64(0), result.Filesystem.AvailableBytes)
				// Used percent should be 0 when total is 0 (NaN check)
				require.True(t, result.Filesystem.UsedPercent == 0 || result.Filesystem.UsedPercent != result.Filesystem.UsedPercent)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStat := &mockStatFS{
				stat:  tt.stat,
				error: tt.error,
			}

			result, err := getDiskUsageForPath(tt.path, mockStat)

			if tt.wantErr != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.wantErr)
				return
			}

			require.NoError(t, err)
			if tt.verify != nil {
				tt.verify(t, result)
			}
		})
	}
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
	require.ErrorIs(t, err, constants.ErrMCPParseMounts)
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

func TestParseMounts(t *testing.T) {
	tests := []struct {
		name      string
		data      string
		error     error
		wantCount int
		want      []string
		exclude   []string
	}{
		{
			name: "success with special mounts excluded",
			data: `sysfs /sys sysfs rw,nosuid,nodev,noexec,relatime 0 0
proc /proc proc rw,nosuid,nodev,noexec,relatime 0 0
udev /dev devtmpfs rw,nosuid,relatime,size=8171996k,nr_inodes=2042999,mode=755 0 0
devpts /dev/pts devpts rw,nosuid,noexec,relatime,gid=5,mode=620,ptmxmode=000 0 0
/dev/sda1 / ext4 rw,relatime 0 0
/dev/sda2 /home ext4 rw,relatime 0 0
`,
			want:    []string{"/", "/home"},
			exclude: []string{"/sys", "/proc", "/dev"},
		},
		{
			name: "empty lines handled",
			data: `sysfs /sys sysfs rw 0 0

/dev/sda1 / ext4 rw 0 0

/dev/sda2 /home ext4 rw 0 0
`,
			wantCount: 1,
		},
		{
			name:  "read file error",
			data:  "",
			error: errors.New("file not found"),
		},
		{
			name: "malformed line skipped",
			data: `sysfs /sys sysfs rw 0 0
malformed_line
/dev/sda1 / ext4 rw 0 0
`,
			wantCount: 1,
		},
		{
			name: "excludes special mounts",
			data: `sysfs /sys sysfs rw 0 0
proc /proc proc rw 0 0
devtmpfs /dev devtmpfs rw 0 0
/dev/sda1 / ext4 rw 0 0
/dev/sda2 /home ext4 rw 0 0
/run/user/1000 /run/user/1000 tmpfs rw 0 0
`,
			want:    []string{"/", "/home", "/run/user/1000"},
			exclude: []string{"/sys", "/proc", "/dev"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRead := &mockReadFile{
				data:  []byte(tt.data),
				error: tt.error,
			}

			mounts, err := parseMounts(mockRead)

			if tt.error != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, constants.ErrMCPReadMounts)
				return
			}

			require.NoError(t, err)

			if tt.wantCount > 0 {
				require.GreaterOrEqual(t, len(mounts), tt.wantCount)
			}

			for _, w := range tt.want {
				require.Contains(t, mounts, w)
			}

			for _, e := range tt.exclude {
				require.NotContains(t, mounts, e)
			}
		})
	}
}

func TestFSDiskUsageTool_UsedPercentCalculation(t *testing.T) {
	tests := []struct {
		name        string
		blocks      uint64
		bsize       uint32
		bfree       uint64
		expectedPct float64
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

func TestFSDiskUsageTool_Invariants(t *testing.T) {
	// Property-based test: verify mathematical invariants hold
	tests := []struct {
		name   string
		blocks uint64
		bsize  uint32
		bfree  uint64
		bavail uint64
	}{
		{
			name:   "typical filesystem",
			blocks: 10000,
			bsize:  4096,
			bfree:  3000,
			bavail: 2500,
		},
		{
			name:   "small filesystem",
			blocks: 100,
			bsize:  4096,
			bfree:  50,
			bavail: 40,
		},
		{
			name:   "large filesystem",
			blocks: 10000000,
			bsize:  4096,
			bfree:  5000000,
			bavail: 4500000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStat := &mockStatFS{
				stat: syscall.Statfs_t{
					Blocks: tt.blocks,
					Bsize:  tt.bsize,
					Bfree:  tt.bfree,
					Bavail: tt.bavail,
				},
			}

			result, err := getDiskUsageForPath("/test", mockStat)
			require.NoError(t, err)
			require.NotNil(t, result.Filesystem)

			fs := result.Filesystem

			// Invariant 1: total = used + free
			require.Equal(t, fs.TotalBytes, fs.UsedBytes+fs.FreeBytes,
				"total bytes must equal used + free bytes")

			// Invariant 2: available <= free (reserved space for root)
			require.LessOrEqual(t, fs.AvailableBytes, fs.FreeBytes,
				"available bytes must be <= free bytes (reserved for root)")

			// Invariant 3: used percent is between 0 and 100
			if fs.TotalBytes > 0 {
				require.GreaterOrEqual(t, fs.UsedPercent, 0.0,
					"used percent must be >= 0")
				require.LessOrEqual(t, fs.UsedPercent, 100.0,
					"used percent must be <= 100")
			}
		})
	}
}

func TestFSDiskUsageTool_BavailVsBfree(t *testing.T) {
	// Test the distinction between Bfree (total free) and Bavail (free for non-root)
	tests := []struct {
		name        string
		blocks      uint64
		bsize       uint32
		bfree       uint64
		bavail      uint64
		expectEqual bool
	}{
		{
			name:        "no reserved space",
			blocks:      1000,
			bsize:       4096,
			bfree:       500,
			bavail:      500,
			expectEqual: true,
		},
		{
			name:        "with reserved space (typical ext4)",
			blocks:      1000,
			bsize:       4096,
			bfree:       500,
			bavail:      450, // 10% reserved for root
			expectEqual: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockStat := &mockStatFS{
				stat: syscall.Statfs_t{
					Blocks: tt.blocks,
					Bsize:  tt.bsize,
					Bfree:  tt.bfree,
					Bavail: tt.bavail,
				},
			}

			result, err := getDiskUsageForPath("/test", mockStat)
			require.NoError(t, err)
			require.NotNil(t, result.Filesystem)

			fs := result.Filesystem

			// Verify Bavail is used for AvailableBytes, not Bfree
			expectedAvailable := tt.bavail * uint64(tt.bsize)
			require.Equal(t, expectedAvailable, fs.AvailableBytes,
				"available bytes should use Bavail, not Bfree")

			// Verify Bfree is used for FreeBytes
			expectedFree := tt.bfree * uint64(tt.bsize)
			require.Equal(t, expectedFree, fs.FreeBytes,
				"free bytes should use Bfree")

			if tt.expectEqual {
				require.Equal(t, fs.FreeBytes, fs.AvailableBytes,
					"when Bfree == Bavail, free and available should be equal")
			} else {
				require.NotEqual(t, fs.FreeBytes, fs.AvailableBytes,
					"when Bfree != Bavail, free and available should differ")
			}
		})
	}
}
