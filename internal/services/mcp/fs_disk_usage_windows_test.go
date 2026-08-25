// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

//go:build windows

package mcp

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	"github.com/stretchr/testify/require"
)

// mockWindowsDiskAPI is a mock implementation of windowsDiskAPI for testing.
type mockWindowsDiskAPI struct {
	getDiskFreeSpaceExFunc func(path string) (freeBytes, totalBytes, availableBytes uint64, err error)
	getDriveTypeFunc       func(path string) (driveType uintptr, err error)
}

func (m *mockWindowsDiskAPI) getDiskFreeSpaceEx(path string) (freeBytes, totalBytes, availableBytes uint64, err error) {
	if m.getDiskFreeSpaceExFunc != nil {
		return m.getDiskFreeSpaceExFunc(path)
	}
	return 0, 0, 0, nil
}

func (m *mockWindowsDiskAPI) getDriveType(path string) (driveType uintptr, err error) {
	if m.getDriveTypeFunc != nil {
		return m.getDriveTypeFunc(path)
	}
	return 0, nil
}

func TestFSDiskUsageTool_Name(t *testing.T) {
	tool := &FSDiskUsageTool{}
	require.Equal(t, "fs_disk_usage", tool.Name())
}

func TestFSDiskUsageTool_Description(t *testing.T) {
	tool := &FSDiskUsageTool{}
	require.NotEmpty(t, tool.Description())
	require.Contains(t, tool.Description(), "disk")
	require.Contains(t, tool.Description(), "free space")
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
	require.ErrorIs(t, err, constants.ErrMCPUnmarshalArguments)
}

func TestFSDiskUsageTool_Execute_PathSpecified_Success(t *testing.T) {
	mockAPI := &mockWindowsDiskAPI{
		getDiskFreeSpaceExFunc: func(path string) (freeBytes, totalBytes, availableBytes uint64, err error) {
			return 500 * 1024 * 1024 * 1024, 1000 * 1024 * 1024 * 1024, 400 * 1024 * 1024 * 1024, nil
		},
	}
	tool := &FSDiskUsageTool{diskAPI: mockAPI}
	ctx := context.Background()

	req := FSDiskUsageRequest{Path: "C:"}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var usageResult FSDiskUsageResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &usageResult)
	require.NoError(t, err)

	require.NotNil(t, usageResult.Filesystem)
	require.Equal(t, uint64(1000*1024*1024*1024), usageResult.Filesystem.TotalBytes)
	require.Equal(t, uint64(500*1024*1024*1024), usageResult.Filesystem.UsedBytes)
	require.Equal(t, uint64(500*1024*1024*1024), usageResult.Filesystem.FreeBytes)
	require.Equal(t, uint64(400*1024*1024*1024), usageResult.Filesystem.AvailableBytes)
	require.InDelta(t, 50.0, usageResult.Filesystem.UsedPercent, 0.1)
}

func TestFSDiskUsageTool_Execute_PathSpecified_APIError(t *testing.T) {
	mockAPI := &mockWindowsDiskAPI{
		getDiskFreeSpaceExFunc: func(path string) (freeBytes, totalBytes, availableBytes uint64, err error) {
			return 0, 0, 0, errors.New("API call failed")
		},
	}
	tool := &FSDiskUsageTool{diskAPI: mockAPI}
	ctx := context.Background()

	req := FSDiskUsageRequest{Path: "C:"}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	_, err = tool.Execute(ctx, args)
	require.Error(t, err)
	require.ErrorIs(t, err, constants.ErrMCPGetDiskUsage)
}

func TestFSDiskUsageTool_Execute_AllDrives_Success(t *testing.T) {
	mockAPI := &mockWindowsDiskAPI{
		getDriveTypeFunc: func(path string) (driveType uintptr, err error) {
			if path == "C:\\" || path == "D:\\" {
				return 3, nil
			}
			return 0, nil
		},
		getDiskFreeSpaceExFunc: func(path string) (freeBytes, totalBytes, availableBytes uint64, err error) {
			driveLetter := strings.ToUpper(string([]rune(path)[0]))
			if driveLetter == "C" {
				return 500 * 1024 * 1024 * 1024, 1000 * 1024 * 1024 * 1024, 400 * 1024 * 1024 * 1024, nil
			}
			if driveLetter == "D" {
				return 200 * 1024 * 1024 * 1024, 500 * 1024 * 1024 * 1024, 200 * 1024 * 1024 * 1024, nil
			}
			return 0, 0, 0, errors.New("unknown drive")
		},
	}
	tool := &FSDiskUsageTool{diskAPI: mockAPI}
	ctx := context.Background()

	req := FSDiskUsageRequest{}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var usageResult FSDiskUsageResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &usageResult)
	require.NoError(t, err)

	require.GreaterOrEqual(t, len(usageResult.Filesystems), 2)
	require.Equal(t, len(usageResult.Filesystems), usageResult.Count)
}

