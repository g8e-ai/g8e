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

package mcp

import (
	"encoding/json"

	"github.com/g8e-ai/g8e/internal/models"
	"github.com/g8e-ai/g8e/internal/response"
)

// JSONRPCRequest is an alias for response.JSONRPCRequest
type JSONRPCRequest = response.JSONRPCRequest

// JSONRPCResponse is an alias for response.JSONRPCResponse
type JSONRPCResponse = response.JSONRPCResponse

// JSONRPCError is an alias for response.JSONRPCError
type JSONRPCError = response.JSONRPCError

// Protocol-specific error codes for g8eo (reserved range -32000 to -32099)
const (
	// Verification Errors (-32000 range)
	ErrCodeInvalidEnvelope     = response.ErrCodeInvalidEnvelope
	ErrCodeHashMismatch        = response.ErrCodeHashMismatch
	ErrCodeExpired             = response.ErrCodeExpired
	ErrCodeReplay              = response.ErrCodeReplay
	ErrCodeStateMismatch       = response.ErrCodeStateMismatch
	ErrCodeL1ValidationFailed  = response.ErrCodeL1ValidationFailed
	ErrCodeL2SignatureInvalid  = response.ErrCodeL2SignatureInvalid
	ErrCodeL3ProofInvalid      = response.ErrCodeL3ProofInvalid
	ErrCodePayloadDecodeFailed = response.ErrCodePayloadDecodeFailed

	// Resource/State Errors (-32100 range)
	ErrCodeResourceNotFound = response.ErrCodeResourceNotFound
	ErrCodeGatewayNotReady  = response.ErrCodeGatewayNotReady
)

// CallToolRequest is the params for the "tools/call" method.
type CallToolRequest struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// CallToolResult is the result for the "tools/call" method.
type CallToolResult struct {
	Content []TextContent `json:"content"`
	IsError bool          `json:"isError,omitempty"`
}

// TextContent represents a text part of a tool response.
type TextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// SuspendedTransaction is an alias for the shared models type.
type SuspendedTransaction = models.SuspendedTransaction

// ListResourcesRequest is the params for the "resources/list" method.
type ListResourcesRequest struct {
	// Optional cursor for pagination
	Cursor *string `json:"cursor,omitempty"`
}

// Resource represents an MCP resource.
type Resource struct {
	URI         string                 `json:"uri"`
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	MimeType    string                 `json:"mimeType,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// ListResourcesResult is the result for the "resources/list" method.
type ListResourcesResult struct {
	Resources []Resource `json:"resources"`
}

// ReadResourceRequest is the params for the "resources/read" method.
type ReadResourceRequest struct {
	URI string `json:"uri"`
}

// ReadResourceResult is the result for the "resources/read" method.
type ReadResourceResult struct {
	Contents []TextContent `json:"contents"`
	MIMEType string        `json:"mimeType,omitempty"`
}

// ListPromptsRequest is the params for the "prompts/list" method.
type ListPromptsRequest struct {
	// Optional cursor for pagination
	Cursor *string `json:"cursor,omitempty"`
}

// Prompt represents an MCP prompt template.
type Prompt struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	Arguments   []PromptArgument       `json:"arguments,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// PromptArgument represents an argument for a prompt template.
type PromptArgument struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Required    bool   `json:"required,omitempty"`
}

// ListPromptsResult is the result for the "prompts/list" method.
type ListPromptsResult struct {
	Prompts []Prompt `json:"prompts"`
}

// GetPromptRequest is the params for the "prompts/get" method.
type GetPromptRequest struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// GetPromptResult is the result for the "prompts/get" method.
type GetPromptResult struct {
	Description string          `json:"description,omitempty"`
	Messages    []PromptMessage `json:"messages,omitempty"`
}

// PromptMessage represents a message in a prompt template.
type PromptMessage struct {
	Role    string      `json:"role"`
	Content TextContent `json:"content"`
}

// ToolsListResult is the result for the "tools/list" method.
type ToolsListResult struct {
	Tools []Tool `json:"tools"`
}

// Tool represents an MCP tool.
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description,omitempty"`
	InputSchema map[string]interface{} `json:"inputSchema,omitempty"`
}

// ResourcesListResult is the result for the "resources/list" method.
type ResourcesListResult struct {
	Resources []Resource `json:"resources"`
}

// PromptsListResult is the result for the "prompts/list" method.
type PromptsListResult struct {
	Prompts []Prompt `json:"prompts"`
}

