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
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/g8e-ai/g8e/internal/governance"
	commonv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/common/v1"
	operatorv1 "github.com/g8e-ai/g8e/protocol/proto/g8e/operator/v1"
)

// MarshalJSON marshals v to json.RawMessage, fatally failing the test on error.
func MarshalJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("MarshalJSON: %v", err)
	}
	return json.RawMessage(b)
}

// BuildCommandRequestedPayload builds a CommandRequested payload bytes.
func BuildCommandRequestedPayload(t *testing.T, cmd string, execID string, justification string, sentinelMode string, timeout int) []byte {
	t.Helper()
	protoCmd := &operatorv1.CommandRequested{
		Command:        cmd,
		ExecutionId:    execID,
		Justification:  justification,
		VaultMode:      sentinelMode,
		TimeoutSeconds: int32(timeout), //nolint:gosec // test utility, timeout values bounded
	}
	b, err := proto.Marshal(protoCmd)
	if err != nil {
		t.Fatalf("failed to marshal protobuf CommandRequested: %v", err)
	}
	return b
}

// BuildCommandCancelRequestedPayload builds a CommandCancelRequested payload bytes.
func BuildCommandCancelRequestedPayload(t *testing.T, execID string) []byte {
	t.Helper()
	protoCancel := &operatorv1.CommandCancelRequested{
		ExecutionId: execID,
	}
	b, err := proto.Marshal(protoCancel)
	if err != nil {
		t.Fatalf("failed to marshal protobuf CommandCancelRequested: %v", err)
	}
	return b
}

// UnmarshalPayload unmarshals bytes to a specific proto.Message, fatally failing the test on error.
func UnmarshalPayload(t *testing.T, data []byte, m proto.Message) {
	t.Helper()
	if err := proto.Unmarshal(data, m); err != nil {
		t.Fatalf("failed to unmarshal payload: %v", err)
	}
}

// FileEditRequestFields is a helper struct for BuildFileEditRequestedPayload
type FileEditRequestFields struct {
	FilePath        string
	Operation       string
	ExecutionId     string
	Justification   string
	Content         string
	OldContent      string
	NewContent      string
	InsertContent   string
	InsertPosition  int32
	StartLine       int32
	EndLine         int32
	PatchContent    string
	CreateBackup    bool
	CreateIfMissing bool
}

// BuildFileEditRequestedPayload builds a FileEditRequested payload bytes.
func BuildFileEditRequestedPayload(t *testing.T, f *FileEditRequestFields) []byte {
	t.Helper()
	protoFileEdit := &operatorv1.FileEditRequested{
		FilePath:        f.FilePath,
		Operation:       f.Operation,
		ExecutionId:     f.ExecutionId,
		Justification:   f.Justification,
		Content:         f.Content,
		OldContent:      f.OldContent,
		NewContent:      f.NewContent,
		InsertContent:   f.InsertContent,
		InsertPosition:  f.InsertPosition,
		StartLine:       f.StartLine,
		EndLine:         f.EndLine,
		PatchContent:    f.PatchContent,
		CreateBackup:    f.CreateBackup,
		CreateIfMissing: f.CreateIfMissing,
	}
	b, err := proto.Marshal(protoFileEdit)
	if err != nil {
		t.Fatalf("failed to marshal protobuf FileEditRequested: %v", err)
	}
	return b
}

// BuildFsListRequestedPayload builds a FsListRequested payload bytes.
func BuildFsListRequestedPayload(t *testing.T, path string, execID string, maxEntries int32, maxDepth int32) []byte {
	t.Helper()
	protoFsList := &operatorv1.FsListRequested{
		Path:        path,
		ExecutionId: execID,
		MaxEntries:  maxEntries,
		MaxDepth:    maxDepth,
	}
	b, err := proto.Marshal(protoFsList)
	if err != nil {
		t.Fatalf("failed to marshal protobuf FsListRequested: %v", err)
	}
	return b
}

// BuildCheckPortRequestedPayload builds a CheckPortRequested payload bytes.
func BuildCheckPortRequestedPayload(t *testing.T, host string, port int32, protocol string, execID string) []byte {
	t.Helper()
	p := &operatorv1.CheckPortRequested{
		ExecutionId: execID,
		Host:        host,
		Port:        port,
		Protocol:    protocol,
	}
	b, err := proto.Marshal(p)
	if err != nil {
		t.Fatalf("failed to marshal protobuf CheckPortRequested: %v", err)
	}
	return b
}

// BuildFsReadRequestedPayload builds a FsReadRequested payload bytes.
func BuildFsReadRequestedPayload(t *testing.T, path string, execID string, maxSize int32) []byte {
	t.Helper()
	protoFsRead := &operatorv1.FsReadRequested{
		Path:        path,
		ExecutionId: execID,
		MaxSize:     maxSize,
	}
	b, err := proto.Marshal(protoFsRead)
	if err != nil {
		t.Fatalf("failed to marshal protobuf FsReadRequested: %v", err)
	}
	return b
}

