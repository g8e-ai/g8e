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

// FsGrepRequest represents a request to search for a pattern in files
type FsGrepRequest struct {
	ExecutionID     string  `json:"execution_id"`
	CaseID          string  `json:"case_id"`
	TaskID          *string `json:"task_id,omitempty"`
	InvestigationID string  `json:"investigation_id"`

	Path       string   `json:"path"`
	Pattern    string   `json:"pattern"`
	Includes   []string `json:"includes,omitempty"`
	MaxMatches int      `json:"max_matches"`
}

// FsGrepMatch represents a single grep match
type FsGrepMatch struct {
	Path       string   `json:"path"`
	LineNumber int      `json:"line_number"`
	Content    string   `json:"content"`
	Before     []string `json:"before,omitempty"`
	After      []string `json:"after,omitempty"`
}

// FsGrepResult represents the result of a grep operation
type FsGrepResult struct {
	ExecutionID     string                    `json:"execution_id"`
	CaseID          string                    `json:"case_id"`
	TaskID          *string                   `json:"task_id,omitempty"`
	InvestigationID string                    `json:"investigation_id"`
	Status          constants.ExecutionStatus `json:"status"`

	Path         string        `json:"path"`
	Pattern      string        `json:"pattern"`
	Matches      []FsGrepMatch `json:"matches"`
	TotalMatches int           `json:"total_matches"`
	Truncated    bool          `json:"truncated"`

	StartTime       *time.Time `json:"start_time,omitempty"`
	EndTime         *time.Time `json:"end_time,omitempty"`
	DurationSeconds float64    `json:"duration_seconds"`

	ErrorMessage *string `json:"error_message,omitempty"`
	ErrorType    *string `json:"error_type,omitempty"`
}
