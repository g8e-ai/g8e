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

package models

import (
	"time"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
)

// FileEditOperation represents a single edit operation on a file
type FileEditOperation string

const (
	FileEditOperationRead    FileEditOperation = "read"
	FileEditOperationWrite   FileEditOperation = "write"
	FileEditOperationReplace FileEditOperation = "replace"
	FileEditOperationInsert  FileEditOperation = "insert"
	FileEditOperationDelete  FileEditOperation = "delete"
	FileEditOperationPatch   FileEditOperation = "patch"
)

// FileEditRequest represents a request to perform file editing operations
type FileEditRequest struct {
	ExecutionID     string            `json:"execution_id"`
	CaseID          string            `json:"case_id"`
	TaskID          *string           `json:"task_id,omitempty"`
	InvestigationID string            `json:"investigation_id"`
	Operation       FileEditOperation `json:"operation"`
	FilePath        string            `json:"file_path"`

	ReadOptions *FileReadOptions `json:"read_options,omitempty"`
	Content     *string          `json:"content,omitempty"`
	OldContent  *string          `json:"old_content,omitempty"`
	NewContent  *string          `json:"new_content,omitempty"`

	InsertPosition *int    `json:"insert_position,omitempty"`
	InsertContent  *string `json:"insert_content,omitempty"`

	StartLine *int `json:"start_line,omitempty"`
	EndLine   *int `json:"end_line,omitempty"`

	PatchContent *string `json:"patch_content,omitempty"`

	CreateBackup    bool   `json:"create_backup"`
	CreateIfMissing bool   `json:"create_if_missing"`
	Encoding        string `json:"encoding,omitempty"`

	RequestedBy   string `json:"requested_by"`
	Justification string `json:"justification"`
}

// FileReadOptions contains options for reading files
type FileReadOptions struct {
	StartLine    *int `json:"start_line,omitempty"`
	EndLine      *int `json:"end_line,omitempty"`
	MaxLines     *int `json:"max_lines,omitempty"`
	IncludeStats bool `json:"include_stats"`
}

// FileEditResult represents the result of a file editing operation
type FileEditResult struct {
	ExecutionID     string                    `json:"execution_id"`
	CaseID          string                    `json:"case_id"`
	TaskID          *string                   `json:"task_id,omitempty"`
	InvestigationID string                    `json:"investigation_id"`
	Operation       FileEditOperation         `json:"operation"`
	FilePath        string                    `json:"file_path"`
	Status          constants.ExecutionStatus `json:"status"`

	Content      *string    `json:"content,omitempty"`
	BackupPath   *string    `json:"backup_path,omitempty"`
	FileStats    *FileStats `json:"file_stats,omitempty"`
	LinesChanged *int       `json:"lines_changed,omitempty"`
	BytesWritten *int64     `json:"bytes_written,omitempty"`

	StartTime       *time.Time `json:"start_time,omitempty"`
	EndTime         *time.Time `json:"end_time,omitempty"`
	DurationSeconds float64    `json:"duration_seconds"`

	ErrorMessage *string `json:"error_message,omitempty"`
	ErrorType    *string `json:"error_type,omitempty"`

	SystemInfo      *ExecutionSystemInfo      `json:"system_info,omitempty"`
	EnvironmentInfo *ExecutionEnvironmentInfo `json:"environment_info,omitempty"`
}

// FileStats contains file statistics
type FileStats struct {
	Size          int64      `json:"size"`
	Lines         int        `json:"lines"`
	Mode          string     `json:"mode"`
	ModTime       *time.Time `json:"mod_time,omitempty"`
	IsSymlink     bool       `json:"is_symlink"`
	SymlinkTarget *string    `json:"symlink_target,omitempty"`
	Owner         *string    `json:"owner,omitempty"`
	Group         *string    `json:"group,omitempty"`
}
