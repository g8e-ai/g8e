// Copyright (c) 2026 Lateralus Labs, LLC.
// Use of this source code is governed by the Business Source License
// included in the LICENSE file.
//
// As of the Change Date listed in the LICENSE file, this software is
// released under the Apache License, Version 2.0.

package models

import "time"

// ExecutionRecord represents a stored command execution.
type ExecutionRecord struct {
	ID               string
	TimestampUTC     time.Time
	Command          string
	ExitCode         int
	DurationMs       int64
	StdoutCompressed []byte
	StderrCompressed []byte
	StdoutHash       string
	StderrHash       string
	StdoutSize       int
	StderrSize       int
	UserID           string
	CaseID           string
	TaskID           string
	InvestigationID  string
	OperatorID       string
}

// FileDiffRecord represents a stored file diff (Sentinel-scrubbed).
type FileDiffRecord struct {
	ID                string
	TimestampUTC      time.Time
	FilePath          string
	Operation         string
	LedgerHashBefore  string
	LedgerHashAfter   string
	DiffStat          string
	DiffCompressed    []byte
	DiffHash          string
	DiffSize          int
	OperatorSessionID string
	UserID            string
	CaseID            string
	OperatorID        string
}
