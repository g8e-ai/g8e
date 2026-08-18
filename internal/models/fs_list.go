// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package models

import (
	"time"

	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

// FsListRequest represents a request to list directory contents
type FsListRequest struct {
	ExecutionID     string `json:"execution_id"`
	CaseID          string `json:"case_id"`
	TaskID          string `json:"task_id,omitempty"`
	InvestigationID string `json:"investigation_id"`

	Path        string `json:"path"`
	MaxDepth    int    `json:"max_depth"`
	MaxEntries  int    `json:"max_entries"`
	RequestedBy string `json:"requested_by"`
}

// FsListEntry represents a single file/directory entry with readdirplus-style metadata
type FsListEntry struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	IsDir   bool   `json:"is_dir"`
	Size    int64  `json:"size"`
	Mode    string `json:"mode"`
	ModTime int64  `json:"mod_time"`

	IsSymlink     bool   `json:"is_symlink,omitempty"`
	SymlinkTarget string `json:"symlink_target,omitempty"`
	Owner         string `json:"owner,omitempty"`
	Group         string `json:"group,omitempty"`
	Inode         uint64 `json:"inode,omitempty"`
	Nlink         uint64 `json:"nlink,omitempty"`
}

// FsListResult represents the result of a directory listing operation
type FsListResult struct {
	ExecutionID     string                     `json:"execution_id"`
	CaseID          string                     `json:"case_id"`
	TaskID          string                     `json:"task_id,omitempty"`
	InvestigationID string                     `json:"investigation_id"`
	Status          operatorv1.ExecutionStatus `json:"status"`

	Path       string        `json:"path"`
	Entries    []FsListEntry `json:"entries"`
	TotalCount int           `json:"total_count"`
	Truncated  bool          `json:"truncated"`

	StartTime       time.Time `json:"start_time,omitempty"`
	EndTime         time.Time `json:"end_time,omitempty"`
	DurationSeconds float64   `json:"duration_seconds"`

	ErrorMessage string `json:"error_message,omitempty"`
	ErrorType    string `json:"error_type,omitempty"`

	SystemInfo *ExecutionSystemInfo `json:"system_info,omitempty"`
}
