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

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/g8e-ai/g8e/components/g8eo/models"
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
		Status:          constants.ExecutionStatusExecuting,
		Path:            req.Path,
		Pattern:         req.Pattern,
		Matches:         []models.FsGrepMatch{},
	}
	result.StartTime = &startTime

	// Resolve path
	path := req.Path
	if path == "" || path == "." {
		path = s.workDir
	}

	absPath, err := filepath.Abs(path)
	if err != nil {
		return s.failResult(result, "path_resolution_error", fmt.Sprintf("failed to resolve absolute path: %v", err))
	}
	result.Path = absPath

	// Compile regex
	re, err := regexp.Compile(req.Pattern)
	if err != nil {
		return s.failResult(result, "invalid_pattern", fmt.Sprintf("invalid regex pattern: %v", err))
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
		maxMatches = 100
	}
	if maxMatches > 500 {
		maxMatches = 500
	}

	matches := []models.FsGrepMatch{}
	truncated := false

	err = filepath.WalkDir(absPath, func(path string, d os.DirEntry, err error) error {
		if err != nil {
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
				// Skip symlinks we can't resolve
				return nil
			}

			// Check if resolved target is within the base directory
			rel, err := filepath.Rel(absPath, target)
			if err != nil {
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
			rel, _ := filepath.Rel(absPath, path)
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
		return s.failResult(result, "grep_error", fmt.Sprintf("failed to perform grep: %v", err))
	}

	result.Matches = matches
	result.TotalMatches = len(matches)
	result.Truncated = truncated
	result.Status = constants.ExecutionStatusCompleted

	endTime := time.Now().UTC()
	result.EndTime = &endTime
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
		return nil, err
	}
	defer file.Close()

	var matches []models.FsGrepMatch
	scanner := bufio.NewScanner(file)
	// Limit line size to avoid OOM
	buf := make([]byte, 64*1024)
	scanner.Buffer(buf, 1024*1024)

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

func (s *FsGrepService) failResult(result *models.FsGrepResult, errorType, errorMsg string) (*models.FsGrepResult, error) {
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
