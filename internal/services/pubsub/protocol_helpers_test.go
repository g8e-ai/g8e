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

package pubsub

import (
	"testing"

	"github.com/g8e-ai/g8e/internal/constants"
	"github.com/g8e-ai/g8e/internal/testutil"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
)

func TestMapProtoToPayloadType(t *testing.T) {
	t.Run("maps CommandResult to execution_result", func(t *testing.T) {
		t.Parallel()
		msg := &operatorv1.CommandResult{}
		result := mapProtoToPayloadType(msg)
		assert.Equal(t, "execution_result", result)
	})

	t.Run("maps ExecutionStatusUpdate to execution_status", func(t *testing.T) {
		t.Parallel()
		msg := &operatorv1.ExecutionStatusUpdate{}
		result := mapProtoToPayloadType(msg)
		assert.Equal(t, "execution_status", result)
	})

	t.Run("maps FileEditResult to file_edit_result", func(t *testing.T) {
		t.Parallel()
		msg := &operatorv1.FileEditResult{}
		result := mapProtoToPayloadType(msg)
		assert.Equal(t, "file_edit_result", result)
	})

	t.Run("maps FsListResult to fs_list_result", func(t *testing.T) {
		t.Parallel()
		msg := &operatorv1.FsListResult{}
		result := mapProtoToPayloadType(msg)
		assert.Equal(t, "fs_list_result", result)
	})

	t.Run("maps FsGrepResult to fs_grep_result", func(t *testing.T) {
		t.Parallel()
		msg := &operatorv1.FsGrepResult{}
		result := mapProtoToPayloadType(msg)
		assert.Equal(t, "fs_grep_result", result)
	})

	t.Run("maps FsReadResult to fs_read_result", func(t *testing.T) {
		t.Parallel()
		msg := &operatorv1.FsReadResult{}
		result := mapProtoToPayloadType(msg)
		assert.Equal(t, "fs_read_result", result)
	})

	t.Run("maps PortCheckResult to port_check_result", func(t *testing.T) {
		t.Parallel()
		msg := &operatorv1.PortCheckResult{}
		result := mapProtoToPayloadType(msg)
		assert.Equal(t, "port_check_result", result)
	})

	t.Run("maps FetchLogsResult with error to fetch_logs_error", func(t *testing.T) {
		t.Parallel()
		msg := &operatorv1.FetchLogsResult{Error: "test error"}
		result := mapProtoToPayloadType(msg)
		assert.Equal(t, "fetch_logs_error", result)
	})

	t.Run("maps FetchLogsResult without error to fetch_logs_result", func(t *testing.T) {
		t.Parallel()
		msg := &operatorv1.FetchLogsResult{Error: ""}
		result := mapProtoToPayloadType(msg)
		assert.Equal(t, "fetch_logs_result", result)
	})

	t.Run("maps FetchHistoryResult with success=false to fetch_history_error", func(t *testing.T) {
		t.Parallel()
		msg := &operatorv1.FetchHistoryResult{Success: false}
		result := mapProtoToPayloadType(msg)
		assert.Equal(t, "fetch_history_error", result)
	})

	t.Run("maps FetchHistoryResult with success=true to fetch_history_success", func(t *testing.T) {
		t.Parallel()
		msg := &operatorv1.FetchHistoryResult{Success: true}
		result := mapProtoToPayloadType(msg)
		assert.Equal(t, "fetch_history_success", result)
	})

	t.Run("maps HeartbeatResult to heartbeat", func(t *testing.T) {
		t.Parallel()
		msg := &operatorv1.HeartbeatResult{}
		result := mapProtoToPayloadType(msg)
		assert.Equal(t, "heartbeat", result)
	})

	t.Run("maps FetchFileHistoryResult with success=false to fetch_file_history_error", func(t *testing.T) {
		t.Parallel()
		msg := &operatorv1.FetchFileHistoryResult{Success: false}
		result := mapProtoToPayloadType(msg)
		assert.Equal(t, "fetch_file_history_error", result)
	})

	t.Run("maps FetchFileHistoryResult with success=true to fetch_file_history_success", func(t *testing.T) {
		t.Parallel()
		msg := &operatorv1.FetchFileHistoryResult{Success: true}
		result := mapProtoToPayloadType(msg)
		assert.Equal(t, "fetch_file_history_success", result)
	})

	t.Run("maps RestoreFileResult with success=false to restore_file_error", func(t *testing.T) {
		t.Parallel()
		msg := &operatorv1.RestoreFileResult{Success: false}
		result := mapProtoToPayloadType(msg)
		assert.Equal(t, "restore_file_error", result)
	})

	t.Run("maps RestoreFileResult with success=true to restore_file_success", func(t *testing.T) {
		t.Parallel()
		msg := &operatorv1.RestoreFileResult{Success: true}
		result := mapProtoToPayloadType(msg)
		assert.Equal(t, "restore_file_success", result)
	})

	t.Run("maps FetchFileDiffResult with success=false to fetch_file_diff_error", func(t *testing.T) {
		t.Parallel()
		msg := &operatorv1.FetchFileDiffResult{Success: false}
		result := mapProtoToPayloadType(msg)
		assert.Equal(t, "fetch_file_diff_error", result)
	})

	t.Run("maps FetchFileDiffResult with success=true and diff to fetch_file_diff_by_id_success", func(t *testing.T) {
		t.Parallel()
		msg := &operatorv1.FetchFileDiffResult{Success: true, Diff: &operatorv1.FileDiffEntry{}}
		result := mapProtoToPayloadType(msg)
		assert.Equal(t, "fetch_file_diff_by_id_success", result)
	})

	t.Run("maps FetchFileDiffResult with success=true and no diff to fetch_file_diff_by_session_success", func(t *testing.T) {
		t.Parallel()
		msg := &operatorv1.FetchFileDiffResult{Success: true, Diff: nil}
		result := mapProtoToPayloadType(msg)
		assert.Equal(t, "fetch_file_diff_by_session_success", result)
	})

	t.Run("maps unknown type to unknown", func(t *testing.T) {
		t.Parallel()
		msg := &operatorv1.ShutdownRequested{}
		result := mapProtoToPayloadType(msg)
		assert.Equal(t, string(constants.SystemHealthUnknown), result)
	})
}

