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

	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

type ExecutionRequestPayload struct {
	ExecutionID       string            `json:"execution_id"`
	CaseID            string            `json:"case_id"`
	TaskID            *string           `json:"task_id,omitempty"`
	InvestigationID   string            `json:"investigation_id"`
	Command           string            `json:"command"`
	Args              []string          `json:"args,omitempty"`
	TimeoutSeconds    int               `json:"timeout_seconds"`
	RequestedBy       string            `json:"requested_by"`
	Environment       map[string]string `json:"environment,omitempty"`
	WorkingDirectory  *string           `json:"working_directory,omitempty"`
	NetworkTopologyID *string           `json:"network_topology_id,omitempty"`
	TargetAddress     *string           `json:"target_address,omitempty"`
	Priority          int               `json:"priority,omitempty"`
	Tags              map[string]string `json:"tags,omitempty"`
	Justification     string            `json:"justification,omitempty"`
	Intent            string            `json:"intent,omitempty"`
	VaultMode         string            `json:"vault_mode,omitempty"`
}

// ExecutionResult is the domain type for command execution results.
// It uses value types (int, string, time.Time) with zero values representing "not set".
// This is the type returned by ExecutionService.ExecuteCommand and used internally.
type ExecutionResult struct {
	ExecutionID       string
	CaseID            string
	TaskID            string
	InvestigationID   string
	Command           string
	Args              []string
	Status            operatorv1.ExecutionStatus
	DurationSeconds   float64
	OperatorID        string
	OperatorSessionID string
	Stdout            string
	Stderr            string
	StdoutSize        int
	StderrSize        int
	StdoutHash        string
	StderrHash        string
	StoredLocally     bool
	ReturnCode        int
	ErrorMessage      string
	ErrorType         string
	StartTime         time.Time
	EndTime           time.Time
	TerminalOutput    *TerminalOutput
	SystemInfo        *ExecutionSystemInfo
	EnvironmentInfo   *ExecutionEnvironmentInfo
}

// ExecutionResultsPayload is the wire type for JSON/protobuf serialization.
// It uses pointer types (*int, *string, *time.Time) to represent optional fields.
// This is used at the HTTP/pubsub boundary only.
type ExecutionResultsPayload struct {
	PayloadType       string                     `json:"payload_type"`
	ExecutionID       string                     `json:"execution_id"`
	CaseID            string                     `json:"case_id"`
	TaskID            *string                    `json:"task_id,omitempty"`
	InvestigationID   string                     `json:"investigation_id"`
	Command           string                     `json:"command"`
	Args              []string                   `json:"args,omitempty"`
	Status            operatorv1.ExecutionStatus `json:"status"`
	DurationSeconds   float64                    `json:"duration_seconds"`
	OperatorID        string                     `json:"operator_id"`
	OperatorSessionID string                     `json:"operator_session_id"`
	Stdout            string                     `json:"stdout,omitempty"`
	Stderr            string                     `json:"stderr,omitempty"`
	StdoutSize        int                        `json:"stdout_size"`
	StderrSize        int                        `json:"stderr_size"`
	StdoutHash        string                     `json:"stdout_hash,omitempty"`
	StderrHash        string                     `json:"stderr_hash,omitempty"`
	StoredLocally     bool                       `json:"stored_locally"`
	ReturnCode        *int                       `json:"return_code,omitempty"`
	ErrorMessage      *string                    `json:"error_message,omitempty"`
	ErrorType         *string                    `json:"error_type,omitempty"`
	StartTime         *time.Time                 `json:"start_time,omitempty"`
	EndTime           *time.Time                 `json:"end_time,omitempty"`
	TerminalOutput    *TerminalOutput            `json:"terminal_output,omitempty"`
	SystemInfo        *ExecutionSystemInfo       `json:"system_info,omitempty"`
	EnvironmentInfo   *ExecutionEnvironmentInfo  `json:"environment_info,omitempty"`
}

// PayloadFromDomainResult converts a domain ExecutionResult to a wire ExecutionResultsPayload.
// This is used when publishing results to the wire.
func PayloadFromDomainResult(r *ExecutionResult) *ExecutionResultsPayload {
	if r == nil {
		return nil
	}
	payload := &ExecutionResultsPayload{
		PayloadType:       "execution_result",
		ExecutionID:       r.ExecutionID,
		CaseID:            r.CaseID,
		InvestigationID:   r.InvestigationID,
		Command:           r.Command,
		Args:              r.Args,
		Status:            r.Status,
		DurationSeconds:   r.DurationSeconds,
		OperatorID:        r.OperatorID,
		OperatorSessionID: r.OperatorSessionID,
		Stdout:            r.Stdout,
		Stderr:            r.Stderr,
		StdoutSize:        r.StdoutSize,
		StderrSize:        r.StderrSize,
		StdoutHash:        r.StdoutHash,
		StderrHash:        r.StderrHash,
		StoredLocally:     r.StoredLocally,
		TerminalOutput:    r.TerminalOutput,
		SystemInfo:        r.SystemInfo,
		EnvironmentInfo:   r.EnvironmentInfo,
	}
	if r.TaskID != "" {
		payload.TaskID = &r.TaskID
	}
	if r.ReturnCode != 0 {
		payload.ReturnCode = &r.ReturnCode
	}
	if r.ErrorMessage != "" {
		payload.ErrorMessage = &r.ErrorMessage
	}
	if r.ErrorType != "" {
		payload.ErrorType = &r.ErrorType
	}
	if !r.StartTime.IsZero() {
		payload.StartTime = &r.StartTime
	}
	if !r.EndTime.IsZero() {
		payload.EndTime = &r.EndTime
	}
	return payload
}

