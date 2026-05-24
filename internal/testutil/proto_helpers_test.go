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

package testutil

import (
	"encoding/json"
	"testing"

	"google.golang.org/protobuf/proto"

	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
	"github.com/g8e-ai/g8e/pkg/uap"
	"github.com/stretchr/testify/require"
)

func TestMustMarshalJSON(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
	}{
		{
			name:  "string",
			value: "test-string",
		},
		{
			name:  "number",
			value: 123,
		},
		{
			name:  "map",
			value: map[string]string{"key": "value"},
		},
		{
			name:  "slice",
			value: []string{"a", "b", "c"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := MustMarshalJSON(t, tt.value)
			require.NotNil(t, result)

			// Verify it's valid JSON
			var decoded interface{}
			err := json.Unmarshal(result, &decoded)
			require.NoError(t, err)
		})
	}
}

func TestMustBuildCommandRequestedPayload(t *testing.T) {
	payload := MustBuildCommandRequestedPayload(t, "ls -la", "exec-123", "test justification", "strict", 30)
	require.NotNil(t, payload)
	require.Greater(t, len(payload), 0)

	// Verify it can be unmarshaled
	cmd := &operatorv1.CommandRequested{}
	err := proto.Unmarshal(payload, cmd)
	require.NoError(t, err)
	require.Equal(t, "ls -la", cmd.Command)
	require.Equal(t, "exec-123", cmd.ExecutionId)
	require.Equal(t, "test justification", cmd.Justification)
	require.Equal(t, "strict", cmd.SentinelMode)
	require.Equal(t, int32(30), cmd.TimeoutSeconds)
}

func TestMustBuildCommandCancelRequestedPayload(t *testing.T) {
	payload := MustBuildCommandCancelRequestedPayload(t, "exec-456")
	require.NotNil(t, payload)
	require.Greater(t, len(payload), 0)

	// Verify it can be unmarshaled
	cancel := &operatorv1.CommandCancelRequested{}
	err := proto.Unmarshal(payload, cancel)
	require.NoError(t, err)
	require.Equal(t, "exec-456", cancel.ExecutionId)
}

func TestMustUnmarshalPayload(t *testing.T) {
	// Create a test payload
	testCmd := &operatorv1.CommandRequested{
		Command:        "test",
		ExecutionId:    "exec-789",
		Justification:  "test",
		SentinelMode:   "strict",
		TimeoutSeconds: 30,
	}
	payload, err := proto.Marshal(testCmd)
	require.NoError(t, err)

	// Test unmarshaling
	result := &operatorv1.CommandRequested{}
	MustUnmarshalPayload(t, payload, result)
	require.Equal(t, "test", result.Command)
	require.Equal(t, "exec-789", result.ExecutionId)
}

func TestMustBuildFileEditRequestedPayload(t *testing.T) {
	fields := FileEditRequestFields{
		FilePath:        "/tmp/test.txt",
		Operation:       "replace",
		ExecutionId:     "exec-001",
		Justification:   "test edit",
		Content:         "old content",
		OldContent:      "old content",
		NewContent:      "new content",
		InsertContent:   "",
		InsertPosition:  0,
		StartLine:       1,
		EndLine:         5,
		PatchContent:    "",
		CreateBackup:    true,
		CreateIfMissing: false,
	}

	payload := MustBuildFileEditRequestedPayload(t, fields)
	require.NotNil(t, payload)

	edit := &operatorv1.FileEditRequested{}
	err := proto.Unmarshal(payload, edit)
	require.NoError(t, err)
	require.Equal(t, "/tmp/test.txt", edit.FilePath)
	require.Equal(t, "replace", edit.Operation)
	require.Equal(t, "exec-001", edit.ExecutionId)
	require.True(t, edit.CreateBackup)
}

func TestMustBuildFsListRequestedPayload(t *testing.T) {
	payload := MustBuildFsListRequestedPayload(t, "/tmp", "exec-002", 100, 5)
	require.NotNil(t, payload)

	list := &operatorv1.FsListRequested{}
	err := proto.Unmarshal(payload, list)
	require.NoError(t, err)
	require.Equal(t, "/tmp", list.Path)
	require.Equal(t, "exec-002", list.ExecutionId)
	require.Equal(t, int32(100), list.MaxEntries)
	require.Equal(t, int32(5), list.MaxDepth)
}

func TestMustBuildCheckPortRequestedPayload(t *testing.T) {
	payload := MustBuildCheckPortRequestedPayload(t, "localhost", 8080, "tcp", "exec-003")
	require.NotNil(t, payload)

	check := &operatorv1.CheckPortRequested{}
	err := proto.Unmarshal(payload, check)
	require.NoError(t, err)
	require.Equal(t, "localhost", check.Host)
	require.Equal(t, int32(8080), check.Port)
	require.Equal(t, "tcp", check.Protocol)
	require.Equal(t, "exec-003", check.ExecutionId)
}

