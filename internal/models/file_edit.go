// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package models

import (
	"time"

	"github.com/g8e-ai/g8e/v2/internal/constants"
	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// FileEditRequest represents a request to perform file editing operations
type FileEditRequest struct {
	ExecutionID     string                  `json:"execution_id"`
	CaseID          string                  `json:"case_id"`
	TaskID          string                  `json:"task_id,omitempty"`
	InvestigationID string                  `json:"investigation_id"`
	Operation       constants.FileOperation `json:"operation"`
	FilePath        string                  `json:"file_path"`

	ReadOptions *FileReadOptions `json:"read_options,omitempty"`
	Content     string           `json:"content,omitempty"`
	OldContent  string           `json:"old_content,omitempty"`
	NewContent  string           `json:"new_content,omitempty"`

	InsertPosition int    `json:"insert_position,omitempty"`
	InsertContent  string `json:"insert_content,omitempty"`

	StartLine int `json:"start_line,omitempty"`
	EndLine   int `json:"end_line,omitempty"`

	PatchContent string `json:"patch_content,omitempty"`

	CreateBackup    bool   `json:"create_backup"`
	CreateIfMissing bool   `json:"create_if_missing"`
	Encoding        string `json:"encoding,omitempty"`

	RequestedBy   string `json:"requested_by"`
	Justification string `json:"justification"`
}

// FileReadOptions contains options for reading files
type FileReadOptions struct {
	StartLine    int  `json:"start_line,omitempty"`
	EndLine      int  `json:"end_line,omitempty"`
	MaxLines     int  `json:"max_lines,omitempty"`
	IncludeStats bool `json:"include_stats"`
}

// FileEditResult represents the result of a file editing operation
type FileEditResult struct {
	ExecutionID     string                     `json:"execution_id"`
	CaseID          string                     `json:"case_id"`
	TaskID          string                     `json:"task_id,omitempty"`
	InvestigationID string                     `json:"investigation_id"`
	Operation       constants.FileOperation    `json:"operation"`
	FilePath        string                     `json:"file_path"`
	Status          operatorv1.ExecutionStatus `json:"status"`

	Content      string     `json:"content,omitempty"`
	BackupPath   string     `json:"backup_path,omitempty"`
	FileStats    *FileStats `json:"file_stats,omitempty"`
	LinesChanged int        `json:"lines_changed,omitempty"`
	BytesWritten int64      `json:"bytes_written,omitempty"`

	StartTime       time.Time `json:"start_time,omitempty"`
	EndTime         time.Time `json:"end_time,omitempty"`
	DurationSeconds float64   `json:"duration_seconds"`

	ErrorMessage string `json:"error_message,omitempty"`
	ErrorType    string `json:"error_type,omitempty"`

	SystemInfo      *ExecutionSystemInfo      `json:"system_info,omitempty"`
	EnvironmentInfo *ExecutionEnvironmentInfo `json:"environment_info,omitempty"`
}

// FileStats contains file statistics
type FileStats struct {
	Size          int64     `json:"size"`
	Lines         int       `json:"lines"`
	Mode          string    `json:"mode"`
	ModTime       time.Time `json:"mod_time,omitempty"`
	IsSymlink     bool      `json:"is_symlink"`
	SymlinkTarget string    `json:"symlink_target,omitempty"`
	Owner         string    `json:"owner,omitempty"`
	Group         string    `json:"group,omitempty"`
}