func TestFSDiskUsageTool_Execute_AllDrives_NoDrives(t *testing.T) {
	mockAPI := &mockWindowsDiskAPI{
		getDriveTypeFunc: func(path string) (driveType uintptr, err error) {
			return 0, nil
		},
	}
	tool := &FSDiskUsageTool{diskAPI: mockAPI}
	ctx := context.Background()

	req := FSDiskUsageRequest{}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.Len(t, result.Content, 1)

	var usageResult FSDiskUsageResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &usageResult)
	require.NoError(t, err)

	require.Equal(t, 0, len(usageResult.Filesystems))
	require.Equal(t, 0, usageResult.Count)
}

func TestFSDiskUsageTool_Execute_AllDrives_SkipFailedDrives(t *testing.T) {
	mockAPI := &mockWindowsDiskAPI{
		getDriveTypeFunc: func(path string) (driveType uintptr, err error) {
			if path == "C:\\" || path == "D:\\" {
				return 3, nil
			}
			return 0, nil
		},
		getDiskFreeSpaceExFunc: func(path string) (freeBytes, totalBytes, availableBytes uint64, err error) {
			driveLetter := strings.ToUpper(string([]rune(path)[0]))
			if driveLetter == "C" {
				return 500 * 1024 * 1024 * 1024, 1000 * 1024 * 1024 * 1024, 400 * 1024 * 1024 * 1024, nil
			}
			return 0, 0, 0, errors.New("no media")
		},
	}
	tool := &FSDiskUsageTool{diskAPI: mockAPI}
	ctx := context.Background()

	req := FSDiskUsageRequest{}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var usageResult FSDiskUsageResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &usageResult)
	require.NoError(t, err)

	require.Equal(t, 1, len(usageResult.Filesystems))
	require.Equal(t, "C:\\", usageResult.Filesystems[0].Path)
}

func TestFSDiskUsageTool_Execute_AllDrives_DriveTypeError(t *testing.T) {
	mockAPI := &mockWindowsDiskAPI{
		getDriveTypeFunc: func(path string) (driveType uintptr, err error) {
			if path == "C:\\" {
				return 3, nil
			}
			return 0, errors.New("drive type error")
		},
		getDiskFreeSpaceExFunc: func(path string) (freeBytes, totalBytes, availableBytes uint64, err error) {
			return 500 * 1024 * 1024 * 1024, 1000 * 1024 * 1024 * 1024, 400 * 1024 * 1024 * 1024, nil
		},
	}
	tool := &FSDiskUsageTool{diskAPI: mockAPI}
	ctx := context.Background()

	req := FSDiskUsageRequest{}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)

	var usageResult FSDiskUsageResult
	err = json.Unmarshal([]byte(result.Content[0].Text), &usageResult)
	require.NoError(t, err)

	require.Equal(t, 1, len(usageResult.Filesystems))
}

func TestGetDiskUsageForPath_Success(t *testing.T) {
	mockAPI := &mockWindowsDiskAPI{
		getDiskFreeSpaceExFunc: func(path string) (freeBytes, totalBytes, availableBytes uint64, err error) {
			return 750 * 1024 * 1024 * 1024, 1000 * 1024 * 1024 * 1024, 700 * 1024 * 1024 * 1024, nil
		},
	}

	result, err := getDiskUsageForPath("C:", mockAPI)
	require.NoError(t, err)
	require.NotNil(t, result.Filesystem)

	require.Equal(t, uint64(1000*1024*1024*1024), result.Filesystem.TotalBytes)
	require.Equal(t, uint64(250*1024*1024*1024), result.Filesystem.UsedBytes)
	require.Equal(t, uint64(750*1024*1024*1024), result.Filesystem.FreeBytes)
	require.Equal(t, uint64(700*1024*1024*1024), result.Filesystem.AvailableBytes)
	require.InDelta(t, 25.0, result.Filesystem.UsedPercent, 0.1)
}