func TestMustBuildFsReadRequestedPayload(t *testing.T) {
	payload := MustBuildFsReadRequestedPayload(t, "/tmp/file.txt", "exec-004", 1024)
	require.NotNil(t, payload)

	read := &operatorv1.FsReadRequested{}
	err := proto.Unmarshal(payload, read)
	require.NoError(t, err)
	require.Equal(t, "/tmp/file.txt", read.Path)
	require.Equal(t, "exec-004", read.ExecutionId)
	require.Equal(t, int32(1024), read.MaxSize)
}

func TestMustBuildFetchLogsRequestedPayload(t *testing.T) {
	payload := MustBuildFetchLogsRequestedPayload(t, "exec-005")
	require.NotNil(t, payload)

	logs := &operatorv1.FetchLogsRequested{}
	err := proto.Unmarshal(payload, logs)
	require.NoError(t, err)
	require.Equal(t, "exec-005", logs.ExecutionId)
}

func TestMustBuildFetchHistoryRequestedPayload(t *testing.T) {
	payload := MustBuildFetchHistoryRequestedPayload(t, "exec-006", "session-123", 50, 0)
	require.NotNil(t, payload)

	hist := &operatorv1.FetchHistoryRequested{}
	err := proto.Unmarshal(payload, hist)
	require.NoError(t, err)
	require.Equal(t, "exec-006", hist.ExecutionId)
	require.Equal(t, "session-123", hist.OperatorSessionId)
	require.Equal(t, int32(50), hist.Limit)
	require.Equal(t, int32(0), hist.Offset)
}

func TestMustBuildFetchFileHistoryRequestedPayload(t *testing.T) {
	payload := MustBuildFetchFileHistoryRequestedPayload(t, "exec-007", "/tmp/file.txt", 20, "session-456")
	require.NotNil(t, payload)

	fhist := &operatorv1.FetchFileHistoryRequested{}
	err := proto.Unmarshal(payload, fhist)
	require.NoError(t, err)
	require.Equal(t, "exec-007", fhist.ExecutionId)
	require.Equal(t, "/tmp/file.txt", fhist.FilePath)
	require.Equal(t, int32(20), fhist.Limit)
	require.Equal(t, "session-456", fhist.OperatorSessionId)
}

func TestMustBuildFetchFileDiffRequestedPayload(t *testing.T) {
	payload := MustBuildFetchFileDiffRequestedPayload(t, "exec-008", "/tmp/file.txt")
	require.NotNil(t, payload)

	diff := &operatorv1.FetchFileDiffRequested{}
	err := proto.Unmarshal(payload, diff)
	require.NoError(t, err)
	require.Equal(t, "exec-008", diff.ExecutionId)
	require.Equal(t, "/tmp/file.txt", diff.FilePath)
}

func TestMustBuildRestoreFileRequestedPayload(t *testing.T) {
	payload := MustBuildRestoreFileRequestedPayload(t, "exec-009", "/tmp/file.txt", "abc123", "session-789")
	require.NotNil(t, payload)

	restore := &operatorv1.RestoreFileRequested{}
	err := proto.Unmarshal(payload, restore)
	require.NoError(t, err)
	require.Equal(t, "exec-009", restore.ExecutionId)
	require.Equal(t, "/tmp/file.txt", restore.FilePath)
	require.Equal(t, "abc123", restore.CommitHash)
	require.Equal(t, "session-789", restore.OperatorSessionId)
}

func TestMustBuildAuditMsgRequestedPayload(t *testing.T) {
	payload := MustBuildAuditMsgRequestedPayload(t, "audit message content")
	require.NotNil(t, payload)

	audit := &operatorv1.AuditMsgRequested{}
	err := proto.Unmarshal(payload, audit)
	require.NoError(t, err)
	require.Equal(t, "audit message content", audit.Content)
}

func TestMustBuildDirectCommandAuditRequestedPayload(t *testing.T) {
	payload := MustBuildDirectCommandAuditRequestedPayload(t, "ls", "exec-010", "session-111", "command")
	require.NotNil(t, payload)

	audit := &operatorv1.DirectCommandAuditRequested{}
	err := proto.Unmarshal(payload, audit)
	require.NoError(t, err)
	require.Equal(t, "ls", audit.Command)
	require.Equal(t, "exec-010", audit.ExecutionId)
	require.Equal(t, "session-111", audit.OperatorSessionId)
	require.Equal(t, "command", audit.Type)
}

func TestMustBuildDirectCommandResultAuditRequestedPayload(t *testing.T) {
	payload := MustBuildDirectCommandResultAuditRequestedPayload(t, "ls", "exec-011", "output", "stderr", 0, 1.5)
	require.NotNil(t, payload)

	result := &operatorv1.DirectCommandResultAuditRequested{}
	err := proto.Unmarshal(payload, result)
	require.NoError(t, err)
	require.Equal(t, "ls", result.Command)
	require.Equal(t, "exec-011", result.ExecutionId)
	require.Equal(t, "output", result.Output)
	require.Equal(t, "stderr", result.Stderr)
	require.Equal(t, int32(0), result.ExitCode)
	require.Equal(t, float32(1.5), result.ExecutionTimeSeconds)
}

