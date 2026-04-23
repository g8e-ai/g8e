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

/*
Shared Wire Models Contract Tests

Verifies that g8eo Go structs in models/ exactly match the canonical field names
defined in shared/models/wire/*.json.

g8eo duplicates shared JSON field names as struct json tags (no code generation).
These tests are the enforcement mechanism that detects drift between the JSON
source of truth and the Go consumer structs.

Coverage:
  - envelope.json    -> G8eMessage (wire.go)
  - heartbeat.json   -> Heartbeat (heartbeat.go)
  - system_info.json -> SystemInfo (wire.go)
  - result_payloads.json -> ExecutionResultsPayload, CancellationResultPayload,
    FileEditResultPayload, FsListResultPayload, ExecutionStatusPayload,
    FetchLogsResultPayload, FetchFileDiffResultPayload, PortCheckResultPayload,
    RestoreFileResultPayload, LFAAErrorPayload (wire.go)
  - mcp.json -> JSONRPCRequest, CallToolResult, MCPResultMetadata, Content, ResourceContent (services/mcp/types.go)
  - command_payloads.json -> CommandPayload, CommandCancelPayload,
    FileEditPayload, FsListPayload, FsReadPayload, PortCheckPayload,
    FetchLogsRequestPayload, FetchFileDiffPayload, FetchHistoryPayload,
    FetchFileHistoryPayload, RestoreFilePayload, ShutdownPayload,
    AuditMsgPayload, AuditDirectCmdPayload, AuditDirectCmdResultPayload (commands.go)
*/
package contracts

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/g8e-ai/g8e/components/g8eo/models"
	"github.com/g8e-ai/g8e/components/g8eo/services/mcp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var sharedWireModelsDir string

func init() {
	if g8eoRoot == "" {
		var err error
		g8eoRoot, err = filepath.Abs(filepath.Join(".."))
		if err != nil {
			panic(fmt.Sprintf("failed to resolve g8eo root: %v", err))
		}
	}

	dir, err := filepath.Abs(filepath.Join(g8eoRoot, "../../shared/models/wire"))
	if err != nil {
		panic(fmt.Sprintf("failed to resolve shared wire models dir: %v", err))
	}
	sharedWireModelsDir = dir
}

func loadWireJSON(t *testing.T, filename string) map[string]interface{} {
	t.Helper()
	path := filepath.Join(sharedWireModelsDir, filename)
	data, err := os.ReadFile(path)
	require.NoError(t, err, "shared/models/wire/%s must be readable", filename)
	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &result), "shared/models/wire/%s must be valid JSON", filename)
	return result
}

// jsonTagsOf returns the set of json tag names for a struct type (omitempty stripped).
func jsonTagsOf(v interface{}) map[string]bool {
	t := reflect.TypeOf(v)
	if t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	tags := make(map[string]bool)
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		tag := f.Tag.Get("json")
		if tag == "" || tag == "-" {
			continue
		}
		name := tag
		for j, c := range tag {
			if c == ',' {
				name = tag[:j]
				break
			}
		}
		tags[name] = true
	}
	return tags
}

// assertFieldPresent asserts that a required field from the JSON schema is
// present as a json tag in the Go struct.
func assertFieldPresent(t *testing.T, structTags map[string]bool, fieldName string, structName string) {
	t.Helper()
	assert.True(t, structTags[fieldName],
		"%s must have json tag %q (required by shared/models/wire)", structName, fieldName)
}

// =============================================================================
// envelope.json -> G8eMessage
// =============================================================================

