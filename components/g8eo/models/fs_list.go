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

// FsListRequest represents a request to list directory contents
type FsListRequest struct {
	ExecutionID     string  `json:"execution_id"`
	CaseID          string  `json:"case_id"`
	TaskID          *string `json:"task_id,omitempty"`
	InvestigationID string  `json:"investigation_id"`

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

	IsSymlink     bool    `json:"is_symlink,omitempty"`
	SymlinkTarget *string `json:"symlink_target,omitempty"`
	Owner         *string `json:"owner,omitempty"`
	Group         *string `json:"group,omitempty"`
	Inode         uint64  `json:"inode,omitempty"`
	Nlink         uint64  `json:"nlink,omitempty"`
}

// FsListResult represents the result of a directory listing operation
type FsListResult struct {
	ExecutionID     string                    `json:"execution_id"`
	CaseID          string                    `json:"case_id"`
	TaskID          *string                   `json:"task_id,omitempty"`
	InvestigationID string                    `json:"investigation_id"`
	Status          constants.ExecutionStatus `json:"status"`

	Path       string        `json:"path"`
	Entries    []FsListEntry `json:"entries"`
	TotalCount int           `json:"total_count"`
	Truncated  bool          `json:"truncated"`

	StartTime       *time.Time `json:"start_time,omitempty"`
	EndTime         *time.Time `json:"end_time,omitempty"`
	DurationSeconds float64    `json:"duration_seconds"`

	ErrorMessage *string `json:"error_message,omitempty"`
	ErrorType    *string `json:"error_type,omitempty"`

	SystemInfo *ExecutionSystemInfo `json:"system_info,omitempty"`
}
