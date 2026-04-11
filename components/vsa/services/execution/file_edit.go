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
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/g8e-ai/g8e/components/vsa/config"
	"github.com/g8e-ai/g8e/components/vsa/constants"
	"github.com/g8e-ai/g8e/components/vsa/models"
)

// FileEditService handles file editing operations with modern best practices
type FileEditService struct {
	config *config.Config
	logger *slog.Logger
}

const (
	// maxFileOperationSize is the maximum file size we'll read into memory (50MB)
	maxFileOperationSize = 50 * 1024 * 1024
)

// NewFileEditService creates a new file editing service
func NewFileEditService(cfg *config.Config, logger *slog.Logger) *FileEditService {
	fes := &FileEditService{
		config: cfg,
		logger: logger,
	}

	fes.logger.Info("File operations ready")
	return fes
}

// ExecuteFileEdit performs a file editing operation
func (fes *FileEditService) ExecuteFileEdit(ctx context.Context, request *models.FileEditRequest) (*models.FileEditResult, error) {
	fes.logger.Info("Executing file edit operation",
		"execution_id", request.ExecutionID,
		"operation", request.Operation,
		"file_path", request.FilePath)

	startTime := time.Now().UTC()
	result := &models.FileEditResult{
		ExecutionID:     request.ExecutionID,
		CaseID:          request.CaseID,
		TaskID:          request.TaskID,
		InvestigationID: request.InvestigationID,
		Operation:       request.Operation,
		FilePath:        request.FilePath,
		Status:          constants.ExecutionStatusExecuting,
		StartTime:       &startTime,
	}

	// Validate file path (security check)
	if err := fes.validateFilePath(request.FilePath); err != nil {
		result.Status = constants.ExecutionStatusFailed
		errMsg := fmt.Sprintf("Invalid file path: %v", err)
		result.ErrorMessage = &errMsg
		errType := "validation_error"
		result.ErrorType = &errType
		fes.finalizeResult(result)
		return result, fmt.Errorf("invalid file path: %w", err)
	}

	// Execute operation based on type
	var err error
	switch request.Operation {
	case models.FileEditOperationRead:
		err = fes.executeRead(ctx, request, result)
	case models.FileEditOperationWrite:
		err = fes.executeWrite(ctx, request, result)
	case models.FileEditOperationReplace:
		err = fes.executeReplace(ctx, request, result)
	case models.FileEditOperationInsert:
		err = fes.executeInsert(ctx, request, result)
	case models.FileEditOperationDelete:
		err = fes.executeDelete(ctx, request, result)
	case models.FileEditOperationPatch:
		err = fes.executePatch(ctx, request, result)
	default:
		err = fmt.Errorf("unsupported operation: %s", request.Operation)
	}

	if err != nil {
		if result.Status == constants.ExecutionStatusExecuting {
			result.Status = constants.ExecutionStatusFailed
			errMsg := err.Error()
			result.ErrorMessage = &errMsg
			errType := "execution_error"
			result.ErrorType = &errType
		}
	} else {
		result.Status = constants.ExecutionStatusCompleted
	}

	// Finalize result
	fes.finalizeResult(result)

	logArgs := []any{
		"execution_id", request.ExecutionID,
		"operation", request.Operation,
		"status", result.Status,
		"duration_seconds", result.DurationSeconds,
	}

	// Include error details if present
	if result.ErrorMessage != nil {
		logArgs = append(logArgs, "error_message", *result.ErrorMessage)
	}
	if result.ErrorType != nil {
		logArgs = append(logArgs, "error_type", *result.ErrorType)
	}

	fes.logger.Info("File edit operation completed", logArgs...)

	return result, nil
}

// validateFilePath ensures the file path is safe and valid
func (fes *FileEditService) validateFilePath(filePath string) error {
	// Resolve absolute path
	absPath, err := filepath.Abs(filePath)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path: %w", err)
	}

	// Check for path traversal attempts (basic security)
	if strings.Contains(absPath, "..") {
		return fmt.Errorf("path traversal detected")
	}

	// Ensure path is not empty
	if absPath == "" {
		return fmt.Errorf("empty file path")
	}

	fes.logger.Info("File path validated", "absolute_path", absPath)
	return nil
}

