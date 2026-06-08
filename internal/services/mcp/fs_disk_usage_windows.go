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

//go:build windows

package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"syscall"
	"unsafe"
)

// FSDiskUsageTool provides df-style free space reporting for Windows.
type FSDiskUsageTool struct{}

// Name returns the tool identifier.
func (t *FSDiskUsageTool) Name() string {
	return "fs_disk_usage"
}

// Description returns a human-readable description.
func (t *FSDiskUsageTool) Description() string {
	return "Provides df-style free space reporting for mounted filesystems."
}

// InputSchema returns the JSON Schema for tool validation.
func (t *FSDiskUsageTool) InputSchema() *InputSchema {
	return &InputSchema{
		Type: "object",
		Properties: map[string]*PropertySchema{
			"path": {
				Type:        "string",
				Description: "Path to check disk usage for (default all mounted filesystems)",
			},
		},
	}
}

// Execute implements the tool logic.
func (t *FSDiskUsageTool) Execute(ctx context.Context, args json.RawMessage) (CallToolResult, error) {
	var req FSDiskUsageRequest
	if err := json.Unmarshal(args, &req); err != nil {
		return CallToolResult{}, fmt.Errorf("fs_disk_usage: unmarshal arguments: %w", err)
	}

	var result FSDiskUsageResult
	var err error

	if req.Path != "" {
		result, err = getDiskUsageForPath(req.Path)
	} else {
		result, err = getAllDiskUsage()
	}

	if err != nil {
		return CallToolResult{}, fmt.Errorf("fs_disk_usage: get disk usage: %w", err)
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("fs_disk_usage: marshal result: %w", err)
	}

	return CallToolResult{
		Content: []TextContent{
			{
				Type: "text",
				Text: string(resultJSON),
			},
		},
	}, nil
}

var (
	modkernel32             = syscall.NewLazyDLL("kernel32.dll")
	procGetDiskFreeSpaceExW = modkernel32.NewProc("GetDiskFreeSpaceExW")
	procGetDriveTypeW       = modkernel32.NewProc("GetDriveTypeW")
)

func getDiskUsageForPath(path string) (FSDiskUsageResult, error) {
	// Convert path to absolute path if not already
	absPath, err := filepath.Abs(path)
	if err != nil {
		return FSDiskUsageResult{}, fmt.Errorf("fs_disk_usage: get absolute path: %w", err)
	}

	// Convert to UTF-16 pointer for Windows API
	pathPtr, err := syscall.UTF16PtrFromString(absPath)
	if err != nil {
		return FSDiskUsageResult{}, fmt.Errorf("fs_disk_usage: convert path to UTF-16: %w", err)
	}

	var freeBytes, totalBytes, availableBytes uint64

	// Call GetDiskFreeSpaceExW
	ret, _, err := procGetDiskFreeSpaceExW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytes)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&availableBytes)),
	)

	if ret == 0 {
		return FSDiskUsageResult{}, fmt.Errorf("fs_disk_usage: GetDiskFreeSpaceExW failed: %w", err)
	}

	usedBytes := totalBytes - freeBytes
	usedPercent := float64(usedBytes) / float64(totalBytes) * 100

	return FSDiskUsageResult{
		Path: absPath,
		Filesystem: &FilesystemInfo{
			Path:           absPath,
			TotalBytes:     totalBytes,
			UsedBytes:      usedBytes,
			FreeBytes:      freeBytes,
			AvailableBytes: availableBytes,
			UsedPercent:    usedPercent,
		},
	}, nil
}

func getAllDiskUsage() (FSDiskUsageResult, error) {
	// Enumerate all logical drives (A: through Z:)
	var filesystems []FilesystemInfo

	for drive := 'A'; drive <= 'Z'; drive++ {
		drivePath := string(drive) + ":\\"
		driveRoot := string(drive) + ":"

		// Check if drive exists using GetDriveType
		driveTypePtr, err := syscall.UTF16PtrFromString(drivePath)
		if err != nil {
			continue
		}
		driveType, _, _ := procGetDriveTypeW.Call(uintptr(unsafe.Pointer(driveTypePtr)))

		// DRIVE_FIXED = 3 (hard drive), DRIVE_REMOVABLE = 2, DRIVE_CDROM = 5
		// We skip drives that don't exist (DRIVE_UNKNOWN = 0, DRIVE_NO_ROOT_DIR = 1)
		if driveType == 0 || driveType == 1 {
			continue
		}

		result, err := getDiskUsageForPath(driveRoot)
		if err != nil {
			// Skip drives that fail (e.g., no media in removable drive)
			continue
		}
		if result.Filesystem != nil {
			// Add drive letter to path for clarity
			result.Filesystem.Path = drivePath
			filesystems = append(filesystems, *result.Filesystem)
		}
	}

	return FSDiskUsageResult{
		Filesystems: filesystems,
		Count:       len(filesystems),
	}, nil
}
