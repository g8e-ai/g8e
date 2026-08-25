// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package models

import (
	"time"

	operatorv1 "github.com/g8e-ai/g8e/v2/protocol/proto/g8e/operator/v1"
)

// FsGrepRequest represents a request to search for a pattern in files
type FsGrepRequest struct {
	ExecutionID     string `json:"execution_id"`
	CaseID          string `json:"case_id"`
	TaskID          string `json:"task_id,omitempty"`
	InvestigationID string `json:"investigation_id"`

	Path       string   `json:"path"`
	Pattern    string   `json:"pattern"`
	Includes   []string `json:"includes,omitempty"`
	MaxMatches int      `json:"max_matches"`
}

// FsGrepResult represents the result of a grep operation
type FsGrepResult struct {
	ExecutionID     string                     `json:"execution_id"`
	CaseID          string                     `json:"case_id"`
	TaskID          string                     `json:"task_id,omitempty"`
	InvestigationID string                     `json:"investigation_id"`
	Status          operatorv1.ExecutionStatus `json:"status"`

	Path         string        `json:"path"`
	Pattern      string        `json:"pattern"`
	Matches      []FsGrepMatch `json:"matches"`
	TotalMatches int           `json:"total_matches"`
	Truncated    bool          `json:"truncated"`

	StartTime       time.Time `json:"start_time,omitempty"`
	EndTime         time.Time `json:"end_time,omitempty"`
	DurationSeconds float64   `json:"duration_seconds"`

	ErrorMessage string `json:"error_message,omitempty"`
	ErrorType    string `json:"error_type,omitempty"`
}
