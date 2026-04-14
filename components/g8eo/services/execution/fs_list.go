//go:build unix || linux || darwin

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

package execution

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
)

// FsListService handles file system listing operations using readdirplus-style metadata
type FsListService struct {
	workDir string
	logger  *slog.Logger
}

// NewFsListService creates a new FsListService.
// workDir is the operator's working directory used as the default path when none is specified.
func NewFsListService(workDir string, logger *slog.Logger) *FsListService {
	return &FsListService{
		workDir: workDir,
		logger:  logger,
	}
}

// ExecuteFsList performs a directory listing with readdirplus-style metadata
func (s *FsListService) ExecuteFsList(ctx context.Context, req *models.FsListRequest) (*models.FsListResult, error) {
	startTime := time.Now().UTC()
	s.logger.Info("Executing fs_list operation",
		"path", req.Path,
		"max_depth", req.MaxDepth,
		"max_entries", req.MaxEntries)

	result := &models.FsListResult{
		ExecutionID:     req.ExecutionID,
		CaseID:          req.CaseID,
		TaskID:          req.TaskID,
		InvestigationID: req.InvestigationID,
		Status:          constants.ExecutionStatusExecuting,
		Entries:         []models.FsListEntry{},
	}
	result.StartTime = &startTime

	// Resolve path — default to operator's working directory when none is specified
	path := req.Path
	if path == "" || path == "." {
		path = s.workDir
	}

	// Make path absolute
	absPath, err := filepath.Abs(path)
	if err != nil {
		return s.failResult(result, "path_resolution_error", fmt.Sprintf("failed to resolve absolute path: %v", err))
	}
	result.Path = absPath

	// Validate path exists and is a directory
	info, err := os.Stat(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return s.failResult(result, "path_not_found", fmt.Sprintf("path does not exist: %s", absPath))
		}
		return s.failResult(result, "stat_error", fmt.Sprintf("failed to stat path: %v", err))
	}
	if !info.IsDir() {
		return s.failResult(result, "not_a_directory", fmt.Sprintf("path is not a directory: %s", absPath))
	}

	// Apply limits
	maxDepth := req.MaxDepth
	if maxDepth < 0 {
		maxDepth = 0
	}
	if maxDepth > 3 {
		maxDepth = 3
	}

	maxEntries := req.MaxEntries
	if maxEntries <= 0 {
		maxEntries = 100
	}
	if maxEntries > 500 {
		maxEntries = 500
	}

	// Perform directory listing with readdirplus
	entries, truncated, err := s.listDirectory(ctx, absPath, maxDepth, maxEntries, 0)
	if err != nil {
		return s.failResult(result, "list_error", fmt.Sprintf("failed to list directory: %v", err))
	}

	result.Entries = entries
	result.TotalCount = len(entries)
	result.Truncated = truncated
	result.Status = constants.ExecutionStatusCompleted

	endTime := time.Now().UTC()
	result.EndTime = &endTime
	result.DurationSeconds = endTime.Sub(startTime).Seconds()

	s.logger.Info("fs_list operation completed",
		"path", absPath,
		"entries", len(entries),
		"truncated", truncated,
		"duration_ms", result.DurationSeconds*1000)

	return result, nil
}

// listDirectory recursively lists directory contents with readdirplus-style metadata
func (s *FsListService) listDirectory(ctx context.Context, dirPath string, maxDepth, maxEntries, currentDepth int) ([]models.FsListEntry, bool, error) {
	select {
	case <-ctx.Done():
		return nil, false, ctx.Err()
	default:
	}

	entries := []models.FsListEntry{}
	truncated := false

	// Open directory
	dir, err := os.Open(dirPath)
	if err != nil {
		return nil, false, fmt.Errorf("failed to open directory: %w", err)
	}
	defer dir.Close()

	// SECURITY: Use Readdir in chunks to prevent OOM on massive directories.
	// Instead of Readdir(-1), we read up to maxEntries in smaller batches.
	const batchSize = 100

	for {
		fileInfos, err := dir.Readdir(batchSize)
		if err != nil {
			if err.Error() == "EOF" || len(fileInfos) == 0 {
				break
			}
			return nil, false, fmt.Errorf("failed to read directory: %w", err)
		}

		for _, fi := range fileInfos {
			if len(entries) >= maxEntries {
				truncated = true
				break
			}

			entryPath := filepath.Join(dirPath, fi.Name())
			entry := s.buildEntry(fi, entryPath)
			entries = append(entries, entry)

			// Recurse into subdirectories if depth allows
			if fi.IsDir() && currentDepth < maxDepth {
				subEntries, subTruncated, err := s.listDirectory(ctx, entryPath, maxDepth, maxEntries-len(entries), currentDepth+1)
				if err != nil {
					s.logger.Warn("Failed to list subdirectory", "error", err, "path", entryPath)
					continue
				}
				entries = append(entries, subEntries...)
				if subTruncated {
					truncated = true
					break
				}
			}
		}

		if truncated {
			break
		}
	}

	return entries, truncated, nil
}

// buildEntry creates an FsListEntry with readdirplus-style metadata
func (s *FsListService) buildEntry(fi os.FileInfo, fullPath string) models.FsListEntry {
	entry := models.FsListEntry{
		Name:    fi.Name(),
		Path:    fullPath,
		IsDir:   fi.IsDir(),
		Size:    fi.Size(),
		Mode:    fmt.Sprintf("%04o", fi.Mode().Perm()),
		ModTime: fi.ModTime().Unix(),
	}

	// Check for symlink
	if fi.Mode()&os.ModeSymlink != 0 {
		entry.IsSymlink = true
		if target, err := os.Readlink(fullPath); err == nil {
			entry.SymlinkTarget = &target
		}
	}

	// Get extended attributes from syscall.Stat_t (Unix-specific)
	if sys := fi.Sys(); sys != nil {
		if stat, ok := sys.(*syscall.Stat_t); ok {
			entry.Inode = stat.Ino
			entry.Nlink = uint64(stat.Nlink)

			// Get owner/group names
			if owner := getUsername(stat.Uid); owner != "" {
				entry.Owner = &owner
			}
			if group := getGroupname(stat.Gid); group != "" {
				entry.Group = &group
			}
		}
	}

	return entry
}

// failResult sets error state on result
func (s *FsListService) failResult(result *models.FsListResult, errorType, errorMsg string) (*models.FsListResult, error) {
	result.Status = constants.ExecutionStatusFailed
	result.ErrorType = &errorType
	result.ErrorMessage = &errorMsg
	endTime := time.Now().UTC()
	result.EndTime = &endTime
	if result.StartTime != nil {
		result.DurationSeconds = endTime.Sub(*result.StartTime).Seconds()
	}
	return result, nil
}