func TestBuildUniversalResultEnvelope(t *testing.T) {
	t.Run("builds envelope successfully", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		payload := &operatorv1.CommandResult{ExecutionId: "exec-1"}

		env, err := BuildUniversalResultEnvelope(
			cfg,
			constants.Event.Operator.Command.Completed,
			payload,
			"msg-1",
			"operator-1",
			"case-1",
			"investigation-1",
			nil,
			"web-session-1",
			"cli-session-1",
		)

		require.NoError(t, err)
		require.NotNil(t, env)
		assert.Equal(t, "msg-1", env.Id)
		assert.Equal(t, "operator-1", env.OperatorId)
		assert.Equal(t, "case-1", env.CaseId)
		assert.Equal(t, "investigation-1", env.InvestigationId)
		assert.Equal(t, "web-session-1", env.WebSessionId)
		assert.Equal(t, "cli-session-1", env.CliSessionId)
		assert.NotNil(t, env.IntentData)
	})

	t.Run("generates ID when not provided", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		payload := &operatorv1.CommandResult{ExecutionId: "exec-1"}

		env, err := BuildUniversalResultEnvelope(
			cfg,
			constants.Event.Operator.Command.Completed,
			payload,
			"",
			"operator-1",
			"case-1",
			"investigation-1",
			nil,
			"web-session-1",
			"cli-session-1",
		)

		require.NoError(t, err)
		require.NotNil(t, env)
		assert.NotEmpty(t, env.Id)
		assert.Contains(t, env.Id, "res_")
	})

	t.Run("includes task ID when provided", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		payload := &operatorv1.CommandResult{ExecutionId: "exec-1"}
		taskID := "task-1"

		env, err := BuildUniversalResultEnvelope(
			cfg,
			constants.Event.Operator.Command.Completed,
			payload,
			"msg-1",
			"operator-1",
			"case-1",
			"investigation-1",
			&taskID,
			"web-session-1",
			"cli-session-1",
		)

		require.NoError(t, err)
		require.NotNil(t, env)
		assert.Equal(t, "task-1", env.TaskId)
	})

	t.Run("injects payload_type into IntentData", func(t *testing.T) {
		t.Parallel()
		cfg := testutil.NewTestConfig(t)
		payload := &operatorv1.CommandResult{ExecutionId: "exec-1"}

		env, err := BuildUniversalResultEnvelope(
			cfg,
			constants.Event.Operator.Command.Completed,
			payload,
			"msg-1",
			"operator-1",
			"case-1",
			"investigation-1",
			nil,
			"web-session-1",
			"cli-session-1",
		)

		require.NoError(t, err)
		require.NotNil(t, env.IntentData)
		payloadType, ok := env.IntentData.Fields["payload_type"]
		require.True(t, ok)
		assert.Equal(t, "execution_result", payloadType.GetStringValue())
	})
}