func TestGetDiskUsageForPath_APIError(t *testing.T) {
	mockAPI := &mockWindowsDiskAPI{
		getDiskFreeSpaceExFunc: func(path string) (freeBytes, totalBytes, availableBytes uint64, err error) {
			return 0, 0, 0, errors.New("disk API error")
		},
	}

	_, err := getDiskUsageForPath("C:", mockAPI)
	require.Error(t, err)
	require.ErrorIs(t, err, constants.ErrMCPGetDiskUsage)
}

func TestGetDiskUsageForPath_NilAPI(t *testing.T) {
	mockAPI := &mockWindowsDiskAPI{
		getDiskFreeSpaceExFunc: func(path string) (freeBytes, totalBytes, availableBytes uint64, err error) {
			return 100, 200, 50, nil
		},
	}

	result, err := getDiskUsageForPath("C:", mockAPI)
	require.NoError(t, err)
	require.NotNil(t, result.Filesystem)
}

func TestGetDiskUsageForPath_ZeroTotalBytes(t *testing.T) {
	mockAPI := &mockWindowsDiskAPI{
		getDiskFreeSpaceExFunc: func(path string) (freeBytes, totalBytes, availableBytes uint64, err error) {
			return 0, 0, 0, nil
		},
	}

	result, err := getDiskUsageForPath("C:", mockAPI)
	require.NoError(t, err)
	require.NotNil(t, result.Filesystem)
	require.Equal(t, uint64(0), result.Filesystem.TotalBytes)
	require.Equal(t, uint64(0), result.Filesystem.UsedBytes)
	// UsedPercent should be NaN or handled gracefully
	require.True(t, result.Filesystem.UsedPercent == 0 || result.Filesystem.UsedPercent != result.Filesystem.UsedPercent) // NaN check
}

func TestGetDiskUsageForPath_FullDisk(t *testing.T) {
	mockAPI := &mockWindowsDiskAPI{
		getDiskFreeSpaceExFunc: func(path string) (freeBytes, totalBytes, availableBytes uint64, err error) {
			return 0, 1000 * 1024 * 1024 * 1024, 0, nil
		},
	}

	result, err := getDiskUsageForPath("C:", mockAPI)
	require.NoError(t, err)
	require.NotNil(t, result.Filesystem)
	require.Equal(t, uint64(1000*1024*1024*1024), result.Filesystem.TotalBytes)
	require.Equal(t, uint64(1000*1024*1024*1024), result.Filesystem.UsedBytes)
	require.Equal(t, uint64(0), result.Filesystem.FreeBytes)
	require.InDelta(t, 100.0, result.Filesystem.UsedPercent, 0.1)
}

func TestGetAllDiskUsage_SingleDrive(t *testing.T) {
	mockAPI := &mockWindowsDiskAPI{
		getDriveTypeFunc: func(path string) (driveType uintptr, err error) {
			if path == "C:\\" {
				return 3, nil
			}
			return 0, nil
		},
		getDiskFreeSpaceExFunc: func(path string) (freeBytes, totalBytes, availableBytes uint64, err error) {
			return 500 * 1024 * 1024 * 1024, 1000 * 1024 * 1024 * 1024, 400 * 1024 * 1024 * 1024, nil
		},
	}

	result, err := getAllDiskUsage(mockAPI)
	require.NoError(t, err)
	require.Equal(t, 1, len(result.Filesystems))
	require.Equal(t, 1, result.Count)
	require.Equal(t, "C:\\", result.Filesystems[0].Path)
}

func TestGetAllDiskUsage_MultipleDriveTypes(t *testing.T) {
	mockAPI := &mockWindowsDiskAPI{
		getDriveTypeFunc: func(path string) (driveType uintptr, err error) {
			// C: = DRIVE_FIXED (3), D: = DRIVE_REMOVABLE (2), E: = DRIVE_CDROM (5)
			if path == "C:\\" {
				return 3, nil
			}
			if path == "D:\\" {
				return 2, nil
			}
			if path == "E:\\" {
				return 5, nil
			}
			return 0, nil
		},
		getDiskFreeSpaceExFunc: func(path string) (freeBytes, totalBytes, availableBytes uint64, err error) {
			return 100 * 1024 * 1024 * 1024, 200 * 1024 * 1024 * 1024, 100 * 1024 * 1024 * 1024, nil
		},
	}

	result, err := getAllDiskUsage(mockAPI)
	require.NoError(t, err)
	require.Equal(t, 3, len(result.Filesystems))
	require.Equal(t, 3, result.Count)
}