// A2ASuspensionResponse is returned when an A2A call is suspended for L3 approval.
type A2ASuspensionResponse struct {
	ID          string `json:"id"`
	Status      string `json:"status"`
	TxHash      string `json:"tx_hash"`
	ApprovalURL string `json:"approval_url"`
	Message     string `json:"message"`
}

// A2ASuccessResponse is returned when an A2A call succeeds.
type A2ASuccessResponse struct {
	ID     string      `json:"id"`
	Result interface{} `json:"result"`
}

// A2ADownstreamRequest is the request sent to a downstream A2A server.
type A2ADownstreamRequest struct {
	SkillName   string          `json:"skill_name"`
	PayloadJSON json.RawMessage `json:"payload"`
	ExecutionID string          `json:"execution_id,omitempty"`
}

// FieldReadRequest is the params for the "read_field" tool.
type FieldReadRequest struct {
	Collection        string `json:"collection"`
	DocumentID        string `json:"document_id"`
	FieldPath         string `json:"field_path"`
	OperatorSessionID string `json:"operator_session_id"`
}

// FieldReadResult is the result for the "read_field" tool.
type FieldReadResult struct {
	Value interface{} `json:"value"`
}

// Native tool definitions compiled into the Node binary

// DBDiscoverTopologyRequest is the params for the "db_discover_topology" tool.
type DBDiscoverTopologyRequest struct {
	DatabasePath string `json:"database_path"`
}

// DBDiscoverTopologyResult is the result for the "db_discover_topology" tool.
type DBDiscoverTopologyResult struct {
	Schema map[string]map[string]string `json:"schema"`
}

// DBQueryValidateRequest is the params for the "db_query_validate" tool.
type DBQueryValidateRequest struct {
	DatabasePath string `json:"database_path"`
	Query        string `json:"query"`
}

// DBQueryValidateResult is the result for the "db_query_validate" tool.
type DBQueryValidateResult struct {
	Valid    bool   `json:"valid"`
	Plan     string `json:"plan,omitempty"`
	Warning  string `json:"warning,omitempty"`
	Rejected bool   `json:"rejected"`
	Reason   string `json:"reason,omitempty"`
}

// DBIsolatedReadRequest is the params for the "db_isolated_read" tool.
type DBIsolatedReadRequest struct {
	DatabasePath string `json:"database_path"`
	Query        string `json:"query"`
}

// DBIsolatedReadResult is the result for the "db_isolated_read" tool.
type DBIsolatedReadResult struct {
	Rows    []map[string]interface{} `json:"rows"`
	Columns []string                 `json:"columns"`
}

// DBIndexTriageRequest is the params for the "db_index_triage" tool.
type DBIndexTriageRequest struct {
	DatabasePath string `json:"database_path"`
}

// DBIndexTriageResult is the result for the "db_index_triage" tool.
type DBIndexTriageResult struct {
	Indexes       []IndexInfo `json:"indexes"`
	Fragmentation float64     `json:"fragmentation"`
}

// IndexInfo represents database index information.
type IndexInfo struct {
	Name    string   `json:"name"`
	Table   string   `json:"table"`
	Columns []string `json:"columns"`
	Unique  bool     `json:"unique"`
	Used    bool     `json:"used"`
}

// LogStreamFilterRequest is the params for the "log_stream_filter" tool.
type LogStreamFilterRequest struct {
	LogPath string `json:"log_path"`
	Pattern string `json:"pattern"`
	Limit   int    `json:"limit,omitempty"`
}

// LogStreamFilterResult is the result for the "log_stream_filter" tool.
type LogStreamFilterResult struct {
	Lines []string `json:"lines"`
	Count int      `json:"count"`
}

// SysOOMDetectRequest is the params for the "sys_oom_detect" tool.
type SysOOMDetectRequest struct {
	LogPath string `json:"log_path,omitempty"`
}

// SysOOMDetectResult is the result for the "sys_oom_detect" tool.
type SysOOMDetectResult struct {
	Events []OOMEvent `json:"events"`
}

// OOMEvent represents an OOM killer event.
type OOMEvent struct {
	Timestamp string `json:"timestamp"`
	PID       int    `json:"pid"`
	Process   string `json:"process"`
	MemoryMB  int    `json:"memory_mb"`
}

// ConfigDiffMaskRequest is the params for the "config_diff_mask" tool.
type ConfigDiffMaskRequest struct {
	ConfigPath string `json:"config_path"`
	Baseline   string `json:"baseline"`
}

// ConfigDiffMaskResult is the result for the "config_diff_mask" tool.
type ConfigDiffMaskResult struct {
	Differences []ConfigDiff `json:"differences"`
}