// BuildFetchLogsRequestedPayload builds a FetchLogsRequested payload bytes.
func BuildFetchLogsRequestedPayload(t *testing.T, execID string) []byte {
	t.Helper()
	p := &operatorv1.FetchLogsRequested{
		ExecutionId: execID,
	}
	b, err := proto.Marshal(p)
	if err != nil {
		t.Fatalf("failed to marshal protobuf FetchLogsRequested: %v", err)
	}
	return b
}

// BuildFetchHistoryRequestedPayload builds a FetchHistoryRequested payload bytes.
func BuildFetchHistoryRequestedPayload(t *testing.T, execID string, sessionID string, limit int32, offset int32) []byte {
	t.Helper()
	p := &operatorv1.FetchHistoryRequested{
		ExecutionId:       execID,
		OperatorSessionId: sessionID,
		Limit:             limit,
		Offset:            offset,
	}
	b, err := proto.Marshal(p)
	if err != nil {
		t.Fatalf("failed to marshal protobuf FetchHistoryRequested: %v", err)
	}
	return b
}

// BuildFetchFileHistoryRequestedPayload builds a FetchFileHistoryRequested payload bytes.
func BuildFetchFileHistoryRequestedPayload(t *testing.T, execID string, filePath string, limit int32, operatorSessionID string) []byte {
	t.Helper()
	p := &operatorv1.FetchFileHistoryRequested{
		ExecutionId:       execID,
		FilePath:          filePath,
		Limit:             limit,
		OperatorSessionId: operatorSessionID,
	}
	b, err := proto.Marshal(p)
	if err != nil {
		t.Fatalf("failed to marshal protobuf FetchFileHistoryRequested: %v", err)
	}
	return b
}

// BuildFetchFileDiffRequestedPayload builds a FetchFileDiffRequested payload bytes.
func BuildFetchFileDiffRequestedPayload(t *testing.T, execID string, filePath string) []byte {
	t.Helper()
	p := &operatorv1.FetchFileDiffRequested{
		ExecutionId: execID,
		FilePath:    filePath,
	}
	b, err := proto.Marshal(p)
	if err != nil {
		t.Fatalf("failed to marshal protobuf FetchFileDiffRequested: %v", err)
	}
	return b
}

// BuildRestoreFileRequestedPayload builds a RestoreFileRequested payload bytes.
func BuildRestoreFileRequestedPayload(t *testing.T, execID string, filePath string, commitHash string, sessionID string) []byte {
	t.Helper()
	p := &operatorv1.RestoreFileRequested{
		ExecutionId:       execID,
		FilePath:          filePath,
		CommitHash:        commitHash,
		OperatorSessionId: sessionID,
	}
	b, err := proto.Marshal(p)
	if err != nil {
		t.Fatalf("failed to marshal protobuf RestoreFileRequested: %v", err)
	}
	return b
}

// BuildAuditMsgRequestedPayload builds an AuditMsgRequested payload bytes.
func BuildAuditMsgRequestedPayload(t *testing.T, content string) []byte {
	t.Helper()
	p := &operatorv1.AuditMsgRequested{
		Content: content,
	}
	b, err := proto.Marshal(p)
	if err != nil {
		t.Fatalf("failed to marshal protobuf AuditMsgRequested: %v", err)
	}
	return b
}

// BuildDirectCommandAuditRequestedPayload builds a DirectCommandAuditRequested payload bytes.
func BuildDirectCommandAuditRequestedPayload(t *testing.T, cmd string, execID string, sessionID string, typeStr string) []byte {
	t.Helper()
	p := &operatorv1.DirectCommandAuditRequested{
		Command:           cmd,
		ExecutionId:       execID,
		OperatorSessionId: sessionID,
		Type:              typeStr,
	}
	b, err := proto.Marshal(p)
	if err != nil {
		t.Fatalf("failed to marshal protobuf DirectCommandAuditRequested: %v", err)
	}
	return b
}

// BuildDirectCommandResultAuditRequestedPayload builds a DirectCommandResultAuditRequested payload bytes.
func BuildDirectCommandResultAuditRequestedPayload(t *testing.T, cmd string, execID string, output string, stderr string, exitCode int32, duration float32) []byte {
	t.Helper()
	p := &operatorv1.DirectCommandResultAuditRequested{
		Command:              cmd,
		ExecutionId:          execID,
		Output:               output,
		Stderr:               stderr,
		ExitCode:             exitCode,
		ExecutionTimeSeconds: duration,
	}
	b, err := proto.Marshal(p)
	if err != nil {
		t.Fatalf("failed to marshal protobuf DirectCommandResultAuditRequested: %v", err)
	}
	return b
}

// BuildHeartbeatRequestedPayload builds an empty HeartbeatRequested payload bytes.
func BuildHeartbeatRequestedPayload(t *testing.T) []byte {
	t.Helper()
	protoHb := &operatorv1.HeartbeatRequested{}
	b, err := proto.Marshal(protoHb)
	if err != nil {
		t.Fatalf("failed to marshal protobuf HeartbeatRequested: %v", err)
	}
	return b
}