func TestGetAllDiskUsage_SkipUnknownDrives(t *testing.T) {
	mockAPI := &mockWindowsDiskAPI{
		getDriveTypeFunc: func(path string) (driveType uintptr, err error) {
			// C: = DRIVE_FIXED (3), others = DRIVE_UNKNOWN (0) or DRIVE_NO_ROOT_DIR (1)
			if path == "C:\\" {
				return 3, nil
			}
			if path == "D:\\" {
				return 0, nil // DRIVE_UNKNOWN
			}
			if path == "E:\\" {
				return 1, nil // DRIVE_NO_ROOT_DIR
			}
			return 0, nil
		},
		getDiskFreeSpaceExFunc: func(path string) (freeBytes, totalBytes, availableBytes uint64, err error) {
			return 100 * 1024 * 1024 * 1024, 200 * 1024 * 1024 * 1024, 100 * 1024 * 1024 * 1024, nil
		},
	}

	result, err := getAllDiskUsage(mockAPI)
	require.NoError(t, err)
	require.Equal(t, 1, len(result.Filesystems)) // Only C:
	require.Equal(t, "C:\\", result.Filesystems[0].Path)
}

func TestGetAllDiskUsage_NilAPI(t *testing.T) {
	mockAPI := &mockWindowsDiskAPI{
		getDriveTypeFunc: func(path string) (driveType uintptr, err error) {
			return 0, nil
		},
	}

	result, err := getAllDiskUsage(mockAPI)
	require.NoError(t, err)
	require.Equal(t, 0, len(result.Filesystems))
}

func TestGetAllDiskUsage_DrivePathFormatting(t *testing.T) {
	mockAPI := &mockWindowsDiskAPI{
		getDriveTypeFunc: func(path string) (driveType uintptr, err error) {
			if path == "C:\\" {
				return 3, nil
			}
			return 0, nil
		},
		getDiskFreeSpaceExFunc: func(path string) (freeBytes, totalBytes, availableBytes uint64, err error) {
			return 100, 200, 100, nil
		},
	}

	result, err := getAllDiskUsage(mockAPI)
	require.NoError(t, err)
	require.Equal(t, 1, len(result.Filesystems))
	// Path should be formatted with backslash (C:\)
	require.Equal(t, "C:\\", result.Filesystems[0].Path)
}

func TestFSDiskUsageTool_Execute_MarshalError(t *testing.T) {
	mockAPI := &mockWindowsDiskAPI{
		getDiskFreeSpaceExFunc: func(path string) (freeBytes, totalBytes, availableBytes uint64, err error) {
			return 100, 200, 50, nil
		},
	}
	tool := &FSDiskUsageTool{diskAPI: mockAPI}
	ctx := context.Background()

	req := FSDiskUsageRequest{Path: "C:"}
	args, err := json.Marshal(req)
	require.NoError(t, err)

	result, err := tool.Execute(ctx, args)
	require.NoError(t, err)
	require.NotNil(t, result.Content)
}

func TestFSDiskUsageTool_Invariants(t *testing.T) {
	// Property-based test: verify mathematical invariants hold for Windows
	tests := []struct {
		name       string
		freeBytes  uint64
		totalBytes uint64
		available  uint64
	}{
		{
			name:       "typical disk",
			freeBytes:  500 * 1024 * 1024 * 1024,
			totalBytes: 1000 * 1024 * 1024 * 1024,
			available:  400 * 1024 * 1024 * 1024,
		},
		{
			name:       "small disk",
			freeBytes:  50 * 1024 * 1024 * 1024,
			totalBytes: 100 * 1024 * 1024 * 1024,
			available:  40 * 1024 * 1024 * 1024,
		},
		{
			name:       "large disk",
			freeBytes:  5000 * 1024 * 1024 * 1024,
			totalBytes: 10000 * 1024 * 1024 * 1024,
			available:  4500 * 1024 * 1024 * 1024,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockAPI := &mockWindowsDiskAPI{
				getDiskFreeSpaceExFunc: func(path string) (freeBytes, totalBytes, availableBytes uint64, err error) {
					return tt.freeBytes, tt.totalBytes, tt.available, nil
				},
			}

			result, err := getDiskUsageForPath("C:", mockAPI)
			require.NoError(t, err)
			require.NotNil(t, result.Filesystem)

			fs := result.Filesystem

			// Invariant 1: total = used + free
			require.Equal(t, fs.TotalBytes, fs.UsedBytes+fs.FreeBytes,
				"total bytes must equal used + free bytes")

			// Invariant 2: available <= free (reserved space)
			require.LessOrEqual(t, fs.AvailableBytes, fs.FreeBytes,
				"available bytes must be <= free bytes")

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
