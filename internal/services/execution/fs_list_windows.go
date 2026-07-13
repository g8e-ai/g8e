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
// +build windows

package execution

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/security"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

// FsListService handles file system listing operations on Windows
type FsListService struct {
	workDir string
	logger  *slog.Logger
}

// NewFsListService creates a new FsListService for Windows
func NewFsListService(workDir string, logger *slog.Logger) *FsListService {
	return &FsListService{
		workDir: workDir,
		logger:  logger,
	}
}

// ExecuteFsList performs a directory listing on Windows
func (s *FsListService) ExecuteFsList(ctx context.Context, req *models.FsListRequest) (*models.FsListResult, error) {
	startTime := time.Now().UTC()
	s.logger.Info("Executing fs_list operation (Windows)",
		"path", req.Path,
		"max_depth", req.MaxDepth,
		"max_entries", req.MaxEntries)

	result := &models.FsListResult{
		ExecutionID:     req.ExecutionID,
		CaseID:          req.CaseID,
		TaskID:          req.TaskID,
		InvestigationID: req.InvestigationID,
		Status:          operatorv1.ExecutionStatus_EXECUTION_STATUS_EXECUTING,
		Entries:         []models.FsListEntry{},
	}
	result.StartTime = startTime

	// Resolve path - default to operator's working directory when none is specified
	path := req.Path
	if path == "" || path == "." {
		path = s.workDir
	}

	// Validate path exists and is a directory before security validation
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return s.failResult(result, constants.ErrPathNotFound, fmt.Sprintf("path does not exist: %s", path))
		}
		return s.failResult(result, constants.ErrStatFailed, fmt.Sprintf("failed to stat path: %v", err))
	}
	if !info.IsDir() {
		return s.failResult(result, constants.ErrNotADirectory, fmt.Sprintf("path is not a directory: %s", path))
	}

	// Validate and resolve path (security check) - only after confirming it exists
	absPath, err := security.ValidatePath(path, s.workDir)
	if err != nil {
		return s.failResult(result, constants.ErrPathValidation, fmt.Sprintf("invalid path: %v", err))
	}

	result.Path = absPath

	// Apply limits
	maxEntries := req.MaxEntries
	if maxEntries <= 0 {
		maxEntries = constants.FsListDefaultEntries
	}

	// List directory contents
	entries, err := os.ReadDir(absPath)
	if err != nil {
		return s.failResult(result, constants.ErrDirectoryRead, fmt.Sprintf("failed to read directory: %v", err))
	}

	// Build entry list
	entryCount := 0
	for _, entry := range entries {
		if entryCount >= maxEntries {
			break
		}

		entryInfo, err := entry.Info()
		if err != nil {
			continue
		}

		fsEntry := models.FsListEntry{
			Name:  entry.Name(),
			IsDir: entry.IsDir(),
			Size:  entryInfo.Size(),
			Mode:  entryInfo.Mode().String(),
		}

		result.Entries = append(result.Entries, fsEntry)
		entryCount++
	}

	// Mark as successful
	endTime := time.Now().UTC()
	result.EndTime = endTime
	result.DurationSeconds = endTime.Sub(startTime).Seconds()
	result.Status = operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED

	return result, nil
}

// failResult creates a failed result with error information
func (s *FsListService) failResult(result *models.FsListResult, err error, errorMessage string) (*models.FsListResult, error) {
	endTime := time.Now().UTC()
	result.EndTime = endTime
	if !result.StartTime.IsZero() {
		result.DurationSeconds = endTime.Sub(result.StartTime).Seconds()
	}
	result.Status = operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED
	result.ErrorType = err.Error()
	result.ErrorMessage = errorMessage
	return result, nil
}
