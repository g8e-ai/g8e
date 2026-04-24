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
	"encoding/json"

	"github.com/g8e-ai/g8e/components/g8eo/constants"
	"github.com/google/uuid"
)

type G8eMessage struct {
	ID                string          `json:"id"`
	Timestamp         string          `json:"timestamp"`
	SourceComponent   string          `json:"source_component"`
	EventType         string          `json:"event_type"`
	CaseID            string          `json:"case_id"`
	OperatorID        string          `json:"operator_id"`
	OperatorSessionID string          `json:"operator_session_id"`
	SystemFingerprint string          `json:"system_fingerprint"`
	APIKey            string          `json:"api_key,omitempty"`
	Payload           json.RawMessage `json:"payload"`
	TaskID            *string         `json:"task_id"`
	InvestigationID   string          `json:"investigation_id"`
}

// NewG8eMessage builds a wire envelope with a freshly generated UUID v4 as `id`.
// The envelope id is NEVER a correlation key — it is unique per message. Callers
// that need to correlate a result back to an in-flight command MUST use
// payload.execution_id (see shared/models/wire/envelope.json for the contract).
func NewG8eMessage(eventType, caseID, operatorID, operatorSessionID, systemFingerprint string, payload interface{}) (*G8eMessage, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	return &G8eMessage{
		ID:                uuid.NewString(),
		Timestamp:         NowTimestamp(),
		SourceComponent:   constants.Status.ComponentName.G8EO,
		EventType:         eventType,
		CaseID:            caseID,
		OperatorID:        operatorID,
		OperatorSessionID: operatorSessionID,
		SystemFingerprint: systemFingerprint,
		Payload:           raw,
	}, nil
}

func (r *G8eMessage) Marshal() ([]byte, error) {
	return json.Marshal(r)
}

type CancellationResultPayload struct {
	ExecutionID       string                    `json:"execution_id"`
	Status            constants.ExecutionStatus `json:"status"`
	OperatorID        string                    `json:"operator_id"`
	OperatorSessionID string                    `json:"operator_session_id"`
	ErrorMessage      *string                   `json:"error_message,omitempty"`
	ErrorType         *string                   `json:"error_type,omitempty"`
}

type FileEditResultPayload struct {
	ExecutionID       string                    `json:"execution_id"`
	Operation         FileEditOperation         `json:"operation"`
	FilePath          string                    `json:"file_path"`
	Status            constants.ExecutionStatus `json:"status"`
	DurationSeconds   float64                   `json:"duration_seconds"`
	OperatorID        string                    `json:"operator_id"`
	OperatorSessionID string                    `json:"operator_session_id"`
	Content           *string                   `json:"content,omitempty"`
	StdoutSize        int                       `json:"stdout_size"`
	StderrSize        int                       `json:"stderr_size"`
	StdoutHash        string                    `json:"stdout_hash,omitempty"`
	StderrHash        string                    `json:"stderr_hash,omitempty"`
	StoredLocally     bool                      `json:"stored_locally"`
	ErrorMessage      *string                   `json:"error_message,omitempty"`
	ErrorType         *string                   `json:"error_type,omitempty"`
	BytesWritten      *int64                    `json:"bytes_written,omitempty"`
	LinesChanged      *int                      `json:"lines_changed,omitempty"`
	BackupPath        *string                   `json:"backup_path,omitempty"`
}

type FsListResultPayload struct {
	ExecutionID       string                    `json:"execution_id"`
	Path              string                    `json:"path"`
	Status            constants.ExecutionStatus `json:"status"`
	TotalCount        int                       `json:"total_count"`
	Truncated         bool                      `json:"truncated"`
	DurationSeconds   float64                   `json:"duration_seconds"`
	OperatorID        string                    `json:"operator_id"`
	OperatorSessionID string                    `json:"operator_session_id"`
	Entries           []FsListEntry             `json:"entries,omitempty"`
	StdoutSize        int                       `json:"stdout_size"`
	StderrSize        int                       `json:"stderr_size"`
	StdoutHash        string                    `json:"stdout_hash,omitempty"`
	StderrHash        string                    `json:"stderr_hash,omitempty"`
	StoredLocally     bool                      `json:"stored_locally"`
	ErrorMessage      *string                   `json:"error_message,omitempty"`
	ErrorType         *string                   `json:"error_type,omitempty"`
}

