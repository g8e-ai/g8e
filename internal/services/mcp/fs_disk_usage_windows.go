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
	"fmt"
	"path/filepath"
	"syscall"
	"unsafe"

	"github.com/g8e-ai/g8e/internal/constants"
)

// FSDiskUsageTool provides df-style free space reporting for Windows.
type FSDiskUsageTool struct {
	diskAPI windowsDiskAPI
}

// Name returns the tool identifier.
func (t *FSDiskUsageTool) Name() string {
	return "fs_disk_usage"
}

// Description returns a human-readable description.
func (t *FSDiskUsageTool) Description() string {
	return "Provides df-style disk free space reporting for mounted filesystems."
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
		return CallToolResult{}, fmt.Errorf("fs_disk_usage: %w", constants.ErrMCPUnmarshalArguments)
	}

	var result FSDiskUsageResult
	var err error

	diskAPI := t.diskAPI
	if diskAPI == nil {
		diskAPI = defaultDiskAPI
	}

	if req.Path != "" {
		result, err = getDiskUsageForPath(req.Path, diskAPI)
	} else {
		result, err = getAllDiskUsage(diskAPI)
	}

	if err != nil {
		return CallToolResult{}, fmt.Errorf("fs_disk_usage: %w", constants.ErrMCPGetDiskUsage)
	}

	resultJSON, err := json.Marshal(result)
	if err != nil {
		return CallToolResult{}, fmt.Errorf("fs_disk_usage: %w", constants.ErrMCPMarshalResult)
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

// windowsDiskAPI provides an interface for Windows disk-related API calls.
type windowsDiskAPI interface {
	getDiskFreeSpaceEx(path string) (freeBytes, totalBytes, availableBytes uint64, err error)
	getDriveType(path string) (driveType uintptr, err error)
}

// realWindowsDiskAPI implements windowsDiskAPI using actual Windows system calls.
type realWindowsDiskAPI struct{}

func (r *realWindowsDiskAPI) getDiskFreeSpaceEx(path string) (freeBytes, totalBytes, availableBytes uint64, err error) {
	pathPtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, 0, 0, err
	}

	ret, _, err := procGetDiskFreeSpaceExW.Call(
		uintptr(unsafe.Pointer(pathPtr)),
		uintptr(unsafe.Pointer(&freeBytes)),
		uintptr(unsafe.Pointer(&totalBytes)),
		uintptr(unsafe.Pointer(&availableBytes)),
	)

	if ret == 0 {
		return 0, 0, 0, err
	}
	return freeBytes, totalBytes, availableBytes, nil
}

func (r *realWindowsDiskAPI) getDriveType(path string) (uintptr, error) {
	driveTypePtr, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	driveType, _, _ := procGetDriveTypeW.Call(uintptr(unsafe.Pointer(driveTypePtr)))
	return driveType, nil
}

var (
	modkernel32             = syscall.NewLazyDLL("kernel32.dll")
	procGetDiskFreeSpaceExW = modkernel32.NewProc("GetDiskFreeSpaceExW")
	procGetDriveTypeW       = modkernel32.NewProc("GetDriveTypeW")

	// Default Windows API implementation
	defaultDiskAPI windowsDiskAPI = &realWindowsDiskAPI{}
)

func getDiskUsageForPath(path string, api windowsDiskAPI) (FSDiskUsageResult, error) {
	if api == nil {
		api = defaultDiskAPI
	}

	// Convert path to absolute path if not already
	absPath, err := filepath.Abs(path)
	if err != nil {
		return FSDiskUsageResult{}, fmt.Errorf("fs_disk_usage: %w: %w", constants.ErrMCPGetAbsolutePath, err)
	}

	freeBytes, totalBytes, availableBytes, err := api.getDiskFreeSpaceEx(absPath)
	if err != nil {
		return FSDiskUsageResult{}, fmt.Errorf("fs_disk_usage: %w: %w", constants.ErrMCPGetDiskUsage, err)
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

func getAllDiskUsage(api windowsDiskAPI) (FSDiskUsageResult, error) {
	if api == nil {
		api = defaultDiskAPI
	}

	// Enumerate all logical drives (A: through Z:)
	var filesystems []FilesystemInfo

	for drive := 'A'; drive <= 'Z'; drive++ {
		drivePath := string(drive) + ":\\"
		driveRoot := string(drive) + ":"

		// Check if drive exists using GetDriveType
		driveType, err := api.getDriveType(drivePath)
		if err != nil {
			continue
		}

		// DRIVE_FIXED = 3 (hard drive), DRIVE_REMOVABLE = 2, DRIVE_CDROM = 5
		// We skip drives that don't exist (DRIVE_UNKNOWN = 0, DRIVE_NO_ROOT_DIR = 1)
		if driveType == 0 || driveType == 1 {
			continue
		}

		result, err := getDiskUsageForPath(driveRoot, api)
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