func TestUnmarshalPayload(t *testing.T) {
	t.Run("rejects oversized payload", func(t *testing.T) {
		t.Parallel()
		largePayload := make([]byte, MaxPayloadSize+1)
		_, err := unmarshalPayload(constants.Event.Operator.Command.Requested, largePayload)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum size limit")
	})

	t.Run("unmarshals HeartbeatRequested", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.HeartbeatRequested{}
		payload, _ := proto.Marshal(req)
		msg, err := unmarshalPayload(constants.Event.Operator.HeartbeatRequested, payload)
		require.NoError(t, err)
		assert.IsType(t, &operatorv1.HeartbeatRequested{}, msg)
	})

	t.Run("unmarshals CommandRequested", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.CommandRequested{Command: "ls -la"}
		payload, _ := proto.Marshal(req)
		msg, err := unmarshalPayload(constants.Event.Operator.Command.Requested, payload)
		require.NoError(t, err)
		assert.IsType(t, &operatorv1.CommandRequested{}, msg)
	})

	t.Run("unmarshals CommandCancelRequested", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.CommandCancelRequested{ExecutionId: "exec-1"}
		payload, _ := proto.Marshal(req)
		msg, err := unmarshalPayload(constants.Event.Operator.Command.CancelRequested, payload)
		require.NoError(t, err)
		assert.IsType(t, &operatorv1.CommandCancelRequested{}, msg)
	})

	t.Run("unmarshals FileEditRequested", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.FileEditRequested{FilePath: "/tmp/test.txt", Operation: "write"}
		payload, _ := proto.Marshal(req)
		msg, err := unmarshalPayload(constants.Event.Operator.FileEdit.Requested, payload)
		require.NoError(t, err)
		assert.IsType(t, &operatorv1.FileEditRequested{}, msg)
	})

	t.Run("unmarshals FsListRequested", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.FsListRequested{Path: "."}
		payload, _ := proto.Marshal(req)
		msg, err := unmarshalPayload(constants.Event.Operator.FsList.Requested, payload)
		require.NoError(t, err)
		assert.IsType(t, &operatorv1.FsListRequested{}, msg)
	})

	t.Run("unmarshals FsReadRequested", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.FsReadRequested{Path: "/tmp/test.txt"}
		payload, _ := proto.Marshal(req)
		msg, err := unmarshalPayload(constants.Event.Operator.FsRead.Requested, payload)
		require.NoError(t, err)
		assert.IsType(t, &operatorv1.FsReadRequested{}, msg)
	})

	t.Run("unmarshals FsGrepRequested", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.FsGrepRequested{Path: ".", Pattern: "test"}
		payload, _ := proto.Marshal(req)
		msg, err := unmarshalPayload(constants.Event.Operator.FsGrep.Requested, payload)
		require.NoError(t, err)
		assert.IsType(t, &operatorv1.FsGrepRequested{}, msg)
	})

	t.Run("unmarshals CheckPortRequested", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.CheckPortRequested{Port: 8080}
		payload, _ := proto.Marshal(req)
		msg, err := unmarshalPayload(constants.Event.Operator.PortCheck.Requested, payload)
		require.NoError(t, err)
		assert.IsType(t, &operatorv1.CheckPortRequested{}, msg)
	})

	t.Run("unmarshals FetchLogsRequested", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.FetchLogsRequested{ExecutionId: "exec-1"}
		payload, _ := proto.Marshal(req)
		msg, err := unmarshalPayload(constants.Event.Operator.FetchLogs.Requested, payload)
		require.NoError(t, err)
		assert.IsType(t, &operatorv1.FetchLogsRequested{}, msg)
	})

	t.Run("unmarshals FetchHistoryRequested", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.FetchHistoryRequested{}
		payload, _ := proto.Marshal(req)
		msg, err := unmarshalPayload(constants.Event.Operator.FetchHistory.Requested, payload)
		require.NoError(t, err)
		assert.IsType(t, &operatorv1.FetchHistoryRequested{}, msg)
	})

	t.Run("unmarshals FetchFileHistoryRequested", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.FetchFileHistoryRequested{FilePath: "/tmp/test.txt"}
		payload, _ := proto.Marshal(req)
		msg, err := unmarshalPayload(constants.Event.Operator.FetchFileHistory.Requested, payload)
		require.NoError(t, err)
		assert.IsType(t, &operatorv1.FetchFileHistoryRequested{}, msg)
	})

	t.Run("unmarshals RestoreFileRequested", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.RestoreFileRequested{FilePath: "/tmp/test.txt", CommitHash: "abc123"}
		payload, _ := proto.Marshal(req)
		msg, err := unmarshalPayload(constants.Event.Operator.RestoreFile.Requested, payload)
		require.NoError(t, err)
		assert.IsType(t, &operatorv1.RestoreFileRequested{}, msg)
	})

	t.Run("unmarshals ShutdownRequested", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.ShutdownRequested{Reason: "test"}
		payload, _ := proto.Marshal(req)
		msg, err := unmarshalPayload(constants.Event.Operator.ShutdownRequested, payload)
		require.NoError(t, err)
		assert.IsType(t, &operatorv1.ShutdownRequested{}, msg)
	})

	t.Run("unmarshals AuditMsgRequested", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.AuditMsgRequested{Content: "test"}
		payload, _ := proto.Marshal(req)
		msg, err := unmarshalPayload(constants.Event.Operator.Audit.UserMsg, payload)
		require.NoError(t, err)
		assert.IsType(t, &operatorv1.AuditMsgRequested{}, msg)
	})

	t.Run("unmarshals DirectCommandAuditRequested", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.DirectCommandAuditRequested{Command: "ls -la"}
		payload, _ := proto.Marshal(req)
		msg, err := unmarshalPayload(constants.Event.Operator.Audit.DirectCmd, payload)
		require.NoError(t, err)
		assert.IsType(t, &operatorv1.DirectCommandAuditRequested{}, msg)
	})

	t.Run("unmarshals DirectCommandResultAuditRequested", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.DirectCommandResultAuditRequested{Command: "ls -la"}
		payload, _ := proto.Marshal(req)
		msg, err := unmarshalPayload(constants.Event.Operator.Audit.DirectCmdResult, payload)
		require.NoError(t, err)
		assert.IsType(t, &operatorv1.DirectCommandResultAuditRequested{}, msg)
	})

	t.Run("unmarshals FetchFileDiffRequested", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.FetchFileDiffRequested{DiffId: "diff-1"}
		payload, _ := proto.Marshal(req)
		msg, err := unmarshalPayload(constants.Event.Operator.FetchFileDiff.Requested, payload)
		require.NoError(t, err)
		assert.IsType(t, &operatorv1.FetchFileDiffRequested{}, msg)
	})

	t.Run("unmarshals McpCallRequested", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.McpCallRequested{ToolName: "test"}
		payload, _ := proto.Marshal(req)
		msg, err := unmarshalPayload(constants.Event.Operator.Mcp.CallRequested, payload)
		require.NoError(t, err)
		assert.IsType(t, &operatorv1.McpCallRequested{}, msg)
	})

	t.Run("unmarshals A2ACallRequested", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.A2ACallRequested{SkillName: "test"}
		payload, _ := proto.Marshal(req)
		msg, err := unmarshalPayload(constants.Event.Operator.A2a.CallRequested, payload)
		require.NoError(t, err)
		assert.IsType(t, &operatorv1.A2ACallRequested{}, msg)
	})

	t.Run("rejects unknown event type", func(t *testing.T) {
		t.Parallel()
		req := &operatorv1.CommandRequested{Command: "ls -la"}
		payload, _ := proto.Marshal(req)
		_, err := unmarshalPayload("UNKNOWN_EVENT", payload)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "unknown event type")
	})

	t.Run("rejects invalid protobuf", func(t *testing.T) {
		t.Parallel()
		invalidPayload := []byte("invalid protobuf")
		_, err := unmarshalPayload(constants.Event.Operator.Command.Requested, invalidPayload)
		assert.Error(t, err)
	})
}
