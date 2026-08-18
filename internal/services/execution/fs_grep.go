// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package execution

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/security"
)

// FsGrepService handles recursive grep operations
type FsGrepService struct {
	workDir string
	logger  *slog.Logger
}

// NewFsGrepService creates a new FsGrepService
func NewFsGrepService(workDir string, logger *slog.Logger) *FsGrepService {
	return &FsGrepService{
		workDir: workDir,
		logger:  logger,
	}
}

// ExecuteFsGrep performs a recursive grep operation
func (s *FsGrepService) ExecuteFsGrep(ctx context.Context, req *models.FsGrepRequest) (*models.FsGrepResult, error) {
	startTime := time.Now().UTC()
	s.logger.Info("Executing fs_grep operation",
		"path", req.Path,
		"pattern", req.Pattern,
		"max_matches", req.MaxMatches)

	result := &models.FsGrepResult{
		ExecutionID:     req.ExecutionID,
		CaseID:          req.CaseID,
		TaskID:          req.TaskID,
		InvestigationID: req.InvestigationID,
		Status:          operatorv1.ExecutionStatus_EXECUTION_STATUS_EXECUTING,
		Path:            req.Path,
		Pattern:         req.Pattern,
		Matches:         []models.FsGrepMatch{},
	}
	result.StartTime = startTime

	// Resolve path
	path := req.Path
	if path == "" || path == constants.PathCurrentDir {
		path = s.workDir
	}

	// Validate and resolve path (security check)
	absPath, err := security.ValidatePath(path, s.workDir)
	if err != nil {
		return s.failResult(result, constants.ErrFsGrepValidation, fmt.Errorf("fs_grep: path validation failed: %w", err))
	}

	result.Path = absPath

	// Compile regex
	re, err := regexp.Compile(req.Pattern)
	if err != nil {
		return s.failResult(result, constants.ErrFsGrepInvalidPattern, fmt.Errorf("fs_grep: invalid regex pattern: %w", err))
	}

	// Prepare includes filters
	var includePatterns []*regexp.Regexp
	for _, inc := range req.Includes {
		// Convert glob-ish patterns to regex if needed, or just use simple string matching
		// For simplicity, we'll treat them as substrings or basic globs
		p := strings.ReplaceAll(inc, ".", "\\.")
		p = strings.ReplaceAll(p, "*", ".*")
		ir, err := regexp.Compile("^" + p + "$")
		if err == nil {
			includePatterns = append(includePatterns, ir)
		}
	}

	maxMatches := req.MaxMatches
	if maxMatches <= 0 {
		maxMatches = constants.FsGrepDefaultMaxMatches
	}
	if maxMatches > constants.FsGrepMaxMatches {
		maxMatches = constants.FsGrepMaxMatches
	}

	matches := []models.FsGrepMatch{}
	truncated := false

	err = filepath.WalkDir(absPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			s.logger.Debug("Skipping inaccessible path during walk", "path", path, "error", err)
			return nil // Skip files we can't access
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Security: Check for symlinks and enforce boundary constraints
		info, err := d.Info()
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			// Resolve symlink target
			target, err := filepath.EvalSymlinks(path)
			if err != nil {
				s.logger.Debug("Skipping symlink that cannot be resolved", "symlink", path, "error", err)
				// Skip symlinks we can't resolve
				return nil
			}

			// Check if resolved target is within the base directory
			rel, err := filepath.Rel(absPath, target)
			if err != nil {
				s.logger.Debug("Skipping symlink with unresolvable relative path", "symlink", path, "target", target, "error", err)
				// Skip symlinks we can't resolve relative path
				return nil
			}

			// If the relative path starts with "..", it points outside the base directory
			if strings.HasPrefix(rel, "..") {
				s.logger.Warn("Skipping symlink that points outside search boundary",
					"symlink", path,
					"target", target,
					"base", absPath)
				return nil
			}
		}

		if d.IsDir() {
			// Skip hidden directories (like .git)
			if d.Name() != "." && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}

		// Apply include filters if any
		if len(includePatterns) > 0 {
			matched := false
			rel, err := filepath.Rel(absPath, path)
			if err != nil {
				s.logger.Debug("Failed to get relative path for include filter", "path", path, "error", err)
				return nil
			}
			for _, ip := range includePatterns {
				if ip.MatchString(rel) || ip.MatchString(d.Name()) {
					matched = true
					break
				}
			}
			if !matched {
				return nil
			}
		}

		// Search in file
		fileMatches, err := s.searchInFile(path, re, maxMatches-len(matches))
		if err != nil {
			s.logger.Debug("Skipping file that cannot be read", "path", path, "error", err)
			return nil // Skip files we can't read
		}

		matches = append(matches, fileMatches...)

		if len(matches) >= maxMatches {
			truncated = true
			return io.EOF // Stop walking
		}

		return nil
	})

	if err != nil && err != io.EOF {
		return s.failResult(result, constants.ErrFsGrepExecution, fmt.Errorf("fs_grep: grep execution failed: %w", err))
	}

	result.Matches = matches
	result.TotalMatches = len(matches)
	result.Truncated = truncated
	result.Status = operatorv1.ExecutionStatus_EXECUTION_STATUS_COMPLETED

	endTime := time.Now().UTC()
	result.EndTime = endTime
	result.DurationSeconds = endTime.Sub(startTime).Seconds()

	s.logger.Info("fs_grep operation completed",
		"path", absPath,
		"matches", len(matches),
		"truncated", truncated,
		"duration_ms", result.DurationSeconds*1000)

	return result, nil
}

func (s *FsGrepService) searchInFile(path string, re *regexp.Regexp, limit int) ([]models.FsGrepMatch, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("fs_grep: failed to open file %s: %w", path, constants.ErrFileOpenFailed)
	}
	defer file.Close()

	var matches []models.FsGrepMatch
	scanner := bufio.NewScanner(file)
	// Limit line size to avoid OOM
	buf := make([]byte, constants.FsGrepScannerInitialBufSize)
	scanner.Buffer(buf, constants.FsGrepScannerMaxBufSize)

	lineNum := 0
	for scanner.Scan() {
		lineNum++
		text := scanner.Text()
		if re.MatchString(text) {
			matches = append(matches, models.FsGrepMatch{
				Path:       path,
				LineNumber: lineNum,
				Content:    text,
			})
			if len(matches) >= limit {
				break
			}
		}
	}

	return matches, scanner.Err()
}

func (s *FsGrepService) failResult(result *models.FsGrepResult, errType error, err error) (*models.FsGrepResult, error) {
	result.Status = operatorv1.ExecutionStatus_EXECUTION_STATUS_FAILED
	result.ErrorType = errType.Error()
	result.ErrorMessage = err.Error()
	endTime := time.Now().UTC()
	result.EndTime = endTime
	if !result.StartTime.IsZero() {
		result.DurationSeconds = endTime.Sub(result.StartTime).Seconds()
	}
	return result, err
}