// executeRead reads a file or file section
func (fes *FileEditService) executeRead(ctx context.Context, request *models.FileEditRequest, result *models.FileEditResult) error {
	fes.logger.Info("Reading file", "file_path", request.FilePath)

	// Check if file exists
	fileInfo, err := os.Stat(request.FilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("file does not exist: %s", request.FilePath)
		}
		return fmt.Errorf("failed to stat file: %w", err)
	}

	// Collect file stats if requested
	if request.ReadOptions != nil && request.ReadOptions.IncludeStats {
		stats, err := fes.collectFileStats(request.FilePath, fileInfo)
		if err != nil {
			fes.logger.Warn("Failed to collect file stats", "error", err)
		} else {
			result.FileStats = stats
		}
	}

	// Read file content
	file, err := os.Open(request.FilePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Handle line-based reading
	if request.ReadOptions != nil && (request.ReadOptions.StartLine != nil || request.ReadOptions.EndLine != nil || request.ReadOptions.MaxLines != nil) {
		content, err := fes.readFileLines(file, request.ReadOptions)
		if err != nil {
			return fmt.Errorf("failed to read file lines: %w", err)
		}
		result.Content = &content
	} else {
		// Read entire file with limit
		fileInfo, err := file.Stat()
		if err != nil {
			return fmt.Errorf("failed to stat file: %w", err)
		}

		if fileInfo.Size() > maxFileOperationSize {
			return fmt.Errorf("file too large to read: %d bytes (max %d)", fileInfo.Size(), maxFileOperationSize)
		}

		var buf bytes.Buffer
		_, err = io.Copy(&buf, io.LimitReader(file, maxFileOperationSize))
		if err != nil {
			return fmt.Errorf("failed to read file: %w", err)
		}
		content := buf.String()
		result.Content = &content
	}

	fes.logger.Info("File read successfully",
		"file_path", request.FilePath,
		"content_size", len(*result.Content))

	return nil
}

// readFileLines reads specific lines from a file
func (fes *FileEditService) readFileLines(file *os.File, opts *models.FileReadOptions) (string, error) {
	scanner := bufio.NewScanner(file)
	var lines []string
	lineNum := 1

	startLine := 1
	if opts.StartLine != nil {
		startLine = *opts.StartLine
	}

	endLine := -1
	if opts.EndLine != nil {
		endLine = *opts.EndLine
	}

	maxLines := -1
	if opts.MaxLines != nil {
		maxLines = *opts.MaxLines
	}

	for scanner.Scan() {
		if lineNum >= startLine {
			if endLine > 0 && lineNum > endLine {
				break
			}
			if maxLines > 0 && len(lines) >= maxLines {
				break
			}
			lines = append(lines, scanner.Text())
		}
		lineNum++
	}

	if err := scanner.Err(); err != nil {
		return "", err
	}

	return strings.Join(lines, "\n"), nil
}