type ExecutionStatusPayload struct {
	ExecutionID       string                    `json:"execution_id"`
	Command           string                    `json:"command"`
	Status            constants.ExecutionStatus `json:"status"`
	ProcessAlive      bool                      `json:"process_alive"`
	ElapsedSeconds    float64                   `json:"elapsed_seconds"`
	OperatorID        string                    `json:"operator_id"`
	OperatorSessionID string                    `json:"operator_session_id"`
	NewOutput         string                    `json:"new_output,omitempty"`
	NewStderr         string                    `json:"new_stderr,omitempty"`
	Message           string                    `json:"message,omitempty"`
	StoredLocally     bool                      `json:"stored_locally,omitempty"`
}

type FileDiffEntry struct {
	ID                string `json:"id"`
	Timestamp         string `json:"timestamp"`
	FilePath          string `json:"file_path"`
	Operation         string `json:"operation"`
	LedgerHashBefore  string `json:"ledger_hash_before"`
	LedgerHashAfter   string `json:"ledger_hash_after"`
	DiffStat          string `json:"diff_stat"`
	DiffContent       string `json:"diff_content,omitempty"`
	DiffSize          int    `json:"diff_size"`
	OperatorSessionID string `json:"operator_session_id"`
}

type FetchFileDiffResultPayload struct {
	Success           bool            `json:"success"`
	ExecutionID       string          `json:"execution_id"`
	Diffs             []FileDiffEntry `json:"diffs,omitempty"`
	Diff              *FileDiffEntry  `json:"diff,omitempty"`
	Total             *int            `json:"total,omitempty"`
	OperatorSessionID string          `json:"operator_session_id"`
	Error             *string         `json:"error,omitempty"`
}

type PortCheckEntry struct {
	Host      string   `json:"host"`
	Port      int      `json:"port"`
	Open      bool     `json:"open"`
	LatencyMs *float64 `json:"latency_ms,omitempty"`
	Error     *string  `json:"error,omitempty"`
}

type PortCheckResultPayload struct {
	ExecutionID       string                    `json:"execution_id"`
	Status            constants.ExecutionStatus `json:"status"`
	OperatorID        string                    `json:"operator_id"`
	OperatorSessionID string                    `json:"operator_session_id"`
	Results           []PortCheckEntry          `json:"results,omitempty"`
	ErrorMessage      *string                   `json:"error_message,omitempty"`
	ErrorType         *string                   `json:"error_type,omitempty"`
}

type LFAAErrorPayload struct {
	Success           bool   `json:"success"`
	Error             string `json:"error"`
	ExecutionID       string `json:"execution_id"`
	OperatorID        string `json:"operator_id"`
	OperatorSessionID string `json:"operator_session_id"`
}

type FetchLogsResultPayload struct {
	ExecutionID       string `json:"execution_id"`
	Command           string `json:"command"`
	ExitCode          *int   `json:"exit_code,omitempty"`
	DurationMs        int64  `json:"duration_ms"`
	Stdout            string `json:"stdout"`
	Stderr            string `json:"stderr"`
	StdoutSize        int    `json:"stdout_size"`
	StderrSize        int    `json:"stderr_size"`
	Timestamp         string `json:"timestamp"`
	OperatorID        string `json:"operator_id"`
	OperatorSessionID string `json:"operator_session_id"`
	SentinelMode      string `json:"sentinel_mode,omitempty"`
	Error             string `json:"error,omitempty"`
}

type FetchHistoryResultPayload struct {
	Success           bool             `json:"success"`
	ExecutionID       string           `json:"execution_id"`
	OperatorSessionID string           `json:"operator_session_id,omitempty"`
	WebSession        *AuditWebSession `json:"web_session,omitempty"`
	Events            []AuditEvent     `json:"events,omitempty"`
	Total             int              `json:"total,omitempty"`
	Limit             int              `json:"limit,omitempty"`
	Offset            int              `json:"offset,omitempty"`
	Error             string           `json:"error,omitempty"`
}

type AuditWebSession struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	CreatedAt    string `json:"created_at,omitempty"`
	UserIdentity string `json:"user_identity"`
}

type AuditFileMutation struct {
	ID               int64  `json:"id"`
	Filepath         string `json:"filepath"`
	Operation        string `json:"operation"`
	LedgerHashBefore string `json:"ledger_hash_before,omitempty"`
	LedgerHashAfter  string `json:"ledger_hash_after,omitempty"`
	DiffStat         string `json:"diff_stat,omitempty"`
}