// ConfigDiff represents a configuration difference.
type ConfigDiff struct {
	Key      string `json:"key"`
	Current  string `json:"current,omitempty"`
	Baseline string `json:"baseline,omitempty"`
	Type     string `json:"type"`
}

// ProcMetricTopRequest is the params for the "proc_metric_top" tool.
type ProcMetricTopRequest struct {
	Limit int `json:"limit,omitempty"`
}

// ProcMetricTopResult is the result for the "proc_metric_top" tool.
type ProcMetricTopResult struct {
	Processes []ProcessInfo `json:"processes"`
}

// ProcessInfo represents process information from /proc.
type ProcessInfo struct {
	PID        int     `json:"pid"`
	Name       string  `json:"name"`
	CPUPercent float64 `json:"cpu_percent"`
	MemoryMB   float64 `json:"memory_mb"`
	User       string  `json:"user"`
	Command    string  `json:"command"`
}

// FSDiskProfileRequest is the params for the "fs_disk_profile" tool.
type FSDiskProfileRequest struct {
	Path     string `json:"path"`
	MaxDepth int    `json:"max_depth,omitempty"`
}

// FSDiskProfileResult is the result for the "fs_disk_profile" tool.
type FSDiskProfileResult struct {
	Entries []DirEntry `json:"entries"`
	TotalMB int64      `json:"total_mb"`
}

// DirEntry represents a directory entry for disk profiling.
type DirEntry struct {
	Path     string `json:"path"`
	SizeMB   int64  `json:"size_mb"`
	IsDir    bool   `json:"is_dir"`
	Modified int64  `json:"modified"`
}

// ProcSignalSafeRequest is the params for the "proc_signal_safe" tool.
type ProcSignalSafeRequest struct {
	PID    int    `json:"pid"`
	Signal string `json:"signal"`
}

// ProcSignalSafeResult is the result for the "proc_signal_safe" tool.
type ProcSignalSafeResult struct {
	Sent   bool   `json:"sent"`
	PID    int    `json:"pid"`
	Signal string `json:"signal"`
	Error  string `json:"error,omitempty"`
}

// NetSocketAuditRequest is the params for the "net_socket_audit" tool.
type NetSocketAuditRequest struct {
	Protocol string `json:"protocol,omitempty"`
}

// NetSocketAuditResult is the result for the "net_socket_audit" tool.
type NetSocketAuditResult struct {
	Sockets []SocketInfo `json:"sockets"`
}

// SocketInfo represents network socket information.
type SocketInfo struct {
	Protocol   string `json:"protocol"`
	LocalAddr  string `json:"local_addr"`
	LocalPort  int    `json:"local_port"`
	RemoteAddr string `json:"remote_addr,omitempty"`
	RemotePort int    `json:"remote_port,omitempty"`
	State      string `json:"state,omitempty"`
	PID        int    `json:"pid,omitempty"`
	Process    string `json:"process,omitempty"`
}

// NetEndpointPingRequest is the params for the "net_endpoint_ping" tool.
type NetEndpointPingRequest struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

// NetEndpointPingResult is the result for the "net_endpoint_ping" tool.
type NetEndpointPingResult struct {
	Reachable bool    `json:"reachable"`
	LatencyMs float64 `json:"latency_ms"`
	Error     string  `json:"error,omitempty"`
}

// NetHTTPProbeRequest is the params for the "net_http_probe" tool.
type NetHTTPProbeRequest struct {
	URL    string `json:"url"`
	Method string `json:"method,omitempty"`
}

// NetHTTPProbeResult is the result for the "net_http_probe" tool.
type NetHTTPProbeResult struct {
	StatusCode int               `json:"status_code"`
	Headers    map[string]string `json:"headers"`
	LatencyMs  float64           `json:"latency_ms"`
	Error      string            `json:"error,omitempty"`
}

// ShellExecuteRequest is the params for the "shell_execute" tool.
type ShellExecuteRequest struct {
	Command    string   `json:"command"`
	Args       []string `json:"args,omitempty"`
	Timeout    int      `json:"timeout,omitempty"`
	WorkingDir string   `json:"working_dir,omitempty"`
	Hostnames  []string `json:"hostnames,omitempty"`
}

// ShellExecuteResult is the result for the "shell_execute" tool.
type ShellExecuteResult struct {
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	TimedOut bool   `json:"timed_out"`
	Error    string `json:"error,omitempty"`
	Hostname string `json:"hostname,omitempty"`
}