func TestMustBuildHeartbeatRequestedPayload(t *testing.T) {
	payload := MustBuildHeartbeatRequestedPayload(t)
	require.NotNil(t, payload)

	hb := &operatorv1.HeartbeatRequested{}
	err := proto.Unmarshal(payload, hb)
	require.NoError(t, err)
}

func TestMustBuildShutdownRequestedPayload(t *testing.T) {
	payload := MustBuildShutdownRequestedPayload(t, "maintenance")
	require.NotNil(t, payload)

	shutdown := &operatorv1.ShutdownRequested{}
	err := proto.Unmarshal(payload, shutdown)
	require.NoError(t, err)
	require.Equal(t, "maintenance", shutdown.Reason)
}

func TestMustMarshalUAPEnvelope(t *testing.T) {
	taskID := "task-123"
	payload := MustMarshalUAPEnvelope(t, "msg-001", "v1.0", "operator-1", "execute", "/tmp", "json", "{}", 3, "case-001", "inv-001", &taskID)
	require.NotNil(t, payload)

	// Verify it's valid JSON
	env := &uap.UAPEnvelope{}
	err := json.Unmarshal(payload, env)
	require.NoError(t, err)
	require.Equal(t, "msg-001", env.Id)
	require.Equal(t, "v1.0", env.ProtocolVersion)
	require.Equal(t, "operator-1", env.OperatorId)
	require.Equal(t, "execute", env.ActionType)
	require.Equal(t, "/tmp", env.TargetResource)
	require.Equal(t, "case-001", env.CaseId)
	require.Equal(t, "inv-001", env.InvestigationId)
	require.Equal(t, "task-123", env.TaskId)
}

func TestMustMarshalUAPEnvelopeWithVotes(t *testing.T) {
	agentIDs := []string{"agent-1", "agent-2", "agent-3"}
	tribunalSig := "sig-abc123"
	payload := MustMarshalUAPEnvelopeWithVotes(t, "msg-002", "v1.0", "operator-2", "execute", "/tmp", "json", "{}", 3, agentIDs, tribunalSig)
	require.NotNil(t, payload)

	env := &uap.UAPEnvelope{}
	err := json.Unmarshal(payload, env)
	require.NoError(t, err)
	require.Equal(t, "msg-002", env.Id)
	require.NotNil(t, env.Governance)
	require.NotNil(t, env.Governance.L2)
	require.Equal(t, agentIDs, env.Governance.L2.AgentIds)
	require.Equal(t, tribunalSig, env.Governance.L2.TribunalSignature)
}

func TestMustUnmarshalUAPEnvelope(t *testing.T) {
	taskID := "task-456"
	payload := MustMarshalUAPEnvelope(t, "msg-003", "v1.0", "operator-3", "execute", "/tmp", "json", "{}", 3, "case-002", "inv-002", &taskID)

	env := MustUnmarshalUAPEnvelope(t, payload)
	require.NotNil(t, env)
	require.Equal(t, "msg-003", env.Id)
	require.Equal(t, "v1.0", env.ProtocolVersion)
	require.Equal(t, "operator-3", env.OperatorId)
}

func TestMustGenerateUAPMessageID(t *testing.T) {
	id := MustGenerateUAPMessageID(t, "execute", "/tmp", "json", "{}")
	require.NotEmpty(t, id)

	// Different inputs should generate different ID
	id2 := MustGenerateUAPMessageID(t, "read", "/tmp", "json", "{}")
	require.NotEqual(t, id, id2, "Different inputs should generate different IDs")

	// Different target should generate different ID
	id3 := MustGenerateUAPMessageID(t, "execute", "/var", "json", "{}")
	require.NotEqual(t, id, id3, "Different target should generate different IDs")
}

func TestMustMarshalUAPEnvelopeWithNonce(t *testing.T) {
	taskID := "task-789"
	nonce := "nonce-xyz123"
	payload := MustMarshalUAPEnvelopeWithNonce(t, "msg-004", "v1.0", "operator-4", "execute", "/tmp", "json", "{}", 3, "case-003", "inv-003", &taskID, nonce)
	require.NotNil(t, payload)

	env := &uap.UAPEnvelope{}
	err := json.Unmarshal(payload, env)
	require.NoError(t, err)
	require.Equal(t, "msg-004", env.Id)
	require.Equal(t, nonce, env.Nonce)
}

func TestMustCreateUAPVote(t *testing.T) {
	vote := MustCreateUAPVote(t, "node-1", "sig-123", true)
	require.NotNil(t, vote)
	require.Len(t, vote, 1)
	require.Equal(t, "node-1", vote[0])
}