// executeWrite writes content to a file (overwrites existing content)
func (fes *FileEditService) executeWrite(ctx context.Context, request *models.FileEditRequest, result *models.FileEditResult) error {
	if request.Content == nil {
		return fmt.Errorf("content is required for write operation")
	}

	fes.logger.Info("Writing to file", "file_path", request.FilePath)

	// Check if file exists
	fileInfo, err := os.Stat(request.FilePath)
	fileExists := err == nil

	if !fileExists && !request.CreateIfMissing {
		return fmt.Errorf("file does not exist and create_if_missing is false")
	}

	// Create backup if requested and file exists
	if fileExists && request.CreateBackup {
		if fileInfo.Size() > maxFileOperationSize {
			return fmt.Errorf("file too large to backup: %d bytes (max %d)", fileInfo.Size(), maxFileOperationSize)
		}
		backupPath, err := fes.createBackup(request.FilePath)
		if err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
		result.BackupPath = &backupPath
		fes.logger.Info("Backup created", "backup_path", backupPath)
	}

	// Ensure parent directory exists
	dir := filepath.Dir(request.FilePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create parent directory: %w", err)
	}

	// Write content to file
	bytesWritten := int64(len(*request.Content))
	if err := os.WriteFile(request.FilePath, []byte(*request.Content), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	result.BytesWritten = &bytesWritten

	// Count lines
	lines := strings.Count(*request.Content, "\n") + 1
	result.LinesChanged = &lines

	fes.logger.Info("File written successfully",
		"file_path", request.FilePath,
		"bytes_written", bytesWritten,
		"lines", lines)

	return nil
}

// executeReplace replaces old content with new content in a file
func (fes *FileEditService) executeReplace(ctx context.Context, request *models.FileEditRequest, result *models.FileEditResult) error {
	if request.OldContent == nil || request.NewContent == nil {
		return fmt.Errorf("old_content and new_content are required for replace operation")
	}

	fes.logger.Info("Replacing content in file", "file_path", request.FilePath)

	// Read current file content (always fresh read)
	fileInfo, err := os.Stat(request.FilePath)
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}
	if fileInfo.Size() > maxFileOperationSize {
		return fmt.Errorf("file too large to edit: %d bytes (max %d)", fileInfo.Size(), maxFileOperationSize)
	}

	content, err := os.ReadFile(request.FilePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Create backup if requested
	if request.CreateBackup {
		backupPath, err := fes.createBackup(request.FilePath)
		if err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
		result.BackupPath = &backupPath
	}

	// Perform replacement
	oldStr := *request.OldContent
	newStr := *request.NewContent
	originalContent := string(content)

	// Check if old content exists in file (exact match required)
	if !strings.Contains(originalContent, oldStr) {
		preview := oldStr
		if len(preview) > 100 {
			preview = preview[:100] + "..."
		}
		// Provide actionable error message to help AI recover
		return fmt.Errorf("REPLACE FAILED: old_content not found (exact match required). "+
			"You must READ the file first and copy the exact text including whitespace. "+
			"Do NOT guess or retry with variations. Use operation='read' on this file, "+
			"then copy the exact content from the read result. Searched for: %q", preview)
	}

	// Replace content (exact match found) - replace all occurrences
	originalContent = strings.ReplaceAll(originalContent, oldStr, newStr)

	// Write back to file
	bytesWritten := int64(len(originalContent))
	if err := os.WriteFile(request.FilePath, []byte(originalContent), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	result.BytesWritten = &bytesWritten

	// Count changed lines (approximation)
	linesChanged := strings.Count(oldStr, "\n") + strings.Count(newStr, "\n")
	result.LinesChanged = &linesChanged

	fes.logger.Info("Content replaced successfully",
		"file_path", request.FilePath,
		"bytes_written", bytesWritten,
		"lines_changed", linesChanged)

	return nil
}

// executeInsert inserts content at a specific line
func (fes *FileEditService) executeInsert(ctx context.Context, request *models.FileEditRequest, result *models.FileEditResult) error {
	if request.InsertContent == nil || request.InsertPosition == nil {
		return fmt.Errorf("insert_content and insert_position are required for insert operation")
	}

	fes.logger.Info("Inserting content into file",
		"file_path", request.FilePath,
		"position", *request.InsertPosition)

	// Read current file content
	fileInfo, err := os.Stat(request.FilePath)
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}
	if fileInfo.Size() > maxFileOperationSize {
		return fmt.Errorf("file too large to edit: %d bytes (max %d)", fileInfo.Size(), maxFileOperationSize)
	}

	content, err := os.ReadFile(request.FilePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Create backup if requested
	if request.CreateBackup {
		backupPath, err := fes.createBackup(request.FilePath)
		if err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
		result.BackupPath = &backupPath
	}

	// Split content into lines
	lines := strings.Split(string(content), "\n")
	insertPos := *request.InsertPosition - 1 // Convert to 0-indexed

	if insertPos < 0 || insertPos > len(lines) {
		return fmt.Errorf("insert position out of range: %d (file has %d lines)", *request.InsertPosition, len(lines))
	}

	// Insert new content
	insertLines := strings.Split(*request.InsertContent, "\n")
	newLines := append(lines[:insertPos], append(insertLines, lines[insertPos:]...)...)
	newContent := strings.Join(newLines, "\n")

	// Write back to file
	bytesWritten := int64(len(newContent))
	if err := os.WriteFile(request.FilePath, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	result.BytesWritten = &bytesWritten
	linesChanged := len(insertLines)
	result.LinesChanged = &linesChanged

	fes.logger.Info("Content inserted successfully",
		"file_path", request.FilePath,
		"bytes_written", bytesWritten,
		"lines_inserted", linesChanged)

	return nil
}

// executeDelete deletes lines from a file
func (fes *FileEditService) executeDelete(ctx context.Context, request *models.FileEditRequest, result *models.FileEditResult) error {
	if request.StartLine == nil || request.EndLine == nil {
		return fmt.Errorf("start_line and end_line are required for delete operation")
	}

	fes.logger.Info("Deleting lines from file",
		"file_path", request.FilePath,
		"start_line", *request.StartLine,
		"end_line", *request.EndLine)

	// Read current file content
	fileInfo, err := os.Stat(request.FilePath)
	if err != nil {
		return fmt.Errorf("failed to stat file: %w", err)
	}
	if fileInfo.Size() > maxFileOperationSize {
		return fmt.Errorf("file too large to edit: %d bytes (max %d)", fileInfo.Size(), maxFileOperationSize)
	}

	content, err := os.ReadFile(request.FilePath)
	if err != nil {
		return fmt.Errorf("failed to read file: %w", err)
	}

	// Create backup if requested
	if request.CreateBackup {
		backupPath, err := fes.createBackup(request.FilePath)
		if err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
		result.BackupPath = &backupPath
	}

	// Split content into lines
	lines := strings.Split(string(content), "\n")
	startLine := *request.StartLine - 1 // Convert to 0-indexed
	endLine := *request.EndLine - 1     // Convert to 0-indexed

	if startLine < 0 || endLine >= len(lines) || startLine > endLine {
		return fmt.Errorf("invalid line range: %d-%d (file has %d lines)", *request.StartLine, *request.EndLine, len(lines))
	}

	// Delete lines
	newLines := append(lines[:startLine], lines[endLine+1:]...)
	newContent := strings.Join(newLines, "\n")

	// Write back to file
	bytesWritten := int64(len(newContent))
	if err := os.WriteFile(request.FilePath, []byte(newContent), 0644); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	result.BytesWritten = &bytesWritten
	linesDeleted := (endLine - startLine + 1)
	result.LinesChanged = &linesDeleted

	fes.logger.Info("Lines deleted successfully",
		"file_path", request.FilePath,
		"bytes_written", bytesWritten,
		"lines_deleted", linesDeleted)

	return nil
}

// executePatch applies a unified diff patch to a file
func (fes *FileEditService) executePatch(ctx context.Context, request *models.FileEditRequest, result *models.FileEditResult) error {
	if request.PatchContent == nil {
		return fmt.Errorf("patch_content is required for patch operation")
	}

	fes.logger.Info("Applying patch to file", "file_path", request.FilePath)

	// Create backup if requested
	if request.CreateBackup {
		backupPath, err := fes.createBackup(request.FilePath)
		if err != nil {
			return fmt.Errorf("failed to create backup: %w", err)
		}
		result.BackupPath = &backupPath
	}

	// For now, return an error indicating patch is not yet implemented
	// Full unified diff parsing and application would require additional libraries
	return fmt.Errorf("patch operation not yet implemented - use replace or write operations instead")
}

// createBackup creates a backup of a file using streaming to prevent OOM
func (fes *FileEditService) createBackup(filePath string) (string, error) {
	// Generate backup filename with timestamp and hash
	timestamp := time.Now().UTC().Format("20060102-150405")

	// Calculate file hash for uniqueness (streaming)
	file, err := os.Open(filePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	h := sha256.New()
	if _, err := io.Copy(h, file); err != nil {
		return "", err
	}
	hashStr := hex.EncodeToString(h.Sum(nil))[:8]

	backupPath := fmt.Sprintf("%s.backup-%s-%s", filePath, timestamp, hashStr)

	// Reset file pointer to beginning
	if _, err := file.Seek(0, 0); err != nil {
		return "", err
	}

	// Create backup file
	backupFile, err := os.OpenFile(backupPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return "", err
	}
	defer backupFile.Close()

	// Copy content streaming
	if _, err := io.Copy(backupFile, file); err != nil {
		return "", err
	}

	return backupPath, nil
}

// collectFileStats collects file statistics
func (fes *FileEditService) collectFileStats(filePath string, fileInfo os.FileInfo) (*models.FileStats, error) {
	stats := &models.FileStats{
		Size:    fileInfo.Size(),
		Mode:    fmt.Sprintf("%o", fileInfo.Mode().Perm()),
		ModTime: ptrTime(fileInfo.ModTime()),
	}

	// Count lines
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	lineCount := 0
	for scanner.Scan() {
		lineCount++
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	stats.Lines = lineCount

	// Check if symlink
	if fileInfo.Mode()&os.ModeSymlink != 0 {
		stats.IsSymlink = true
		target, err := os.Readlink(filePath)
		if err == nil {
			stats.SymlinkTarget = &target
		}
	}

	// Platform-specific ownership information
	fes.collectFileOwnership(fileInfo, stats)

	return stats, nil
}

// finalizeResult finalizes the file edit result
func (fes *FileEditService) finalizeResult(result *models.FileEditResult) {
	if result.EndTime == nil {
		endTime := time.Now().UTC()
		result.EndTime = &endTime
	}

	if result.StartTime != nil && result.EndTime != nil {
		result.DurationSeconds = result.EndTime.Sub(*result.StartTime).Seconds()
	}
}

// Helper function to create time pointer
func ptrTime(t time.Time) *time.Time {
	return &t
}