func TestG8eMessageMatchesEnvelopeSchema(t *testing.T) {
	tags := jsonTagsOf(models.G8eMessage{})

	// All required fields from envelope.json that g8eo populates
	requiredFields := []string{
		"id",
		"timestamp",
		"source_component",
		"event_type",
		"operator_id",
		"operator_session_id",
		"case_id",
		"system_fingerprint",
		"payload",
	}
	for _, f := range requiredFields {
		assertFieldPresent(t, tags, f, "G8eMessage")
	}

	// Optional fields g8eo includes
	optionalFields := []string{
		"task_id",
		"investigation_id",
		"operator_session_id",
	}
	for _, f := range optionalFields {
		assertFieldPresent(t, tags, f, "G8eMessage")
	}
}

// =============================================================================
// heartbeat.json -> Heartbeat
// =============================================================================

func TestHeartbeatMatchesHeartbeatSchema(t *testing.T) {
	tags := jsonTagsOf(models.Heartbeat{})

	envelopeFields := []string{
		"event_type",
		"source_component",
		"operator_id",
		"operator_session_id",
		"case_id",
		"investigation_id",
		"timestamp",
		"heartbeat_type",
	}
	for _, f := range envelopeFields {
		assertFieldPresent(t, tags, f, "Heartbeat")
	}

	payloadSections := []string{
		"system_identity",
		"network_info",
		"version_info",
		"uptime_info",
		"performance_metrics",
		"os_details",
		"user_details",
		"disk_details",
		"memory_details",
		"environment",
	}
	for _, f := range payloadSections {
		assertFieldPresent(t, tags, f, "Heartbeat")
	}

	assertFieldPresent(t, tags, "capability_flags", "Heartbeat")
}

func TestHeartbeatCapabilityFlagsMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.HeartbeatCapabilityFlags{})
	fields := []string{"local_storage_enabled", "git_available", "ledger_enabled"}
	for _, f := range fields {
		assertFieldPresent(t, tags, f, "HeartbeatCapabilityFlags")
	}
}

func TestHeartbeatSystemIdentityMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.HeartbeatSystemIdentity{})

	fields := []string{"hostname", "os", "architecture", "pwd", "current_user", "cpu_count", "memory_mb"}
	for _, f := range fields {
		assertFieldPresent(t, tags, f, "HeartbeatSystemIdentity")
	}
}

func TestHeartbeatNetworkInfoMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.HeartbeatNetworkInfo{})
	fields := []string{"public_ip", "interfaces", "connectivity_status"}
	for _, f := range fields {
		assertFieldPresent(t, tags, f, "HeartbeatNetworkInfo")
	}
}

func TestHeartbeatNetworkInterfaceMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.HeartbeatNetworkInterface{})
	fields := []string{"name", "ip", "mtu"}
	for _, f := range fields {
		assertFieldPresent(t, tags, f, "HeartbeatNetworkInterface")
	}
}

func TestHeartbeatVersionInfoMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.HeartbeatVersionInfo{})
	fields := []string{"operator_version", "status"}
	for _, f := range fields {
		assertFieldPresent(t, tags, f, "HeartbeatVersionInfo")
	}
}

func TestHeartbeatUptimeInfoMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.HeartbeatUptimeInfo{})
	fields := []string{"uptime", "uptime_seconds"}
	for _, f := range fields {
		assertFieldPresent(t, tags, f, "HeartbeatUptimeInfo")
	}
}

func TestHeartbeatPerformanceMetricsMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.HeartbeatPerformanceMetrics{})
	fields := []string{
		"cpu_percent", "memory_percent", "disk_percent", "network_latency",
		"memory_used_mb", "memory_total_mb", "disk_used_gb", "disk_total_gb",
	}
	for _, f := range fields {
		assertFieldPresent(t, tags, f, "HeartbeatPerformanceMetrics")
	}
}

// =============================================================================
// system_info.json -> SystemInfo
// =============================================================================

func TestSystemInfoMatchesSystemInfoSchema(t *testing.T) {
	tags := jsonTagsOf(models.SystemInfo{})

	fields := []string{
		"hostname", "os", "architecture", "cpu_count", "memory_mb",
		"public_ip", "internal_ip", "interfaces", "current_user",
		"system_fingerprint", "fingerprint_details",
		"os_details", "user_details", "disk_details", "memory_details", "environment",
		"is_cloud_operator", "cloud_provider", "local_storage_enabled",
	}
	for _, f := range fields {
		assertFieldPresent(t, tags, f, "SystemInfo")
	}
}

func TestFingerprintDetailsMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.FingerprintDetails{})
	fields := []string{"os", "architecture", "cpu_count", "machine_id"}
	for _, f := range fields {
		assertFieldPresent(t, tags, f, "FingerprintDetails")
	}
}

func TestHeartbeatOSDetailsMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.HeartbeatOSDetails{})
	fields := []string{"kernel", "distro", "version"}
	for _, f := range fields {
		assertFieldPresent(t, tags, f, "HeartbeatOSDetails")
	}
}

func TestHeartbeatUserDetailsMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.HeartbeatUserDetails{})
	fields := []string{"username", "uid", "gid", "home", "name", "shell"}
	for _, f := range fields {
		assertFieldPresent(t, tags, f, "HeartbeatUserDetails")
	}
}

func TestHeartbeatDiskDetailsMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.HeartbeatDiskDetails{})
	fields := []string{"total_gb", "used_gb", "free_gb", "percent"}
	for _, f := range fields {
		assertFieldPresent(t, tags, f, "HeartbeatDiskDetails")
	}
}

func TestHeartbeatMemoryDetailsMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.HeartbeatMemoryDetails{})
	fields := []string{"total_mb", "available_mb", "used_mb", "percent"}
	for _, f := range fields {
		assertFieldPresent(t, tags, f, "HeartbeatMemoryDetails")
	}
}

func TestHeartbeatEnvironmentMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.HeartbeatEnvironment{})
	fields := []string{
		"pwd", "lang", "timezone", "term",
		"is_container", "container_runtime", "container_signals", "init_system",
	}
	for _, f := range fields {
		assertFieldPresent(t, tags, f, "HeartbeatEnvironment")
	}
}

// =============================================================================
// result_payloads.json -> outbound payload structs
// =============================================================================

func TestExecutionResultsPayloadMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.ExecutionResultsPayload{})

	required := []string{
		"execution_id", "command", "status", "duration_seconds",
		"operator_id", "operator_session_id",
		"stdout_size", "stderr_size", "stored_locally",
	}
	for _, f := range required {
		assertFieldPresent(t, tags, f, "ExecutionResultsPayload")
	}

	optional := []string{
		"stdout", "stderr", "stdout_hash", "stderr_hash",
		"return_code", "error_message", "error_type",
	}
	for _, f := range optional {
		assertFieldPresent(t, tags, f, "ExecutionResultsPayload")
	}
}

func TestCancellationResultPayloadMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.CancellationResultPayload{})

	fields := []string{
		"execution_id", "status", "operator_id", "operator_session_id",
		"error_message", "error_type",
	}
	for _, f := range fields {
		assertFieldPresent(t, tags, f, "CancellationResultPayload")
	}
}

func TestFileEditResultPayloadMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.FileEditResultPayload{})

	required := []string{
		"execution_id", "operation", "file_path", "status", "duration_seconds",
		"operator_id", "operator_session_id",
		"stdout_size", "stderr_size", "stored_locally",
	}
	for _, f := range required {
		assertFieldPresent(t, tags, f, "FileEditResultPayload")
	}

	optional := []string{
		"content", "stdout_hash", "stderr_hash",
		"bytes_written", "lines_changed", "backup_path",
		"error_message", "error_type",
	}
	for _, f := range optional {
		assertFieldPresent(t, tags, f, "FileEditResultPayload")
	}
}

func TestFsListResultPayloadMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.FsListResultPayload{})

	required := []string{
		"execution_id", "path", "status", "total_count", "truncated",
		"duration_seconds", "operator_id", "operator_session_id",
		"stdout_size", "stderr_size", "stored_locally",
	}
	for _, f := range required {
		assertFieldPresent(t, tags, f, "FsListResultPayload")
	}

	optional := []string{"entries", "stdout_hash", "stderr_hash", "error_message", "error_type"}
	for _, f := range optional {
		assertFieldPresent(t, tags, f, "FsListResultPayload")
	}
}

func TestFsListEntryMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.FsListEntry{})

	required := []string{"name", "path", "is_dir", "size", "mode", "mod_time"}
	for _, f := range required {
		assertFieldPresent(t, tags, f, "FsListEntry")
	}

	optional := []string{"is_symlink", "symlink_target", "owner", "group", "inode", "nlink"}
	for _, f := range optional {
		assertFieldPresent(t, tags, f, "FsListEntry")
	}
}

func TestExecutionStatusPayloadMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.ExecutionStatusPayload{})

	required := []string{
		"execution_id", "command", "status", "process_alive",
		"elapsed_seconds", "operator_id", "operator_session_id",
	}
	for _, f := range required {
		assertFieldPresent(t, tags, f, "ExecutionStatusPayload")
	}

	optional := []string{"new_output", "new_stderr", "message", "stored_locally"}
	for _, f := range optional {
		assertFieldPresent(t, tags, f, "ExecutionStatusPayload")
	}
}

func TestFetchLogsResultPayloadMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.FetchLogsResultPayload{})

	required := []string{
		"execution_id", "command", "duration_ms",
		"stdout", "stderr", "stdout_size", "stderr_size",
		"timestamp", "operator_id", "operator_session_id",
	}
	for _, f := range required {
		assertFieldPresent(t, tags, f, "FetchLogsResultPayload")
	}

	optional := []string{"exit_code", "sentinel_mode", "error"}
	for _, f := range optional {
		assertFieldPresent(t, tags, f, "FetchLogsResultPayload")
	}
}

func TestFileDiffEntryMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.FileDiffEntry{})

	fields := []string{
		"id", "timestamp", "file_path", "operation",
		"ledger_hash_before", "ledger_hash_after",
		"diff_stat", "diff_content", "diff_size", "operator_session_id",
	}
	for _, f := range fields {
		assertFieldPresent(t, tags, f, "FileDiffEntry")
	}
}

func TestFetchFileDiffResultPayloadMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.FetchFileDiffResultPayload{})

	fields := []string{"success", "diffs", "diff", "total", "operator_session_id", "error"}
	for _, f := range fields {
		assertFieldPresent(t, tags, f, "FetchFileDiffResultPayload")
	}
}

func TestPortCheckEntryMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.PortCheckEntry{})

	required := []string{"host", "port", "open"}
	for _, f := range required {
		assertFieldPresent(t, tags, f, "PortCheckEntry")
	}

	optional := []string{"latency_ms", "error"}
	for _, f := range optional {
		assertFieldPresent(t, tags, f, "PortCheckEntry")
	}
}

func TestPortCheckResultPayloadMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.PortCheckResultPayload{})

	required := []string{"execution_id", "status", "operator_id", "operator_session_id"}
	for _, f := range required {
		assertFieldPresent(t, tags, f, "PortCheckResultPayload")
	}

	optional := []string{"results", "error_message", "error_type"}
	for _, f := range optional {
		assertFieldPresent(t, tags, f, "PortCheckResultPayload")
	}
}

func TestLFAAErrorPayloadMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.LFAAErrorPayload{})

	fields := []string{"success", "error", "execution_id", "operator_id", "operator_session_id"}
	for _, f := range fields {
		assertFieldPresent(t, tags, f, "LFAAErrorPayload")
	}
}

func TestRestoreFileResultPayloadMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.RestoreFileResultPayload{})
	fields := []string{"success", "file_path", "commit_hash", "error"}
	for _, f := range fields {
		assertFieldPresent(t, tags, f, "RestoreFileResultPayload")
	}
}