type CommandRequestPayload struct {
	Command        string `json:"command"`
	ExecutionID    string `json:"execution_id,omitempty"`
	Justification  string `json:"justification,omitempty"`
	VaultMode      string `json:"vault_mode,omitempty"`
	TimeoutSeconds int    `json:"timeout_seconds,omitempty"`
}

type CommandCancelRequestPayload struct {
	ExecutionID string `json:"execution_id"`
}

type FileEditRequestPayload struct {
	FilePath        string `json:"file_path"`
	Operation       string `json:"operation"`
	ExecutionID     string `json:"execution_id,omitempty"`
	VaultMode       string `json:"vault_mode,omitempty"`
	Justification   string `json:"justification,omitempty"`
	Content         string `json:"content,omitempty"`
	OldContent      string `json:"old_content,omitempty"`
	NewContent      string `json:"new_content,omitempty"`
	InsertContent   string `json:"insert_content,omitempty"`
	InsertPosition  *int   `json:"insert_position,omitempty"`
	StartLine       *int   `json:"start_line,omitempty"`
	EndLine         *int   `json:"end_line,omitempty"`
	PatchContent    string `json:"patch_content,omitempty"`
	CreateBackup    bool   `json:"create_backup,omitempty"`
	CreateIfMissing bool   `json:"create_if_missing,omitempty"`
}

type FsListRequestPayload struct {
	Path        string `json:"path,omitempty"`
	ExecutionID string `json:"execution_id,omitempty"`
	MaxDepth    int    `json:"max_depth,omitempty"`
	MaxEntries  int    `json:"max_entries,omitempty"`
}

type FsGrepRequestPayload struct {
	Path        string   `json:"path,omitempty"`
	ExecutionID string   `json:"execution_id,omitempty"`
	Pattern     string   `json:"pattern"`
	Includes    []string `json:"includes,omitempty"`
	MaxMatches  int      `json:"max_matches,omitempty"`
}

type FetchLogsRequestPayload struct {
	ExecutionID string `json:"execution_id"`
	VaultMode   string `json:"vault_mode,omitempty"`
}

type FetchFileDiffRequestPayload struct {
	DiffID            string `json:"diff_id,omitempty"`
	OperatorSessionID string `json:"operator_session_id"`
	FilePath          string `json:"file_path,omitempty"`
	Limit             int    `json:"limit,omitempty"`
}

type FetchHistoryRequestPayload struct{}

type FetchFileHistoryRequestPayload struct {
	FilePath string `json:"file_path"`
}

type RestoreFileRequestPayload struct {
	FilePath   string `json:"file_path"`
	CommitHash string `json:"commit_hash"`
}

type ShutdownRequestPayload struct {
	Reason string `json:"reason,omitempty"`
}

type AuditMsgRequestPayload struct {
	Content           string `json:"content"`
	OperatorSessionID string `json:"operator_session_id"`
}

type AuditDirectCmdRequestPayload struct {
	Command           string `json:"command"`
	ExecutionID       string `json:"execution_id,omitempty"`
	OperatorSessionID string `json:"operator_session_id"`
}

type AuditDirectCmdResultPayload struct {
	Command              string                     `json:"command"`
	ExecutionID          string                     `json:"execution_id,omitempty"`
	ExitCode             *int                       `json:"exit_code,omitempty"`
	Status               operatorv1.ExecutionStatus `json:"status,omitempty"`
	Output               string                     `json:"output,omitempty"`
	Stderr               string                     `json:"stderr,omitempty"`
	ExecutionTimeSeconds float64                    `json:"execution_time_seconds,omitempty"`
	OperatorSessionID    string                     `json:"operator_session_id"`
}

type FsReadRequestPayload struct {
	Path        string `json:"path"`
	ExecutionID string `json:"execution_id,omitempty"`
	MaxSize     int    `json:"max_size,omitempty"`
}

type PortCheckRequestPayload struct {
	ExecutionID string `json:"execution_id,omitempty"`
	Host        string `json:"host,omitempty"`
	Port        int    `json:"port"`
	Protocol    string `json:"protocol,omitempty"`
}

type HeartbeatRequestPayload struct{}