// BuildShutdownRequestedPayload builds a ShutdownRequested payload bytes.
func BuildShutdownRequestedPayload(t *testing.T, reason string) []byte {
	t.Helper()
	p := &operatorv1.ShutdownRequested{
		Reason: reason,
	}
	b, err := proto.Marshal(p)
	if err != nil {
		t.Fatalf("failed to marshal protobuf ShutdownRequested: %v", err)
	}
	return b
}

// =============================================================================
// GovernanceEnvelope Helpers (Phase 3 - JSON-first protocol)
// =============================================================================

// MarshalGovernanceEnvelope creates a GovernanceEnvelope JSON with the given fields.
func MarshalGovernanceEnvelope(t *testing.T, messageID string, protocolVersion string, senderID string, actionType string, targetResource string, dataBlob string, caseID string, investigationID string, taskID *string) []byte {
	t.Helper()
	env := &governance.GovernanceEnvelope{
		ProtocolVersion: protocolVersion,
		Id:              messageID,
		OperatorId:      senderID,
		Timestamp:       timestamppb.Now(),
		ActionType:      actionType,
		TargetResource:  targetResource,
		Payload:         []byte(dataBlob),
		CaseId:          caseID,
		InvestigationId: investigationID,
	}
	if taskID != nil {
		env.TaskId = *taskID
	}

	b, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("failed to marshal GovernanceEnvelope JSON: %v", err)
	}
	return b
}

// MarshalGovernanceEnvelopeWithVotes creates a GovernanceEnvelope JSON with pre-populated votes.
func MarshalGovernanceEnvelopeWithVotes(t *testing.T, messageID string, protocolVersion string, senderID string, actionType string, targetResource string, dataBlob string, agentIDs []string, consensusSig string) []byte {
	t.Helper()
	env := &governance.GovernanceEnvelope{
		ProtocolVersion: protocolVersion,
		Id:              messageID,
		OperatorId:      senderID,
		Timestamp:       timestamppb.Now(),
		ActionType:      actionType,
		TargetResource:  targetResource,
		Payload:         []byte(dataBlob),
		Governance: &commonv1.GovernanceMetadata{
			L2: &commonv1.L2Metadata{
				TribunalId: "test-tribunal",
				Votes: []*commonv1.L2Vote{
					{
						SignerKeyId:        agentIDs[0],
						ConsensusSignature: consensusSig,
						Decision:           true,
					},
				},
			},
		},
	}
	b, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("failed to marshal GovernanceEnvelope JSON with votes: %v", err)
	}
	return b
}

// UnmarshalGovernanceEnvelope unmarshals bytes to a GovernanceEnvelope, fatally failing the test on error.
func UnmarshalGovernanceEnvelope(t *testing.T, data []byte) *governance.GovernanceEnvelope {
	t.Helper()
	env := &governance.GovernanceEnvelope{}
	if err := json.Unmarshal(data, env); err != nil {
		t.Fatalf("failed to unmarshal GovernanceEnvelope JSON: %v", err)
	}
	return env
}

// GenerateGovernanceMessageID generates a deterministic MessageID from intent and context.
func GenerateGovernanceMessageID(t *testing.T, actionType string, targetResource string, dataBlob string) string {
	t.Helper()
	env := &governance.GovernanceEnvelope{
		ActionType:     actionType,
		TargetResource: targetResource,
		Payload:        []byte(dataBlob),
		ExpiresAt:      timestamppb.New(time.Now().Add(5 * time.Minute)),
	}
	id, err := governance.GenerateMessageID(env)
	if err != nil {
		t.Fatalf("failed to generate GovernanceEnvelope MessageID: %v", err)
	}
	return id
}

// MarshalGovernanceEnvelopeWithNonce creates a GovernanceEnvelope JSON with the given fields and a nonce.
func MarshalGovernanceEnvelopeWithNonce(t *testing.T, messageID string, protocolVersion string, senderID string, actionType string, targetResource string, dataBlob string, caseID string, investigationID string, taskID *string, nonce string) []byte {
	t.Helper()
	env := &governance.GovernanceEnvelope{
		ProtocolVersion: protocolVersion,
		Id:              messageID,
		OperatorId:      senderID,
		Timestamp:       timestamppb.Now(),
		ActionType:      actionType,
		TargetResource:  targetResource,
		Payload:         []byte(dataBlob),
		CaseId:          caseID,
		InvestigationId: investigationID,
		Nonce:           nonce,
	}
	if taskID != nil {
		env.TaskId = *taskID
	}

	b, err := json.Marshal(env)
	if err != nil {
		t.Fatalf("failed to marshal GovernanceEnvelope JSON: %v", err)
	}
	return b
}

// CreateGovernanceVote creates a slice of agent IDs for testing.
func CreateGovernanceVote(t *testing.T, nodeID string) []string {
	t.Helper()
	return []string{nodeID}
}