type AuditEvent struct {
	ID                  int64               `json:"id"`
	OperatorSessionID   string              `json:"operator_session_id"`
	Timestamp           string              `json:"timestamp,omitempty"`
	Type                string              `json:"type"`
	ContentText         string              `json:"content_text,omitempty"`
	CommandRaw          string              `json:"command_raw,omitempty"`
	CommandExitCode     *int                `json:"command_exit_code,omitempty"`
	CommandStdout       string              `json:"command_stdout,omitempty"`
	CommandStderr       string              `json:"command_stderr,omitempty"`
	ExecutionDurationMs int64               `json:"execution_duration_ms,omitempty"`
	StoredLocally       bool                `json:"stored_locally"`
	StdoutTruncated     bool                `json:"stdout_truncated"`
	StderrTruncated     bool                `json:"stderr_truncated"`
	FileMutations       []AuditFileMutation `json:"file_mutations"`
}

type FileHistoryEntry struct {
	CommitHash string `json:"commit_hash"`
	Timestamp  string `json:"timestamp,omitempty"`
	Message    string `json:"message"`
}

type FetchFileHistoryResultPayload struct {
	Success     bool               `json:"success"`
	ExecutionID string             `json:"execution_id"`
	FilePath    string             `json:"file_path,omitempty"`
	History     []FileHistoryEntry `json:"history,omitempty"`
	Error       string             `json:"error,omitempty"`
}

type RestoreFileResultPayload struct {
	Success     bool   `json:"success"`
	ExecutionID string `json:"execution_id"`
	FilePath    string `json:"file_path,omitempty"`
	CommitHash  string `json:"commit_hash,omitempty"`
	Error       string `json:"error,omitempty"`
}

type FsReadResultPayload struct {
	ExecutionID       string                    `json:"execution_id"`
	Path              string                    `json:"path"`
	Status            constants.ExecutionStatus `json:"status"`
	Content           string                    `json:"content,omitempty"`
	SizeBytes         int                       `json:"size_bytes"`
	Truncated         bool                      `json:"truncated"`
	DurationSeconds   float64                   `json:"duration_seconds"`
	OperatorID        string                    `json:"operator_id"`
	OperatorSessionID string                    `json:"operator_session_id"`
	ErrorMessage      *string                   `json:"error_message,omitempty"`
	ErrorType         *string                   `json:"error_type,omitempty"`
}

type SystemInfo struct {
	Hostname            string                 `json:"hostname"`
	OS                  string                 `json:"os"`
	Architecture        string                 `json:"architecture"`
	CPUCount            int                    `json:"cpu_count"`
	MemoryMB            uint64                 `json:"memory_mb"`
	PublicIP            string                 `json:"public_ip"`
	InternalIP          string                 `json:"internal_ip"`
	Interfaces          []string               `json:"interfaces"`
	CurrentUser         string                 `json:"current_user"`
	SystemFingerprint   string                 `json:"system_fingerprint"`
	FingerprintDetails  FingerprintDetails     `json:"fingerprint_details"`
	OSDetails           HeartbeatOSDetails     `json:"os_details"`
	UserDetails         HeartbeatUserDetails   `json:"user_details"`
	DiskDetails         HeartbeatDiskDetails   `json:"disk_details"`
	MemoryDetails       HeartbeatMemoryDetails `json:"memory_details"`
	Environment         HeartbeatEnvironment   `json:"environment"`
	IsCloudOperator     bool                   `json:"is_cloud_operator"`
	CloudProvider       string                 `json:"cloud_provider"`
	LocalStorageEnabled bool                   `json:"local_storage_enabled"`
}

type FingerprintDetails struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	CPUCount     int    `json:"cpu_count"`
	MachineID    string `json:"machine_id"`
}

// RuntimeConfig captures the CLI flags and env var overrides active when the operator was started.
// Sent to g8ed at bootstrap and stored in operator_document.runtime_config.
type RuntimeConfig struct {
	CloudMode           bool   `json:"cloud_mode"`
	CloudProvider       string `json:"cloud_provider,omitempty"`
	LocalStorageEnabled bool   `json:"local_storage_enabled"`
	NoGit               bool   `json:"no_git"`
	LogLevel            string `json:"log_level"`
	WSSPort             int    `json:"wss_port"`
	HTTPPort            int    `json:"http_port"`
}