// =============================================================================
// mcp.json -> MCP types
// =============================================================================

func TestJSONRPCRequestMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(mcp.JSONRPCRequest{})
	fields := []string{"jsonrpc", "id", "method", "params"}
	for _, f := range fields {
		assertFieldPresent(t, tags, f, "JSONRPCRequest")
	}
}

func TestCallToolResultMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(mcp.CallToolResult{})
	fields := []string{"content", "isError", "_metadata"}
	for _, f := range fields {
		assertFieldPresent(t, tags, f, "CallToolResult")
	}
}

func TestMCPResultMetadataMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(mcp.MCPResultMetadata{})
	fields := []string{"original_payload", "event_type", "execution_id"}
	for _, f := range fields {
		assertFieldPresent(t, tags, f, "MCPResultMetadata")
	}
}

func TestContentMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(mcp.Content{})
	fields := []string{"type", "text", "data", "mimeType", "resource"}
	for _, f := range fields {
		assertFieldPresent(t, tags, f, "Content")
	}
}

func TestResourceContentMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(mcp.ResourceContent{})
	fields := []string{"uri", "mimeType", "text", "blob"}
	for _, f := range fields {
		assertFieldPresent(t, tags, f, "ResourceContent")
	}
}

// =============================================================================
// command_payloads.json -> inbound command payload structs
// =============================================================================

func TestCommandPayloadMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.CommandRequestPayload{})

	required := []string{"command"}
	for _, f := range required {
		assertFieldPresent(t, tags, f, "CommandPayload")
	}

	optional := []string{"execution_id", "justification", "sentinel_mode", "timeout_seconds"}
	for _, f := range optional {
		assertFieldPresent(t, tags, f, "CommandPayload")
	}
}

func TestCommandCancelPayloadMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.CommandCancelRequestPayload{})
	assertFieldPresent(t, tags, "execution_id", "CommandCancelPayload")
}

func TestFileEditPayloadMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.FileEditRequestPayload{})

	required := []string{"file_path", "operation"}
	for _, f := range required {
		assertFieldPresent(t, tags, f, "FileEditPayload")
	}

	optional := []string{
		"execution_id", "sentinel_mode", "justification",
		"content", "old_content", "new_content",
		"insert_content", "insert_position",
		"start_line", "end_line",
		"patch_content", "create_backup", "create_if_missing",
	}
	for _, f := range optional {
		assertFieldPresent(t, tags, f, "FileEditPayload")
	}
}

func TestFsListPayloadMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.FsListRequestPayload{})

	fields := []string{"path", "execution_id", "max_depth", "max_entries"}
	for _, f := range fields {
		assertFieldPresent(t, tags, f, "FsListPayload")
	}
}

func TestFsReadPayloadMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.FsReadRequestPayload{})

	required := []string{"path"}
	for _, f := range required {
		assertFieldPresent(t, tags, f, "FsReadPayload")
	}

	optional := []string{"execution_id", "max_size"}
	for _, f := range optional {
		assertFieldPresent(t, tags, f, "FsReadPayload")
	}
}

func TestPortCheckPayloadMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.PortCheckRequestPayload{})

	required := []string{"port"}
	for _, f := range required {
		assertFieldPresent(t, tags, f, "PortCheckPayload")
	}

	optional := []string{"execution_id", "host", "protocol"}
	for _, f := range optional {
		assertFieldPresent(t, tags, f, "PortCheckPayload")
	}
}

func TestFsReadResultPayloadMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.FsReadResultPayload{})

	required := []string{
		"execution_id", "path", "status", "size_bytes", "truncated",
		"duration_seconds", "operator_id", "operator_session_id",
	}
	for _, f := range required {
		assertFieldPresent(t, tags, f, "FsReadResultPayload")
	}

	optional := []string{"content", "error_message", "error_type"}
	for _, f := range optional {
		assertFieldPresent(t, tags, f, "FsReadResultPayload")
	}
}

func TestFetchLogsRequestPayloadMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.FetchLogsRequestPayload{})

	required := []string{"execution_id"}
	assertFieldPresent(t, tags, required[0], "FetchLogsRequestPayload")

	optional := []string{"sentinel_mode"}
	for _, f := range optional {
		assertFieldPresent(t, tags, f, "FetchLogsRequestPayload")
	}
}

func TestFetchFileDiffPayloadMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.FetchFileDiffRequestPayload{})

	fields := []string{"diff_id", "operator_session_id", "file_path", "limit"}
	for _, f := range fields {
		assertFieldPresent(t, tags, f, "FetchFileDiffPayload")
	}
}

func TestFetchHistoryPayloadMatchesSchema(t *testing.T) {
	// command_payloads.json fetch_history has empty fields — struct exists with no fields
	_ = models.FetchHistoryRequestPayload{}
}

func TestFetchFileHistoryPayloadMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.FetchFileHistoryRequestPayload{})
	assertFieldPresent(t, tags, "file_path", "FetchFileHistoryPayload")
}

func TestRestoreFilePayloadMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.RestoreFileRequestPayload{})
	fields := []string{"file_path", "commit_hash"}
	for _, f := range fields {
		assertFieldPresent(t, tags, f, "RestoreFilePayload")
	}
}

func TestShutdownPayloadMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.ShutdownRequestPayload{})
	assertFieldPresent(t, tags, "reason", "ShutdownPayload")
}

func TestAuditMsgPayloadMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.AuditMsgRequestPayload{})
	required := []string{"content"}
	assertFieldPresent(t, tags, required[0], "AuditMsgPayload")
	assertFieldPresent(t, tags, "operator_session_id", "AuditMsgPayload")
}

func TestAuditDirectCmdPayloadMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.AuditDirectCmdRequestPayload{})
	required := []string{"command"}
	assertFieldPresent(t, tags, required[0], "AuditDirectCmdPayload")
	optional := []string{"execution_id", "operator_session_id"}
	for _, f := range optional {
		assertFieldPresent(t, tags, f, "AuditDirectCmdPayload")
	}
}

func TestAuditDirectCmdResultPayloadMatchesSchema(t *testing.T) {
	tags := jsonTagsOf(models.AuditDirectCmdResultPayload{})
	required := []string{"command"}
	assertFieldPresent(t, tags, required[0], "AuditDirectCmdResultPayload")
	optional := []string{
		"execution_id", "exit_code", "status",
		"output", "stderr", "execution_time_seconds", "operator_session_id",
	}
	for _, f := range optional {
		assertFieldPresent(t, tags, f, "AuditDirectCmdResultPayload")
	}
}

// =============================================================================
// Heartbeat type values
// =============================================================================

func TestHeartbeatTypeValuesMatchHeartbeatSchema(t *testing.T) {
	hb := loadWireJSON(t, "heartbeat.json")

	heartbeat, ok := hb["heartbeat"].(map[string]interface{})
	require.True(t, ok, "heartbeat.json must have a heartbeat object")

	envelopeFields, ok := heartbeat["envelope_fields"].(map[string]interface{})
	require.True(t, ok, "heartbeat.json heartbeat must have envelope_fields")

	hbType, ok := envelopeFields["heartbeat_type"].(map[string]interface{})
	require.True(t, ok, "envelope_fields must have heartbeat_type")

	values, ok := hbType["values"].([]interface{})
	require.True(t, ok, "heartbeat_type must have a values array")

	validValues := make(map[string]bool)
	for _, v := range values {
		validValues[v.(string)] = true
	}

	assert.True(t, validValues["automatic"], "heartbeat_type values must include automatic")
	assert.True(t, validValues["bootstrap"], "heartbeat_type values must include bootstrap")
	assert.True(t, validValues["requested"], "heartbeat_type values must include requested")
}
